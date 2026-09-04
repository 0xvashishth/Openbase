package server

import (
	"strings"
	"testing"
)

func TestValidateRawSQL(t *testing.T) {
	allowed := []string{
		// Reads (legacy read-only surface still allowed).
		"SELECT * FROM users",
		"  select id from orders limit 10",
		"WITH recent AS (SELECT * FROM users) SELECT * FROM recent",
		"EXPLAIN SELECT * FROM users",
		"-- a comment\nSELECT 1",
		"/* block */ SELECT 1",
		"(SELECT 1)",
		"SELECT * FROM users LIMIT 25;",
		// Writes/DDL now allowed (full read+write editor).
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET a=1",
		"DELETE FROM users",
		"DROP TABLE users",
		"CREATE TABLE x (id int)",
		"ALTER TABLE users ADD COLUMN x int",
		"TRUNCATE users",
		"GRANT SELECT ON users TO x",
		"CALL do_something()",
		// Non-SQL native raw languages.
		`db.users.find({ "status": "active" })`,
		`db.users.insertOne({ "name": "Ada" })`,
		`{"collection":"users","op":"find","filter":{}}`,
		"KEYS *",
		"SET k v",
		`SCROLL mycol {"limit": 25}`,
		"SELECT FROM Person LIMIT 25",
		// Semicolons inside strings are not stacked statements.
		"INSERT INTO t (a) VALUES ('a;b')",
		`db.users.find({ "note": "a;b" })`,
	}
	for _, q := range allowed {
		if err := validateRawSQL(q); err != nil {
			t.Errorf("expected allowed, got %v for %q", err, q)
		}
	}

	denied := []string{
		"",
		"   ",
		"-- only a comment",
		"/* unterminated",
		// Stacked statements are rejected (one statement per run).
		"SELECT 1; DROP TABLE users",
		"SELECT 1; SELECT 2",
		"INSERT INTO t VALUES (1); DELETE FROM t",
	}
	for _, q := range denied {
		if err := validateRawSQL(q); err == nil {
			t.Errorf("expected denied for %q", q)
		}
	}

	if err := validateRawSQL(strings.Repeat("SELECT 1 ", 5000)); err == nil {
		t.Error("expected oversized query to be denied")
	}
}
