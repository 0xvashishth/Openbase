package arcadedb

import (
	"context"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
)

func TestExecRawSelectUsesQueryEndpoint(t *testing.T) {
	a, f := newConnected(t)
	f.types["Person"] = []map[string]any{
		{"@rid": "#1:0", "name": "Ada", "age": float64(36)},
		{"@rid": "#2:0", "name": "Lin", "age": float64(30)},
	}

	rs, err := a.ExecRaw(context.Background(), "SELECT FROM Person")
	if err != nil {
		t.Fatalf("ExecRaw select: %v", err)
	}
	if len(rs.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %+v", rs.Rows)
	}
	if !hasColumn(rs.Columns, "name") {
		t.Fatalf("columns = %+v, want name", rs.Columns)
	}
}

func TestExecRawWriteUsesCommandEndpoint(t *testing.T) {
	a, f := newConnected(t)
	ctx := context.Background()

	// INSERT returns the created record, so the editor shows the @rid row.
	rs, err := a.ExecRaw(ctx, "INSERT INTO Person SET name = 'Ada'")
	if err != nil {
		t.Fatalf("ExecRaw insert: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("insert result = %+v, want 1 row", rs.Rows)
	}
	if len(f.types["Person"]) != 1 || f.types["Person"][0]["name"] != "Ada" {
		t.Fatalf("insert did not persist: %+v", f.types["Person"])
	}

	// UPDATE returns no rows from ArcadeDB -> normalized to an OK row.
	rs, err = a.ExecRaw(ctx, "UPDATE Person SET name = 'Ada L' WHERE name = 'Ada'")
	if err != nil {
		t.Fatalf("ExecRaw update: %v", err)
	}
	if rs.Rows[0]["result"] != "OK" {
		t.Fatalf("update result = %+v, want OK", rs.Rows)
	}
	if f.types["Person"][0]["name"] != "Ada L" {
		t.Fatalf("update did not persist: %+v", f.types["Person"])
	}

	// DELETE likewise.
	rs, err = a.ExecRaw(ctx, "DELETE FROM Person WHERE name = 'Ada L'")
	if err != nil {
		t.Fatalf("ExecRaw delete: %v", err)
	}
	if rs.Rows[0]["result"] != "OK" {
		t.Fatalf("delete result = %+v, want OK", rs.Rows)
	}
	if len(f.types["Person"]) != 0 {
		t.Fatalf("delete did not persist: %+v", f.types["Person"])
	}
}

func TestExecRawTrailingSemicolonAndComments(t *testing.T) {
	a, f := newConnected(t)
	f.types["Person"] = []map[string]any{{"@rid": "#1:0", "name": "Ada"}}

	if _, err := a.ExecRaw(context.Background(), "SELECT FROM Person;"); err != nil {
		t.Fatalf("trailing semicolon: %v", err)
	}
	if _, err := a.ExecRaw(context.Background(), "-- a comment\nSELECT FROM Person"); err != nil {
		t.Fatalf("leading comment: %v", err)
	}
}

func TestExecRawErrors(t *testing.T) {
	a, _ := newConnected(t)
	ctx := context.Background()

	if _, err := a.ExecRaw(ctx, "   "); err == nil {
		t.Fatal("expected error for empty query")
	}
	// The fake server rejects SQL it doesn't understand; the adapter must
	// surface the engine's error rather than swallow it.
	if _, err := a.ExecRaw(ctx, "EXPLAIN SELECT FROM Person"); err == nil {
		t.Fatal("expected engine error to surface")
	} else if !strings.Contains(err.Error(), "arcadedb") {
		t.Fatalf("error should be attributed to arcadedb: %v", err)
	}

	notConnected := New()
	if _, err := notConnected.ExecRaw(ctx, "SELECT FROM Person"); err == nil {
		t.Fatal("expected not-connected error")
	}
}

func TestRawFirstKeyword(t *testing.T) {
	cases := map[string]string{
		"SELECT FROM Person":               "SELECT",
		"  select from person":             "SELECT",
		"-- c\nINSERT INTO Person SET a=1": "INSERT",
		"/* b */ UPDATE Person SET a=1":    "UPDATE",
		"(SELECT FROM Person)":             "SELECT",
		"":                                 "",
	}
	for in, want := range cases {
		if got := rawFirstKeyword(in); got != want {
			t.Errorf("rawFirstKeyword(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRawQuerierImplemented(t *testing.T) {
	var _ adapter.RawQuerier = New()
}

func hasColumn(cols []string, want string) bool {
	for _, c := range cols {
		if c == want {
			return true
		}
	}
	return false
}
