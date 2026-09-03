package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/openbase/openbase/internal/adapter"
	"github.com/openbase/openbase/internal/metadata"
)

// fullSchema is the aggregated schema response for the explorer UI.
type fullSchema struct {
	Collections   []schemaCollection        `json:"collections"`
	Relationships []adapter.Relationship    `json:"relationships"`
	Capabilities  adapter.CapabilitySet     `json:"capabilities"`
}

type schemaCollection struct {
	Name    string               `json:"name"`
	Columns []adapter.ColumnInfo `json:"columns"`
	Indexes []adapter.IndexInfo  `json:"indexes,omitempty"`
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
			Name:    schema.Collection,
			Columns: cols,
			Indexes: idx,
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

	filter := adapter.Filter{
		Collection: collection,
		Conditions: []adapter.Condition{
			{Field: "_id", Operator: adapter.OpEqual, Value: id},
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

	filter := adapter.Filter{
		Collection: collection,
		Conditions: []adapter.Condition{
			{Field: "_id", Operator: adapter.OpEqual, Value: id},
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
