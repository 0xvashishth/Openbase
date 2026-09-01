// Package engine wires adapters together: it resolves an adapter for a detected
// engine, connects it, and supports BYODB test connections. See ADAPTERS.md.
package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/adapter/ferretdb"
	"github.com/openbase/openbase/internal/adapter/postgres"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/server"
)

// Conn holds a connected adapter.
type Conn struct {
	Engine   adapter.Engine
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
