package provision

import (
	"context"

	"github.com/openbase/openbase/internal/server"
)

// ServerProvisioner adapts the Compose ProvisionerInterface to the server's
// Provisioner contract (ARCHITECTURE.md §2.7 keeps provisioning swappable).
type ServerProvisioner struct {
	Inner ProvisionerInterface
}

var _ server.Provisioner = (*ServerProvisioner)(nil)

// Provision implements server.Provisioner.
func (p *ServerProvisioner) Provision(ctx context.Context, engine server.Engine) (server.ProvisionedInstance, error) {
	inst, err := p.Inner.Provision(ctx, Engine(engine))
	if err != nil {
		return server.ProvisionedInstance{}, err
	}
	return server.ProvisionedInstance{
		ContainerID: inst.ContainerID,
		Engine:      server.Engine(inst.Engine),
		ConnString:  inst.ConnString,
	}, nil
}

// Destroy implements server.Provisioner.
func (p *ServerProvisioner) Destroy(ctx context.Context, containerID string) error {
	return p.Inner.Destroy(ctx, containerID)
}
