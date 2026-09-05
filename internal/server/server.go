// Package server exposes the platform's REST HTTP API used by the dashboard
// and external clients (auto-generated CRUD endpoints come later; this serves
// platform auth/org/project management).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/pool"
	"github.com/openbase/openbase/internal/realtime"
)

// Services carries the dependencies the API handlers need.
type Services struct {
	Store  metadata.Store
	Tokens *auth.TokenManager
	Log    *slog.Logger

	// AdapterFactory resolves and connects an adapter for a project
	// connection. Injected so tests and provisioning modes can differ.
	AdapterFactory AdapterFactory

	// Secrets decrypts stored connection credentials (SCHEMA.md §2). If nil,
	// decryption-dependent endpoints (connect/schema/rows) return 501.
	Secrets SecretsProvider

	// AllowedOrigins for CORS. Empty means allow any origin (dev default).
	// The dashboard origin should be listed in production.
	AllowedOrigins []string

	// PublicBaseURL is the externally-reachable origin of this API (no trailing
	// slash), e.g. "https://api.example.com". Advertised by the Connect
	// endpoint so copy-paste snippets work outside the dashboard. Empty means
	// "derive from the request" (honouring X-Forwarded-Proto/Host).
	PublicBaseURL string

	// Provisioner creates and destroys dedicated per-project database
	// instances (ARCHITECTURE.md §2.7). If nil, "provisioned" project creation
	// returns 501.
	Provisioner Provisioner

	// TriggerService wires project triggers onto their DB adapter and
	// dispatches change events. If nil, trigger CRUD still works but events are
	// not delivered to actions.
	TriggerService TriggerRuntime

	// RealtimeHub is the Phase 5 WebSocket gateway. If nil, the realtime
	// endpoint returns 503.
	RealtimeHub *realtime.Hub
}

// TriggerRuntime re-registers a project's DB triggers and starts change
// subscriptions after trigger config changes or a (re)connect.
type TriggerRuntime interface {
	RegisterProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) error
	StopProject(projectID string)
}

// Provisioner creates and destroys dedicated per-project database instances.
type Provisioner interface {
	// Provision starts a dedicated instance for engine and returns its
	// connection details (with embedded generated credentials).
	Provision(ctx context.Context, engine Engine) (ProvisionedInstance, error)
	// Destroy removes a previously provisioned instance.
	Destroy(ctx context.Context, containerID string) error
}

// ProvisionedInstance is the result of provisioning a dedicated database.
type ProvisionedInstance struct {
	ContainerID string
	Engine      Engine
	ConnString  string
}

// Engine identifies a database engine.
type Engine string

// SecretsProvider decrypts a stored connection's credentials for runtime use.
type SecretsProvider interface {
	// DecryptConnection returns the plaintext credentials for a connection.
	DecryptConnection(conn *metadata.Connection) (metadata.ConnectionSecret, error)
	// EncryptConnection encrypts credential fields into the connection struct.
	EncryptConnection(c *metadata.Connection, secret metadata.ConnectionSecret) error
}

// AdapterFactory builds a live adapter for a project connection. It is used by
// the project-facing endpoints (e.g. schema browsing).
type AdapterFactory interface {
	// ConnectForProject returns a connected adapter given decrypted credentials.
	ConnectForProject(ctx context.Context, conn metadata.Connection, secret metadata.ConnectionSecret) (Adapter, error)
	// TestConnection verifies a connection string against the appropriate
	// adapter and returns the detected engine, or an error carrying the
	// adapter's real message.
	TestConnection(ctx context.Context, connString string) (adapter.Engine, error)
}

// Adapter is the subset of the adapters API the server needs directly. It is
// exported so the engine package can implement the AdapterFactory contract.
type Adapter interface {
	ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error)
	GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error)
	ListRelationships(ctx context.Context) ([]adapter.Relationship, error)
	Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error)
	Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error)
	Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error)
	Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error)
	RegisterRealtimeBroadcast(ctx context.Context, collection string) error
	SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error)
	Capabilities() adapter.CapabilitySet
	Disconnect(ctx context.Context) error
}

// Server owns the HTTP route table.
type Server struct {
	mux *http.ServeMux
	svc *Services

	// pkCache memoizes single-column primary-key resolution for id-based
	// PUT/DELETE (see resolveRowKey). Per-Server so tests never share entries.
	pkCache *rowKeyCache

	// adapters holds one live adapter per project connection instead of
	// dialing and tearing down a pool on every request (see adapter_pool.go).
	adapters *pool.Pool[adapterKey, Adapter]
}

// Handler is the http.Handler returned by New, extended with the shutdown hook
// the pooled adapters need. cmd/server defers Close so a restart does not
// leave database sessions open on the far side.
type Handler interface {
	http.Handler
	Close()
}

// handler couples the fully-wrapped middleware chain with the Server that owns
// the pooled resources.
type handler struct {
	http.Handler
	srv *Server
}

func (h handler) Close() { h.srv.Close() }

// New builds a Server with all routes registered.
func New(svc *Services) Handler {
	mux := http.NewServeMux()
	s := &Server{mux: mux, svc: svc, pkCache: newRowKeyCache()}
	s.adapters = s.newAdapterPool()

	mux.HandleFunc("POST /v1/auth/register", s.register)
	mux.HandleFunc("POST /v1/auth/login", s.login)

	mux.Handle("GET /v1/me", s.requireAuth(http.HandlerFunc(s.me)))

	mux.Handle("POST /v1/orgs", s.requireAuth(http.HandlerFunc(s.createOrg)))
	mux.Handle("GET /v1/orgs", s.requireAuth(http.HandlerFunc(s.listOrgs)))
	mux.Handle("GET /v1/orgs/{orgID}", s.requireAuth(http.HandlerFunc(s.getOrg)))
	mux.Handle("POST /v1/orgs/{orgID}/projects", s.requireAuth(http.HandlerFunc(s.createProject)))
	mux.Handle("GET /v1/orgs/{orgID}/projects", s.requireAuth(http.HandlerFunc(s.listProjects)))

	// Single-project resolver: the dashboard previously listed every project in
	// an org just to render one.
	mux.Handle("GET /v1/projects/{projectID}", s.requireAuth(http.HandlerFunc(s.getProject)))

	mux.Handle("GET /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.getConnection)))
	mux.Handle("POST /v1/projects/{projectID}/connections/test", s.requireAuth(http.HandlerFunc(s.testConnection)))
	mux.Handle("POST /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.saveConnection)))
	mux.Handle("DELETE /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.deleteConnection)))

	// Outbound wiring info for external apps (powers the dashboard Connect tab).
	mux.Handle("GET /v1/projects/{projectID}/connect-info", s.requireAuth(http.HandlerFunc(s.getConnectInfo)))

	mux.Handle("GET /v1/projects/{projectID}/collections", s.requireAuth(http.HandlerFunc(s.listCollections)))
	mux.Handle("GET /v1/projects/{projectID}/collections/{collection}", s.requireAuth(http.HandlerFunc(s.getSchema)))
	mux.Handle("POST /v1/projects/{projectID}/query", s.requireAuth(http.HandlerFunc(s.queryRows)))
	mux.Handle("POST /v1/projects/{projectID}/sql", s.requireAuth(http.HandlerFunc(s.execSQL)))
	mux.Handle("GET /v1/projects/{projectID}/schema", s.requireAuth(http.HandlerFunc(s.getFullSchema)))

	// API key management (dashboard-authenticated).
	mux.Handle("GET /v1/projects/{projectID}/api-keys", s.requireAuth(http.HandlerFunc(s.listAPIKeys)))
	mux.Handle("POST /v1/projects/{projectID}/api-keys", s.requireAuth(http.HandlerFunc(s.createAPIKey)))
	mux.Handle("DELETE /v1/projects/{projectID}/api-keys/{keyID}", s.requireAuth(http.HandlerFunc(s.revokeAPIKey)))

	// Triggers + runtime functions.
	mux.Handle("GET /v1/projects/{projectID}/triggers", s.requireAuth(http.HandlerFunc(s.listTriggers)))
	mux.Handle("POST /v1/projects/{projectID}/triggers", s.requireAuth(http.HandlerFunc(s.createTrigger)))
	mux.Handle("PUT /v1/projects/{projectID}/triggers/{triggerID}", s.requireAuth(http.HandlerFunc(s.updateTrigger)))
	mux.Handle("DELETE /v1/projects/{projectID}/triggers/{triggerID}", s.requireAuth(http.HandlerFunc(s.deleteTrigger)))

	mux.Handle("GET /v1/projects/{projectID}/functions", s.requireAuth(http.HandlerFunc(s.listFunctions)))
	mux.Handle("POST /v1/projects/{projectID}/functions", s.requireAuth(http.HandlerFunc(s.createFunction)))
	mux.Handle("GET /v1/projects/{projectID}/functions/{fnID}", s.requireAuth(http.HandlerFunc(s.getFunction)))
	mux.Handle("DELETE /v1/projects/{projectID}/functions/{fnID}", s.requireAuth(http.HandlerFunc(s.deleteFunction)))

	// Auto-generated REST API (API-key-authenticated, project scoped via key).
	mux.Handle("GET /v1/api/tables", s.requireAPIKey(http.HandlerFunc(s.apiListTables)))
	mux.Handle("GET /v1/api/{collection}", s.requireAPIKey(http.HandlerFunc(s.apiQueryRows)))
	mux.Handle("GET /v1/api/{collection}/_schema", s.requireAPIKey(http.HandlerFunc(s.apiGetTableSchema)))
	mux.Handle("POST /v1/api/{collection}", s.requireAPIKey(http.HandlerFunc(s.apiInsertRow)))
	mux.Handle("PUT /v1/api/{collection}/{id}", s.requireAPIKey(http.HandlerFunc(s.apiUpdateRow)))
	mux.Handle("DELETE /v1/api/{collection}/{id}", s.requireAPIKey(http.HandlerFunc(s.apiDeleteRow)))

	// Realtime WebSocket gateway (API-key-authenticated, project scoped via key).
	mux.Handle("GET /v1/realtime", s.requireAPIKey(http.HandlerFunc(s.realtimeWS)))

	cors := &CORS{AllowedOrigins: svc.AllowedOrigins}
	return handler{Handler: s.withRecovery(s.withLogging(cors.Middleware(mux))), srv: s}
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.svc.Log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start).String(),
		)
	})
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.svc.Log.Error("panic", "value", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type ctxKey string

const (
	ctxUserID    ctxKey = "userID"
	ctxProjectID ctxKey = "projectID"
)

// ---- helpers ----

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func decodeBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty body: leave dst as its zero value so callers can apply
			// defaults (e.g. saveConnection auto-provisions Postgres).
			return nil
		}
		return errors.New("invalid JSON body")
	}
	return nil
}
