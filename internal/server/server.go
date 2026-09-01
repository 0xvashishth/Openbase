// Package server exposes the platform's REST HTTP API used by the dashboard
// and external clients (auto-generated CRUD endpoints come later; this serves
// platform auth/org/project management).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/metadata"
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

	// Provisioner creates and destroys dedicated per-project database
	// instances (ARCHITECTURE.md §2.7). If nil, "provisioned" project creation
	// returns 501.
	Provisioner Provisioner
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
	Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error)
	Disconnect(ctx context.Context) error
}

// Server owns the HTTP route table.
type Server struct {
	mux  *http.ServeMux
	svc  *Services
}

// New builds a Server with all routes registered.
func New(svc *Services) http.Handler {
	mux := http.NewServeMux()
	s := &Server{mux: mux, svc: svc}

	mux.HandleFunc("POST /v1/auth/register", s.register)
	mux.HandleFunc("POST /v1/auth/login", s.login)

	mux.Handle("GET /v1/me", s.requireAuth(http.HandlerFunc(s.me)))

	mux.Handle("POST /v1/orgs", s.requireAuth(http.HandlerFunc(s.createOrg)))
	mux.Handle("GET /v1/orgs", s.requireAuth(http.HandlerFunc(s.listOrgs)))
	mux.Handle("GET /v1/orgs/{orgID}", s.requireAuth(http.HandlerFunc(s.getOrg)))
	mux.Handle("POST /v1/orgs/{orgID}/projects", s.requireAuth(http.HandlerFunc(s.createProject)))
	mux.Handle("GET /v1/orgs/{orgID}/projects", s.requireAuth(http.HandlerFunc(s.listProjects)))

	mux.Handle("GET /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.getConnection)))
	mux.Handle("POST /v1/projects/{projectID}/connections/test", s.requireAuth(http.HandlerFunc(s.testConnection)))
	mux.Handle("POST /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.saveConnection)))
	mux.Handle("DELETE /v1/projects/{projectID}/connections", s.requireAuth(http.HandlerFunc(s.deleteConnection)))

	mux.Handle("GET /v1/projects/{projectID}/collections", s.requireAuth(http.HandlerFunc(s.listCollections)))
	mux.Handle("GET /v1/projects/{projectID}/collections/{collection}", s.requireAuth(http.HandlerFunc(s.getSchema)))
	mux.Handle("POST /v1/projects/{projectID}/query", s.requireAuth(http.HandlerFunc(s.queryRows)))

	cors := &CORS{AllowedOrigins: svc.AllowedOrigins}
	return s.withRecovery(s.withLogging(cors.Middleware(mux)))
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

const ctxUserID ctxKey = "userID"

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
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	return nil
}
