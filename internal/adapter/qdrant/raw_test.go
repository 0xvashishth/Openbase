package qdrant

import (
	"context"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
)

func TestExecRawScroll(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{
		{ID: "1", Vector: []float64{1, 0, 0}, Payload: map[string]any{"name": "Ada"}},
		{ID: "2", Vector: []float64{0, 1, 0}, Payload: map[string]any{"name": "Lin"}},
	}
	fake.mu.Unlock()

	rs, err := a.ExecRaw(context.Background(), `SCROLL users {"limit": 1}`)
	if err != nil {
		t.Fatalf("SCROLL: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("limit not applied: %+v", rs.Rows)
	}
	if rs.Rows[0]["name"] != "Ada" {
		t.Errorf("row = %+v, want Ada", rs.Rows[0])
	}
	if !hasCol(rs.Columns, "vector") || !hasCol(rs.Columns, "id") {
		t.Errorf("columns = %+v, want id+vector", rs.Columns)
	}
}

func TestExecRawScrollShorthandFilter(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{{ID: "1", Vector: []float64{1}, Payload: map[string]any{"name": "Ada"}}}
	fake.mu.Unlock()

	if _, err := a.ExecRaw(context.Background(), `SCROLL users {"name": "Ada"}`); err != nil {
		t.Fatalf("SCROLL shorthand: %v", err)
	}
	fake.mu.Lock()
	got := fake.lastScroll
	fake.mu.Unlock()
	if got == nil {
		t.Fatal("expected a filter to be sent")
	}
	must, ok := got["must"].([]any)
	if !ok || len(must) != 1 {
		t.Fatalf("filter = %+v, want must[1]", got)
	}
	cond, _ := must[0].(map[string]any)
	if cond["key"] != "name" || cond["match"] != "Ada" {
		t.Fatalf("condition = %+v", cond)
	}
}

func TestExecRawSearch(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{{ID: "1", Vector: []float64{1, 2, 3}, Payload: map[string]any{"name": "Ada"}}}
	fake.mu.Unlock()

	rs, err := a.ExecRaw(context.Background(), `SEARCH users {"vector": [1, 2, 3], "limit": 5}`)
	if err != nil {
		t.Fatalf("SEARCH: %v", err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0]["name"] != "Ada" {
		t.Fatalf("search rows = %+v", rs.Rows)
	}
	fake.mu.Lock()
	vec := fake.lastSearchVector
	fake.mu.Unlock()
	if len(vec) != 3 || vec[0] != 1 {
		t.Fatalf("vector sent = %+v", vec)
	}
}

func TestExecRawSearchRequiresVector(t *testing.T) {
	a, _ := startAdapter(t)
	if _, err := a.ExecRaw(context.Background(), `SEARCH users {"limit": 5}`); err == nil {
		t.Fatal("expected error when vector missing")
	}
	if _, err := a.ExecRaw(context.Background(), `SEARCH users`); err == nil {
		t.Fatal("expected error when json missing")
	}
}

func TestExecRawUpsertVariants(t *testing.T) {
	a, fake := startAdapter(t)
	ctx := context.Background()

	// Array form.
	rs, err := a.ExecRaw(ctx, `UPSERT cats [{"id": 1, "vector": [1,2,3], "payload": {"name": "whiskers"}}]`)
	if err != nil {
		t.Fatalf("UPSERT array: %v", err)
	}
	if rs.Rows[0]["result"] != "OK" {
		t.Fatalf("result = %+v, want OK", rs.Rows)
	}

	// {"points": [...]} form.
	if _, err := a.ExecRaw(ctx, `UPSERT cats {"points": [{"id": 2, "vector": [4,5,6], "payload": {"name": "mittens"}}]}`); err != nil {
		t.Fatalf("UPSERT points: %v", err)
	}

	// Single-point form.
	if _, err := a.ExecRaw(ctx, `UPSERT cats {"id": 3, "vector": [7,8,9], "payload": {"name": "tom"}}`); err != nil {
		t.Fatalf("UPSERT single: %v", err)
	}

	fake.mu.Lock()
	n := len(fake.points["cats"])
	fake.mu.Unlock()
	if n != 3 {
		t.Fatalf("stored %d points, want 3", n)
	}

	if _, err := a.ExecRaw(ctx, `UPSERT cats {"name": "no vector, no id"}`); err == nil {
		t.Fatal("expected error for a payload-only upsert body")
	}
}

func TestExecRawDeleteAndCreateAndDrop(t *testing.T) {
	a, fake := startAdapter(t)
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, `CREATE mycol {"vectors": {"size": 3, "distance": "Cosine"}}`); err != nil {
		t.Fatalf("CREATE: %v", err)
	}
	fake.mu.Lock()
	created := append([]string(nil), fake.created...)
	fake.mu.Unlock()
	if len(created) != 1 || created[0] != "mycol" {
		t.Fatalf("created = %+v", created)
	}

	if _, err := a.ExecRaw(ctx, `DELETE mycol {"name": "Ada"}`); err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	fake.mu.Lock()
	del := fake.lastDelete
	fake.mu.Unlock()
	filter, _ := del["filter"].(map[string]any)
	if filter == nil || filter["must"] == nil {
		t.Fatalf("delete filter = %+v, want must clause", del)
	}

	if _, err := a.ExecRaw(ctx, `DROP mycol`); err != nil {
		t.Fatalf("DROP: %v", err)
	}
	fake.mu.Lock()
	dropped := append([]string(nil), fake.dropped...)
	fake.mu.Unlock()
	if len(dropped) != 1 || dropped[0] != "mycol" {
		t.Fatalf("dropped = %+v", dropped)
	}
}

func TestExecRawJSONForm(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{{ID: "1", Vector: []float64{1}, Payload: map[string]any{"name": "Ada"}}}
	fake.mu.Unlock()

	rs, err := a.ExecRaw(context.Background(), `{"collection": "users", "op": "scroll", "limit": 10}`)
	if err != nil {
		t.Fatalf("JSON scroll: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %+v", rs.Rows)
	}

	if _, err := a.ExecRaw(context.Background(), `{"op": "scroll"}`); err == nil {
		t.Fatal("expected error when collection missing")
	}
}

func TestExecRawErrors(t *testing.T) {
	a, _ := startAdapter(t)
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, "   "); err == nil {
		t.Fatal("expected error for empty query")
	}
	if _, err := a.ExecRaw(ctx, "FROBNICATE users"); err == nil {
		t.Fatal("expected error for unknown op")
	} else if !strings.Contains(err.Error(), "SCROLL") {
		t.Fatalf("error should list supported ops: %v", err)
	}
	if _, err := a.ExecRaw(ctx, `SCROLL users {bad json}`); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if _, err := a.ExecRaw(ctx, "SCROLL"); err == nil {
		t.Fatal("expected error when collection missing")
	}

	notConnected := New()
	if _, err := notConnected.ExecRaw(ctx, "SCROLL users"); err == nil {
		t.Fatal("expected not-connected error")
	}
}

func TestExecRawStripsComments(t *testing.T) {
	a, fake := startAdapter(t)
	fake.mu.Lock()
	fake.points["users"] = []fakePoint{{ID: "1", Vector: []float64{1}, Payload: map[string]any{"name": "Ada"}}}
	fake.mu.Unlock()

	if _, err := a.ExecRaw(context.Background(), "// a comment\nSCROLL users {\"limit\": 5}"); err != nil {
		t.Fatalf("comment-prefixed query: %v", err)
	}
}

func TestRawQuerierImplemented(t *testing.T) {
	var _ adapter.RawQuerier = New()
}

func hasCol(cols []string, want string) bool {
	for _, c := range cols {
		if c == want {
			return true
		}
	}
	return false
}
