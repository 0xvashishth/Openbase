package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/adapter/postgres"
	"github.com/openbase/openbase/internal/testutil"
)

// connect returns a connected postgres adapter for tests.
func connect(t *testing.T) *postgres.Adapter {
	t.Helper()
	pg := testutil.StartPostgres(t)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, pg.DSN)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	// Seed a schema with PK/FK relationships to exercise introspection.
	setupSQL := `
		CREATE TABLE users (
			id   SERIAL PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			age  INT,
			active BOOLEAN NOT NULL DEFAULT true
		);
		CREATE TABLE posts (
			id     SERIAL PRIMARY KEY,
			user_id INT NOT NULL REFERENCES users(id),
			title  TEXT NOT NULL,
			body   TEXT
		);
		CREATE TABLE tags (
			id   SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
	`
	if _, err := pool.Exec(ctx, setupSQL); err != nil {
		t.Fatalf("seed schema: %v", err)
	}

	a := postgres.New()
	if err := a.Connect(ctx, adapter.ConnectionConfig{Engine: adapter.EnginePostgres, ConnStr: pg.DSN}); err != nil {
		t.Fatalf("connect adapter: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

func TestCapabilities(t *testing.T) {
	a := postgres.New()
	caps := a.Capabilities()
	if !caps.SupportsRelationalJoins ||
		!caps.SupportsForeignKeys ||
		!caps.SupportsNativeTriggers ||
		!caps.SupportsTransactions ||
		!caps.SupportsVectorSearch {
		t.Fatalf("postgres capabilities too weak: %+v", caps)
	}
	if caps.SupportsRealtime != adapter.RealtimeNative {
		t.Fatalf("realtime = %q, want native", caps.SupportsRealtime)
	}
}

func TestListCollectionsAndSchema(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	cols, err := a.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	names := []string{}
	for _, c := range cols {
		names = append(names, c.Name)
	}
	want := map[string]bool{"users": true, "posts": true, "tags": true}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected collection %q in %v", n, names)
		}
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 collections, got %v", names)
	}

	schema, err := a.GetSchema(ctx, "users")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if schema.Collection != "users" {
		t.Fatalf("collection = %q", schema.Collection)
	}
	var idColIsPK bool
	for _, col := range schema.Columns {
		if col.Name == "id" {
			idColIsPK = col.IsPrimary
		}
		if col.Name == "active" && !col.Nullable {
			// NOT NULL DEFAULT true should report nullable=false.
		}
	}
	if !idColIsPK {
		t.Fatalf("id column should be primary key: %+v", schema.Columns)
	}

	if _, err := a.GetSchema(ctx, "no_such_table; DROP TABLE users"); err == nil {
		t.Fatal("invalid table identifier should be rejected")
	}
}

func TestRelationships(t *testing.T) {
	a := connect(t)
	rels, err := a.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("ListRelationships: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 FK relationship, got %+v", rels)
	}
	r := rels[0]
	if r.FromCollection != "posts" || r.FromColumn != "user_id" ||
		r.ToCollection != "users" || r.ToColumn != "id" {
		t.Fatalf("unexpected relationship: %+v", r)
	}
}

func TestCRUD(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	// Insert
	res, err := a.Insert(ctx, "users", map[string]any{"email": "a@example.com", "age": 30})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if res.ID == nil {
		t.Fatal("insert should return generated id")
	}

	// Query with filter
	qs, err := a.Query(ctx, adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			Conditions: []adapter.Condition{{Field: "email", Operator: adapter.OpEqual, Value: "a@example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(qs.Rows) != 1 || qs.Rows[0]["email"] != "a@example.com" {
		t.Fatalf("unexpected rows: %+v", qs.Rows)
	}
	if len(qs.Columns) == 0 {
		t.Fatal("result should include column names")
	}

	// Insert a second user to test limit/order.
	if _, err := a.Insert(ctx, "users", map[string]any{"email": "b@example.com", "age": 20}); err != nil {
		t.Fatal(err)
	}
	limit := 1
	qs, err = a.Query(ctx, adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "users",
			OrderBy:    []adapter.OrderBy{{Field: "age", Desc: true}},
			Limit:      &limit,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(qs.Rows) != 1 || qs.Rows[0]["email"] != "a@example.com" {
		t.Fatalf("desc limit query wrong: %+v", qs.Rows)
	}

	// Update
	ur, err := a.Update(ctx,
		adapter.Filter{Collection: "users", Conditions: []adapter.Condition{{Field: "email", Operator: adapter.OpEqual, Value: "b@example.com"}}},
		map[string]any{"age": 21})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if ur.ModifiedCount != 1 {
		t.Fatalf("ModifiedCount = %d, want 1", ur.ModifiedCount)
	}

	// Delete
	dr, err := a.Delete(ctx, adapter.Filter{
		Collection: "users",
		Conditions: []adapter.Condition{{Field: "email", Operator: adapter.OpEqual, Value: "b@example.com"}},
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if dr.DeletedCount != 1 {
		t.Fatalf("DeletedCount = %d, want 1", dr.DeletedCount)
	}
}

func TestQueryOperators(t *testing.T) {
	a := connect(t)
	ctx := context.Background()
	for i, email := range []string{"one@example.com", "two@example.com", "three@example.com"} {
		_, err := a.Insert(ctx, "users", map[string]any{"email": email, "age": 10 + i})
		if err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name string
		op   adapter.Op
		val  any
		want int
	}{
		{"eq", adapter.OpEqual, "one@example.com", 1},
		{"neq", adapter.OpNotEqual, "one@example.com", 2},
		{"gt", adapter.OpGreaterThan, 11, 1},
		{"gtep", adapter.OpGreaterEq, 11, 2},
		{"lt", adapter.OpLessThan, 11, 1},
		{"lte", adapter.OpLessEq, 11, 2},
		{"contains", adapter.OpContains, "three%", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			field := "email"
			val := tc.val
			if tc.name == "gt" || tc.name == "gtep" || tc.name == "lt" || tc.name == "lte" {
				field = "age"
			}
			rs, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{
				Collection: "users",
				Conditions: []adapter.Condition{{Field: field, Operator: tc.op, Value: val}},
			}})
			if err != nil {
				t.Fatalf("query %s: %v", tc.name, err)
			}
			if len(rs.Rows) != tc.want {
				t.Fatalf("query %s: got %d rows, want %d (%+v)", tc.name, len(rs.Rows), tc.want, rs.Rows)
			}
		})
	}
}

func TestOperationsRequireConnection(t *testing.T) {
	a := postgres.New()
	ctx := context.Background()
	if _, err := a.ListCollections(ctx); err == nil {
		t.Fatal("ListCollections before Connect should error")
	}
	if _, err := a.Insert(ctx, "users", map[string]any{"a": 1}); err == nil {
		t.Fatal("Insert before Connect should error")
	}
}

func TestInsertRejectsBadIdentifiers(t *testing.T) {
	a := connect(t)
	ctx := context.Background()
	if _, err := a.Insert(ctx, "users; DROP TABLE users", map[string]any{"a": 1}); err == nil {
		t.Fatal("tablename injection should be rejected")
	}
	if _, err := a.Insert(ctx, "users", map[string]any{"email=x": 1}); err == nil {
		t.Fatal("columnname injection should be rejected")
	}
}

func TestTriggerRegisterAndRemove(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	tr := adapter.TriggerDefinition{
		ID:           "trg_1",
		ProjectID:    "proj_1",
		Name:         "on-user-insert",
		Collection:   "users",
		Event:        adapter.TriggerInsert,
		ActionType:   "webhook",
		ActionTarget: "https://example.com/hook",
	}
	if err := a.RegisterTrigger(ctx, tr); err != nil {
		t.Fatalf("RegisterTrigger: %v", err)
	}

	// Insert should now invoke the NOTIFY trigger; verify by subscribing.
	received := make(chan string, 1)
	sub, err := a.SubscribeToChanges(ctx, "users", func(c string, e adapter.TriggerEvent, rec map[string]any) {
		received <- fmt.Sprintf("%s|%v", c, rec["payload"])
	})
	if err != nil {
		t.Fatalf("SubscribeToChanges: %v", err)
	}
	defer sub.Close()

	if _, err := a.Insert(ctx, "users", map[string]any{"email": "notify@example.com", "age": 1}); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-received:
		if msg == "" {
			t.Fatal("empty notification payload")
		}
	case <-ctxDone(ctx):
		t.Fatal("did not receive NOTIFY within timeout")
	}

	if err := a.RemoveTriggerOn(ctx, "users", tr.ID); err != nil {
		t.Fatalf("RemoveTriggerOn: %v", err)
	}
}

func TestExecRawReadsAndWrites(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	// DDL: no rows, no columns -> normalized to an OK row.
	rs, err := a.ExecRaw(ctx, `CREATE TABLE raw_items (id SERIAL PRIMARY KEY, name TEXT NOT NULL, qty INT NOT NULL DEFAULT 0)`)
	if err != nil {
		t.Fatalf("ExecRaw create: %v", err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0]["result"] != "OK" {
		t.Fatalf("create result = %+v, want OK row", rs.Rows)
	}

	// INSERT without RETURNING -> affected_rows.
	rs, err = a.ExecRaw(ctx, `INSERT INTO raw_items (name, qty) VALUES ('widget', 2), ('gadget', 5)`)
	if err != nil {
		t.Fatalf("ExecRaw insert: %v", err)
	}
	if rs.Rows[0]["affected_rows"] != int64(2) {
		t.Fatalf("insert affected_rows = %+v, want 2", rs.Rows)
	}

	// INSERT ... RETURNING -> real rows.
	rs, err = a.ExecRaw(ctx, `INSERT INTO raw_items (name, qty) VALUES ('doohickey', 7) RETURNING id, name`)
	if err != nil {
		t.Fatalf("ExecRaw insert returning: %v", err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0]["name"] != "doohickey" {
		t.Fatalf("returning rows = %+v", rs.Rows)
	}

	// SELECT read path.
	rs, err = a.ExecRaw(ctx, `SELECT name, qty FROM raw_items ORDER BY qty`)
	if err != nil {
		t.Fatalf("ExecRaw select: %v", err)
	}
	if len(rs.Rows) != 3 || rs.Rows[0]["name"] != "widget" {
		t.Fatalf("select rows = %+v", rs.Rows)
	}

	// UPDATE / DELETE report affected rows.
	rs, err = a.ExecRaw(ctx, `UPDATE raw_items SET qty = qty + 1 WHERE qty >= 5`)
	if err != nil {
		t.Fatalf("ExecRaw update: %v", err)
	}
	if rs.Rows[0]["affected_rows"] != int64(2) {
		t.Fatalf("update affected_rows = %+v, want 2", rs.Rows)
	}
	rs, err = a.ExecRaw(ctx, `DELETE FROM raw_items WHERE name = 'widget'`)
	if err != nil {
		t.Fatalf("ExecRaw delete: %v", err)
	}
	if rs.Rows[0]["affected_rows"] != int64(1) {
		t.Fatalf("delete affected_rows = %+v, want 1", rs.Rows)
	}

	// DROP is DDL again.
	if _, err := a.ExecRaw(ctx, `DROP TABLE raw_items`); err != nil {
		t.Fatalf("ExecRaw drop: %v", err)
	}
}

func TestExecRawCapsRows(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	rs, err := a.ExecRaw(ctx, `SELECT g FROM generate_series(1, 500) AS g`)
	if err != nil {
		t.Fatalf("ExecRaw generate_series: %v", err)
	}
	if len(rs.Rows) != adapter.MaxRawRows {
		t.Fatalf("got %d rows, want cap %d", len(rs.Rows), adapter.MaxRawRows)
	}
}

func TestExecRawSurfacesEngineErrors(t *testing.T) {
	a := connect(t)
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, `SELECT * FROM does_not_exist`); err == nil {
		t.Fatal("expected engine error for unknown table")
	}

	notConnected := postgres.New()
	if _, err := notConnected.ExecRaw(ctx, `SELECT 1`); err == nil {
		t.Fatal("expected not-connected error")
	}
}

func ctxDone(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(5 * time.Second):
			close(done)
		}
	}()
	return done
}