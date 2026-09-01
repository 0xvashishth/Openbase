package provision

import (
	"context"
	"errors"
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