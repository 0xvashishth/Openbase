package server

import (
	"strings"
	"testing"
)

func TestValidateReadOnlySQL(t *testing.T) {
	allowed := []string{
		"SELECT * FROM users",
		"  select id from orders limit 10",
		"WITH recent AS (SELECT * FROM users) SELECT * FROM recent",
		"EXPLAIN SELECT * FROM users",
		"-- a comment\nSELECT 1",
		"/* block */ SELECT 1",
		"(SELECT 1)",
	}
	for _, q := range allowed {
		if err := validateReadOnlySQL(q); err != nil {
			t.Errorf("expected allowed, got %v for %q", err, q)
		}
	}

	denied := []string{
		"",
		"   ",
		"INSERT INTO users VALUES (1)",
		"UPDATE users SET a=1",
		"DELETE FROM users",
		"DROP TABLE users",
		"CREATE TABLE x (id int)",
		"ALTER TABLE users ADD COLUMN x int",
		"TRUNCATE users",
		"GRANT SELECT ON users TO x",
		"COPY users FROM '/tmp/x'",
		"CALL do_something()",
		"-- only a comment",
		"/* unterminated",
	}
	for _, q := range denied {
		if err := validateReadOnlySQL(q); err == nil {
			t.Errorf("expected denied for %q", q)
		}
	}

	if err := validateReadOnlySQL(strings.Repeat("SELECT 1 ", 5000)); err == nil {
		t.Error("expected oversized query to be denied")
	}
}
