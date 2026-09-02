package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/realtime"
)

// realtimeConnector implements realtime.Connector on top of the server's
// AdapterFactory + Secrets so change subscriptions reach real adapters. It
// holds dependencies directly (not the Server) so it can be built before the
// route table is wired.
type realtimeConnector struct {
	store   metadata.Store
	secrets SecretsProvider
	factory AdapterFactory
}

var _ realtime.Connector = (*realtimeConnector)(nil)

// ConnectForProject resolves the project's connection, connects its adapter,
// installs a native broadcast trigger (so every row change notifies), and
// returns the change subscription for the collection.
func (c *realtimeConnector) ConnectForProject(ctx context.Context, projectID, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	conn, err := c.store.GetConnectionByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, errNotFound
		}
		return nil, err
	}
	if c.secrets == nil {
		return nil, errors.New("secrets provider not configured")
	}
	secret, err := c.secrets.DecryptConnection(conn)
	if err != nil {
		return nil, err
	}
	a, err := c.factory.ConnectForProject(ctx, *conn, secret)
	if err != nil {
		return nil, err
	}
	// Install an all-operations native broadcast trigger if the engine supports
	// it (Postgres LISTEN/NOTIFY). Adapters without native triggers surface the
	// error honestly instead of pretending.
	if err := a.RegisterRealtimeBroadcast(ctx, collection); err != nil {
		_ = a.Disconnect(ctx)
		return nil, err
	}
	sub, err := a.SubscribeToChanges(ctx, collection, handler)
	if err != nil {
		_ = a.Disconnect(ctx)
		return nil, err
	}
	return &subWithAdapter{sub: sub, a: a}, nil
}

// subWithAdapter closes the subscription and the underlying adapter together.
type subWithAdapter struct {
	sub adapter.Subscription
	a   Adapter
}

func (s *subWithAdapter) Close() error {
	if s.sub != nil {
		_ = s.sub.Close()
	}
	if s.a != nil {
		return s.a.Disconnect(context.Background())
	}
	return nil
}

// NewRealtimeHub builds a realtime hub wired to real adapters through the
// given store/secrets/factory. Pass svc to use the services' dependencies.
func NewRealtimeHub(store metadata.Store, secrets SecretsProvider, factory AdapterFactory, log *slog.Logger) *realtime.Hub {
	connector := &realtimeConnector{store: store, secrets: secrets, factory: factory}
	return realtime.NewHub(nil, connector, log)
}

// realtimeWS is the WebSocket upgrade endpoint. It is wrapped by requireAPIKey
// (which injects the project ID) so the client's topic scope is its own key.
func (s *Server) realtimeWS(w http.ResponseWriter, r *http.Request) {
	projectID, ok := r.Context().Value(ctxProjectID).(string)
	if !ok {
		http.Error(w, "project not resolved", http.StatusUnauthorized)
		return
	}
	hub := s.svc.RealtimeHub
	if hub == nil {
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	hub.ServeWS(w, r, projectID)
}
