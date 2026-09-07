package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pg := testutil.StartPostgres(t)
	pool, err := pgxpool.New(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestApplyIsIdempotent(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("second apply should be a no-op: %v", err)
	}
}

func TestTablesExistAfterApply(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"users", "organizations", "organization_members", "projects",
		"connections", "triggers", "functions", "api_keys", "schema_migrations",
		"webhook_deliveries",
	}
	for _, table := range want {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s missing after migration", table)
		}
	}

	// Constraints from SCHEMA.md.
	var checks []string
	rows, err := pool.Query(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'connections'::regclass`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var def string
		_ = rows.Scan(&def)
		checks = append(checks, def)
	}
	joined := strings.Join(checks, "\n")
	for _, wantSub := range []string{"'pending'", "'provisioned'", "'postgres'"} {
		if !strings.Contains(joined, wantSub) {
			t.Fatalf("connections constraints missing %q in:\n%s", wantSub, joined)
		}
	}
}
func TestAPIKeyHashIndexExists(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	if err := Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var isUnique bool
	if err := pool.QueryRow(ctx, `
		SELECT indisunique FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = 'idx_api_keys_hash'`).Scan(&isUnique); err != nil {
		t.Fatalf("idx_api_keys_hash missing: %v", err)
	}
	if !isUnique {
		t.Fatal("idx_api_keys_hash must be UNIQUE to close the duplicate-hash hole")
	}
}
