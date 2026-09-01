package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FerretDBContainer manages a throwaway FerretDB instance (MongoDB-wire
// compatible) together with its DocumentDB-enabled PostgreSQL backend, both on
// a private Docker network so FerretDB can reach Postgres by name.
type FerretDBContainer struct {
	// ConnString is the mongodb:// URL for tests to connect to.
	ConnString string
	// Port is the published FerretDB host port.
	Port string
}

const (
	ferretImage    = "ghcr.io/ferretdb/ferretdb:2.7.0"
	ferretPGImage  = "ghcr.io/ferretdb/postgres-documentdb:17-0.107.0-ferretdb-2.7.0"
	ferretUser     = "ferret"
	ferretPassword = "ferret"
	// The DocumentDB-enabled Postgres only installs its extension into the
	// database named here (its init script targets 'postgres').
	ferretDatabase = "postgres"
)

// StartFerretDB boots FerretDB + its Postgres backend in Docker. It skips (t.Skip)
// when Docker is unavailable, mirroring StartPostgres. Returns a mongodb URL.
func StartFerretDB(t *testing.T) *FerretDBContainer {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not available; skipping ferretdb integration test: %v", err)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("docker daemon not reachable; skipping ferretdb integration test: %v", err)
	}

	base := fmt.Sprintf("ob-ferret-%d-%d", os.Getpid(), nextNameCounter())
	network := base + "-net"
	pgName := base + "-pg"
	fName := base + "-ferret"
	fPort := mustFreePort(t)

	rmAll := func() {
		_ = exec.Command("docker", "rm", "-f", fName).Run()
		_ = exec.Command("docker", "rm", "-f", pgName).Run()
		_ = exec.Command("docker", "network", "rm", network).Run()
	}
	t.Cleanup(rmAll)

	if out, err := exec.Command("docker", "network", "create", network).CombinedOutput(); err != nil {
		rmAll()
		t.Skipf("could not create network: %v: %s", err, out)
	}

	pgOut, err := exec.Command("docker", "run", "-d",
		"--name", pgName,
		"--network", network,
		"-e", "POSTGRES_USER="+ferretUser,
		"-e", "POSTGRES_PASSWORD="+ferretPassword,
		"-e", "POSTGRES_DB="+ferretDatabase,
		ferretPGImage,
	).CombinedOutput()
	if err != nil {
		rmAll()
		t.Skipf("could not start ferretdb postgres backend: %v: %s", err, pgOut)
	}

	fOut, err := exec.Command("docker", "run", "-d",
		"--name", fName,
		"--network", network,
		"-p", fPort+":27017",
		"-e", "FERRETDB_POSTGRESQL_URL=postgres://"+ferretUser+":"+ferretPassword+"@"+pgName+":5432/"+ferretDatabase,
		// Disable auth for test simplicity and disable telemetry.
		"-e", "FERRETDB_AUTH=false",
		"-e", "FERRETDB_TELEMETRY=disable",
		ferretImage,
	).CombinedOutput()
	if err != nil {
		rmAll()
		t.Skipf("could not start ferretdb container: %v: %s", err, fOut)
	}

	c := &FerretDBContainer{
		ConnString: fmt.Sprintf("mongodb://127.0.0.1:%s/", fPort),
		Port:       fPort,
	}
	if err := c.waitReady(t); err != nil {
		rmAll()
		t.Skipf("ferretdb container did not become ready: %v", err)
	}
	return c
}

func (c *FerretDBContainer) waitReady(t *testing.T) error {
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		client, err := mongo.Connect(options.Client().ApplyURI(c.ConnString))
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			pingErr := client.Ping(ctx, nil)
			cancel()
			_ = client.Disconnect(context.Background())
			if pingErr == nil {
				return nil
			}
		}
		time.Sleep(700 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for ferretdb readiness")
}

var ferretCounter int

func nextNameCounter() int {
	ferretCounter++
	return ferretCounter
}

func mustFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprintf("%d", port)
}