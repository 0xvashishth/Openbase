package valkey

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/openbase/openbase/internal/adapter"
)

func newTestAdapter(t *testing.T) (*Adapter, func()) {
	t.Helper()
	s := miniredis.RunT(t)
	a := New()
	ctx := context.Background()
	if err := a.Connect(ctx, adapter.ConnectionConfig{ConnStr: "redis://" + s.Addr()}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	cleanup := func() { _ = a.Disconnect(ctx) }
	return a, cleanup
}

func TestValkeyCRUD(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	// Insert with auto id.
	r1, err := a.Insert(ctx, "items", map[string]any{"name": "widget"})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if r1.ID != "1" {
		t.Fatalf("auto id = %v", r1.ID)
	}

	// Insert with explicit id.
	r2, err := a.Insert(ctx, "items", map[string]any{"id": "abc", "name": "gadget"})
	if err != nil {
		t.Fatalf("insert2: %v", err)
	}
	if r2.ID != "abc" {
		t.Fatalf("explicit id = %v", r2.ID)
	}

	// Query all.
	q, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{Collection: "items"}})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(q.Rows) != 2 {
		t.Fatalf("got %d rows", len(q.Rows))
	}

	// Filter by equality.
	eq, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{
		Collection: "items",
		Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "widget"}},
	}})
	if err != nil {
		t.Fatalf("filter query: %v", err)
	}
	if len(eq.Rows) != 1 {
		t.Fatalf("filter got %d rows, want 1", len(eq.Rows))
	}
	if eq.Rows[0]["name"] != "widget" {
		t.Fatalf("filter row name = %v", eq.Rows[0]["name"])
	}

	// Update.
	up, err := a.Update(ctx, adapter.Filter{
		Collection: "items",
		Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "widget"}},
	}, map[string]any{"stock": 7})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if up.MatchedCount != 1 || up.ModifiedCount != 1 {
		t.Fatalf("update counts = %+v", up)
	}
	got, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{
		Collection: "items",
		Conditions: []adapter.Condition{{Field: "stock", Operator: adapter.OpEqual, Value: "7"}},
	}})
	if err != nil || len(got.Rows) != 1 {
		t.Fatalf("updated row not found: %v rows=%d", err, len(got.Rows))
	}

	// Delete.
	del, err := a.Delete(ctx, adapter.Filter{
		Collection: "items",
		Conditions: []adapter.Condition{{Field: "id", Operator: adapter.OpEqual, Value: "abc"}},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if del.DeletedCount != 1 {
		t.Fatalf("deleted = %d", del.DeletedCount)
	}
	final, _ := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{Collection: "items"}})
	if len(final.Rows) != 1 {
		t.Fatalf("after delete got %d rows, want 1", len(final.Rows))
	}
}

func TestValkeySchemaAndCollections(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	_, _ = a.Insert(ctx, "users", map[string]any{"name": "alice", "email": "a@x.io"})
	_, _ = a.Insert(ctx, "users", map[string]any{"name": "bob", "age": 30})

	cols, err := a.ListCollections(ctx)
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	found := false
	for _, c := range cols {
		if c.Name == "users" {
			found = true
		}
	}
	if !found {
		t.Fatalf("users not in collections: %+v", cols)
	}

	schema, err := a.GetSchema(ctx, "users")
	if err != nil {
		t.Fatalf("get schema: %v", err)
	}
	keys := map[string]bool{}
	for _, c := range schema.Columns {
		keys[c.Name] = true
	}
	for _, want := range []string{"id", "name", "email", "age"} {
		if !keys[want] {
			t.Fatalf("schema missing %q: %+v", want, schema.Columns)
		}
	}
	if len(schema.Indexes) != 1 || schema.Indexes[0].Kind != adapter.IndexPrimary {
		t.Fatalf("expected single primary index, got %+v", schema.Indexes)
	}

	rels, err := a.ListRelationships(ctx)
	if err != nil || len(rels) != 0 {
		t.Fatalf("relationships should be empty: %v %+v", err, rels)
	}
}

func TestValkeyCapabilitiesHonest(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()

	caps := a.Capabilities()
	if caps.SupportsForeignKeys || caps.SupportsNativeTriggers || caps.SupportsTransactions {
		t.Fatalf("key-value engine should not claim relational capabilities: %+v", caps)
	}
	if caps.SupportsRealtime != adapter.RealtimeNone {
		t.Fatalf("realtime = %q, want none", caps.SupportsRealtime)
	}
}

func TestValkeyTriggersAndRealtimeUnsupported(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	if err := a.RegisterTrigger(ctx, adapter.TriggerDefinition{Collection: "x"}); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("register trigger err = %v", err)
	}
	if err := a.RegisterRealtimeBroadcast(ctx, "x"); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("register realtime err = %v", err)
	}
	if _, err := a.SubscribeToChanges(ctx, "x", nil); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("subscribe err = %v", err)
	}
}

func TestValkeyPingFailsOnDeadEndpoint(t *testing.T) {
	a := New()
	ctx := context.Background()
	if err := a.Connect(ctx, adapter.ConnectionConfig{ConnStr: "redis://127.0.0.1:1"}); err == nil {
		t.Fatal("connect to dead endpoint should fail")
	}
}

func TestValkeyExecRawReadWrite(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	// Write.
	res, err := a.ExecRaw(ctx, `SET greeting hello`)
	if err != nil {
		t.Fatalf("SET: %v", err)
	}
	if len(res.Rows) != 1 || res.Rows[0]["result"] != "OK" {
		t.Fatalf("SET result = %+v, want OK", res.Rows)
	}

	// Read back.
	res, err = a.ExecRaw(ctx, `GET greeting`)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if res.Rows[0]["result"] != "hello" {
		t.Fatalf("GET result = %+v, want hello", res.Rows)
	}

	// Quoted values with spaces survive tokenization.
	if _, err := a.ExecRaw(ctx, `SET doc '{"name": "Ada Lovelace"}'`); err != nil {
		t.Fatalf("SET quoted: %v", err)
	}
	res, err = a.ExecRaw(ctx, `GET doc`)
	if err != nil {
		t.Fatalf("GET quoted: %v", err)
	}
	if res.Rows[0]["result"] != `{"name": "Ada Lovelace"}` {
		t.Fatalf("GET doc = %+v", res.Rows)
	}

	// Delete + missing key reads as nil rather than an error.
	if _, err := a.ExecRaw(ctx, `DEL greeting`); err != nil {
		t.Fatalf("DEL: %v", err)
	}
	res, err = a.ExecRaw(ctx, `GET greeting`)
	if err != nil {
		t.Fatalf("GET missing: %v", err)
	}
	if res.Rows[0]["result"] != nil {
		t.Fatalf("missing key result = %+v, want nil", res.Rows)
	}
}

func TestValkeyExecRawMultiLineAndComments(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	// Comment lines are skipped; commands run in order; last result returned.
	res, err := a.ExecRaw(ctx, `// seed a key
SET a 1
-- another comment
SET b 2
GET b`)
	if err != nil {
		t.Fatalf("multi-line: %v", err)
	}
	if res.Rows[0]["result"] != "2" {
		t.Fatalf("last result = %+v, want 2", res.Rows)
	}
}

func TestValkeyExecRawArrayResult(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, "SET k1 v1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := a.ExecRaw(ctx, "SET k2 v2"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := a.ExecRaw(ctx, "KEYS *")
	if err != nil {
		t.Fatalf("KEYS: %v", err)
	}
	if len(res.Columns) != 1 || res.Columns[0] != "value" {
		t.Fatalf("columns = %+v, want [value]", res.Columns)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("expected 2 keys, got %+v", res.Rows)
	}
}

func TestValkeyExecRawErrors(t *testing.T) {
	a, cleanup := newTestAdapter(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, "   "); err == nil {
		t.Fatal("expected error for empty query")
	}
	if _, err := a.ExecRaw(ctx, `SET k 'unterminated`); err == nil {
		t.Fatal("expected error for unterminated quote")
	}
	if _, err := a.ExecRaw(ctx, "NOTACOMMAND x"); err == nil {
		t.Fatal("expected error for unknown command")
	}

	notConnected := New()
	if _, err := notConnected.ExecRaw(ctx, "GET x"); err == nil {
		t.Fatal("expected not-connected error")
	}
}

func TestTokenizeRedis(t *testing.T) {
	got, err := tokenizeRedis(`SET doc "{\"a\": 1}" EX 60`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"SET", "doc", `{"a": 1}`, "EX", "60"}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}
