package ferretdb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/adapter/ferretdb"
	"github.com/openbase/openbase/internal/testutil"
)

func newAdapter(t *testing.T) *ferretdb.Adapter {
	t.Helper()
	c := testutil.StartFerretDB(t)
	a := ferretdb.New()
	if err := a.Connect(context.Background(), adapter.ConnectionConfig{
		Engine:  adapter.EngineFerretDB,
		ConnStr: c.ConnString,
	}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

func TestCapabilitiesAreHonest(t *testing.T) {
	a := ferretdb.New()
	caps := a.Capabilities()
	if caps.SupportsForeignKeys || caps.SupportsRelationalJoins || caps.SupportsNativeTriggers ||
		caps.SupportsChangeStreams || caps.SupportsRealtime != adapter.RealtimeNone ||
		caps.SupportsVectorSearch {
		t.Fatalf("ferretdb capabilities should be conservative, got %+v", caps)
	}
}

func TestCRUDAndIntrospection(t *testing.T) {
	a := newAdapter(t)
	ctx := context.Background()
	coll := "movies"

	// Insert.
	titles := []string{"Alien", "Arrival", "The Matrix", "Blade Runner"}
	for i, title := range titles {
		doc := map[string]any{
			"title":    title,
			"year":     1979 + i*10,
			"rating":   float64(8 + i),
			"is_classic": i < 3,
		}
		res, err := a.Insert(ctx, coll, doc)
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		if res.ID == nil || res.ID == "" {
			t.Fatalf("insert %d returned no id: %+v", i, res)
		}
	}

	// ListCollections.
	cols, err := a.ListCollections(ctx)
	if err != nil {
		t.Fatalf("list collections: %v", err)
	}
	found := false
	for _, c := range cols {
		if c.Name == coll {
			found = true
		}
	}
	if !found {
		t.Fatalf("collection %q not listed: %v", coll, cols)
	}

	// GetSchema: fields sampled from documents plus _id primary.
	schema, err := a.GetSchema(ctx, coll)
	if err != nil {
		t.Fatalf("get schema: %v", err)
	}
	nameSet := map[string]adapter.ColumnInfo{}
	for _, col := range schema.Columns {
		nameSet[col.Name] = col
	}
	if nameSet["title"].Name == "" || nameSet["year"].Name == "" || nameSet["rating"].Name == "" {
		t.Fatalf("schema should sample document fields, got %+v", schema.Columns)
	}
	if !nameSet["_id"].IsPrimary {
		t.Fatalf("_id should be primary in ferretdb schema, got %+v", nameSet)
	}

	// Query with a filter + sort + limit.
	rs, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{
		Collection: coll,
		Conditions: []adapter.Condition{{Field: "year", Operator: adapter.OpGreaterThan, Value: 1995}},
		OrderBy:    []adapter.OrderBy{{Field: "year", Desc: true}},
		Limit:      intPtr(2),
	}})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rs.Rows) != 2 {
		t.Fatalf("expected 2 rows (year>1995), got %d: %+v", len(rs.Rows), rs.Rows)
	}
	if got := num(rs.Rows[0]["year"]); got != 2009 {
		t.Fatalf("first row year = %v, want 2009 (desc order)", got)
	}
	if !containsCol(rs.Columns, "title") || !containsCol(rs.Columns, "_id") {
		t.Fatalf("result columns missing keys: %v", rs.Columns)
	}

	// String contains.
	contains, err := a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{
		Collection: coll,
		Conditions: []adapter.Condition{{Field: "title", Operator: adapter.OpContains, Value: "Matrix"}},
	}})
	if err != nil {
		t.Fatalf("contains query: %v", err)
	}
	if len(contains.Rows) != 1 {
		t.Fatalf("contains should match 1 row, got %d", len(contains.Rows))
	}

	// Update matching rows.
	upd, err := a.Update(ctx, adapter.Filter{
		Collection: coll,
		Conditions: []adapter.Condition{{Field: "title", Operator: adapter.OpEqual, Value: "Alien"}},
	}, map[string]any{"rating": 9.5})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.MatchedCount != 1 || upd.ModifiedCount != 1 {
		t.Fatalf("update counts = %+v, want 1/1", upd)
	}

	// Delete matching rows.
	del, err := a.Delete(ctx, adapter.Filter{
		Collection: coll,
		Conditions: []adapter.Condition{{Field: "year", Operator: adapter.OpEqual, Value: 2009}},
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if del.DeletedCount != 1 {
		t.Fatalf("deleted count = %d, want 1", del.DeletedCount)
	}
}

func TestUnsupportedMethodsReturnErrUnsupported(t *testing.T) {
	a := newAdapter(t)
	ctx := context.Background()

	if err := a.RegisterTrigger(ctx, adapter.TriggerDefinition{}); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("RegisterTrigger err = %v, want ErrUnsupported", err)
	}
	if err := a.RemoveTrigger(ctx, "x"); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("RemoveTrigger err = %v, want ErrUnsupported", err)
	}
	if _, err := a.SubscribeToChanges(ctx, "x", nil); !errors.Is(err, adapter.ErrUnsupported) {
		t.Fatalf("SubscribeToChanges err = %v, want ErrUnsupported", err)
	}

	rels, err := a.ListRelationships(ctx)
	if err != nil {
		t.Fatalf("list relationships should not error on ferretdb (no FKs): %v", err)
	}
	if len(rels) != 0 {
		t.Fatalf("relationships should be empty, got %v", rels)
	}
}

func containsCol(cols []string, want string) bool {
	for _, c := range cols {
		if c == want {
			return true
		}
	}
	return false
}

func intPtr(v int) *int { return &v }

// num coerces integer/float values for numeric comparison across engines.
func num(v any) float64 {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case float64:
		return t
	default:
		return -1
	}
}