package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

// connectRawSQLProject wires a project to a real "user database" and returns
// (token, projectID) plus a pool on that database for out-of-band assertions.
func connectRawSQLProject(t *testing.T, ts *testServer) (string, string, *pgxpool.Pool) {
	t.Helper()
	tok := ts.register(t, "rawsql@example.com", "pw123456")
	org := ts.createOrg(t, tok, "RawOrg", "raw")
	pID := ts.createProject(t, tok, org, "RawProj", "raw-proj")

	userDB := testutil.StartPostgres(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatalf("user db connect: %v", err)
	}
	t.Cleanup(pool.Close)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("save connection failed: %v", js)
	}
	return tok, pID, pool
}

// TestExecSQLReadWriteEndToEnd is the regression test for the SQL editor: it
// used to fail with "raw SQL is only supported for postgres and mysql" for
// every engine (the adapter Conn wrapper didn't forward ExecRaw), and writes
// were rejected by a read-only guard. Both are exercised here over HTTP.
func TestExecSQLReadWriteEndToEnd(t *testing.T) {
	ts := newTestServer(t)
	tok, pID, pool := connectRawSQLProject(t, ts)
	ctx := context.Background()
	path := "/v1/projects/" + pID + "/sql"

	// DDL through the editor.
	resp, js := ts.do(t, "POST", path, tok,
		map[string]string{"query": "CREATE TABLE editor_items (id SERIAL PRIMARY KEY, name TEXT NOT NULL, qty INT NOT NULL DEFAULT 0)"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create table status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ := js["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("create should return one status row: %v", js)
	}
	if first, _ := rows[0].(map[string]any); first["result"] != "OK" {
		t.Fatalf("create result = %v, want OK", rows[0])
	}

	// DML insert.
	resp, js = ts.do(t, "POST", path, tok,
		map[string]string{"query": "INSERT INTO editor_items (name, qty) VALUES ('widget', 2), ('gadget', 5)"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("insert status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ = js["rows"].([]any)
	first, _ := rows[0].(map[string]any)
	if got, _ := first["affected_rows"].(float64); got != 2 {
		t.Fatalf("insert affected_rows = %v, want 2 (body=%v)", first["affected_rows"], js)
	}

	// The write really landed in the user's database.
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM editor_items").Scan(&count); err != nil {
		t.Fatalf("verify insert: %v", err)
	}
	if count != 2 {
		t.Fatalf("rows in user db = %d, want 2", count)
	}

	// Read back through the editor.
	resp, js = ts.do(t, "POST", path, tok,
		map[string]string{"query": "SELECT name, qty FROM editor_items ORDER BY qty"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("select status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ = js["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("select rows = %v, want 2", js)
	}
	firstRow, _ := rows[0].(map[string]any)
	if firstRow["name"] != "widget" {
		t.Fatalf("first row = %v, want widget", firstRow)
	}

	// UPDATE + DELETE report affected rows.
	resp, js = ts.do(t, "POST", path, tok,
		map[string]string{"query": "UPDATE editor_items SET qty = qty + 1 WHERE qty >= 5"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ = js["rows"].([]any)
	first, _ = rows[0].(map[string]any)
	if got, _ := first["affected_rows"].(float64); got != 1 {
		t.Fatalf("update affected_rows = %v, want 1", first["affected_rows"])
	}

	resp, js = ts.do(t, "POST", path, tok,
		map[string]string{"query": "DELETE FROM editor_items WHERE name = 'widget'"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ = js["rows"].([]any)
	first, _ = rows[0].(map[string]any)
	if got, _ := first["affected_rows"].(float64); got != 1 {
		t.Fatalf("delete affected_rows = %v, want 1", first["affected_rows"])
	}

	// RETURNING surfaces real rows.
	resp, js = ts.do(t, "POST", path, tok,
		map[string]string{"query": "INSERT INTO editor_items (name, qty) VALUES ('doohickey', 9) RETURNING id, name"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("insert returning status = %d body=%v", resp.StatusCode, js)
	}
	rows, _ = js["rows"].([]any)
	first, _ = rows[0].(map[string]any)
	if first["name"] != "doohickey" {
		t.Fatalf("returning row = %v", first)
	}
}

func TestExecSQLRejectsStackedStatementsAndEmpty(t *testing.T) {
	ts := newTestServer(t)
	tok, pID, _ := connectRawSQLProject(t, ts)
	path := "/v1/projects/" + pID + "/sql"

	for _, q := range []string{
		"SELECT 1; DROP TABLE users",
		"",
		"   ",
		"-- just a comment",
	} {
		resp, js := ts.do(t, "POST", path, tok, map[string]string{"query": q})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("query %q status = %d, want 400 (body=%v)", q, resp.StatusCode, js)
		}
	}
}

func TestExecSQLSurfacesEngineErrors(t *testing.T) {
	ts := newTestServer(t)
	tok, pID, _ := connectRawSQLProject(t, ts)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/sql", tok,
		map[string]string{"query": "SELECT * FROM no_such_table"})
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unknown table should not succeed: %v", js)
	}
	if msg, _ := js["error"].(string); msg == "" {
		t.Fatalf("engine error should be surfaced: %v", js)
	}
}

func TestExecSQLRequiresConnectionAndMembership(t *testing.T) {
	ts := newTestServer(t)
	tok := ts.register(t, "nodb@example.com", "pw123456")
	org := ts.createOrg(t, tok, "NoDBOrg", "nodb")
	pID := ts.createProject(t, tok, org, "NoDBProj", "nodb-proj")

	// No connection configured yet.
	resp, _ := ts.do(t, "POST", "/v1/projects/"+pID+"/sql", tok,
		map[string]string{"query": "SELECT 1"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no-connection status = %d, want 404", resp.StatusCode)
	}

	// Non-member is forbidden.
	other := ts.register(t, "intruder-sql@example.com", "pw123456")
	resp, _ = ts.do(t, "POST", "/v1/projects/"+pID+"/sql", other,
		map[string]string{"query": "SELECT 1"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member status = %d, want 403", resp.StatusCode)
	}
}
