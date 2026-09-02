package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

// TestRealtimeLiveUpdate proves the Phase 5 path end-to-end: connect Postgres,
// create an API key, open a WebSocket to /v1/realtime, subscribe to a
// collection, insert a row via the auto-generated REST API, and receive the
// live change event over the socket.
func TestRealtimeLiveUpdate(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	// Connect the user database.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d body=%v", resp.StatusCode, js)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `CREATE TABLE items (id SERIAL PRIMARY KEY, name TEXT NOT NULL);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Create an API key.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "rt"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d", resp.StatusCode)
	}
	key, _ := js["plaintext"].(string)
	if key == "" {
		t.Fatal("expected plaintext key")
	}

	// Open a WebSocket to the realtime gateway authenticated by the API key.
	wsURL := "ws" + strings.TrimPrefix(ts.url, "http") + "/v1/realtime"
	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + key}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "done")

	// Subscribe to the items collection.
	if err := ws.Write(ctx, websocket.MessageText, []byte(`{"type":"subscribe","collection":"items"}`)); err != nil {
		t.Fatal(err)
	}
	_, ack, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	var ackMsg map[string]any
	_ = json.Unmarshal(ack, &ackMsg)
	if ackMsg["type"] != "subscribed" || ackMsg["collection"] != "items" {
		t.Fatalf("expected subscribed ack, got %s", ack)
	}

	// Insert a row via the REST API (authenticated by the same key).
	req, _ := http.NewRequest("POST", ts.url+"/v1/api/items", strings.NewReader(`{"name":"Latte"}`))
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

	// Read the live change event (may take a moment for LISTEN/NOTIFY).
	deadline := time.Now().Add(5 * time.Second)
	for {
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, msg, rerr := ws.Read(readCtx)
		cancel()
		if rerr != nil {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for change event: %v", rerr)
			}
			continue
		}
		var change map[string]any
		if err := json.Unmarshal(msg, &change); err != nil {
			continue
		}
		if change["type"] == "change" && change["collection"] == "items" {
			if change["event"] != "insert" {
				t.Fatalf("unexpected event %v, want insert", change["event"])
			}
			data, _ := change["data"].(map[string]any)
			if name, _ := data["name"]; name != "Latte" {
				t.Fatalf("unexpected row data: %v", data)
			}
			return
		}
	}
}
