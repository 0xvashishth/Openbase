package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

// WithProject returns (token, orgID, projectID) for a fresh project.
func (ts *testServer) newProject(t *testing.T) (token, orgID, projectID string) {
	t.Helper()
	tok := ts.register(t, "data@example.com", "pw123456")
	org := ts.createOrg(t, tok, "DataOrg", "data")
	proj := ts.createProject(t, tok, org, "DataProj", "data-proj")
	return tok, org, proj
}

func TestSaveConnectionAndBrowseData(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// A "user's database" we'll connect to as BYODB.
	userDB := testutil.StartPostgres(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatalf("user db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	seed := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			age INT NOT NULL DEFAULT 0
		);
		INSERT INTO users (email, age) VALUES ('alice@example.com', 30), ('bob@example.com', 25);
	`
	if _, err := pool.Exec(ctx, seed); err != nil {
		t.Fatalf("seed user db: %v", err)
	}

	// No connection yet -> collections returns 404.
	resp, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/collections", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("collections before connect status = %d, want 404", resp.StatusCode)
	}

	// Save the connection (test-before-save, then encrypt+store).
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("save should succeed: %v", js)
	}

	// The stored connection must be encrypted, not plaintext, and connected.
	rc, jsC := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
	if rc.StatusCode != http.StatusOK {
		t.Fatalf("get connection status = %d", rc.StatusCode)
	}
	conn := jsC
	if conn["status"] != "connected" {
		t.Fatalf("connection status = %v, want connected", conn["status"])
	}
	if conn["encrypted_conn_string"] != nil || conn["connection_string"] != nil {
		t.Fatal("connection response must not expose credentials")
	}

	// Stored bytes in the DB should differ from plaintext (encrypted).
	storeConn, err := ts.store.GetConnectionByProject(ctx, pID)
	if err != nil {
		t.Fatal(err)
	}
	if string(storeConn.EncryptedConnString) == userDB.DSN {
		t.Fatal("connection string stored in plaintext!")
	}

	// Collections.
	respC, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/collections", tok, nil)
	if respC.StatusCode != http.StatusOK {
		t.Fatalf("collections status = %d", respC.StatusCode)
	}
	arr := mustDecodeArray(t, ts, tok, "/v1/projects/"+pID+"/collections")
	found := false
	for _, c := range arr {
		if m, ok := c.(map[string]any); ok && m["name"] == "users" {
			found = true
		}
	}
	if !found {
		t.Fatalf("users collection not listed: %v", arr)
	}

	// Schema for users.
	respS, rawS := ts.do(t, "GET", "/v1/projects/"+pID+"/collections/users", tok, nil)
	if respS.StatusCode != http.StatusOK {
		t.Fatalf("get schema status = %d", respS.StatusCode)
	}
	schema := rawS
	colsArr, _ := schema["columns"].([]any)
	if len(colsArr) == 0 {
		t.Fatalf("schema should list columns: %v", schema)
	}

	// Rows via query endpoint.
	respR, rawR := ts.do(t, "POST", "/v1/projects/"+pID+"/query", tok,
		map[string]any{"collection": "users"})
	if respR.StatusCode != http.StatusOK {
		t.Fatalf("query status = %d body=%v", respR.StatusCode, rawR)
	}
	rows, _ := rawR["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %v", rawR)
	}

	// Filtered query.
	respF, rawF := ts.do(t, "POST", "/v1/projects/"+pID+"/query", tok,
		map[string]any{
			"collection": "users",
			"conditions": []map[string]any{
				{"field": "email", "operator": "eq", "value": "bob@example.com"},
			},
		})
	if respF.StatusCode != http.StatusOK {
		t.Fatalf("filtered query status = %d", respF.StatusCode)
	}
	frows, _ := rawF["rows"].([]any)
	if len(frows) != 1 {
		t.Fatalf("expected 1 filtered row, got %v", rawF)
	}

	// A non-member can't access the project data.
	otherTok := ts.register(t, "intruder@example.com", "pw123456")
	respO, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/collections", otherTok, nil)
	if respO.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member collections status = %d, want 403", respO.StatusCode)
	}
}

func TestSaveConnectionBadStringSurfacesError(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// A postgres-scheme string that detects but cannot connect: the handler
	// should test-before-save and report success=false with the adapter's real
	// error, without persisting anything.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": "postgres://u:p@127.0.0.1:1/nodb"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bad conn status = %d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); ok {
		t.Fatalf("unreachable connection string should not succeed: %v", js)
	}
	if msg, _ := js["message"].(string); msg == "" {
		t.Fatal("failure should surface a message")
	}

	// Nothing should have been persisted.
	if _, err := ts.store.GetConnectionByProject(context.Background(), pID); err == nil {
		t.Fatal("failed save should not persist a connection")
	}
}

func TestSaveConnectionUnknownSchemaRejected(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": "not-a-conn-string"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown-scheme status = %d, want 400 body=%v", resp.StatusCode, js)
	}
}
