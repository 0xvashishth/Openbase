package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
)

// connectInfoResponse is everything an external app needs to talk TO Openbase
// (the outbound direction powering the dashboard's Connect tab). It is the
// mirror image of the connections endpoints, which configure the project's
// backing database (inbound).
//
// It deliberately never carries database credentials: the connection string is
// decrypted only for server-side adapter use (SCHEMA.md §2). Clients
// authenticate with an API key against /v1/api/*, never with the DB directly.
type connectInfoResponse struct {
	ProjectID string `json:"project_id"`

	// HasConnection reports whether a backing database is attached AND
	// reachable-as-of-last-check. The REST endpoints below only answer once
	// this is true.
	HasConnection bool   `json:"has_connection"`
	Engine        string `json:"engine,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Status        string `json:"status,omitempty"`

	// APIBaseURL is the public origin clients should call, e.g.
	// "https://api.example.com". RealtimeURL is its ws(s) counterpart.
	APIBaseURL  string `json:"api_base_url"`
	RealtimeURL string `json:"realtime_url"`

	// Endpoints are literal paths (with {collection}/{id} placeholders) so the
	// dashboard renders snippets from server truth instead of hardcoding them.
	Endpoints connectEndpoints `json:"endpoints"`

	// AuthHeader shows the exact header shape expected by requireAPIKey.
	AuthHeader string `json:"auth_header"`

	// Capabilities is the connected engine's honest feature set, or nil when no
	// database is attached (or it could not be reached right now).
	Capabilities *adapter.CapabilitySet `json:"capabilities,omitempty"`

	// SupportsRealtime mirrors Capabilities.SupportsRealtime for convenience
	// ("native" | "polling" | "none"); empty when unknown.
	SupportsRealtime string `json:"supports_realtime,omitempty"`

	// ActiveAPIKeys counts non-revoked keys so the UI can nudge the user to
	// create one. Key material is never returned (only hashes are stored).
	ActiveAPIKeys int `json:"active_api_keys"`
}

type connectEndpoints struct {
	ListTables   string `json:"list_tables"`
	QueryRows    string `json:"query_rows"`
	TableSchema  string `json:"table_schema"`
	InsertRow    string `json:"insert_row"`
	UpdateRow    string `json:"update_row"`
	DeleteRow    string `json:"delete_row"`
	RealtimePath string `json:"realtime"`
}

// connectEndpointPaths mirrors the routes registered in New(). Kept in one
// place so the Connect tab and the router can never drift.
func connectEndpointPaths() connectEndpoints {
	return connectEndpoints{
		ListTables:   "GET /v1/api/tables",
		QueryRows:    "GET /v1/api/{collection}?limit=&offset=&order_by=&order=",
		TableSchema:  "GET /v1/api/{collection}/_schema",
		InsertRow:    "POST /v1/api/{collection}",
		UpdateRow:    "PUT /v1/api/{collection}/{id}",
		DeleteRow:    "DELETE /v1/api/{collection}/{id}",
		RealtimePath: "GET /v1/realtime",
	}
}

// publicBaseURL resolves the origin external clients should use.
//
// Precedence: explicit configuration (OPENBASE_PUBLIC_URL) wins, because only
// the operator knows the public hostname behind a proxy. Otherwise it is
// reconstructed from the request, honouring X-Forwarded-Proto/Host so a
// reverse-proxied deployment still advertises https and the outside hostname.
func publicBaseURL(r *http.Request, configured string) string {
	if c := strings.TrimRight(strings.TrimSpace(configured), "/"); c != "" {
		return c
	}

	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}

	scheme := firstForwardedValue(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + host
}

// firstForwardedValue takes the left-most entry of a comma-separated
// X-Forwarded-* header (the original client-facing value).
func firstForwardedValue(v string) string {
	if v == "" {
		return ""
	}
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// websocketURL converts an http(s) base URL into the realtime ws(s) endpoint.
func websocketURL(baseURL string) string {
	switch {
	case strings.HasPrefix(baseURL, "https://"):
		return "wss://" + strings.TrimPrefix(baseURL, "https://") + "/v1/realtime"
	case strings.HasPrefix(baseURL, "http://"):
		return "ws://" + strings.TrimPrefix(baseURL, "http://") + "/v1/realtime"
	default:
		return baseURL + "/v1/realtime"
	}
}

// getConnectInfo serves the Connect tab. Auth is the dashboard JWT (org
// membership), same as the other project endpoints.
func (s *Server) getConnectInfo(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}

	base := publicBaseURL(r, s.svc.PublicBaseURL)
	out := connectInfoResponse{
		ProjectID:   projectID,
		APIBaseURL:  base,
		RealtimeURL: websocketURL(base),
		Endpoints:   connectEndpointPaths(),
		AuthHeader:  "Authorization: Bearer ob_...",
	}

	if keys, err := s.svc.Store.ListAPIKeys(r.Context(), projectID); err == nil {
		for _, k := range keys {
			if k.RevokedAt == nil {
				out.ActiveAPIKeys++
			}
		}
	}

	conn, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			// No database attached yet: URLs are still useful, endpoints just
			// have nothing to answer with.
			writeJSON(w, http.StatusOK, out)
			return
		}
		s.writeErr(w, err)
		return
	}

	out.Engine = conn.Engine
	out.Mode = string(conn.Mode)
	out.Status = string(conn.Status)
	out.HasConnection = conn.Status == metadata.StatusConnected

	// Capabilities need a live adapter. A dial failure must not fail the whole
	// response — the UI still needs the URLs — so report it as "unknown caps".
	if out.HasConnection && s.svc.Secrets != nil && s.svc.AdapterFactory != nil {
		if a, _, err := s.connectProject(r.Context(), projectID); err == nil {
			defer func() { _ = a.Disconnect(r.Context()) }()
			caps := a.Capabilities()
			out.Capabilities = &caps
			out.SupportsRealtime = string(caps.SupportsRealtime)
		}
	}

	writeJSON(w, http.StatusOK, out)
}
