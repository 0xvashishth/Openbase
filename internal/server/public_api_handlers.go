package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
)

// rowKeyCacheTTL bounds how long a resolved primary key is reused. Schema
// introspection (notably FerretDB's document sampling) is too expensive to pay
// on every id-addressed write; a 60s window means DDL can leave a stale entry
// briefly, in which case the write fails loudly at the database — never with
// a wrong predicate.
const rowKeyCacheTTL = 60 * time.Second

type rowKeyEntry struct {
	field    string
	dataType string
	expires  time.Time
}

// rowKeyCache memoizes single-column primary-key resolution per
// (project, collection). It lives on Server (not package-global) so tests and
// tenants never share entries.
type rowKeyCache struct {
	mu      sync.Mutex
	entries map[string]rowKeyEntry
}

func newRowKeyCache() *rowKeyCache {
	return &rowKeyCache{entries: make(map[string]rowKeyEntry)}
}

func (c *rowKeyCache) get(projectID, collection string) (field, dataType string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, found := c.entries[projectID+"\x00"+collection]
	if !found || time.Now().After(e.expires) {
		if found {
			delete(c.entries, projectID+"\x00"+collection)
		}
		return "", "", false
	}
	return e.field, e.dataType, true
}

func (c *rowKeyCache) set(projectID, collection, field, dataType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[projectID+"\x00"+collection] = rowKeyEntry{
		field:    field,
		dataType: dataType,
		expires:  time.Now().Add(rowKeyCacheTTL),
	}
}

// resolveRowKey returns the single-column primary-key field used to address
// one row in id-based PUT/DELETE, plus its declared data type for id
// coercion. Resolution order: exactly one IsPrimary column → a single-column
// primary index → the `_id` convention (document engines) → explicit error.
// Composite primary keys are rejected: addressing one row by a single id
// would be ambiguous, so the API says so instead of guessing.
func (s *Server) resolveRowKey(ctx context.Context, a Adapter, projectID, collection string) (field, dataType string, err error) {
	if field, dataType, ok := s.pkCache.get(projectID, collection); ok {
		return field, dataType, nil
	}
	schema, err := a.GetSchema(ctx, collection)
	if err != nil {
		return "", "", err
	}
	var pkCols []adapter.ColumnInfo
	for _, c := range schema.Columns {
		if c.IsPrimary {
			pkCols = append(pkCols, c)
		}
	}
	switch {
	case len(pkCols) == 1:
		field, dataType = pkCols[0].Name, pkCols[0].DataType
	case len(pkCols) > 1:
		return "", "", fmt.Errorf("collection %q has a composite primary key: PUT/DELETE by a single id is not supported", collection)
	default:
		for _, idx := range schema.Indexes {
			if idx.Kind == adapter.IndexPrimary && len(idx.Columns) == 1 {
				field = idx.Columns[0]
			}
		}
		if field == "" {
			for _, c := range schema.Columns {
				if c.Name == "_id" {
					field = "_id"
				}
			}
		}
		if field == "" {
			return "", "", fmt.Errorf("collection %q has no single-column primary key: PUT/DELETE by id is not supported", collection)
		}
	}
	s.pkCache.set(projectID, collection, field, dataType)
	return field, dataType, nil
}

// coerceRowID converts the `{id}` path segment (always a string) to the Go
// value the adapter must bind for the primary-key column. Integers, floats
// and booleans are parsed strictly so `/items/abc` on an integer PK is a 400,
// not a database error; everything else (text, varchar, uuid, ObjectId hex,
// timestamps) passes through as a string and the engine casts or rejects it.
func coerceRowID(id, dataType string) (any, error) {
	base := strings.ToLower(strings.TrimSpace(dataType))
	if i := strings.IndexByte(base, '('); i >= 0 {
		base = strings.TrimSpace(base[:i]) // int(11), tinyint(1), varchar(255)
	}
	if i := strings.IndexByte(base, ' '); i >= 0 {
		base = base[:i] // double precision, timestamp with time zone
	}
	switch base {
	case "smallint", "integer", "int", "int2", "int4", "int8",
		"bigint", "serial", "serial2", "serial4", "serial8",
		"bigserial", "smallserial", "tinyint", "mediumint":
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q for integer primary key", id)
		}
		return n, nil
	case "real", "float", "float4", "float8", "double",
		"numeric", "decimal":
		f, err := strconv.ParseFloat(id, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q for numeric primary key", id)
		}
		return f, nil
	case "boolean", "bool":
		b, err := strconv.ParseBool(id)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q for boolean primary key", id)
		}
		return b, nil
	default:
		return id, nil
	}
}

// fullSchema is the aggregated schema response for the explorer UI.
type fullSchema struct {
	Collections   []schemaCollection        `json:"collections"`
	Relationships []adapter.Relationship    `json:"relationships"`
	Capabilities  adapter.CapabilitySet     `json:"capabilities"`
}

type schemaCollection struct {
	Collection string               `json:"collection"`
	Name       string               `json:"name"`
	Columns    []adapter.ColumnInfo `json:"columns"`
	Indexes    []adapter.IndexInfo  `json:"indexes,omitempty"`
}

func (s *Server) getFullSchema(w http.ResponseWriter, r *http.Request) {
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

	collections, err := a.ListCollections(r.Context())
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}

	out := make([]schemaCollection, 0, len(collections))
	for _, c := range collections {
		schema, err := a.GetSchema(r.Context(), c.Name)
		if err != nil {
			continue
		}
		cols := schema.Columns
		if cols == nil {
			cols = []adapter.ColumnInfo{}
		}
		idx := schema.Indexes
		if idx == nil {
			idx = []adapter.IndexInfo{}
		}
		out = append(out, schemaCollection{
			Collection: schema.Collection,
			Name:       schema.Collection,
			Columns:    cols,
			Indexes:    idx,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	// FK relationships are only meaningful on relational engines; the explorer
	// UI gates the diagram on Capabilities.SupportsForeignKeys.
	// Always return [] (never null) so the dashboard can call .length safely.
	relationships := []adapter.Relationship{}
	if a.Capabilities().SupportsForeignKeys {
		if rels, err := a.ListRelationships(r.Context()); err == nil && rels != nil {
			relationships = rels
		}
	}

	writeJSON(w, http.StatusOK, fullSchema{
		Collections:   out,
		Relationships: relationships,
		Capabilities:  a.Capabilities(),
	})
}

func (s *Server) connectProjectForAPIKey(ctx context.Context, projectID string) (Adapter, error) {
	conn, err := s.svc.Store.GetConnectionByProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, errNotFound
		}
		return nil, err
	}
	if s.svc.Secrets == nil {
		return nil, errors.New("server: secrets provider not configured")
	}
	secret, err := s.svc.Secrets.DecryptConnection(conn)
	if err != nil {
		return nil, err
	}
	return s.svc.AdapterFactory.ConnectForProject(ctx, *conn, secret)
}

// ---- Auto-generated REST endpoints (scoped by API key) ----

func (s *Server) apiListTables(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
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
	type tableInfo struct {
		Name string `json:"name"`
	}
	out := make([]tableInfo, 0, len(cols))
	for _, c := range cols {
		out = append(out, tableInfo{Name: c.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) apiGetTableSchema(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	collection := r.PathValue("collection")
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
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

func (s *Server) apiQueryRows(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	collection := r.PathValue("collection")
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	filter := adapter.Filter{Collection: collection}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		var n int
		if _, err := fmt.Sscanf(limit, "%d", &n); err == nil && n > 0 {
			filter.Limit = &n
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		var n int
		if _, err := fmt.Sscanf(offset, "%d", &n); err == nil && n > 0 {
			filter.Offset = &n
		}
	}
	if orderBy := r.URL.Query().Get("order_by"); orderBy != "" {
		desc := r.URL.Query().Get("order") == "desc"
		filter.OrderBy = []adapter.OrderBy{{Field: orderBy, Desc: desc}}
	}

	rs, err := a.Query(r.Context(), adapter.UniversalQuery{Filter: filter})
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) apiInsertRow(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	collection := r.PathValue("collection")
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	var doc map[string]any
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	res, err := a.Insert(r.Context(), collection, doc)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         res.ID,
		"collection": res.Collection,
		"generated":  res.Generated,
	})
}

func (s *Server) apiUpdateRow(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	collection := r.PathValue("collection")
	id := r.PathValue("id")
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	var doc map[string]any
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// The primary-key field is resolved from the live schema, not hard-coded:
	// `_id` is Mongo's key, not Postgres'/MySQL's. The URL is authoritative
	// for identity, so a key carried in the body is dropped rather than
	// applied (rewriting a PK, or tripping FerretDB's immutable `_id`).
	keyField, keyType, err := s.resolveRowKey(r.Context(), a, projectID, collection)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	keyValue, err := coerceRowID(id, keyType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	delete(doc, keyField)

	filter := adapter.Filter{
		Collection: collection,
		Conditions: []adapter.Condition{
			{Field: keyField, Operator: adapter.OpEqual, Value: keyValue},
		},
	}
	res, err := a.Update(r.Context(), filter, doc)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"matched_count":  res.MatchedCount,
		"modified_count": res.ModifiedCount,
	})
}

func (s *Server) apiDeleteRow(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	collection := r.PathValue("collection")
	id := r.PathValue("id")
	a, err := s.connectProjectForAPIKey(r.Context(), projectID)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	defer func() { _ = a.Disconnect(r.Context()) }()

	keyField, keyType, err := s.resolveRowKey(r.Context(), a, projectID, collection)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	keyValue, err := coerceRowID(id, keyType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	filter := adapter.Filter{
		Collection: collection,
		Conditions: []adapter.Condition{
			{Field: keyField, Operator: adapter.OpEqual, Value: keyValue},
		},
	}
	res, err := a.Delete(r.Context(), filter)
	if err != nil {
		s.writeConnectionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted_count": res.DeletedCount,
	})
}
