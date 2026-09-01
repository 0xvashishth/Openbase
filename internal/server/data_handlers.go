package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
)

// connectProject dials the project's adapter using its stored (decrypted)
// credentials. It returns the live adapter and its engine, or an error.
func (s *Server) connectProject(ctx context.Context, projectID string) (Adapter, adapter.Engine, error) {
	conn, err := s.svc.Store.GetConnectionByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, "", errNotFound
		}
		return nil, "", err
	}
	if s.svc.Secrets == nil {
		return nil, "", errors.New("server: secrets provider not configured")
	}
	secret, err := s.svc.Secrets.DecryptConnection(conn)
	if err != nil {
		return nil, "", err
	}
	adapterConn, err := s.svc.AdapterFactory.ConnectForProject(ctx, *conn, secret)
	if err != nil {
		return nil, "", err
	}
	return adapterConn, adapter.Engine(conn.Engine), nil
}

// ---- Save / connect an existing database (BYODB) ----

type saveConnectionRequest struct {
	ConnectionString string `json:"connection_string"`
	Engine           string `json:"engine,omitempty"` // optional explicit override
}

// saveConnection tests the string, detects the engine, encrypts credentials and
// stores the connection. Follows ADAPTERS.md §5 (test before save) and
// SCHEMA.md §2 (encrypt at rest). Idempotent: a second save overwrites.
func (s *Server) saveConnection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	if s.svc.Secrets == nil || s.svc.AdapterFactory == nil {
		writeError(w, http.StatusNotImplemented, "adapter/session engine not configured")
		return
	}

	var req saveConnectionRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.ConnectionString) == "" {
		writeError(w, http.StatusBadRequest, "connection_string is required")
		return
	}

	// Detect engine (allow an explicit override for ambiguous strings).
	engine, err := adapter.DetectEngine(req.ConnectionString)
	if err != nil {
		if req.Engine != "" {
			engine = adapter.Engine(req.Engine)
		} else {
			writeError(w, http.StatusBadRequest, "could not detect engine; specify engine explicitly")
			return
		}
	}

	// Test before saving (ADAPTERS.md §5) with the adapter's real error surfaced.
	if _, err := s.svc.AdapterFactory.TestConnection(r.Context(), req.ConnectionString); err != nil {
		writeJSON(w, http.StatusOK, testConnectionResponse{Success: false, Engine: string(engine), Message: err.Error()})
		return
	}

	secret := metadata.ConnectionSecret{
		ConnString: req.ConnectionString,
		Username:   "", // connection string carries embedded creds in v1
		Password:   "",
	}

	conn := &metadata.Connection{
		ProjectID: projectID,
		Mode:      metadata.ModeBYODB,
		Engine:    string(engine),
		Status:    metadata.StatusPending,
	}
	if err := s.svc.Secrets.EncryptConnection(conn, secret); err != nil {
		s.writeErr(w, err)
		return
	}

	// Upsert: replace any existing connection for this project.
	existing, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	switch {
	case err == nil:
		conn.ID = existing.ID
		conn.CreatedAt = existing.CreatedAt
		if err := s.svc.Store.UpdateConnection(r.Context(), conn); err != nil {
			s.writeErr(w, err)
			return
		}
	case errors.Is(err, metadata.ErrNotFound):
		if err := s.svc.Store.CreateConnection(r.Context(), conn); err != nil {
			s.writeErr(w, err)
			return
		}
	default:
		s.writeErr(w, err)
		return
	}

	// Mark connected on success.
	if err := s.svc.Store.UpdateConnectionStatus(r.Context(), conn.ID, metadata.StatusConnected); err != nil {
		s.writeErr(w, err)
		return
	}

	// Never echo ciphertext or raw secret back.
	conn.EncryptedConnString = nil
	conn.EncryptedUsername = nil
	conn.EncryptedPassword = nil
	writeJSON(w, http.StatusOK, testConnectionResponse{Success: true, Engine: string(engine)})
}

// ---- Schema / rows browsing ----

type collectionName struct {
	Name string `json:"name"`
}

func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	a, _, err := s.connectProject(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	cols, err := a.ListCollections(r.Context())
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	out := make([]collectionName, 0, len(cols))
	for _, c := range cols {
		out = append(out, collectionName{Name: c.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getSchema(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	collection := r.PathValue("collection")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	a, _, err := s.connectProject(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	schema, err := a.GetSchema(r.Context(), collection)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, schema)
}

// QueryRowsRequest carries a UniversalQuery filter for the rows endpoint.
type queryRowsRequest struct {
	Collection string        `json:"collection"`
	Conditions []condReq     `json:"conditions,omitempty"`
	OrderBy    []orderReq    `json:"order_by,omitempty"`
	Limit      *int          `json:"limit,omitempty"`
	Offset     *int          `json:"offset,omitempty"`
}

type condReq struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type orderReq struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

func (s *Server) queryRows(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	var req queryRowsRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Collection == "" {
		writeError(w, http.StatusBadRequest, "collection is required")
		return
	}

	a, _, err := s.connectProject(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	filter := adapter.Filter{Collection: req.Collection, Limit: req.Limit, Offset: req.Offset}
	for _, c := range req.Conditions {
		op := adapter.Op(c.Operator)
		if op == "" {
			op = adapter.OpEqual
		}
		filter.Conditions = append(filter.Conditions, adapter.Condition{Field: c.Field, Operator: op, Value: c.Value})
	}
	for _, o := range req.OrderBy {
		filter.OrderBy = append(filter.OrderBy, adapter.OrderBy{Field: o.Field, Desc: o.Desc})
	}

	rs, err := a.Query(r.Context(), adapter.UniversalQuery{Filter: filter})
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) writeConnectionErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "no connection configured for this project")
		return
	}
	s.svc.Log.Error("data request failed", "err", err)
	writeError(w, http.StatusBadGateway, "failed to reach database: "+err.Error())
}