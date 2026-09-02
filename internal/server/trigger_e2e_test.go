package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

// TestTriggerFiresWebhookOnInsert proves the full Phase 4 path: a connected
// Postgres DB, a webhook trigger registered via the API, and a row inserted
// through the auto-generated REST API causes the webhook to be invoked.
func TestTriggerFiresWebhookOnInsert(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	// Connect the user database.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d body=%v", resp.StatusCode, js)
	}

	// Seed a collection.
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `CREATE TABLE orders (id SERIAL PRIMARY KEY, total INT NOT NULL);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Webhook receiver.
	var hits atomic.Int32
	var got atomic.Value
	rx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(b, &p)
		got.Store(p)
		hits.Add(1)
		w.WriteHeader(200)
	}))
	t.Cleanup(rx.Close)

	// Create a webhook trigger for orders.insert.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/triggers", tok, map[string]string{
		"name":          "order-placed",
		"collection":    "orders",
		"event":         "insert",
		"action_type":   "webhook",
		"action_target": rx.URL,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create trigger status = %d body=%v", resp.StatusCode, js)
	}

	// Create an API key and insert a row through the public API.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "e2e"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d", resp.StatusCode)
	}
	key, _ := js["plaintext"].(string)

	req, _ := http.NewRequest("POST", ts.url+"/v1/api/orders", strings.NewReader(`{"total": 1200}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	iresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, iresp.Body)
	iresp.Body.Close()
	if iresp.StatusCode != http.StatusCreated {
		t.Fatalf("insert status = %d", iresp.StatusCode)
	}

	// The trigger runtime listens via LISTEN/NOTIFY; allow it to deliver.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && hits.Load() == 0 {
		time.Sleep(100 * time.Millisecond)
	}
	if hits.Load() == 0 {
		t.Fatal("expected webhook to be invoked after insert")
	}
	if p, _ := got.Load().(map[string]any); p != nil {
		if p["collection"] != "orders" || p["event"] != "insert" {
			t.Fatalf("unexpected webhook payload: %v", p)
		}
		data, _ := p["data"].(map[string]any)
		if total, _ := data["total"]; total != nil && total.(float64) != 1200 {
			t.Fatalf("unexpected data: %v", data)
		}
	}
}
