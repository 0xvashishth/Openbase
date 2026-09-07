// Package triggers implements the platform's trigger runtime: it wires a
// project's configured triggers (SCHEMA.md `triggers`) to the backing database
// adapter, subscribes to change events, and dispatches each event to its
// action — a webhook HTTP call or a sandboxed user function.
package triggers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/metadata"
)

// Action runs the action associated with a trigger: a webhook or a function.
type Action interface {
	// Run executes the action for one event. It returns the outcome description.
	Run(ctx context.Context, trigger metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error
}

// Dispatcher looks up and dispatches an event to matching triggers. It is
// separate from the Service so the pure dispatch logic is unit-testable with a
// fake store and action.
type Dispatcher struct {
	Store  metadata.Store
	Action Action
	Log    *slog.Logger
}

// DispatchGroup routes each trigger to the Action for its action type.
type DispatchGroup struct {
	Actions []Action
	Log     *slog.Logger
}

// Run picks the first Action matching the trigger's action type.
func (g *DispatchGroup) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	for _, a := range g.Actions {
		if actionMatch(a, t) {
			return a.Run(ctx, t, event, record)
		}
	}
	return fmt.Errorf("no action configured for type %q", t.ActionType)
}

// actionMatch reports whether an Action handles a trigger (webhook actions only
// run webhook triggers, function actions only function triggers).
func actionMatch(a Action, t metadata.Trigger) bool {
	switch a.(type) {
	case *EndpointAction:
		return t.ActionType == metadata.ActionWebhook
	case *FunctionAction:
		return t.ActionType == metadata.ActionFunction
	default:
		return false
	}
}

// Dispatch runs every enabled trigger on (collection, event) for the project,
// passing the record through.
func (d *Dispatcher) Dispatch(ctx context.Context, projectID, collection string, event adapter.TriggerEvent, record map[string]any) {
	if d.Store == nil || d.Action == nil {
		return
	}
	triggers, err := d.Store.ListTriggers(ctx, projectID)
	if err != nil {
		if d.Log != nil {
			d.Log.Error("trigger: list triggers", "project", projectID, "err", err)
		}
		return
	}
	for _, t := range triggers {
		if !t.Enabled || t.Collection != collection || metadata.TriggerEvent(event) != t.Event {
			continue
		}
		if err := d.Action.Run(ctx, t, event, record); err != nil {
			if d.Log != nil {
				d.Log.Error("trigger: action failed", "project", projectID, "trigger", t.ID, "err", err)
			}
			continue
		}
	}
}

// WebhookStore is the persistence EndpointAction needs: the per-project HMAC
// signing secret and the delivery log. metadata.Store implements it; tests
// use an in-memory fake.
type WebhookStore interface {
	GetOrCreateWebhookSecret(ctx context.Context, projectID string) (string, error)
	RecordWebhookDelivery(ctx context.Context, d *metadata.WebhookDelivery) error
}

// EndpointAction implements Action for webhook triggers: POST the event to the
// configured URL as JSON.
type EndpointAction struct {
	Client *http.Client
	Log    *slog.Logger
	// AllowPrivate disables the SSRF guard for self-hosters that intentionally
	// target LAN/internal URLs. Default false: webhook destinations must be
	// public http(s) hosts. Configure via OPENBASE_ALLOW_PRIVATE_WEBHOOKS.
	AllowPrivate bool
	// Store enables HMAC signing and the delivery log. Nil tolerates
	// store-less construction (unit tests): deliveries go out unsigned and
	// unrecorded. Production always sets it.
	Store WebhookStore
	// MaxAttempts bounds delivery tries including the first (default 3).
	// Only network errors, 429 and 5xx are retried, with exponential backoff.
	MaxAttempts int
}

// webhookTimeout bounds a single webhook delivery. Dispatch runs on
// context.Background(), so without this a hung destination would wedge the
// dispatch goroutine forever.
const webhookTimeout = 10 * time.Second

// defaultWebhookAttempts is the delivery try budget (first try + retries).
const defaultWebhookAttempts = 3

// webhookRetryBase is the first backoff pause; it doubles per retry.
const webhookRetryBase = 500 * time.Millisecond

func (e *EndpointAction) maxAttempts() int {
	if e.MaxAttempts > 0 {
		return e.MaxAttempts
	}
	return defaultWebhookAttempts
}

func (e *EndpointAction) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	if t.ActionType != metadata.ActionWebhook {
		return fmt.Errorf("endpoint action requires a webhook trigger")
	}
	if err := ValidateWebhookURL(t.ActionTarget, e.AllowPrivate); err != nil {
		return err
	}
	body := map[string]any{
		"trigger_id": t.ID,
		"project_id": t.ProjectID,
		"collection": t.Collection,
		"event":      string(event),
		"data":       record,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}

	// Per-project HMAC secret so receivers can verify authenticity. A store
	// failure must not silently send unsigned traffic that looks signed —
	// fail the delivery instead.
	var secret string
	if e.Store != nil {
		secret, err = e.Store.GetOrCreateWebhookSecret(ctx, t.ProjectID)
		if err != nil {
			return fmt.Errorf("webhook: signing secret: %w", err)
		}
	}

	start := time.Now()
	delivery := &metadata.WebhookDelivery{
		ProjectID:  t.ProjectID,
		TriggerID:  t.ID,
		TargetURL:  t.ActionTarget,
		Collection: t.Collection,
		Event:      string(event),
	}
	// Bound the delivery even when the caller's context has no deadline.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, webhookTimeout)
		defer cancel()
	}

	var lastErr error
	for attempt := 1; attempt <= e.maxAttempts(); attempt++ {
		delivery.Attempts = attempt
		status, retryable, err := e.tryDeliver(ctx, client, t.ActionTarget, payload, secret)
		if err == nil {
			delivery.StatusCode = &status
			delivery.OK = true
			delivery.DurationMs = time.Since(start).Milliseconds()
			e.record(ctx, delivery)
			return nil
		}
		lastErr = err
		if !retryable || attempt == e.maxAttempts() {
			break
		}
		backoff := webhookRetryBase * time.Duration(1<<(attempt-1))
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			attempt = e.maxAttempts() // stop retrying
		case <-time.After(backoff):
		}
	}
	if sc, ok := lastStatus(lastErr); ok {
		delivery.StatusCode = &sc
	}
	delivery.Error = lastErr.Error()
	delivery.DurationMs = time.Since(start).Milliseconds()
	e.record(ctx, delivery)
	return lastErr
}

// webhookError carries the HTTP status of a failed delivery so Run can record
// it and decide retryability.
type webhookError struct {
	status     int
	msg        string
	retryable  bool
}

func (e *webhookError) Error() string { return e.msg }

func lastStatus(err error) (int, bool) {
	var we *webhookError
	if errors.As(err, &we) && we.status > 0 {
		return we.status, true
	}
	return 0, false
}

// tryDeliver POSTs one attempt. It returns (status, retryable, error); a nil
// error means the delivery succeeded.
func (e *EndpointAction) tryDeliver(ctx context.Context, client *http.Client, target string, payload []byte, secret string) (int, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		req.Header.Set("X-Openbase-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, true, fmt.Errorf("webhook: delivery failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, false, nil
	}
	retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
	return 0, retryable, &webhookError{
		status:    resp.StatusCode,
		msg:       fmt.Sprintf("webhook returned %d", resp.StatusCode),
		retryable: retryable,
	}
}

// record persists the delivery outcome. Recording failures are logged, never
// propagated — the delivery result itself is authoritative.
func (e *EndpointAction) record(ctx context.Context, d *metadata.WebhookDelivery) {
	if e.Store == nil {
		return
	}
	if err := e.Store.RecordWebhookDelivery(ctx, d); err != nil && e.Log != nil {
		e.Log.Warn("webhook: record delivery", "trigger", d.TriggerID, "err", err)
	}
}

// cgnatRange is 100.64.0.0/10 (carrier-grade NAT) — not covered by
// netip.Addr.IsPrivate, so it needs an explicit check.
var cgnatRange = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n
}()

// ValidateWebhookURL is the SSRF guard for webhook destinations. It rejects
// anything that is not a public http(s) host: non-http schemes, unresolvable
// names, and every resolved IP that is loopback, link-local (including the
// 169.254.169.254 cloud-metadata address), unspecified, multicast, private
// (RFC1918/ULA) or carrier-grade NAT. Literal IPs are checked without DNS.
//
// allowPrivate is the self-host escape hatch (OPENBASE_ALLOW_PRIVATE_WEBHOOKS):
// scheme validation still applies, but IP-range checks are skipped.
func ValidateWebhookURL(target string, allowPrivate bool) error {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return fmt.Errorf("webhook: invalid URL %q", target)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook: scheme must be http or https, got %q", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("webhook: credentials must not be embedded in the URL")
	}
	if allowPrivate {
		return nil
	}
	host := u.Hostname()
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil || len(ips) == 0 {
		// A literal IP that failed DNS still parses as an IP — check it
		// directly so numeric targets get a range verdict, not a DNS error.
		if ip := net.ParseIP(host); ip != nil {
			return checkWebhookIP(ip)
		}
		return fmt.Errorf("webhook: cannot resolve host %q", host)
	}
	for _, ia := range ips {
		if err := checkWebhookIP(ia.IP); err != nil {
			return err
		}
	}
	return nil
}

// checkWebhookIP rejects non-public IP addresses for webhook delivery.
func checkWebhookIP(ip net.IP) error {
	if ip.IsLoopback() {
		return fmt.Errorf("webhook: loopback addresses are not allowed (%s)", ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("webhook: unspecified addresses are not allowed (%s)", ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("webhook: link-local addresses are not allowed (%s)", ip)
	}
	if ip.IsMulticast() {
		return fmt.Errorf("webhook: multicast addresses are not allowed (%s)", ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("webhook: private-network addresses are not allowed (%s)", ip)
	}
	if cgnatRange.Contains(ip) {
		return fmt.Errorf("webhook: carrier-grade-NAT addresses are not allowed (%s)", ip)
	}
	// IPv4-mapped IPv6 forms (e.g. ::ffff:10.0.0.1) must not smuggle a private
	// address past the checks above.
	if ip.To4() != nil && ip.To16() != nil && strings.Contains(ip.String(), ":") {
		if v4 := ip.To4(); !v4.IsGlobalUnicast() || v4.IsPrivate() || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
			return fmt.Errorf("webhook: mapped private address is not allowed (%s)", ip)
		}
	}
	return nil
}

// FunctionAction implements Action for function triggers using the function
// runner.
type FunctionAction struct {
	Store  metadata.Store
	Runner function.Runner
	Log    *slog.Logger
	// MaxConcurrentPerProject caps simultaneous function runs per project so
	// a trigger storm on one project cannot starve every other. Zero selects
	// defaultFunctionConcurrency. Acquisition is non-blocking: when the cap
	// is hit the run fails fast with an error (visible in logs) instead of
	// wedging the dispatch goroutine behind an unbounded queue.
	MaxConcurrentPerProject int

	mu    sync.Mutex
	slots map[string]int
}

// defaultFunctionConcurrency bounds concurrent sandboxed runs per project.
const defaultFunctionConcurrency = 4

func (f *FunctionAction) maxConcurrent() int {
	if f.MaxConcurrentPerProject > 0 {
		return f.MaxConcurrentPerProject
	}
	return defaultFunctionConcurrency
}

// tryAcquire takes a run slot for the project, reporting false when the cap
// is already reached. Pair every true with release.
func (f *FunctionAction) tryAcquire(projectID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.slots == nil {
		f.slots = map[string]int{}
	}
	if f.slots[projectID] >= f.maxConcurrent() {
		return false
	}
	f.slots[projectID]++
	return true
}

func (f *FunctionAction) release(projectID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.slots[projectID] > 0 {
		f.slots[projectID]--
	}
}

func (f *FunctionAction) Run(ctx context.Context, t metadata.Trigger, event adapter.TriggerEvent, record map[string]any) error {
	if t.ActionType != metadata.ActionFunction {
		return fmt.Errorf("function action requires a function trigger")
	}
	fn, err := f.Store.GetFunction(ctx, t.ProjectID, t.ActionTarget)
	if err != nil {
		return fmt.Errorf("function %s: %w", t.ActionTarget, err)
	}
	payload := map[string]any{
		"trigger_id": t.ID,
		"project_id": t.ProjectID,
		"collection": t.Collection,
		"event":      string(event),
		"data":       record,
	}
	if f.Runner == nil {
		return fmt.Errorf("no function runner configured")
	}
	if !f.tryAcquire(t.ProjectID) {
		return fmt.Errorf("function: too many concurrent runs for project (cap %d)", f.maxConcurrent())
	}
	defer f.release(t.ProjectID)
	_, err = f.Runner.Run(ctx, fn.Source, string(fn.Runtime), payload)
	return err
}

// ---- Adapter subscription wiring ----

// DB is the subset of the adapter the service needs to wire subscriptions.
type DB interface {
	RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error
	SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error)
	Disconnect(ctx context.Context) error
}

// Connector connects a project's database adapter, given its decrypted secret.
type Connector interface {
	Connect(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (DB, error)
}

// Service wires a project's triggers onto its adapter and forwards events to a
// Dispatcher. One goroutine runs per collection, fed by the adapter's
// subscription.
type Service struct {
	Store    metadata.Store
	Connect  Connector
	Dispatch *Dispatcher
	Log      *slog.Logger

	mu    sync.Mutex
	conns map[string]*projectConn
}

type projectConn struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewService builds a trigger runtime with the given injectable dependencies.
func NewService(store metadata.Store, conn Connector, dispatch *Dispatcher, log *slog.Logger) *Service {
	return &Service{
		Store:    store,
		Connect:  conn,
		Dispatch: dispatch,
		Log:      log,
		conns:    map[string]*projectConn{},
	}
}

// RegisterProject connects the project adapter, registers its native DB
// triggers, and starts change subscriptions for every enabled trigger.
// It is idempotent: calling again reloads (re-registers) with the latest config.
func (s *Service) RegisterProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) error {
	s.StopProject(conn.ProjectID)

	db, err := s.Connect.Connect(ctx, conn, secret)
	if err != nil {
		return fmt.Errorf("trigger: connect: %w", err)
	}

	triggers, err := s.Store.ListTriggers(ctx, conn.ProjectID)
	if err != nil {
		_ = db.Disconnect(ctx)
		return err
	}

	subCtx, cancel := context.WithCancel(context.Background())
	pc := &projectConn{cancel: cancel}

	for _, t := range triggers {
		if !t.Enabled {
			continue
		}
		def := adapter.TriggerDefinition{
			ID:           t.ID,
			ProjectID:    t.ProjectID,
			Name:         t.Name,
			Collection:   t.Collection,
			Event:        adapter.TriggerEvent(t.Event),
			ActionType:   string(t.ActionType),
			ActionTarget: t.ActionTarget,
		}
		if err := db.RegisterTrigger(ctx, def); err != nil {
			if s.Log != nil {
				s.Log.Warn("trigger: register failed (continuing)", "trigger", t.ID, "err", err)
			}
			continue
		}
	}

	s.mu.Lock()
	s.conns[conn.ProjectID] = pc
	s.mu.Unlock()

	// One listening goroutine per unique collection among enabled triggers. They
	// all share the one cancel; a WaitGroup tracks when they've all returned.
	seen := map[string]bool{}
	for _, t := range triggers {
		if !t.Enabled || seen[t.Collection] {
			continue
		}
		seen[t.Collection] = true
		pc.wg.Add(1)
		collection := t.Collection
		projectID := conn.ProjectID
		go func() {
			defer pc.wg.Done()
			defer func() { _ = db.Disconnect(subCtx) }()
			sub, err := db.SubscribeToChanges(subCtx, collection,
				func(c string, ev adapter.TriggerEvent, rec map[string]any) {
					s.Dispatch.Dispatch(context.Background(), projectID, c, ev, rec)
				})
			if err != nil {
				if s.Log != nil {
					s.Log.Error("trigger: subscribe", "project", projectID, "collection", collection, "err", err)
				}
				return
			}
			<-subCtx.Done()
			_ = sub.Close()
		}()
	}

	return nil
}

// StopProject tears down subscriptions for a project (if any).
func (s *Service) StopProject(projectID string) {
	s.mu.Lock()
	pc := s.conns[projectID]
	delete(s.conns, projectID)
	s.mu.Unlock()
	if pc != nil {
		pc.cancel()
		waitTimeout(&pc.wg, 2*time.Second)
	}
}

// Stop tears down all projects.
func (s *Service) Stop() {
	s.mu.Lock()
	pcs := make([]*projectConn, 0, len(s.conns))
	for _, pc := range s.conns {
		pcs = append(pcs, pc)
	}
	s.conns = map[string]*projectConn{}
	s.mu.Unlock()
	for _, pc := range pcs {
		pc.cancel()
	}
	for _, pc := range pcs {
		waitTimeout(&pc.wg, 2*time.Second)
	}
}

// waitTimeout waits for a WaitGroup with a deadline.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}
