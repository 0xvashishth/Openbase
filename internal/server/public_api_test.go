package server_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

// seedRowDB connects a project to a BYODB Postgres, creates the given schema
// and returns a plaintext API key for the public data routes.
func seedRowDB(t *testing.T, ts *testServer, dsn, schema string) (projectID, apiKey string) {
	t.Helper()
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": dsn})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("user db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), schema); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "row-test"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d body=%v", resp.StatusCode, js)
	}
	plaintext, _ := js["plaintext"].(string)
	if plaintext == "" {
		t.Fatal("expected plaintext key")
	}
	return pID, plaintext
}

func rowByID(t *testing.T, ts *testServer, key, collection string, id any) map[string]any {
	t.Helper()
	resp, js := ts.do(t, "GET", "/v1/api/"+collection+"?limit=100", key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ := js["rows"].([]any)
	want := fmt.Sprint(id)
	for _, r := range rows {
		row, _ := r.(map[string]any)
		if row == nil {
			continue
		}
		for _, pk := range []string{"id", "_id"} {
			if v, ok := row[pk]; ok && fmt.Sprint(v) == want {
				return row
			}
		}
	}
	return nil
}

// TestPublicAPIPutDeletePostgres proves PUT/DELETE resolve the real primary
// key from the live schema instead of the old hard-coded `_id` (which
// compiled to `WHERE "_id" = $1` and errored on every SQL engine).
func TestPublicAPIPutDeletePostgres(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	_, key := seedRowDB(t, ts, userDB.DSN, `
		CREATE TABLE items (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price INT NOT NULL DEFAULT 0
		);
		INSERT INTO items (name, price) VALUES ('Widget', 99);
		CREATE TABLE nopk (name TEXT NOT NULL);
		CREATE TABLE compk (a INT NOT NULL, b INT NOT NULL, PRIMARY KEY (a, b));
	`)

	// PUT updates the addressed row.
	resp, js := ts.do(t, "PUT", "/v1/api/items/1", key,
		map[string]any{"name": "Updated", "price": 5})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d body=%v", resp.StatusCode, js)
	}
	if js["matched_count"] != float64(1) || js["modified_count"] != float64(1) {
		t.Fatalf("unexpected PUT counts: %v", js)
	}
	if row := rowByID(t, ts, key, "items", 1); row == nil || row["name"] != "Updated" {
		t.Fatalf("row was not updated: %v", row)
	}

	// The URL is authoritative: a PK carried in the body is dropped, not applied.
	resp, js = ts.do(t, "PUT", "/v1/api/items/1", key,
		map[string]any{"id": 999, "name": "Kept"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT with body id status = %d body=%v", resp.StatusCode, js)
	}
	if row := rowByID(t, ts, key, "items", 1); row == nil || row["name"] != "Kept" {
		t.Fatalf("PK in body must not rewrite identity: %v", row)
	}

	// A non-numeric id on an integer PK is a 400, not a database error.
	resp, js = ts.do(t, "PUT", "/v1/api/items/abc", key, map[string]any{"name": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT bad id status = %d body=%v, want 400", resp.StatusCode, js)
	}

	// A table with no primary key gets an explicit 400, not a wrong predicate.
	resp, js = ts.do(t, "PUT", "/v1/api/nopk/x", key, map[string]any{"name": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT no-pk status = %d body=%v, want 400", resp.StatusCode, js)
	}

	// Composite primary keys are rejected as ambiguous.
	resp, js = ts.do(t, "PUT", "/v1/api/compk/1", key, map[string]any{"a": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT composite-pk status = %d body=%v, want 400", resp.StatusCode, js)
	}

	// DELETE removes the addressed row; a second DELETE matches nothing.
	resp, js = ts.do(t, "DELETE", "/v1/api/items/1", key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d body=%v", resp.StatusCode, js)
	}
	if js["deleted_count"] != float64(1) {
		t.Fatalf("unexpected DELETE count: %v", js)
	}
	if row := rowByID(t, ts, key, "items", 1); row != nil {
		t.Fatalf("row should be gone: %v", row)
	}
	resp, js = ts.do(t, "DELETE", "/v1/api/items/1", key, nil)
	if resp.StatusCode != http.StatusOK || js["deleted_count"] != float64(0) {
		t.Fatalf("repeat DELETE status = %d body=%v, want deleted_count 0", resp.StatusCode, js)
	}
}

// TestPublicAPIPutDeleteFerretDB proves the same id-addressed verbs work on a
// document engine: `_id` is discovered from the schema and a generated
// ObjectID round-trips through its hex string (the adapter matches both the
// ObjectID and string storage forms).
func TestPublicAPIPutDeleteFerretDB(t *testing.T) {
	ts := newTestServer(t)
	fdb := testutil.StartFerretDB(t)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": fdb.ConnString})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "row-test"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d body=%v", resp.StatusCode, js)
	}
	key, _ := js["plaintext"].(string)
	if key == "" {
		t.Fatal("expected plaintext key")
	}

	resp, js = ts.do(t, "POST", "/v1/api/notes", key, map[string]any{"title": "hello"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d body=%v", resp.StatusCode, js)
	}
	id, _ := js["id"].(string)
	if id == "" {
		t.Fatalf("expected generated id: %v", js)
	}

	resp, js = ts.do(t, "PUT", "/v1/api/notes/"+id, key, map[string]any{"title": "world"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d body=%v", resp.StatusCode, js)
	}
	if js["matched_count"] != float64(1) {
		t.Fatalf("PUT should match the generated ObjectID via its hex string: %v", js)
	}
	if row := rowByID(t, ts, key, "notes", id); row == nil || row["title"] != "world" {
		t.Fatalf("row was not updated: %v", row)
	}

	resp, js = ts.do(t, "DELETE", "/v1/api/notes/"+id, key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE status = %d body=%v", resp.StatusCode, js)
	}
	if js["deleted_count"] != float64(1) {
		t.Fatalf("unexpected DELETE count: %v", js)
	}
	if row := rowByID(t, ts, key, "notes", id); row != nil {
		t.Fatalf("row should be gone: %v", row)
	}
}
