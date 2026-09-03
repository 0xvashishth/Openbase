// Package engine wires adapters together: it resolves an adapter for a detected
// engine, connects it, and supports BYODB test connections. See ADAPTERS.md.
package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/adapter/ferretdb"
	"github.com/openbase/openbase/internal/adapter/mysql"
	"github.com/openbase/openbase/internal/adapter/postgres"
	"github.com/openbase/openbase/internal/adapter/qdrant"
	"github.com/openbase/openbase/internal/adapter/valkey"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/server"
)

// Conn holds a connected adapter.
type Conn struct {
	Engine  adapter.Engine
	Adapter adapter.DatabaseAdapter
}

// Disconnect closes the underlying adapter connection.
func (c *Conn) Disconnect(ctx context.Context) error {
	if c.Adapter == nil {
		return nil
	}
	return c.Adapter.Disconnect(ctx)
}

// ListCollections forwards to the underlying adapter.
func (c *Conn) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	return c.Adapter.ListCollections(ctx)
}

// GetSchema forwards to the underlying adapter.
func (c *Conn) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	return c.Adapter.GetSchema(ctx, collection)
}

// Query forwards to the underlying adapter.
func (c *Conn) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	return c.Adapter.Query(ctx, q)
}

// ListRelationships forwards to the underlying adapter.
func (c *Conn) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	return c.Adapter.ListRelationships(ctx)
}

// Insert forwards to the underlying adapter.
func (c *Conn) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	return c.Adapter.Insert(ctx, collection, doc)
}

// Update forwards to the underlying adapter.
func (c *Conn) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	return c.Adapter.Update(ctx, filter, update)
}

// Delete forwards to the underlying adapter.
func (c *Conn) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	return c.Adapter.Delete(ctx, filter)
}

// Capabilities forwards to the underlying adapter.
func (c *Conn) Capabilities() adapter.CapabilitySet {
	return c.Adapter.Capabilities()
}

// RegisterTrigger forwards to the underlying adapter.
func (c *Conn) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return c.Adapter.RegisterTrigger(ctx, t)
}

// RemoveTriggerOn forwards to the underlying adapter.
func (c *Conn) RemoveTriggerOn(ctx context.Context, collection, triggerID string) error {
	// Adapters expose RemoveTriggerOn directly (Postgres); others expose only
	// the generic RemoveTrigger, which may be unimplemented — surface honestly.
	type rma interface {
		RemoveTriggerOn(ctx context.Context, collection, triggerID string) error
	}
	if a, ok := c.Adapter.(rma); ok {
		return a.RemoveTriggerOn(ctx, collection, triggerID)
	}
	return c.Adapter.RemoveTrigger(ctx, triggerID)
}

// SubscribeToChanges forwards to the underlying adapter.
func (c *Conn) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return c.Adapter.SubscribeToChanges(ctx, collection, handler)
}

// RegisterRealtimeBroadcast forwards to the underlying adapter.
func (c *Conn) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return c.Adapter.RegisterRealtimeBroadcast(ctx, collection)
}

// Factory resolves and connects adapters, mirroring the server AdapterFactory
// contract while returning the full DatabaseAdapter for feature use.
type Factory struct{}

var _ server.AdapterFactory = (*Factory)(nil)

// NewFactory returns a Factory supporting the currently-implemented engines.
func NewFactory() *Factory {
	return &Factory{}
}

// ConnectForEngine connects an empty adapter of the given engine. Returns
// ErrUnsupported for engines not yet implemented.
func (f *Factory) ConnectForEngine(ctx context.Context, engine adapter.Engine, cfg adapter.ConnectionConfig) (*Conn, error) {
	var a adapter.DatabaseAdapter
	switch engine {
	case adapter.EnginePostgres:
		a = postgres.New()
	case adapter.EngineFerretDB:
		a = ferretdb.New()
	case adapter.EngineValkey:
		a = valkey.New()
	case adapter.EngineMySQL:
		a = mysql.New()
	case adapter.EngineQdrant:
		a = qdrant.New()
	default:
		return nil, fmt.Errorf("%w: engine %q not yet implemented", adapter.ErrUnsupported, engine)
	}
	if err := a.Connect(ctx, cfg); err != nil {
		return nil, err
	}
	return &Conn{Engine: engine, Adapter: a}, nil
}

// ConnectForProject connects an adapter using a project connection and its
// decrypted secret.
func (f *Factory) ConnectForProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (server.Adapter, error) {
	engine := adapter.Engine(conn.Engine)
	return f.ConnectForEngine(ctx, engine, adapter.ConnectionConfig{
		Engine:   engine,
		ConnStr:  secret.ConnString,
		Username: secret.Username,
		Password: secret.Password,
		Database: "",
	})
}

// TestConnection detects the engine and performs a live read-only connection.
// It surfaces the adapter's real error on failure (ADAPTERS.md §5).
func (f *Factory) TestConnection(ctx context.Context, connString string) (adapter.Engine, error) {
	engine, err := adapter.DetectEngine(connString)
	if err != nil {
		return "", err
	}
	cfg := adapter.ConnectionConfig{Engine: engine, ConnStr: connString}
	conn, err := f.ConnectForEngine(ctx, engine, cfg)
	if err != nil {
		if errors.Is(err, adapter.ErrUnsupported) {
			return "", fmt.Errorf("engine %q detected but not yet supported", engine)
		}
		return "", err
	}
	// Read-only introspection to prove the connection works.
	_, listErr := conn.Adapter.ListCollections(ctx)
	_ = conn.Disconnect(ctx)
	if listErr != nil {
		return "", fmt.Errorf("connected but introspection failed: %w", listErr)
	}
	return engine, nil
}
