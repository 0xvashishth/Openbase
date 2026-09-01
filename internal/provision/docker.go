package provision

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Engine constants for the engines we can provision.
const (
	EnginePostgres Engine = "postgres"
	EngineFerretDB Engine = "ferretdb"
)

// ferretPrefix marks a provisional instance that is really a group of
// containers (FerretDB + its DocumentDB Postgres backend + a network). The
// stored container_id is "<ferretPrefix><deployment name>" so Destroy knows it
// must tear down the whole group rather than one container.
const ferretPrefix = "ferret|"

// imageFor maps an engine to its primary container image.
func imageFor(e Engine) (string, error) {
	switch e {
	case EnginePostgres:
		return "postgres:16-alpine", nil
	case EngineFerretDB:
		return "ghcr.io/ferretdb/ferretdb:2.7.0", nil
	default:
		return "", fmt.Errorf("provisioning engine %q not supported", e)
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Compose is a Docker-backed ProvisionerInterface. It runs each instance as a
// dedicated container (or a small group, in FerretDB's case) published on a
// free host port, which mirrors how a real host-based platform would hand out
// per-project instances.
type Compose struct {
	// Registry is a pass-through host registry for containers (e.g. leave empty
	// for the Docker CLI which talks to the local daemon).
	Registry string
}

// Provision starts a dedicated database instance for engine and returns its
// connection string.
func (p *Compose) Provision(ctx context.Context, engine Engine) (Instance, error) {
	if err := ctx.Err(); err != nil {
		return Instance{}, err
	}
	switch engine {
	case EnginePostgres:
		return p.provisionPostgres(ctx)
	case EngineFerretDB:
		return p.provisionFerretDB(ctx)
	default:
		img, err := imageFor(engine)
		if err != nil {
			return Instance{}, err
		}
		_ = img
		return Instance{}, fmt.Errorf("provisioning engine %q not supported", engine)
	}
}

func (p *Compose) provisionPostgres(ctx context.Context) (Instance, error) {
	user := "openbase"
	password := randomHex(16)
	db := "openbase"
	port, err := freePort()
	if err != nil {
		return Instance{}, fmt.Errorf("reserving host port: %w", err)
	}

	name := "openbase-provisioned-" + randomHex(6)
	args := []string{
		"run", "-d",
		"--name", name,
		"-p", itoa(port) + ":5432",
		"-e", "POSTGRES_USER=" + user,
		"-e", "POSTGRES_PASSWORD=" + password,
		"-e", "POSTGRES_DB=" + db,
		"postgres:16-alpine",
	}
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
		return Instance{}, fmt.Errorf("starting provisioned container: %w: %s", err, out)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%d/%s?sslmode=disable", user, password, port, db)
	instance := Instance{ContainerID: name, Engine: EnginePostgres, ConnString: dsn}

	if err := p.waitPostgresReady(ctx, dsn); err != nil {
		_ = p.Destroy(ctx, name)
		return Instance{}, fmt.Errorf("provisioned database not ready: %w", err)
	}
	return instance, nil
}

// provisionFerretDB spins up a group: a DocumentDB-enabled Postgres backend and
// a FerretDB container on a private network, publishing the Mongo wire port.
func (p *Compose) provisionFerretDB(ctx context.Context) (Instance, error) {
	user := "openbase"
	password := randomHex(16)

	base := "openbase-provisioned-" + randomHex(6)
	netName := base + "-net"
	pgName := base + "-pg"
	fName := base + "-ferret"

	port, err := freePort()
	if err != nil {
		return Instance{}, fmt.Errorf("reserving host port: %w", err)
	}

	docker := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		return string(out), err
	}

	cleanup := func() {
		_, _ = docker("rm", "-f", fName, pgName)
		_, _ = docker("network", "rm", netName)
	}

	if _, err := docker("network", "create", netName); err != nil {
		return Instance{}, fmt.Errorf("creating network %s: %s", netName, err)
	}

	// DocumentDB backend (its init script installs the extension into 'postgres').
	if _, err := docker("run", "-d",
		"--name", pgName,
		"--network", netName,
		"-e", "POSTGRES_USER="+user,
		"-e", "POSTGRES_PASSWORD="+password,
		"-e", "POSTGRES_DB=postgres",
		"ghcr.io/ferretdb/postgres-documentdb:17-0.107.0-ferretdb-2.7.0",
	); err != nil {
		cleanup()
		return Instance{}, fmt.Errorf("starting ferretdb backend: %w", err)
	}

	if _, err := docker("run", "-d",
		"--name", fName,
		"--network", netName,
		"-p", itoa(port)+":27017",
		"-e", "FERRETDB_POSTGRESQL_URL=postgres://"+user+":"+password+"@"+pgName+":5432/postgres",
		"-e", "FERRETDB_AUTH=false",
		"ghcr.io/ferretdb/ferretdb:2.7.0",
	); err != nil {
		cleanup()
		return Instance{}, fmt.Errorf("starting ferretdb container: %w", err)
	}

	dsn := fmt.Sprintf("mongodb://localhost:%d/", port)
	instance := Instance{
		ContainerID: ferretPrefix + base,
		Engine:      EngineFerretDB,
		ConnString:  dsn,
	}

	if err := p.waitFerretReady(ctx, dsn); err != nil {
		_ = p.Destroy(ctx, instance.ContainerID)
		return Instance{}, fmt.Errorf("provisioned ferretdb not ready: %w", err)
	}
	return instance, nil
}

func (p *Compose) waitPostgresReady(ctx context.Context, dsn string) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			pingErr := pool.Ping(pingCtx)
			cancel()
			pool.Close()
			if pingErr == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
	return errors.New("timed out waiting for postgres readiness")
}

func (p *Compose) waitFerretReady(ctx context.Context, dsn string) error {
	deadline := time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		client, err := mongo.Connect(options.Client().ApplyURI(dsn))
		if err == nil {
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			pingErr := client.Ping(pingCtx, nil)
			cancel()
			_ = client.Disconnect(ctx)
			if pingErr == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
	return errors.New("timed out waiting for ferretdb readiness")
}

// Destroy stops and removes a provisioned instance. It understands the grouped
// FerretDB identifier and tears down the whole resource group.
func (p *Compose) Destroy(ctx context.Context, containerID string) error {
	if containerID == "" {
		return nil
	}

	if strings.HasPrefix(containerID, ferretPrefix) {
		base := strings.TrimPrefix(containerID, ferretPrefix)
		out, err := exec.CommandContext(ctx, "docker", "rm", "-f", base+"-ferret", base+"-pg").CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such container") {
			return fmt.Errorf("removing provisioned ferretdb group %s: %w: %s", base, err, out)
		}
		if out2, err2 := exec.CommandContext(ctx, "docker", "network", "rm", base+"-net").CombinedOutput(); err2 != nil &&
			!strings.Contains(string(out2), "No such network") {
			return fmt.Errorf("removing provisioned network %s-net: %w: %s", base, err2, out2)
		}
		return nil
	}

	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", containerID).CombinedOutput()
	if err != nil {
		// A missing container (already removed) is not an error worth surfacing.
		if strings.Contains(string(out), "No such container") {
			return nil
		}
		return fmt.Errorf("removing provisioned container %s: %w: %s", containerID, err, out)
	}
	return nil
}

// Compile-time check that Compose satisfies the interface.
var _ ProvisionerInterface = (*Compose)(nil)