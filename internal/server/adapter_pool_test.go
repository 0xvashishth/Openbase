package server_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/server"
	"github.com/openbase/openbase/internal/testutil"
)

// countingFactory wraps the real engine factory and counts ConnectForProject
// dials, so tests can prove the pool reuses one adapter instead of dialing
// per request.
type countingFactory struct {
	inner server.AdapterFactory
	dials *atomic.Int64
}

func (f *countingFactory) ConnectForProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (server.Adapter, error) {
	f.dials.Add(1)
	return f.inner.ConnectForProject(ctx, conn, secret)
}

func (f *countingFactory) TestConnection(ctx context.Context, connString string) (adapter.Engine, error) {
	return f.inner.TestConnection(ctx, connString)
}

// TestAdapterPoolReusesConnection proves the request path shares one pooled
// adapter per project: N sequential data requests dial exactly once, a
// connection overwrite invalidates (next request re-dials), and a delete
// invalidates (next request 404s without dialing a ghost).
func TestAdapterPoolReusesConnection(t *testing.T) {
	ts := newTestServer(t)
	var dials atomic.Int64
	ts.svc.AdapterFactory = &countingFactory{inner: ts.svc.AdapterFactory, dials: &dials}

	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "pool-test"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d body=%v", resp.StatusCode, js)
	}
	key, _ := js["plaintext"].(string)
	if key == "" {
		t.Fatal("expected plaintext key")
	}

	// For reference: what a fresh dial costs on this machine (the old
	// per-request price: TCP + auth + Ping, then teardown).
	start := time.Now()
	for i := 0; i < 3; i++ {
		a, err := ts.svc.AdapterFactory.ConnectForProject(context.Background(),
			metadata.Connection{Engine: "postgres"},
			metadata.ConnectionSecret{ConnString: userDB.DSN})
		if err != nil {
			t.Fatal(err)
		}
		_ = a.Disconnect(context.Background())
	}
	t.Logf("3 unpooled dial+close cycles: %v", time.Since(start))

	dials.Store(0)
	start = time.Now()
	for i := 0; i < 5; i++ {
		resp, js := ts.do(t, "GET", "/v1/api/tables", key, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("tables request %d status = %d body=%v", i, resp.StatusCode, js)
		}
	}
	t.Logf("5 pooled requests: %v", time.Since(start))
	if n := dials.Load(); n != 1 {
		t.Fatalf("dials = %d for 5 sequential requests, want exactly 1", n)
	}

	// Overwriting the connection invalidates the pool entry: the next request
	// re-dials exactly once and keeps working.
	resp, _ = ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite status = %d", resp.StatusCode)
	}
	resp, js = ts.do(t, "GET", "/v1/api/tables", key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tables after overwrite status = %d body=%v", resp.StatusCode, js)
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("dials = %d after overwrite, want 2", n)
	}

	// Deleting the connection invalidates too: the next request 404s, and no
	// dial is attempted against the detached database.
	resp, js = ts.do(t, "DELETE", "/v1/projects/"+pID+"/connections", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d body=%v", resp.StatusCode, js)
	}
	resp, js = ts.do(t, "GET", "/v1/api/tables", key, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("tables after delete status = %d body=%v, want 404", resp.StatusCode, js)
	}
	if n := dials.Load(); n != 2 {
		t.Fatalf("dials = %d after delete, want still 2 (no ghost dial)", n)
	}
}
