// Package migrations holds the platform metadata schema (SCHEMA.md) and an
// idempotent, embedded-SQL applier. It uses a small schema_migrations ledger
// table so each migration runs exactly once.
package migrations

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 0001_init.sql
var migration0001 string

//go:embed 0002_api_key_hash_index.sql
var migration0002 string

//go:embed 0003_org_project_settings.sql
var migration0003 string

//go:embed 0004_webhook_deliveries.sql
var migration0004 string

//go:embed 0005_mail_settings.sql
var migration0005 string

type migration struct {
	version string
	sql     string
}

var all = []migration{
	{version: "0001_init", sql: migration0001},
	{version: "0002_api_key_hash_index", sql: migration0002},
	{version: "0003_org_project_settings", sql: migration0003},
	{version: "0004_webhook_deliveries", sql: migration0004},
	{version: "0005_mail_settings", sql: migration0005},
}

// Apply runs all unapplied migrations inside transaction per migration.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("migrations: ensure ledger: %w", err)
	}

	for _, m := range all {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
			m.version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrations: apply %s: %w", m.version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, m.version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ExpectedVersions returns the migration versions this binary knows about,
// in apply order. Used by GET /readyz to detect schema drift.
func ExpectedVersions() []string {
	out := make([]string, 0, len(all))
	for _, m := range all {
		out = append(out, m.version)
	}
	return out
}

// AppliedVersions returns the set of migration versions already applied.
func AppliedVersions(ctx context.Context, conn *pgx.Conn) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}