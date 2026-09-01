// Package testutil provides a Docker-backed Postgres for integration tests of
// the metadata store and adapters. It mirrors ADAPTERS.md §4's rule that
// adapter tests must run against a real instance rather than mocks. Tests skip
// themselves (t.Skip) if Docker or a pre-provisioned database is unavailable.
package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresContainer manages a throwaway Postgres instance.
type PostgresContainer struct {
	DSN        string
	Port       string
	container string
}

var (
	mu      sync.Mutex
	lastNum int
)

func nextName() string {
	mu.Lock()
	defer mu.Unlock()
	lastNum++
	return fmt.Sprintf("openbase-test-pg-%d-%d", os.Getpid(), lastNum)
}

const (
	pgImage = "postgres:16-alpine"
	user    = "test"
	password = "test"
	db      = "test"
)

// StartPostgres boots a Postgres 16 container. On any failure (no docker, pull
// issues, boot timeout) it calls t.Skip so suites degrade gracefully in
// environments without docker.
func StartPostgres(t *testing.T) *PostgresContainer {
	t.Helper()

	// Allow callers to point at an existing instance.
	if dsn := os.Getenv("OPENBASE_TEST_DATABASE_URL"); dsn != "" {
		return &PostgresContainer{DSN: dsn}
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not available; skipping integration test: %v", err)
	}

	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker daemon not reachable; skipping integration test: %v", err)
	}

	name := nextName()
	port := freePort(t)

	remove := func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	}
	t.Cleanup(remove)

	cmd := exec.Command("docker", "run", "-d",
		"--name", name,
		"-p", port+":5432",
		"-e", "POSTGRES_USER="+user,
		"-e", "POSTGRES_PASSWORD="+password,
		"-e", "POSTGRES_DB="+db,
		pgImage,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		remove()
		t.Skipf("could not start postgres container: %v: %s", err, out)
	}
	c := &PostgresContainer{
		DSN:        fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", user, password, port, db),
		Port:       port,
		container: name,
	}
	if err := c.waitReady(t); err != nil {
		remove()
		t.Skipf("postgres container did not become ready: %v", err)
	}
	return c
}

func (c *PostgresContainer) waitReady(t *testing.T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		// Probe with a real TCP connection + ping-style round trip. Grepping
		// docker logs is unreliable because the init phase prints a readiness
		// line for its temporary server before the real one boots.
		pool, err := pgxpool.New(ctx, c.DSN)
		if err == nil {
			pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
			err = pool.Ping(pingCtx)
			pingCancel()
			pool.Close()
			if err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for postgres readiness")
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("%d", port)
}