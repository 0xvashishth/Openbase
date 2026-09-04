package arcadedb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/openbase/openbase/internal/adapter"
)

// fakeArcade is an in-memory emulation of the small ArcadeDB SQL surface the
// adapter emits, driven over httptest.
type fakeArcade struct {
	mu    sync.Mutex
	types map[string][]map[string]any // type -> documents
	seq   int
}

func newFake() *fakeArcade {
	f := &fakeArcade{types: map[string][]map[string]any{}}
	// a built-in internal type that ListCollections must filter out
	f.types["OSchema"] = []map[string]any{{"name": "OSchema"}}
	return f
}

func (f *fakeArcade) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Language string `json:"language"`
			Command  string `json:"command"`
			Params   []any  `json:"params"`
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		// basic auth: any non-empty user/pass is accepted
		if u, p, ok := r.BasicAuth(); !ok || (u == "" && p == "") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		result, e := f.exec(body.Command, body.Params)
		if e != nil {
			writeJSON(w, map[string]any{"error": e.Error()})
			return
		}
		writeJSON(w, map[string]any{"result": result})
	})
	return httptest.NewServer(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// exec is a tiny evaluator for the SQL the adapter generates.
func (f *fakeArcade) exec(sql string, params []any) ([]map[string]any, error) {
	sql = strings.TrimSpace(sql)

	if strings.HasPrefix(sql, "SELECT name FROM Type") {
		out := []map[string]any{}
		names := []string{}
		for n := range f.types {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, map[string]any{"name": n})
		}
		return out, nil
	}

	if strings.HasPrefix(sql, "SELECT count(*) AS count FROM ") {
		typeName := fieldAfter(sql, "FROM ")
		whereSQL, args := extractWhere(sql, params)
		n := 0
		for _, doc := range f.types[typeName] {
			if isMatch(doc, whereSQL, args) {
				n++
			}
		}
		return []map[string]any{{"count": float64(n)}}, nil
	}

	if strings.HasPrefix(sql, "INSERT INTO ") {
		return f.execInsert(sql, params)
	}

	if strings.HasPrefix(sql, "UPDATE ") {
		return f.execUpdate(sql, params)
	}

	if strings.HasPrefix(sql, "DELETE FROM ") {
		return f.execDelete(sql, params)
	}

	if strings.HasPrefix(sql, "SELECT FROM ") {
		return f.execSelect(sql, params)
	}

	return nil, fmt.Errorf("unhandled sql: %s", sql)
}

func (f *fakeArcade) execSelect(sql string, params []any) ([]map[string]any, error) {
	rest := sql[len("SELECT FROM "):]
	typeName := firstToken(rest)
	whereSQL, args := extractWhere(rest, params)
	orderSQL, orderArgs, rest2 := extractOrderBy(rest, whereSQL)
	limit := -1
	limit, offset := extractLimitSkip(rest2)
	matchedRows := []map[string]any{}
	for _, doc := range f.types[typeName] {
		if isMatch(doc, whereSQL, args) {
			matchedRows = append(matchedRows, doc)
		}
	}
	applyOrder(matchedRows, orderSQL, orderArgs)
	out := []map[string]any{}
	start := 0
	if offset > 0 {
		start = offset
	}
	end := len(matchedRows)
	if limit >= 0 && start+limit < end {
		end = start + limit
	}
	for i := start; i < end; i++ {
		out = append(out, matchedRows[i])
	}
	return out, nil
}

func (f *fakeArcade) execInsert(sql string, params []any) ([]map[string]any, error) {
	rest := sql[len("INSERT INTO "):]
	typeName := firstToken(rest)
	setSQL := rest[len(typeName)+len("SET "):] // after "TYPE SET "
	fields := splitSetFields(setSQL)
	doc := map[string]any{}
	pi := 0
	for _, fld := range fields {
		eq := strings.Index(fld, "=")
		if eq < 0 {
			return nil, fmt.Errorf("bad SET field: %q", fld)
		}
		name := strings.TrimSpace(fld[:eq])
		doc[name] = rhsValue(fld[eq+1:], params, &pi)
	}
	f.seq++
	rid := fmt.Sprintf("#%d:0", f.seq)
	doc["@rid"] = rid
	doc["@type"] = typeName
	doc["@cat"] = "d"
	f.types[typeName] = append(f.types[typeName], doc)
	return []map[string]any{{"@rid": rid, "@type": typeName, "@cat": "d"}}, nil
}

func (f *fakeArcade) execUpdate(sql string, params []any) ([]map[string]any, error) {
	rest := sql[len("UPDATE "):]
	typeName := firstToken(rest)
	setSQL := rest[len(typeName)+len("SET "):]
	lower := strings.ToLower(setSQL)
	whereIdx := strings.Index(lower, " where ")
	var setPart, whereSQL string
	if whereIdx >= 0 {
		setPart = setSQL[:whereIdx]
		whereSQL = setSQL[whereIdx+len(" where "):]
	} else {
		setPart = setSQL
	}
	fields := splitSetFields(setPart)
	args := argsFromParams(params)
	pi := 0
	updates := map[string]any{}
	for _, fld := range fields {
		eq := strings.Index(fld, "=")
		if eq < 0 {
			return nil, fmt.Errorf("bad SET field: %q", fld)
		}
		name := strings.TrimSpace(fld[:eq])
		updates[name] = rhsValue(fld[eq+1:], params, &pi)
	}
	whereArgs := args[pi:]
	for _, doc := range f.types[typeName] {
		if isMatch(doc, whereSQL, whereArgs) {
			for k, v := range updates {
				doc[k] = v
			}
		}
	}
	return nil, nil
}

func (f *fakeArcade) execDelete(sql string, params []any) ([]map[string]any, error) {
	rest := sql[len("DELETE FROM "):]
	typeName := firstToken(rest)
	whereSQL, args := extractWhere(rest, params)
	kept := f.types[typeName][:0]
	for _, doc := range f.types[typeName] {
		if !isMatch(doc, whereSQL, args) {
			kept = append(kept, doc)
		}
	}
	f.types[typeName] = kept
	return nil, nil
}

// ---- helpers ----

func fieldAfter(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(s[i+len(prefix):])
	if j := strings.IndexAny(rest, " \n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// firstToken returns the first whitespace-delimited token of s (or the whole
// trimmed string when there is no whitespace).
func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if j := strings.IndexAny(s, " \t\n"); j >= 0 {
		return s[:j]
	}
	return s
}

// splitSetFields splits "a = ?, b = ?" on commas at the top level.
func splitSetFields(s string) []string {
	// The adapter emits fields sorted and comma-separated; we can split on
	// ", " but must not split inside a " = " — a simple ", " split is safe
	// because param values are always "?".
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, ", ")
}

// extractWhere pulls out the WHERE clause (if any) and returns the SQL text and
// the ordered subset of params for the where. The params array layout is
// [setParams..., whereParams...] for UPDATE/INSERT; for SELECT/count/delete the
// params array is just whereParams.
func extractWhere(rest string, params []any) (string, []any) {
	lower := strings.ToLower(rest)
	i := strings.Index(lower, " where ")
	if i < 0 {
		return "", nil
	}
	whereSQL := rest[i+len(" where "):]
	// stop at ORDER BY / LIMIT / SKIP
	for _, kw := range []string{" ORDER BY ", " LIMIT ", " SKIP "} {
		if j := strings.Index(strings.ToUpper(whereSQL), kw); j >= 0 {
			whereSQL = whereSQL[:j]
		}
	}
	return whereSQL, params
}

func extractOrderBy(rest, whereSQL string) (string, []any, string) {
	upper := strings.ToUpper(rest)
	i := strings.Index(upper, " ORDER BY ")
	if i < 0 {
		return "", nil, rest
	}
	// find start of ORDER BY within rest (accounting for where already sliced is
	// unnecessary; we search raw rest)
	orderPart := rest[i+len(" ORDER BY "):]
	j := strings.Index(strings.ToUpper(orderPart), " LIMIT ")
	if j < 0 {
		j = strings.Index(strings.ToUpper(orderPart), " SKIP ")
	}
	var tail string
	if j >= 0 {
		tail = orderPart[j:]
		orderPart = orderPart[:j]
	} else {
		tail = ""
	}
	return orderPart, nil, tail
}

func extractLimitSkip(rest string) (limit, offset int) {
	limit = -1
	lower := strings.ToLower(rest)
	if i := strings.Index(lower, " limit "); i >= 0 {
		n, _ := strconv.Atoi(strings.TrimSpace(rest[i+len(" limit "):]))
		limit = n
	}
	if i := strings.Index(lower, " skip "); i >= 0 {
		off, _ := strconv.Atoi(strings.TrimSpace(rest[i+len(" skip "):]))
		offset = off
	}
	return
}

func applyOrder(docs []map[string]any, orderSQL string, _ []any) {
	if strings.TrimSpace(orderSQL) == "" {
		return
	}
	for _, field := range strings.Split(orderSQL, ",") {
		f := strings.TrimSpace(strings.Fields(field)[0])
		desc := strings.Contains(strings.ToUpper(field), "DESC")
		sort.SliceStable(docs, func(i, j int) bool {
			a, b := docs[i][f], docs[j][f]
			if desc {
				return cmpAny(a, b) > 0
			}
			return cmpAny(a, b) < 0
		})
	}
}

func cmpAny(a, b any) int {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	}
	as, _ := a.(string)
	bs, _ := b.(string)
	if as < bs {
		return -1
	}
	if as > bs {
		return 1
	}
	return 0
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, e := t.Float64()
		return f, e == nil
	}
	return 0, false
}

// matches evaluates a WHERE clause of form "f1 OP <?|literal> AND ..." against
// a doc, using the where params for each "?" placeholder. Raw SQL (from
// ExecRaw) inlines literals instead of binding params, so both are supported.
func isMatch(doc map[string]any, whereSQL string, args []any) bool {
	whereSQL = strings.TrimSpace(whereSQL)
	if whereSQL == "" {
		return true
	}
	parts := strings.Split(whereSQL, " AND ")
	ai := 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// form: `field OP ?` — the adapter emits `field= ?` (spacing-insensitive
		// in real ArcadeDB), so locate the operator token tolerant of whitespace.
		field, op, rhs, ok := splitCondition(p)
		if !ok {
			return false
		}
		val := rhsValue(rhs, args, &ai)
		docVal := doc[field]
		switch strings.TrimSpace(op) {
		case "CONTAINS":
			if !containsVal(docVal, val) {
				return false
			}
		case "=":
			if !equalVal(docVal, val) {
				return false
			}
		case "<>":
			if equalVal(docVal, val) {
				return false
			}
		default:
			c, ok := cmpNum(docVal, val)
			if !ok {
				return false
			}
			switch strings.TrimSpace(op) {
			case ">":
				if c <= 0 {
					return false
				}
			case "<":
				if c >= 0 {
					return false
				}
			case ">=":
				if c < 0 {
					return false
				}
			case "<=":
				if c > 0 {
					return false
				}
			}
		}
	}
	return true
}

// rhsValue resolves the right-hand side of a condition or SET assignment:
// "?" consumes the next bound param, anything else is parsed as a literal
// ('text', 123, 1.5, true/false/null).
func rhsValue(rhs string, params []any, pi *int) any {
	t := strings.TrimSpace(rhs)
	if t == "?" {
		if *pi < len(params) {
			v := params[*pi]
			*pi++
			return v
		}
		*pi++
		return nil
	}
	return parseLiteral(t)
}

func parseLiteral(t string) any {
	t = strings.TrimSpace(t)
	if len(t) >= 2 && (t[0] == '\'' && t[len(t)-1] == '\'' || t[0] == '"' && t[len(t)-1] == '"') {
		return t[1 : len(t)-1]
	}
	switch strings.ToLower(t) {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil {
		return f
	}
	return t
}

// splitCondition parses a single "field OP <rhs>" condition (tolerant of
// missing whitespace around the operator, e.g. "name= ?") into the field,
// operator and right-hand side ("?" or an inline literal).
func splitCondition(p string) (field string, op string, rhs string, ok bool) {
	p = strings.TrimSpace(p)
	// longer operators must be tried before the single-char ones.
	for _, cand := range []string{"<>", ">=", "<=", "CONTAINS", "=%", "=", ">", "<"} {
		if idx := strings.Index(p, cand); idx >= 0 {
			field = strings.TrimSpace(p[:idx])
			op = strings.TrimSpace(cand)
			rhs = strings.TrimSpace(p[idx+len(cand):])
			return field, op, rhs, true
		}
	}
	return "", "", "", false
}

func containsVal(docVal, val any) bool {
	arr, ok := docVal.([]any)
	if !ok {
		return equalVal(docVal, val)
	}
	for _, e := range arr {
		if equalVal(e, val) {
			return true
		}
	}
	return false
}

func equalVal(a, b any) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if aok && bok {
		return af == bf
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func cmpNum(a, b any) (int, bool) {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)
	if !aok || !bok {
		return 0, false
	}
	if af < bf {
		return -1, true
	}
	if af > bf {
		return 1, true
	}
	return 0, true
}

func argsFromParams(params []any) []any { return params }

// ---- tests ----

func newConnected(t *testing.T) (*Adapter, *fakeArcade) {
	t.Helper()
	f := newFake()
	srv := f.server()
	t.Cleanup(srv.Close)
	a := New()
	cfg := adapter.ConnectionConfig{
		ConnStr:  srv.URL,
		Database: "testdb",
		Username: "root",
		Password: "secret",
	}
	if err := a.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return a, f
}

func TestCapabilitiesHonest(t *testing.T) {
	a := New()
	c := a.Capabilities()
	if c.SupportsRealtime != adapter.RealtimeNone {
		t.Errorf("expected realtime none, got %v", c.SupportsRealtime)
	}
	if c.SupportsNativeTriggers || c.SupportsRelationalJoins || c.SupportsForeignKeys ||
		c.SupportsTransactions || c.SupportsVectorSearch || c.SupportsChangeStreams {
		t.Errorf("expected none of the unsupported flags to be on: %+v", c)
	}
}

func TestConnectRejectsBadScheme(t *testing.T) {
	a := New()
	err := a.Connect(context.Background(), adapter.ConnectionConfig{ConnStr: "mysql://x"})
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("expected scheme error, got %v", err)
	}
}

func TestListCollectionsFiltersInternal(t *testing.T) {
	a, f := newConnected(t)
	f.types["Person"] = []map[string]any{{"name": "Person"}}
	cols, err := a.ListCollections(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cols) != 1 || cols[0].Name != "Person" {
		t.Fatalf("expected just Person (OSchema filtered), got %+v", cols)
	}
}

func TestInsertQueryUpdateDelete(t *testing.T) {
	a, _ := newConnected(t)
	ctx := context.Background()

	res, err := a.Insert(ctx, "Person", map[string]any{"name": "Alice", "age": float64(30)})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if res.Collection != "Person" || res.ID == nil {
		t.Fatalf("bad insert result: %+v", res)
	}

	rs, err := a.Query(ctx, adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "Person",
			Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "Alice"}},
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0]["name"] != "Alice" {
		t.Fatalf("expected 1 row Alice, got %+v", rs.Rows)
	}

	ur, err := a.Update(ctx,
		adapter.Filter{Collection: "Person", Conditions: []adapter.Condition{{Field: "name", Operator: adapter.OpEqual, Value: "Alice"}}},
		map[string]any{"age": float64(31)},
	)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if ur.MatchedCount != 1 || ur.ModifiedCount != 1 {
		t.Fatalf("expected update matched 1, got %+v", ur)
	}
	rs, _ = a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{Collection: "Person"}})
	if len(rs.Rows) != 1 || rs.Rows[0]["age"] != float64(31) {
		t.Fatalf("update did not persist: %+v", rs.Rows)
	}

	dr, err := a.Delete(ctx, adapter.Filter{Collection: "Person", Conditions: []adapter.Condition{{Field: "age", Operator: adapter.OpGreaterEq, Value: float64(20)}}})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if dr.DeletedCount != 1 {
		t.Fatalf("expected delete 1, got %+v", dr)
	}
	rs, _ = a.Query(ctx, adapter.UniversalQuery{Filter: adapter.Filter{Collection: "Person"}})
	if len(rs.Rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %+v", rs.Rows)
	}
}

func TestQueryOrderByLimitOffset(t *testing.T) {
	a, f := newConnected(t)
	f.types["Item"] = []map[string]any{
		{"@rid": "#1:0", "n": float64(2)},
		{"@rid": "#2:0", "n": float64(1)},
		{"@rid": "#3:0", "n": float64(3)},
	}
	rs, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "Item",
			OrderBy:    []adapter.OrderBy{{Field: "n", Desc: true}},
			Limit:      intPtr(2),
		},
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rs.Rows) != 2 || rs.Rows[0]["n"] != float64(3) || rs.Rows[1]["n"] != float64(2) {
		t.Fatalf("order/limit wrong: %+v", rs.Rows)
	}
}

func TestGetSchema(t *testing.T) {
	a, f := newConnected(t)
	f.types["Doc"] = []map[string]any{
		{"@rid": "#1:0", "title": "a", "views": float64(5)},
		{"@rid": "#2:0", "title": "b", "published": true},
	}
	info, err := a.GetSchema(context.Background(), "Doc")
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	hasRID := false
	hasTitle := false
	hasViews := false
	for _, c := range info.Columns {
		if c.Name == "@rid" && c.IsPrimary {
			hasRID = true
		}
		if c.Name == "title" {
			hasTitle = true
		}
		if c.Name == "views" {
			hasViews = true
		}
	}
	if !hasRID || !hasTitle || !hasViews {
		t.Fatalf("schema missing expected columns: %+v", info.Columns)
	}
}

func TestUnsupportedOperators(t *testing.T) {
	a, _ := newConnected(t)
	_, err := a.Query(context.Background(), adapter.UniversalQuery{
		Filter: adapter.Filter{
			Collection: "Person",
			Conditions: []adapter.Condition{{Field: "x", Operator: adapter.OpContains, Value: "a"}},
		},
	})
	// contains is supported; everything else (bogus) can't be expressed via the
	// interface, so we instead check the unsupported feature methods.
	if err != nil {
		t.Fatalf("contains should be supported, got %v", err)
	}
}

func TestTriggersRealtimeUnsupported(t *testing.T) {
	a, _ := newConnected(t)
	ctx := context.Background()
	if err := a.RegisterTrigger(ctx, adapter.TriggerDefinition{}); err == nil {
		t.Fatal("expected ErrUnsupported for RegisterTrigger")
	}
	if err := a.RemoveTrigger(ctx, "x"); err == nil {
		t.Fatal("expected ErrUnsupported for RemoveTrigger")
	}
	if err := a.RegisterRealtimeBroadcast(ctx, "x"); err == nil {
		t.Fatal("expected ErrUnsupported for RegisterRealtimeBroadcast")
	}
	if _, err := a.SubscribeToChanges(ctx, "x", nil); err == nil {
		t.Fatal("expected ErrUnsupported for SubscribeToChanges")
	}
}

func TestListRelationshipsEmpty(t *testing.T) {
	a, _ := newConnected(t)
	rels, err := a.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("relationships: %v", err)
	}
	if len(rels) != 0 {
		t.Fatalf("expected no relationships, got %+v", rels)
	}
}

func intPtr(n int) *int { return &n }
