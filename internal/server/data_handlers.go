package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

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
	Mode             string `json:"mode,omitempty"`   // "byodb" (default) | "provisioned"
}

// saveConnection attaches a database to a project. Two modes (SCHEMA.md §3):
//   - "byodb" (default when a connection_string is supplied): user-supplied
//     connection string is detected, tested, encrypted and stored (ADAPTERS.md
//     §5, SCHEMA.md §2).
//   - "provisioned": the platform provisions a dedicated instance, generating
//     credentials that are encrypted and stored identically.
//
// Auto-provision fallback: when both mode and connection_string are omitted
// (or empty — e.g. `{}` or an empty body), the handler defaults to
// provisioning a Postgres instance instead of returning 400. This covers the
// "no connection string" UX: a fresh project gets a database without the
// caller having to know the provisioned-mode flag. An explicit
// `mode: "byodb"` with an empty string is still a 400 (user error with a hint
// pointing at the fallback).
//
// Idempotent: a second save overwrites the previous connection (and, for a
// provisioned replacement, destroys the old instance).
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

	rawMode := strings.TrimSpace(req.Mode)
	connStr := strings.TrimSpace(req.ConnectionString)
	mode := metadata.ConnectionMode(rawMode)
	if mode == "" {
		if connStr == "" {
			// No connection details at all: auto-provision Postgres.
			mode = metadata.ModeProvisioned
		} else {
			mode = metadata.ModeBYODB
		}
	}
	if mode != metadata.ModeBYODB && mode != metadata.ModeProvisioned {
		writeError(w, http.StatusBadRequest, "mode must be \"byodb\" or \"provisioned\"")
		return
	}

	var (
		engine adapter.Engine
		secret metadata.ConnectionSecret
		inst   ProvisionedInstance
	)

	if mode == metadata.ModeBYODB {
		if connStr == "" {
			writeError(w, http.StatusBadRequest, "connection_string is required (omit connection_string and mode to auto-provision a Postgres instance)")
			return
		}
		// Detect engine (allow an explicit override for ambiguous strings).
		var err error
		engine, err = adapter.DetectEngine(connStr)
		if err != nil {
			if strings.TrimSpace(req.Engine) != "" {
				engine = adapter.Engine(strings.TrimSpace(req.Engine))
			} else {
				writeError(w, http.StatusBadRequest, "could not detect engine; specify engine explicitly")
				return
			}
		}
		// Test before saving (ADAPTERS.md §5) with the adapter's real error surfaced.
		if _, err := s.svc.AdapterFactory.TestConnection(r.Context(), connStr); err != nil {
			writeJSON(w, http.StatusOK, testConnectionResponse{Success: false, Engine: string(engine), Message: err.Error()})
			return
		}
		secret = metadata.ConnectionSecret{ConnString: connStr}
	} else {
		if s.svc.Provisioner == nil {
			writeError(w, http.StatusNotImplemented, "provisioning is not configured; provide a connection_string or enable it with OPENBASE_PROVISIONER_ENABLED=true")
			return
		}
		// Default engine for auto-provisioning is Postgres; an explicit
		// engine (e.g. ferretdb) is respected when supplied.
		engine = adapter.EnginePostgres
		if strings.TrimSpace(req.Engine) != "" {
			engine = adapter.Engine(strings.TrimSpace(req.Engine))
		}
		var err error
		inst, err = s.svc.Provisioner.Provision(r.Context(), Engine(engine))
		if err != nil {
			s.svc.Log.Error("provisioning failed", "err", err)
			writeError(w, http.StatusBadGateway, "failed to provision database: "+err.Error())
			return
		}
		secret = metadata.ConnectionSecret{ConnString: inst.ConnString}
	}

	conn := &metadata.Connection{
		ProjectID: projectID,
		Mode:      mode,
		Engine:    string(engine),
		Status:    metadata.StatusPending,
	}
	if inst.ContainerID != "" {
		conn.ContainerID = &inst.ContainerID
	}
	if err := s.svc.Secrets.EncryptConnection(conn, secret); err != nil {
		s.writeErr(w, err)
		return
	}

	// If we are overwriting an existing provisioned connection, destroy the old
	// instance so we do not leak containers.
	existing, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	switch {
	case err == nil:
		if existing.Mode == metadata.ModeProvisioned && existing.ContainerID != nil &&
			(*existing.ContainerID != inst.ContainerID) && s.svc.Provisioner != nil {
			_ = s.svc.Provisioner.Destroy(r.Context(), *existing.ContainerID)
		}
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

	// Re-register any triggers on the newly connected database.
	if s.svc.TriggerService != nil {
		_ = s.svc.TriggerService.RegisterProject(r.Context(), *conn, secret)
	}

	// Never echo ciphertext or raw secret back.
	conn.EncryptedConnString = nil
	conn.EncryptedUsername = nil
	conn.EncryptedPassword = nil
	writeJSON(w, http.StatusOK, testConnectionResponse{Success: true, Engine: string(engine), Mode: string(mode)})
}

// ---- Disconnect / destroy ----

// deleteConnection detaches the database from a project. For a provisioned
// instance it also destroys the container; for BYODB it just clears the stored
// (encrypted) config. Idempotent when nothing is connected.
func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}

	existing, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	if errors.Is(err, metadata.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]bool{"removed": false})
		return
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}

	// Destroy any provisioned instance backing this connection.
	if existing.Mode == metadata.ModeProvisioned && existing.ContainerID != nil && s.svc.Provisioner != nil {
		if err := s.svc.Provisioner.Destroy(r.Context(), *existing.ContainerID); err != nil {
			s.svc.Log.Error("failed to destroy provisioned instance", "container_id", *existing.ContainerID, "err", err)
			writeError(w, http.StatusInternalServerError, "failed to destroy provisioned instance")
			return
		}
	}

	if err := s.svc.Store.DeleteConnection(r.Context(), existing.ID); err != nil {
		s.writeErr(w, err)
		return
	}
	if s.svc.TriggerService != nil {
		s.svc.TriggerService.StopProject(projectID)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
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
	// Go nil slices serialize as JSON null; the dashboard calls .length/.map
	// unconditionally, so coerce to [] here (defense in depth with FE normalize).
	if rs.Columns == nil {
		rs.Columns = []string{}
	}
	if rs.Rows == nil {
		rs.Rows = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, rs)
}

// ---- Raw SQL (SQL editor: full read + write) ----

// maxRawSQLBytes caps the request body so a pasted dump can't OOM the host.
const maxRawSQLBytes = 20000

type execSQLRequest struct {
	Query string `json:"query"`
}

// validateRawSQL rejects empty or oversized statements. Unlike the old
// read-only guard, it allows the full statement surface the engine natively
// speaks: SELECT/WITH/EXPLAIN reads plus DML (INSERT/UPDATE/DELETE) and DDL
// (CREATE/ALTER/DROP/TRUNCATE, ...) for SQL engines, and the native command
// language for document/key-value/vector engines. As a safety rail it still
// rejects stacked statements (";" followed by another statement outside
// strings/comments) so one Run executes one statement.
func validateRawSQL(q string) error {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return fmt.Errorf("query is required")
	}
	if len(q) > maxRawSQLBytes {
		return fmt.Errorf("query exceeds %d bytes", maxRawSQLBytes)
	}
	// Unterminated block comment is still a syntax error.
	rest := trimmed
	for {
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(rest, "--") {
			if i := strings.Index(rest, "\n"); i >= 0 {
				rest = rest[i+1:]
				continue
			}
			return fmt.Errorf("query is required")
		}
		if strings.HasPrefix(rest, "/*") {
			if i := strings.Index(rest, "*/"); i >= 0 {
				rest = rest[i+2:]
				continue
			}
			return fmt.Errorf("unterminated comment")
		}
		break
	}
	if strings.TrimSpace(rest) == "" {
		return fmt.Errorf("query is required")
	}
	if hasStackedStatements(q) {
		return fmt.Errorf("run one statement at a time; multiple statements separated by ';' are not allowed")
	}
	return nil
}

// hasStackedStatements reports whether q contains a ";" terminator followed
// by another statement, ignoring semicolons inside single/double/backtick
// quoted strings and inside -- line / /* block */ comments.
func hasStackedStatements(q string) bool {
	var (
		inSingle, inDouble, inBacktick bool
		inLineComment, inBlockComment  bool
		seenSemi                       bool
	)
	for i := 0; i < len(q); i++ {
		c := q[i]
		var next byte
		if i+1 < len(q) {
			next = q[i+1]
		}
		if inLineComment {
			if c == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if c == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inSingle {
			if c == '\'' {
				if next == '\'' {
					i++ // escaped ''
				} else {
					inSingle = false
				}
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(q) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
			continue
		}
		if inBacktick {
			if c == '`' {
				inBacktick = false
			}
			continue
		}
		if c == '-' && next == '-' {
			inLineComment = true
			i++
			continue
		}
		if c == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if c == '\'' {
			inSingle = true
			continue
		}
		if c == '"' {
			inDouble = true
			continue
		}
		if c == '`' {
			inBacktick = true
			continue
		}
		if c == ';' {
			seenSemi = true
			continue
		}
		if seenSemi {
			// Content after a terminator: skip whitespace; comments are
			// handled above, anything else means a stacked statement.
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				continue
			}
			return true
		}
	}
	return false
}

// execSQL runs a raw query against the project's engine (postgres, mysql,
// arcadedb, ferretdb, valkey, qdrant — every adapter implementing
// adapter.RawQuerier). Reads return columns+rows (capped at 200); writes
// return affected_rows / insert_id / OK rows. Engines without RawQuerier get
// an honest 400 instead of a fake execution.
func (s *Server) execSQL(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	var req execSQLRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateRawSQL(req.Query); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a, _, err := s.connectProject(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	raw, ok := a.(adapter.RawQuerier)
	if !ok {
		writeError(w, http.StatusBadRequest, "raw queries are not supported for this engine")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rs, err := raw.ExecRaw(ctx, req.Query)
	if err != nil {
		if errors.Is(err, adapter.ErrUnsupported) {
			writeError(w, http.StatusBadRequest, "raw queries are not supported for this engine: "+err.Error())
			return
		}
		s.writeConnectionErr(w, err)
		return
	}
	if rs.Columns == nil {
		rs.Columns = []string{}
	}
	if rs.Rows == nil {
		rs.Rows = []map[string]any{}
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