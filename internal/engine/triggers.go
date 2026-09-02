package engine

import (
	"context"
	"fmt"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/triggers"
)

// TriggerConnector adapts a Factory for use by the trigger runtime. It connects
// the project adapter and returns it as the subset the runtime needs.
type TriggerConnector struct {
	Factory *Factory
}

var _ triggers.Connector = (*TriggerConnector)(nil)

// Connect connects a project's adapter for trigger wiring.
func (c *TriggerConnector) Connect(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (triggers.DB, error) {
	if c.Factory == nil {
		return nil, fmt.Errorf("no factory configured")
	}
	a, err := c.Factory.ConnectForProject(ctx, conn, secret)
	if err != nil {
		return nil, err
	}
	db, ok := a.(triggers.DB)
	// The engine *Conn implements triggers.DB (RegisterTrigger, SubscribeToChanges,
	// Disconnect); pin the assertion so future adapters report honestly.
	if !ok {
		return nil, fmt.Errorf("%w: connected adapter does not support triggers", adapter.ErrUnsupported)
	}
	return db, nil
}
