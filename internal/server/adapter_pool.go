package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/pool"
)

// adapterIdleTTL bounds how long an unused project adapter stays dialed. Long
// enough that a burst of dashboard or API traffic shares one connection, short
// enough that an idle project does not hold a database session open forever.
const adapterIdleTTL = 5 * time.Minute

// Sharing one adapter across concurrent requests is only sound because every
// adapter's underlying client is itself concurrency-safe: pgxpool.Pool,
// *sql.DB, mongo.Client, redis.Client and http.Client are all documented as
// safe for use by multiple goroutines. An adapter added later that holds a
// single non-shareable session (e.g. a raw net.Conn) must not be pooled here.
//
// Long-lived per-subscription connections stay OUT of the pool on purpose:
// realtime (realtime.go) and the trigger runtime each dial their own adapter,
// because a Postgres LISTEN needs a dedicated session and its lifetime is the
// subscription's, not a request's.

// adapterKey identifies a pooled adapter. The connection id alone is not
// enough: saveConnection reuses the row id when it overwrites credentials, so a
// key that ignored the credential generation would keep serving an adapter
// dialed against the *previous* database. gen is derived from the fields that
// change when credentials change.
type adapterKey struct {
	projectID    string
	connectionID string
	gen          string
}

// generation returns a value that changes whenever a connection's credentials
// or target change. encryption_key_id plus the ciphertext length is not a
// cryptographic fingerprint and is not meant to be — it only needs to differ
// after a rewrite, and callers additionally invalidate by project on every
// save/delete (see invalidateProjectAdapters), so this is defence in depth.
func generation(c *metadata.Connection) string {
	var b []byte
	b = append(b, c.Engine...)
	b = append(b, '|')
	b = append(b, c.EncryptionKeyID...)
	b = append(b, '|')
	if n := len(c.EncryptedConnString); n > 0 {
		b = append(b, byte(n), byte(n>>8))
	}
	if c.ContainerID != nil {
		b = append(b, '|')
		b = append(b, *c.ContainerID...)
	}
	return string(b)
}

// pooledAdapter wraps a live adapter so handlers can use it exactly like an
// owned one. Disconnect is a no-op: the pool owns the lifecycle, and a handler
// running `defer a.Disconnect(ctx)` must not tear down a connection other
// in-flight requests are sharing.
//
// Any OPTIONAL capability interface a handler type-asserts for must be
// forwarded explicitly below. A type assertion cannot see through an embedded
// interface, so an unforwarded capability silently disappears for every engine.
type pooledAdapter struct {
	Adapter
}

// The wrapper must never narrow the capability surface. Assert what handlers
// rely on so adding a capability without forwarding it fails to compile.
var (
	_ Adapter            = pooledAdapter{}
	_ adapter.RawQuerier = pooledAdapter{}
)

// Disconnect is deliberately a no-op. Real disposal happens on pool eviction,
// invalidation or shutdown.
func (pooledAdapter) Disconnect(context.Context) error { return nil }

// ExecRaw forwards raw execution to the pooled adapter. Engines that do not
// implement RawQuerier report ErrUnsupported, which execSQL turns into an
// honest 400 — the same outcome as the pre-pool type assertion failing.
func (p pooledAdapter) ExecRaw(ctx context.Context, query string) (adapter.ResultSet, error) {
	raw, ok := p.Adapter.(adapter.RawQuerier)
	if !ok {
		return adapter.ResultSet{}, fmt.Errorf("%w: engine does not support raw queries", adapter.ErrUnsupported)
	}
	return raw.ExecRaw(ctx, query)
}

// unwrapPooled returns the underlying adapter so the pool can really close it.
func unwrapPooled(a Adapter) Adapter {
	if p, ok := a.(pooledAdapter); ok {
		return p.Adapter
	}
	return a
}

// newAdapterPool builds the server's keyed adapter cache. dial resolves the
// connection, decrypts its secret and hands off to the AdapterFactory — the
// same sequence the old per-request path ran, now paid once per project.
func (s *Server) newAdapterPool() *pool.Pool[adapterKey, Adapter] {
	return pool.New(adapterIdleTTL,
		func(ctx context.Context, key adapterKey) (Adapter, error) {
			conn, err := s.svc.Store.GetConnectionByProject(ctx, key.projectID)
			if err != nil {
				if errors.Is(err, metadata.ErrNotFound) {
					return nil, errNotFound
				}
				return nil, err
			}
			if s.svc.Secrets == nil {
				return nil, errors.New("server: secrets provider not configured")
			}
			if s.svc.AdapterFactory == nil {
				return nil, errors.New("server: adapter factory not configured")
			}
			secret, err := s.svc.Secrets.DecryptConnection(conn)
			if err != nil {
				return nil, err
			}
			return s.svc.AdapterFactory.ConnectForProject(ctx, *conn, secret)
		},
		func(ctx context.Context, a Adapter) error {
			return unwrapPooled(a).Disconnect(ctx)
		},
	)
}

// acquireAdapter returns a pooled, live adapter for a project along with its
// connection row. The returned adapter's Disconnect is a no-op, so existing
// `defer a.Disconnect(ctx)` call sites stay correct without releasing a shared
// connection.
//
// One GetConnectionByProject runs here to build the key; on a hit that is the
// only database round-trip, replacing the previous TCP + TLS + auth + Ping
// handshake and teardown per request.
func (s *Server) acquireAdapter(ctx context.Context, projectID string) (Adapter, *metadata.Connection, error) {
	conn, err := s.svc.Store.GetConnectionByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil, errNotFound
		}
		return nil, nil, err
	}
	key := adapterKey{projectID: projectID, connectionID: conn.ID, gen: generation(conn)}
	a, err := s.adapters.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	return pooledAdapter{Adapter: a}, conn, nil
}

// invalidateProjectAdapters drops every pooled adapter for a project and any
// memoized primary keys. Called after a connection is saved, replaced or
// deleted so the next request re-dials against the new target instead of
// serving a stale session.
func (s *Server) invalidateProjectAdapters(projectID string) {
	s.adapters.InvalidateWhere(func(k adapterKey) bool { return k.projectID == projectID })
	s.pkCache.invalidateProject(projectID)
}

// Close disposes every pooled adapter. Called on server shutdown.
func (s *Server) Close() {
	s.adapters.Close()
}
