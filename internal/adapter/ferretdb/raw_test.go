package ferretdb

import (
	"strings"
	"testing"
)

func TestParseMongoShell(t *testing.T) {
	cases := []struct {
		in        string
		coll      string
		op        string
		args      string
		chain     string
		wantError bool
	}{
		{in: `db.users.find({})`, coll: "users", op: "find", args: "{}"},
		{in: `db.users.find({ "a": 1 }, { "limit": 5 })`, coll: "users", op: "find", args: `{ "a": 1 }, { "limit": 5 }`},
		{in: `db.users.find({}).limit(10).skip(2)`, coll: "users", op: "find", args: "{}", chain: ".limit(10).skip(2)"},
		{in: `db.users.insertOne({ "n": 1 })`, coll: "users", op: "insertOne", args: `{ "n": 1 }`},
		{in: `db.users.drop()`, coll: "users", op: "drop", args: ""},
		{in: `db.createCollection("logs")`, coll: "createCollection", op: "createCollection", args: `"logs"`},
		{in: `SELECT 1`, wantError: true},
		{in: `db.users`, wantError: true},
		{in: `db.users.find({}`, wantError: true},
	}
	for _, c := range cases {
		coll, op, args, chain, err := parseMongoShell(c.in)
		if c.wantError {
			if err == nil {
				t.Errorf("parseMongoShell(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMongoShell(%q) error = %v", c.in, err)
			continue
		}
		if coll != c.coll || op != c.op {
			t.Errorf("parseMongoShell(%q) = (%q,%q), want (%q,%q)", c.in, coll, op, c.coll, c.op)
		}
		if strings.TrimSpace(args) != strings.TrimSpace(c.args) {
			t.Errorf("parseMongoShell(%q) args = %q, want %q", c.in, args, c.args)
		}
		if chain != c.chain {
			t.Errorf("parseMongoShell(%q) chain = %q, want %q", c.in, chain, c.chain)
		}
	}
}

func TestSplitTopLevelArgs(t *testing.T) {
	got, err := splitTopLevelArgs(`{ "a": 1, "b": [1,2] }, { "limit": 5 }`)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d args: %+v", len(got), got)
	}
	if strings.TrimSpace(got[0]) != `{ "a": 1, "b": [1,2] }` {
		t.Errorf("arg0 = %q", got[0])
	}
	if strings.TrimSpace(got[1]) != `{ "limit": 5 }` {
		t.Errorf("arg1 = %q", got[1])
	}

	// Commas inside strings must not split.
	got, err = splitTopLevelArgs(`{ "note": "a,b" }`)
	if err != nil || len(got) != 1 {
		t.Fatalf("string comma split wrong: %v %+v", err, got)
	}

	if _, err := splitTopLevelArgs(`{ "a": 1`); err == nil {
		t.Error("expected unbalanced-brace error")
	}
	if _, err := splitTopLevelArgs(`{ "a": "x }`); err == nil {
		t.Error("expected unterminated-string error")
	}
}

func TestParseChainHelpers(t *testing.T) {
	if n, ok := parseChainInt(".limit(25).skip(5)", "limit"); !ok || n != 25 {
		t.Errorf("limit = %d ok=%v, want 25 true", n, ok)
	}
	if n, ok := parseChainInt(".limit(25).skip(5)", "skip"); !ok || n != 5 {
		t.Errorf("skip = %d ok=%v, want 5 true", n, ok)
	}
	if _, ok := parseChainInt(".limit(25)", "sort"); ok {
		t.Error("sort should not parse as int")
	}
	if got := parseChainDoc(`.sort({ "year": -1 })`, "sort"); strings.TrimSpace(got) != `{ "year": -1 }` {
		t.Errorf("sort doc = %q", got)
	}
	if got := parseChainDoc(".limit(5)", "sort"); got != "" {
		t.Errorf("missing sort should be empty, got %q", got)
	}
}

func TestStripMongoComments(t *testing.T) {
	in := `// leading comment
db.users.find({ "url": "http://x//y" }) // trailing comment`
	out := stripMongoComments(in)
	if strings.Contains(out, "leading comment") || strings.Contains(out, "trailing comment") {
		t.Fatalf("comments not stripped: %q", out)
	}
	if !strings.Contains(out, `"url": "http://x//y"`) {
		t.Fatalf("stripped inside string literal: %q", out)
	}
}

func TestHasDollarKeyAndToInt64(t *testing.T) {
	if !hasDollarKey(map[string]any{"$set": 1}) {
		t.Error("expected $set detected")
	}
	if hasDollarKey(map[string]any{"name": 1}) {
		t.Error("plain doc should not be a dollar update")
	}
	for _, v := range []any{int(3), int32(3), int64(3), float64(3)} {
		if n, ok := toInt64(v); !ok || n != 3 {
			t.Errorf("toInt64(%T) = %d ok=%v", v, n, ok)
		}
	}
	if _, ok := toInt64("3"); ok {
		t.Error("strings should not convert")
	}
}

func TestExecRawNotConnected(t *testing.T) {
	a := New()
	if _, err := a.ExecRaw(nil, `db.users.find({})`); err == nil { //nolint:staticcheck // nil ctx never used before the guard
		t.Fatal("expected not-connected error")
	}
}
