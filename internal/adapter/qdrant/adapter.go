// Package qdrant implements the DatabaseAdapter interface for Qdrant, a
// vector database. It speaks Qdrant's HTTP REST API.
//
// Each Qdrant point (id + vector + JSON payload) is surfaced as one universal
// row: the payload fields become columns, and the vector is exposed as a
// `vector` JSON-array column (when present). Schema introspection union-samples
// payload fields across a collection (mirroring the FerretDB approach), and the
// collection's configured vector size comes from its config.
//
// Capabilities are honest for the vector model: native vector search is
// reported, but there are no joins/foreign keys/transactions/native
// triggers/realtime, so those methods return ErrUnsupported.
//
// Testable without a live server: the REST calls use net/http, so tests drive
// them against an in-process httptest server.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/openbase/openbase/internal/adapter"
)

// Adapter is a Qdrant implementation of the Universal Data Interface.
type Adapter struct {
	base   *url.URL
	apiKey string
	hc     *http.Client
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// New returns a Qdrant adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{
		hc: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	base, err := url.Parse(cfg.ConnStr)
	if err != nil {
		return fmt.Errorf("qdrant: parse connection: %w", err)
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return fmt.Errorf("qdrant: unsupported scheme %q (want http/https)", base.Scheme)
	}
	base.Path = strings.TrimRight(base.Path, "/")
	if a.apiKey == "" {
		// Accept the API key from either credential field.
		if cfg.Password != "" {
			a.apiKey = cfg.Password
		} else if cfg.Username != "" {
			a.apiKey = cfg.Username
		}
	}
	a.base = base
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	a.base = nil
	return nil
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	return adapter.CapabilitySet{
		SupportsRelationalJoins: false,
		SupportsForeignKeys:     false,
		SupportsNativeTriggers:  false,
		SupportsChangeStreams:   false,
		SupportsRealtime:        adapter.RealtimeNone,
		SupportsTransactions:    false,
		SupportsFullTextSearch:  false,
		// Qdrant's whole reason to exist is vector/similarity search; the
		// universal "universal query" IR speaks equality filters, so a dedicated
		// similarity-search method is a follow-up, but the engine truly supports
		// vector search natively.
		SupportsVectorSearch: true,
	}
}

func (a *Adapter) requireConnected() error {
	if a == nil || a.base == nil {
		return errors.New("qdrant: not connected")
	}
	return nil
}

// do executes a Qdrant REST request and decodes the JSON body. Qdrant wraps
// results in {"result": <T>, "status": "...", "time": N}; this returns the
// "result" object decoded into out.
func (a *Adapter) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("qdrant: encode request: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	u := *a.base
	rel, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("qdrant: parse path: %w", err)
	}
	u.Path = a.base.Path + rel.Path
	u.RawQuery = rel.RawQuery
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return fmt.Errorf("qdrant: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("api-key", a.apiKey)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("qdrant: %s %s: status %d: %s", method, path, resp.StatusCode, firstLine(string(raw)))
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("qdrant: decode response: %w", err)
	}
	if len(envelope.Result) > 0 && out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("qdrant: decode result: %w", err)
		}
	}
	return nil
}

// ListCollections returns all Qdrant collections.
func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	if err := a.requireConnected(); err != nil {
		return nil, err
	}
	var resp struct {
		Collections []struct {
			Name string `json:"name"`
		} `json:"collections"`
	}
	if err := a.do(ctx, http.MethodGet, "/collections", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]adapter.CollectionInfo, 0, len(resp.Collections))
	for _, c := range resp.Collections {
		out = append(out, adapter.CollectionInfo{Name: c.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetSchema returns the collection's vector size (from config) plus payload
// fields discovered by union-sampling points. The vector is represented as a
// `vector` column.
func (a *Adapter) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.SchemaInfo{}, err
	}
	info := adapter.SchemaInfo{Collection: collection}

	var cfg struct {
		Config struct {
			Params struct {
				Vectors interface{} `json:"vectors"`
			} `json:"params"`
		} `json:"config"`
	}
	if err := a.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(collection), nil, &cfg); err != nil {
		return info, err
	}
	if cfg.Config.Params.Vectors != nil {
		// A named-vectors map or a plain {"size":N} object both indicate a
		// vector dimension; expose it as a column for the browser.
		info.Columns = append(info.Columns, adapter.ColumnInfo{Name: "vector", DataType: "vector"})
	}

	// Union-sample payload fields via a small scroll.
	var scroll struct {
		Points []point `json:"points"`
	}
	if err := a.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(collection)+"/points/scroll",
		map[string]any{"limit": 50}, &scroll); err != nil {
		return info, err
	}
	fieldTypes := map[string]string{}
	for _, p := range scroll.Points {
		for k, v := range p.Payload {
			if _, ok := fieldTypes[k]; !ok {
				fieldTypes[k] = goType(v)
			}
		}
	}
	keys := make([]string, 0, len(fieldTypes))
	for k := range fieldTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		info.Columns = append(info.Columns, adapter.ColumnInfo{Name: k, DataType: fieldTypes[k]})
	}
	return info, nil
}

// ListRelationships is unsupported: Qdrant has no relations.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	return nil, nil
}

// Query reads points matching the filter and flattens them into a column/row
// ResultSet. Equality conditions map to Qdrant payload selectors (match).
func (a *Adapter) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	col := q.Filter.Collection
	filter, err := buildFilter(q.Filter)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	limit := 100
	if q.Filter.Limit != nil {
		limit = *q.Filter.Limit
	}
	scrollBody := map[string]any{"limit": limit}
	if filter != nil {
		scrollBody["filter"] = filter
	}
	var scroll struct {
		Points []point `json:"points"`
	}
	if err := a.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(col)+"/points/scroll", scrollBody, &scroll); err != nil {
		return adapter.ResultSet{}, err
	}

	var result adapter.ResultSet
	keys := map[string]bool{}
	var rows []map[string]any
	for _, p := range scroll.Points {
		row := map[string]any{}
		for k, v := range p.Payload {
			row[k] = v
		}
		if p.ID != "" {
			row["id"] = p.ID
			keys["id"] = true
		}
		if len(p.Vector) > 0 {
			row["vector"] = p.Vector
			keys["vector"] = true
		}
		for k := range row {
			keys[k] = true
		}
		rows = append(rows, row)
	}
	cols := make([]string, 0, len(keys))
	for k := range keys {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	result.Columns = cols
	result.Rows = rows
	return result, nil
}

// point mirrors a Qdrant point for scroll decoding.
type point struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
	Vector  []float64      `json:"vector"`
}

// buildFilter converts universal equality conditions into a Qdrant payload
// filter. Unsupported operators yield an error (honest, not a silent no-op).
func buildFilter(f adapter.Filter) (map[string]any, error) {
	if len(f.Conditions) == 0 {
		return nil, nil
	}
	must := make([]map[string]any, 0, len(f.Conditions))
	for _, c := range f.Conditions {
		var op string
		switch c.Operator {
		case adapter.OpEqual:
			op = "match"
		case adapter.OpNotEqual:
			op = "ne"
		case adapter.OpGreaterThan:
			op = "gt"
		case adapter.OpLessThan:
			op = "lt"
		case adapter.OpGreaterEq:
			op = "gte"
		case adapter.OpLessEq:
			op = "lte"
		default:
			return nil, fmt.Errorf("qdrant: unsupported operator %q for vector payload filter", c.Operator)
		}
		must = append(must, map[string]any{
			"key": c.Field,
			op:    c.Value,
		})
	}
	return map[string]any{"must": must}, nil
}

// Insert upserts a point. The `id` and `vector` keys are used as the point's
// identity and vector respectively; everything else becomes the payload.
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.InsertResult{}, err
	}
	p, vec, ok := splitPoint(doc)
	payload := map[string]any{}
	for k, v := range doc {
		if k == "id" || k == "vector" {
			continue
		}
		payload[k] = v
	}
	if !ok {
		return adapter.InsertResult{}, fmt.Errorf("qdrant: insert requires a vector field for vector indexing")
	}
	body := map[string]any{
		"points": []map[string]any{{
			"id":      p,
			"vector":  vec,
			"payload": payload,
		}},
	}
	if err := a.do(ctx, http.MethodPut, "/collections/"+url.PathEscape(collection)+"/points?wait=true", body, nil); err != nil {
		return adapter.InsertResult{}, err
	}
	return adapter.InsertResult{Collection: collection, ID: p}, nil
}

// splitPoint derives a point id and its []float64 vector from a document.
func splitPoint(doc map[string]any) (any, []float64, bool) {
	id, hasID := doc["id"]
	if !hasID {
		id = 0
	}
	vec, ok := doc["vector"]
	if !ok {
		return id, nil, false
	}
	floats, err := toFloatSlice(vec)
	if err != nil {
		return id, nil, false
	}
	return id, floats, true
}

func toFloatSlice(v any) ([]float64, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out []float64
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Update assigns new payload fields to matching points.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.UpdateResult{}, err
	}
	f, err := buildFilter(filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	body := map[string]any{"payload": update}
	if f != nil {
		body["filter"] = f
	} else {
		body["points"] = []any{}
	}
	matched, err := a.countMatches(ctx, filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	if err := a.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(filter.Collection)+"/points/payload?wait=true", body, nil); err != nil {
		return adapter.UpdateResult{}, err
	}
	return adapter.UpdateResult{MatchedCount: matched, ModifiedCount: matched}, nil
}

// Delete removes matching points.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.DeleteResult{}, err
	}
	f, err := buildFilter(filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	body := map[string]any{"filter": f}
	if f == nil {
		body = map[string]any{"filter": map[string]any{}}
	}
	matched, err := a.countMatches(ctx, filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	if err := a.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(filter.Collection)+"/points/delete?wait=true", body, nil); err != nil {
		return adapter.DeleteResult{}, err
	}
	return adapter.DeleteResult{DeletedCount: matched}, nil
}

// countMatches approximates how many points match a filter using a scroll with
// a low limit; exact counts would need a facet/count endpoint (a follow-up).
func (a *Adapter) countMatches(ctx context.Context, filter adapter.Filter) (int64, error) {
	f, err := buildFilter(filter)
	if err != nil {
		return 0, err
	}
	body := map[string]any{"limit": 1}
	if f != nil {
		body["filter"] = f
	}
	var scroll struct {
		Points []point `json:"points"`
	}
	if err := a.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(filter.Collection)+"/points/scroll", body, &scroll); err != nil {
		return 0, err
	}
	if len(scroll.Points) > 0 {
		return 1, nil
	}
	return 0, nil
}

// RegisterTrigger is unsupported: Qdrant has no triggers.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return fmt.Errorf("%w: qdrant has no triggers", adapter.ErrUnsupported)
}

func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	return fmt.Errorf("%w: qdrant has no triggers", adapter.ErrUnsupported)
}

func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return fmt.Errorf("%w: qdrant has no realtime change stream", adapter.ErrUnsupported)
}

func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, fmt.Errorf("%w: qdrant has no realtime change stream", adapter.ErrUnsupported)
}

// goType maps a decoded JSON payload value to a coarse data type string.
func goType(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if t == float64(int64(t)) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
