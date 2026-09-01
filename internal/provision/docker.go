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
)

// Engine constants for the engines we can provision. FerretDB will join later.
const (
	EnginePostgres Engine = "postgres"
)

// imageFor maps an engine to its container image.
func imageFor(e Engine) (string, error) {
	switch e {
	case EnginePostgres:
		return "postgres:16-alpine", nil
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
// dedicated container published on a free host port, which mirrors how a real
// host-based platform would hand out per-project instances.
type Compose struct {
	// Registry is a pass-through host registry for containers (e.g. leave empty
	// for the Docker CLI which talks to the local daemon).
	Registry string
}

// Provision starts a dedicated Postgres container for engine and returns its
// connection string.
func (p *Compose) Provision(ctx context.Context, engine Engine) (Instance, error) {
	if err := ctx.Err(); err != nil {
		return Instance{}, err
	}
	img, err := imageFor(engine)
	if err != nil {
		return Instance{}, err
	}

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
		img,
	}
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
		return Instance{}, fmt.Errorf("starting provisioned container: %w: %s", err, out)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%d/%s?sslmode=disable", user, password, port, db)
	instance := Instance{ContainerID: name, Engine: engine, ConnString: dsn}

	if err := p.waitReady(ctx, dsn); err != nil {
		_ = p.Destroy(ctx, name)
		return Instance{}, fmt.Errorf("provisioned database not ready: %w", err)
	}
	return instance, nil
}

func (p *Compose) waitReady(ctx context.Context, dsn string) error {
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
	return errors.New("timed out waiting for database readiness")
}

// Destroy stops and removes a provisioned container.
func (p *Compose) Destroy(ctx context.Context, containerID string) error {
	if containerID == "" {
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
