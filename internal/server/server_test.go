package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/engine"
	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/secrets"
	"github.com/openbase/openbase/internal/server"
	"github.com/openbase/openbase/internal/testutil"
	"github.com/openbase/openbase/internal/triggers"
	"github.com/openbase/openbase/migrations"
)

type testServer struct {
	base *httptest.Server
	url  string
	store metadata.Store
	// svc is the live Services the handler was built from, so tests can swap a
	// dependency (e.g. wrap AdapterFactory to count dials).
	svc *server.Services
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	return newTestServerWith(t, nil)
}

// newTestServerWith builds a server injecting an optional Provisioner (used by
// provisioning tests) while keeping the standard real-DB harness.
func newTestServerWith(t *testing.T, prov server.Provisioner) *testServer {
	t.Helper()
	pg := testutil.StartPostgres(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, pg.DSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := metadata.NewPostgres(pool)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	secretsProv, err := secrets.New("test-master-secret-with-enough-entropy", "test-key-v1")
	if err != nil {
		t.Fatalf("secrets provider: %v", err)
	}
	svc := &server.Services{
		Store:          store,
		Tokens:         auth.NewTokenManager(strings.Repeat("k", 32), "aud", "iss", time.Hour),
		Log:            logger,
		AdapterFactory: engine.NewFactory(),
		Secrets:        secretsProv,
		Provisioner:    prov,
		RealtimeHub:    server.NewRealtimeHub(store, secretsProv, engine.NewFactory(), logger),
	}
	t.Cleanup(svc.RealtimeHub.CloseAll)

	// Wire the trigger runtime (Phase 4) so trigger tests can run end-to-end.
	trigSvc := triggers.NewService(
		store,
		&engine.TriggerConnector{Factory: engine.NewFactory()},
		&triggers.Dispatcher{
			Store:  store,
			Log:    logger,
			Action: &triggers.DispatchGroup{Actions: []triggers.Action{
				&triggers.EndpointAction{Log: logger},
				&triggers.FunctionAction{Store: store, Runner: function.New(), Log: logger},
			}},
		},
		logger,
	)
	svc.TriggerService = trigSvc
	t.Cleanup(trigSvc.Stop)

	h := server.New(svc)
	t.Cleanup(h.Close) // dispose pooled project adapters
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return &testServer{base: ts, url: ts.URL, store: store, svc: svc}
}

func (ts *testServer) do(t *testing.T, method, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, ts.url+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func (ts *testServer) register(t *testing.T, email, password string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/auth/register", "", map[string]string{
		"email": email, "password": password, "full_name": "Test User",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, body=%v", resp.StatusCode, js)
	}
	tok, _ := js["token"].(string)
	if tok == "" {
		t.Fatalf("no token in register response: %v", js)
	}
	return tok
}

func (ts *testServer) createOrg(t *testing.T, token, name, slug string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/orgs", token, map[string]string{"name": name, "slug": slug})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create org status = %d body=%v", resp.StatusCode, js)
	}
	return js["id"].(string)
}

func (ts *testServer) createProject(t *testing.T, token, orgID, name, slug string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/orgs/"+orgID+"/projects", token,
		map[string]string{"name": name, "slug": slug})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project status = %d body=%v", resp.StatusCode, js)
	}
	return js["id"].(string)
}

func (ts *testServer) addMember(t *testing.T, orgID, userID string, role metadata.OrgRole) {
	t.Helper()
	err := ts.store.AddMember(t.Context(), &metadata.Membership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
	})
	if err != nil {
		t.Fatalf("add member %s role=%s: %v", userID, role, err)
	}
}

func TestAuthFlow(t *testing.T) {
	ts := newTestServer(t)

	resp, js := ts.do(t, "POST", "/v1/auth/register", "", map[string]string{
		"email": "user@example.com", "password": "pw123456",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status=%d body=%v", resp.StatusCode, js)
	}
	email, _ := js["user"].(map[string]any)["email"].(string)
	if email != "user@example.com" {
		t.Fatalf("registered email = %q", email)
	}
	if _, ok := js["user"].(map[string]any)["password_hash"]; ok {
		t.Fatal("password_hash must never be returned")
	}

	// Login with correct credentials.
	resp2, js2 := ts.do(t, "POST", "/v1/auth/login", "", map[string]string{
		"email": "user@example.com", "password": "pw123456",
	})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%v", resp2.StatusCode, js2)
	}
	tok, _ := js2["token"].(string)
	if tok == "" {
		t.Fatal("no token from login")
	}

	// /me with token.
	resp3, js3 := ts.do(t, "GET", "/v1/me", tok, nil)
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("me status=%d body=%v", resp3.StatusCode, js3)
	}

	// Wrong password rejected.
	resp4, _ := ts.do(t, "POST", "/v1/auth/login", "", map[string]string{
		"email": "user@example.com", "password": "wrong",
	})
	if resp4.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status=%d", resp4.StatusCode)
	}

	// No token -> 401.
	resp5, _ := ts.do(t, "GET", "/v1/me", "", nil)
	if resp5.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me without token status=%d", resp5.StatusCode)
	}

	// Invalid token -> 401.
	resp6, _ := ts.do(t, "GET", "/v1/me", "not-a-token", nil)
	if resp6.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me with bad token status=%d", resp6.StatusCode)
	}
}

func TestDuplicateRegisterConflicts(t *testing.T) {
	ts := newTestServer(t)
	ts.register(t, "dup@example.com", "pw123456")
	resp, js := ts.do(t, "POST", "/v1/auth/register", "", map[string]string{
		"email": "dup@example.com", "password": "pw123456",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register status=%d body=%v", resp.StatusCode, js)
	}
}

func TestOrgProjectFlow(t *testing.T) {
	ts := newTestServer(t)
	tok := ts.register(t, "boss@example.com", "pw123456")

	orgID := ts.createOrg(t, tok, "Acme", "acme")

	// List orgs.
	resp, _ := ts.do(t, "GET", "/v1/orgs", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list orgs status=%d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatal("expected json content type")
	}
	arr := mustDecodeArray(t, ts, tok, "/v1/orgs")
	if len(arr) != 1 {
		t.Fatalf("expected 1 org, got %v", arr)
	}

	ts.createProject(t, tok, orgID, "Web", "web")

	projects := mustDecodeArray(t, ts, tok, "/v1/orgs/"+orgID+"/projects")
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %v", projects)
	}

	// A different user is not a member and should be forbidden.
	otherTok := ts.register(t, "other@example.com", "pw123456")
	resp3, js3 := ts.do(t, "GET", "/v1/orgs/"+orgID, otherTok, nil)
	if resp3.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member get org status=%d body=%v", resp3.StatusCode, js3)
	}
	resp4, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/projects", otherTok,
		map[string]string{"name": "Intruder", "slug": "intruder"})
	if resp4.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member create project status=%d", resp4.StatusCode)
	}
}

func mustDecodeArray(t *testing.T, ts *testServer, token, path string) []any {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.url+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("path %s status=%d", path, resp.StatusCode)
	}
	var out []any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode array: %v", err)
	}
	return out
}

func TestProjectConnectionGating(t *testing.T) {
	ts := newTestServer(t)
	tok := ts.register(t, "gate@example.com", "pw123456")
	orgID := ts.createOrg(t, tok, "GateOrg", "gate")
	pID := ts.createProject(t, tok, orgID, "Proj", "proj")

	// No connection yet -> 404.
	resp, js := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no connection status=%d body=%v", resp.StatusCode, js)
	}

	// Test connection with a garbage string fails gracefully with detection
	// error surfaced (test returns 200 with success=false in our impl).
	resp2, js2 := ts.do(t, "POST", "/v1/projects/"+pID+"/connections/test", tok,
		map[string]string{"connection_string": "garbage"})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("test bad conn status=%d body=%v", resp2.StatusCode, js2)
	}
	if ok, _ := js2["success"].(bool); ok {
		t.Fatalf("garbage conn should report success=false: %v", js2)
	}

	// Unauthorized user can't see connections.
	otherTok := ts.register(t, "other2@example.com", "pw123456")
	resp3, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", otherTok, nil)
	if resp3.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member connections status=%d", resp3.StatusCode)
	}
}

func TestTestConnectionAgainstRealPostgres(t *testing.T) {
	ts := newTestServer(t)
	pg := tsIntegPostgres(t)
	tok := ts.register(t, "real@example.com", "pw123456")
	orgID := ts.createOrg(t, tok, "RealOrg", "real")
	pID := ts.createProject(t, tok, orgID, "Proj", "proj")

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections/test", tok,
		map[string]string{"connection_string": pg.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test real conn status=%d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("real conn should succeed: %v", js)
	}
	if eng, _ := js["engine"].(string); eng != "postgres" {
		t.Fatalf("engine = %q, want postgres", eng)
	}
}

// tsIntegPostgres starts a second Postgres container representing a "user's
// database" for BYODB-style tests. It shares the testutil harness.
func tsIntegPostgres(t *testing.T) *testutil.PostgresContainer {
	return testutil.StartPostgres(t)
}