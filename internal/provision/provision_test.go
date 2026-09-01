package provision

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/server"
)

// fakeProvisioner records calls and returns canned results.
type fakeProvisioner struct {
	provisioned Instance
	errProvision error
	destroyed    []string
}

func (f *fakeProvisioner) Provision(ctx context.Context, engine Engine) (Instance, error) {
	return f.provisioned, f.errProvision
}

func (f *fakeProvisioner) Destroy(ctx context.Context, containerID string) error {
	f.destroyed = append(f.destroyed, containerID)
	return nil
}

var _ ProvisionerInterface = (*fakeProvisioner)(nil)

func TestServerProvisionerAdapter(t *testing.T) {
	inner := &fakeProvisioner{
		provisioned: Instance{ContainerID: "abc", Engine: EnginePostgres, ConnString: "postgres://u:p@localhost:1/d"},
	}
	sp := &ServerProvisioner{Inner: inner}

	got, err := sp.Provision(context.Background(), server.Engine(EnginePostgres))
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if got.ContainerID != "abc" || got.Engine != server.Engine(EnginePostgres) || got.ConnString == "" {
		t.Fatalf("unexpected instance: %+v", got)
	}

	if err := sp.Destroy(context.Background(), "abc"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if len(inner.destroyed) != 1 || inner.destroyed[0] != "abc" {
		t.Fatalf("destroyed = %v", inner.destroyed)
	}
}

func TestServerProvisionerErrPropagates(t *testing.T) {
	inner := &fakeProvisioner{errProvision: errors.New("no room")}
	sp := &ServerProvisioner{Inner: inner}

	_, err := sp.Provision(context.Background(), server.Engine(EnginePostgres))
	if err == nil {
		t.Fatal("expected error")
	}
}

// containerExists reports whether a Docker container with the given name exists.
func containerExists(name string) (bool, error) {
	_, err := exec.Command("docker", "inspect", name).Output()
	return err == nil, nil
}

func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// TestComposeFerretDBGroupRoundTrip provisions a real FerretDB group and
// verifies Destroy removes every piece: the FerretDB container, the DocumentDB
// backend, and the private network.
func TestComposeFerretDBGroupRoundTrip(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available; skipping ferretdb provisioning test")
	}

	c := &Compose{}
	ctx := context.Background()
	inst, err := c.Provision(ctx, EngineFerretDB)
	if err != nil {
		t.Fatalf("provision ferretdb: %v", err)
	}
	base := strings.TrimPrefix(inst.ContainerID, ferretPrefix)
	if base == inst.ContainerID {
		t.Fatalf("expected ferret-prefixed container id, got %q", inst.ContainerID)
	}

	for _, n := range []string{base + "-ferret", base + "-pg"} {
		exists, _ := containerExists(n)
		if !exists {
			t.Fatalf("expected container %s to exist after provisioning", n)
		}
	}

	if err := c.Destroy(ctx, inst.ContainerID); err != nil {
		t.Fatalf("destroy ferretdb group: %v", err)
	}
	for _, n := range []string{base + "-ferret", base + "-pg"} {
		if exists, _ := containerExists(n); exists {
			t.Fatalf("container %s should be gone after destroy", n)
		}
	}
	if exists, _ := containerExists(base + "-net"); exists {
		t.Fatalf("network %s should be gone after destroy", base+"-net")
	}
}