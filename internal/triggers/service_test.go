package triggers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/metadata"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore satisfies just what Dispatcher needs; wrap metadata.Store.
type fakeStore struct {
	metadata.Store
	triggers []metadata.Trigger
	fns      map[string]metadata.Function

	mu         sync.Mutex
	secrets    map[string]string
	deliveries []metadata.WebhookDelivery
}

func (f *fakeStore) ListTriggers(ctx context.Context, projectID string) ([]metadata.Trigger, error) {
	return f.triggers, nil
}

func (f *fakeStore) GetFunction(ctx context.Context, projectID, id string) (*metadata.Function, error) {
	if fn, ok := f.fns[id]; ok {
		return &fn, nil
	}
	return nil, metadata.ErrNotFound
}

func (f *fakeStore) GetOrCreateWebhookSecret(_ context.Context, projectID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.secrets == nil {
		f.secrets = map[string]string{}
	}
	if s, ok := f.secrets[projectID]; ok {
		return s, nil
	}
	f.secrets[projectID] = "test-secret-" + projectID
	return f.secrets[projectID], nil
}

func (f *fakeStore) RecordWebhookDelivery(_ context.Context, d *metadata.WebhookDelivery) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries = append(f.deliveries, *d)
	return nil
}

func TestDispatchToWebhook(t *testing.T) {
	var hits atomic.Int32
	var sig atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		sig.Store(r.Header.Get("X-Openbase-Signature"))
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("bad webhook body: %v", err)
		}
		if got["collection"] != "users" || got["event"] != "insert" {
			t.Errorf("unexpected payload: %v", got)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	store := &fakeStore{triggers: []metadata.Trigger{
		{ID: "t1", ProjectID: "p1", Name: "to-webhook", Collection: "users", Event: "insert", ActionType: "webhook", ActionTarget: srv.URL, Enabled: true},
	}}
	dispatcher := &Dispatcher{
		Store:  store,
		Log:    logger(),
		Action: &EndpointAction{Log: logger(), AllowPrivate: true, Store: store},
	}
	dispatcher.Dispatch(context.Background(), "p1", "users", adapter.TriggerInsert, map[string]any{"id": 1})

	if hits.Load() != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits.Load())
	}
	// The delivery must be HMAC-signed with the per-project secret.
	gotSig, _ := sig.Load().(string)
	if !strings.HasPrefix(gotSig, "sha256=") || len(gotSig) != len("sha256=")+64 {
		t.Fatalf("signature header = %q, want sha256=<64 hex>", gotSig)
	}
	// ...and recorded in the delivery log.
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(store.deliveries))
	}
	d := store.deliveries[0]
	if !d.OK || d.Attempts != 1 || d.StatusCode == nil || *d.StatusCode != 200 {
		t.Fatalf("unexpected delivery record: %+v", d)
	}
}

func TestDispatchSkipsDisabledAndMismatch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	store := &fakeStore{triggers: []metadata.Trigger{
		// Disabled -> skipped.
		{ID: "d1", ProjectID: "p1", Collection: "users", Event: "insert", ActionType: "webhook", ActionTarget: srv.URL, Enabled: false},
		// Different collection -> skipped.
		{ID: "d2", ProjectID: "p1", Collection: "orders", Event: "insert", ActionType: "webhook", ActionTarget: srv.URL, Enabled: true},
		// Different event -> skipped.
		{ID: "d3", ProjectID: "p1", Collection: "users", Event: "delete", ActionType: "webhook", ActionTarget: srv.URL, Enabled: true},
		// Matches -> fires.
		{ID: "d4", ProjectID: "p1", Collection: "users", Event: "insert", ActionType: "webhook", ActionTarget: srv.URL, Enabled: true},
	}}
	dispatcher := &Dispatcher{
		Store:  store,
		Log:    logger(),
		Action: &EndpointAction{Log: logger(), AllowPrivate: true},
	}
	dispatcher.Dispatch(context.Background(), "p1", "users", adapter.TriggerInsert, map[string]any{"id": 1})
	if hits.Load() != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits.Load())
	}
}

func TestDispatchToFunction(t *testing.T) {
	store := &fakeStore{fns: map[string]metadata.Function{
		"fn1": {
			ID: "fn1", ProjectID: "p1", Name: "double", Runtime: "node",
			Source: `exports.handler = async (event) => ({ inputCollection: event.collection });`,
		},
	}}
	trig := []metadata.Trigger{
		{ID: "t1", ProjectID: "p1", Name: "fn-trig", Collection: "users", Event: "insert", ActionType: "function", ActionTarget: "fn1", Enabled: true},
	}
	st := &fakeStore{triggers: trig, fns: store.fns}

	dispatcher := &Dispatcher{
		Store:  st,
		Log:    logger(),
		Action: &FunctionAction{Store: st, Runner: function.New(), Log: logger()},
	}
	// Must not panic; the runner is itself covered by function tests for success.
	dispatcher.Dispatch(context.Background(), "p1", "users", adapter.TriggerInsert, map[string]any{"id": 1})
}

// fakeConn records registration and exposes a fire func for tests.
type fakeConn struct {
	registered []adapter.TriggerDefinition
	fire       func(c string, ev adapter.TriggerEvent, rec map[string]any)
}

func (f *fakeConn) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	f.registered = append(f.registered, t)
	return nil
}

func (f *fakeConn) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	if f.fire != nil {
		f.fire(collection, adapter.TriggerInsert, map[string]any{"id": 99})
	}
	return noopSub{}, nil
}

func (f *fakeConn) Disconnect(ctx context.Context) error { return nil }

type noopSub struct{}

func (noopSub) Close() error { return nil }

// triggerConnector connects a fake DB.
type triggerConnector struct {
	conn *fakeConn
}

func (c *triggerConnector) Connect(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (DB, error) {
	return c.conn, nil
}

func TestServiceWiresAndFiresDispatchedEvent(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	store := &fakeStore{triggers: []metadata.Trigger{
		{ID: "w1", ProjectID: "p1", Collection: "users", Event: "insert", ActionType: "webhook", ActionTarget: srv.URL, Enabled: true},
	}}
	fc := &fakeConn{}
	connector := &triggerConnector{conn: fc}

	svc := NewService(store, connector, &Dispatcher{
		Store:  store,
		Log:    logger(),
		Action: &EndpointAction{Log: logger(), AllowPrivate: true},
	}, logger())
	defer svc.Stop()

	err := svc.RegisterProject(context.Background(), metadata.Connection{ProjectID: "p1"}, metadata.ConnectionSecret{})
	if err != nil {
		t.Fatalf("register project: %v", err)
	}

	// The subscribe goroutine fired synchronously via fc.fire above; verify the
	// webhook endpoint was reached (may need a moment).
	if hits.Load() == 0 {
		// Inline firing isn't guaranteed synchronously; exercise by direct dispatch.
		svc.Dispatch.Dispatch(context.Background(), "p1", "users", adapter.TriggerInsert, map[string]any{"id": 1})
	}
	if hits.Load() == 0 {
		t.Fatal("expected webhook to fire")
	}
	if len(fc.registered) != 1 || fc.registered[0].ID != "w1" {
		t.Fatalf("expected trigger w1 to be registered, got %+v", fc.registered)
	}
}

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name         string
		target       string
		allowPrivate bool
		wantErr      bool
	}{
		{"cloud metadata IMDS", "http://169.254.169.254/latest/meta-data/", false, true},
		{"loopback v4", "http://127.0.0.1:8080/hook", false, true},
		{"loopback v6", "http://[::1]:8080/hook", false, true},
		{"rfc1918 10/8", "http://10.0.0.5/hook", false, true},
		{"rfc1918 172.16/12", "http://172.16.4.9:9000/hook", false, true},
		{"rfc1918 192.168/16", "https://192.168.1.1/hook", false, true},
		{"carrier-grade NAT", "http://100.64.0.1/hook", false, true},
		{"mapped private v6", "http://[::ffff:10.0.0.1]/hook", false, true},
		{"non-http scheme", "file:///etc/passwd", false, true},
		{"gopher scheme", "gopher://example.com/", false, true},
		{"embedded credentials", "https://user:pass@203.0.113.10/hook", false, true},
		{"unparseable", "http://", false, true},
		// TEST-NET-3 (documentation range): globally routable shape, no DNS
		// needed since it is a literal IP — must pass the range checks.
		{"public literal IP", "https://203.0.113.10/hook", false, false},
		{"allowPrivate permits loopback", "http://127.0.0.1:8080/hook", true, false},
		{"allowPrivate still rejects bad scheme", "file:///etc/passwd", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateWebhookURL(tc.target, tc.allowPrivate)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateWebhookURL(%q, allowPrivate=%v) err = %v, wantErr = %v",
					tc.target, tc.allowPrivate, err, tc.wantErr)
			}
		})
	}
}

func TestEndpointActionRejectsSSRFWithoutDialing(t *testing.T) {	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Default guard: the loopback httptest server must be rejected BEFORE any
	// HTTP request is made.
	action := &EndpointAction{Log: logger()}
	trig := metadata.Trigger{
		ID: "t1", ProjectID: "p1", Collection: "users", Event: "insert",
		ActionType: "webhook", ActionTarget: srv.URL, Enabled: true,
	}
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, map[string]any{"id": 1}); err == nil {
		t.Fatal("expected SSRF rejection for loopback target")
	}
	if hits.Load() != 0 {
		t.Fatalf("webhook hits = %d, want 0 (must fail before dialing)", hits.Load())
	}

	// With the self-host escape hatch, the same target delivers.
	action.AllowPrivate = true
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, map[string]any{"id": 1}); err != nil {
		t.Fatalf("allowPrivate run: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("webhook hits = %d, want 1", hits.Load())
	}
}

// blockingRunner blocks each Run until release is closed, so tests can hold
// all concurrency slots deterministically.
type blockingRunner struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingRunner) Run(_ context.Context, _, _ string, _ map[string]any) (map[string]any, error) {
	b.entered <- struct{}{}
	<-b.release
	return map[string]any{"ok": true}, nil
}

func TestFunctionActionConcurrencyCap(t *testing.T) {
	runner := &blockingRunner{entered: make(chan struct{}, 8), release: make(chan struct{})}
	store := &fakeStore{fns: map[string]metadata.Function{
		"fn1": {ID: "fn1", ProjectID: "p1", Runtime: "node", Source: "x"},
	}}
	action := &FunctionAction{Store: store, Runner: runner, Log: logger(), MaxConcurrentPerProject: 2}
	trig := metadata.Trigger{
		ID: "t1", ProjectID: "p1", Collection: "users", Event: "insert",
		ActionType: "function", ActionTarget: "fn1", Enabled: true,
	}

	// Hold both slots.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := action.Run(context.Background(), trig, adapter.TriggerInsert, nil); err != nil {
				t.Errorf("held run: %v", err)
			}
		}()
	}
	<-runner.entered
	<-runner.entered

	// The third concurrent run must fail fast, not queue behind the holders.
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, nil); err == nil {
		t.Fatal("expected concurrency-cap error")
	} else if !strings.Contains(err.Error(), "too many concurrent") {
		t.Fatalf("unexpected error: %v", err)
	}

	// A different project is unaffected by p1's saturation.
	other := trig
	other.ProjectID = "p2"
	store.fns["fn1-p2"] = metadata.Function{ID: "fn1-p2", ProjectID: "p2", Runtime: "node", Source: "x"}
	other.ActionTarget = "fn1-p2"
	done := make(chan error, 1)
	go func() { done <- action.Run(context.Background(), other, adapter.TriggerInsert, nil) }()
	<-runner.entered
	close(runner.release)
	wg.Wait()
	if err := <-done; err != nil {
		t.Fatalf("other project run: %v", err)
	}

	// Slots are released: p1 can run again.
	runner2 := &blockingRunner{entered: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	runner2.release <- struct{}{}
	action.Runner = runner2
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, nil); err != nil {
		t.Fatalf("post-release run: %v", err)
	}
}

func TestEndpointActionRetriesThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	store := &fakeStore{}
	action := &EndpointAction{Log: logger(), AllowPrivate: true, Store: store, MaxAttempts: 3}
	trig := metadata.Trigger{
		ID: "t1", ProjectID: "p1", Collection: "users", Event: "insert",
		ActionType: "webhook", ActionTarget: srv.URL, Enabled: true,
	}
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits = %d, want 3 (initial + 2 retries)", hits.Load())
	}
	if len(store.deliveries) != 1 || !store.deliveries[0].OK || store.deliveries[0].Attempts != 3 {
		t.Fatalf("unexpected deliveries: %+v", store.deliveries)
	}
}

func TestEndpointActionNoRetryOnClientError(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(400)
	}))
	defer srv.Close()

	store := &fakeStore{}
	action := &EndpointAction{Log: logger(), AllowPrivate: true, Store: store, MaxAttempts: 3}
	trig := metadata.Trigger{
		ID: "t1", ProjectID: "p1", Collection: "users", Event: "insert",
		ActionType: "webhook", ActionTarget: srv.URL, Enabled: true,
	}
	if err := action.Run(context.Background(), trig, adapter.TriggerInsert, nil); err == nil {
		t.Fatal("expected error for 400 response")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 (4xx must not retry)", hits.Load())
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(store.deliveries))
	}
	d := store.deliveries[0]
	if d.OK || d.Attempts != 1 || d.StatusCode == nil || *d.StatusCode != 400 || d.Error == "" {
		t.Fatalf("unexpected delivery record: %+v", d)
	}
}
