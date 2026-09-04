// Package arcadedb implements the DatabaseAdapter interface for ArcadeDB, a
// multi-model (document + graph) database. This adapter uses the **document
// model** surface of ArcadeDB's HTTP REST API: each document type is a
// universal collection and each document a universal row.
//
// It speaks the SQL-style endpoints:
//
//	POST /api/v1/query/{db}    read-only queries   ({"language":"sql","command":...,"params":[...]})
//	POST /api/v1/command/{db}  write / DDL        (same body)
//
// Both respond with {"result":[...]} using the default flat "record"
// serializer. Basic-auth is used against the server root/app user. There is no
// realtime/trigger support, so those methods return ErrUnsupported; the
// graph/traversal surface is deliberately out of scope for the document-model
// adapter, so ListRelationships returns none.
//
// Testable without a live server: the REST calls use net/http, so tests drive
// them against an in-process httptest server.
package arcadedb

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

// internal type names ArcadeDB reserves with the leading "O" prefix; these are
// filtered out of ListCollections so only user-defined types are surfaced.
var internalTypes = map[string]bool{
	"OSchema": true, "OUser": true, "ORole": true, "OSequence": true,
	"OIndex": true, "OFunction": true, "OTrigger": true, "OVertex": true,
	"OEdge": true, "OIdentity": true, "ONode": true, "OLink": true,
	"OType": true, "OAnalyzer": true, "OCluster": true, "ODictionary": true,
	"ORid": true, "OBinary": true, "OString": true, "OInteger": true,
	"OLong": true, "OShort": true, "OFloat": true, "ODouble": true,
	"OBoolean": true, "ODate": true, "ODateTime": true, "OEmbedded": true,
	"OList": true, "OMap": true, "OSet": true, "OArray": true,
	"OByte": true, "ODocument": true, "OJSON": true, "OGraph": true,
}

// Adapter is an ArcadeDB implementation of the Universal Data Interface.
type Adapter struct {
	base     string
	database string
	user     string
	password string
	hc       *http.Client
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// Compile-time assertion that Adapter supports raw SQL execution.
var _ adapter.RawQuerier = (*Adapter)(nil)

// New returns an ArcadeDB adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{
		hc: &http.Client{Timeout: 20 * time.Second},
	}
}

// Connect parses the connection string and records auth + database.
//
// The connection string may be http(s)://host:port or arcadedb://host:port,
// with an optional path naming the database (http://host:2480/MyDb). The
// explicit cfg.Database wins over the path when both are set.
func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	raw := cfg.ConnStr
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("arcadedb: parse connection: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
		// ok
	case "arcadedb", "arcadedbs":
		if u.Scheme == "arcadedbs" {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}
	default:
		return fmt.Errorf("arcadedb: unsupported scheme %q (want http/https/arcadedb)", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	db := cfg.Database
	if db == "" && u.Path != "" {
		db = strings.TrimLeft(u.Path, "/")
		u.Path = ""
	}
	_, portErr := probePort(u)
	if portErr != nil {
		return portErr
	}
	a.base = u.String()
	a.database = db
	a.user = cfg.Username
	a.password = cfg.Password
	return nil
}

// probePort ensures the URL carries an explicit port (or defaults it) so the
// base URL is well-formed.
func probePort(u *url.URL) (*url.URL, error) {
	if u.Port() == "" {
		p := "2480"
		if u.Scheme == "https" {
			p = "2481"
		}
		u.Host = u.Host + ":" + p
	}
	return u, nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	a.base = ""
	a.database = ""
	return nil
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	// Document-model adapter: real CRUD, schema, and Ordered/equality filtering
	// on documents, but no relational joins (graph traversal is out of scope),
	// no transactions exposed through this HTTP surface, and no
	// triggers/realtime. Honest flags; feature methods return ErrUnsupported.
	return adapter.CapabilitySet{
		SupportsRelationalJoins: false,
		SupportsForeignKeys:     false,
		SupportsNativeTriggers:  false,
		SupportsChangeStreams:   false,
		SupportsRealtime:        adapter.RealtimeNone,
		SupportsTransactions:    false,
		SupportsFullTextSearch:  false,
		SupportsVectorSearch:    false,
	}
}

func (a *Adapter) requireConnected() error {
	if a == nil || a.base == "" {
		return errors.New("arcadedb: not connected")
	}
	if a.database == "" {
		return errors.New("arcadedb: no database selected")
	}
	return nil
}

// do executes an ArcadeDB query/command request and decodes the flat
// {"result":[...]} body into *rows. method must be "POST".
func (a *Adapter) do(ctx context.Context, endpoint string, sql string, params []any) ([]map[string]any, error) {
	if err := a.requireConnected(); err != nil {
		return nil, err
	}
	body := map[string]any{
		"language": "sql",
		"command":  sql,
	}
	if len(params) > 0 {
		body["params"] = params
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("arcadedb: encode request: %w", err)
	}
	u := strings.TrimRight(a.base, "/") + "/api/v1/" + endpoint + "/" + url.PathEscape(a.database)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("arcadedb: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.user != "" || a.password != "" {
		req.SetBasicAuth(a.user, a.password)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arcadedb: %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("arcadedb: %s: status %d: %s", endpoint, resp.StatusCode, firstLine(string(raw)))
	}
	var envelope struct {
		Result []map[string]any `json:"result"`
		Error  string           `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("arcadedb: decode response: %w", err)
	}
	if envelope.Error != "" {
		return nil, fmt.Errorf("arcadedb: %s: %s", endpoint, envelope.Error)
	}
	return envelope.Result, nil
}

// query runs a read-only SQL SELECT against ArcadeDB.
func (a *Adapter) query(ctx context.Context, sql string, params []any) ([]map[string]any, error) {
	return a.do(ctx, "query", sql, params)
}

// command runs a write/DDL SQL command against ArcadeDB.
func (a *Adapter) command(ctx context.Context, sql string, params []any) ([]map[string]any, error) {
	return a.do(ctx, "command", sql, params)
}

// ListCollections returns the user-defined document types (ArcadeDB internal
// "O"-prefixed types are filtered out).
func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	rows, err := a.query(ctx, "SELECT name FROM Type", nil)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, r := range rows {
		name, _ := r["name"].(string)
		if name == "" || internalTypes[name] {
			continue
		}
		set[name] = true
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]adapter.CollectionInfo, 0, len(names))
	for _, n := range names {
		out = append(out, adapter.CollectionInfo{Name: n})
	}
	return out, nil
}

// GetSchema samples documents of the type and reports the union of their
// fields, with the ArcadeDB record id (@rid) surfaced as the primary key.
func (a *Adapter) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	if err := validateIdent(collection); err != nil {
		return adapter.SchemaInfo{}, fmt.Errorf("arcadedb: %w", err)
	}
	info := adapter.SchemaInfo{Collection: collection}
	rows, err := a.query(ctx, "SELECT FROM "+collection+" LIMIT 50", nil)
	if err != nil {
		return info, err
	}
	fieldTypes := map[string]string{}
	for _, r := range rows {
		for k, v := range r {
			if k == "@type" || k == "@cat" || k == "out_" || k == "in_" {
				continue
			}
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
		col := adapter.ColumnInfo{Name: k, DataType: fieldTypes[k]}
		if k == "@rid" {
			col.IsPrimary = true
		}
		info.Columns = append(info.Columns, col)
	}
	if len(rows) == 0 {
		// No sample rows: still advertise the rid as the identity column.
		info.Columns = []adapter.ColumnInfo{{Name: "@rid", DataType: "string", IsPrimary: true}}
	}
	return info, nil
}

// ListRelationships is unsupported in the document-model surface: ArcadeDB
// relationships are graph edges (traversal), which this adapter deliberately
// does not model.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	return nil, nil
}

// Query runs a SELECT over the collection, translating universal conditions
// into ArcadeDB SQL WHERE (with bound params), plus ORDER BY / LIMIT / SKIP.
func (a *Adapter) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	f := q.Filter
	if err := validateIdent(f.Collection); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("arcadedb: %w", err)
	}
	sql, params, err := buildSelect(f.Collection, f)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	rows, err := a.query(ctx, sql, params)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	return rowsToResult(rows), nil
}

// ExecRaw executes a raw ArcadeDB SQL statement — reads (SELECT) via the
// /query endpoint and writes/DDL (INSERT/UPDATE/DELETE/CREATE/ALTER/DROP,
// ...) via /command. Reads return columns+rows capped at MaxRawRows; writes
// with an empty result return {"result": "OK"} so the editor renders clearly.
func (a *Adapter) ExecRaw(ctx context.Context, query string) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	// Strip leading comments (editor samples carry them) and tolerate a single
	// trailing semicolon: ArcadeDB's REST API wants one bare statement.
	q := strings.TrimSpace(stripLeadingSQLComments(query))
	q = strings.TrimSpace(strings.TrimSuffix(q, ";"))
	if q == "" {
		return adapter.ResultSet{}, fmt.Errorf("arcadedb: query is required")
	}
	first := rawFirstKeyword(q)
	var (
		rows []map[string]any
		err  error
	)
	if first == "SELECT" {
		rows, err = a.query(ctx, q, nil)
	} else {
		rows, err = a.command(ctx, q, nil)
	}
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("arcadedb: exec raw: %w", err)
	}
	if len(rows) == 0 {
		return adapter.ResultSet{
			Columns: []string{"result"},
			Rows:    []map[string]any{{"result": "OK"}},
		}, nil
	}
	rs := rowsToResult(rows)
	if len(rs.Rows) > adapter.MaxRawRows {
		rs.Rows = rs.Rows[:adapter.MaxRawRows]
	}
	if rs.Columns == nil {
		rs.Columns = []string{}
	}
	if rs.Rows == nil {
		rs.Rows = []map[string]any{}
	}
	return rs, nil
}

// stripLeadingSQLComments removes leading "--" line and "/* */" block
// comments so the statement handed to the engine starts at the keyword.
func stripLeadingSQLComments(q string) string {
	rest := strings.TrimSpace(q)
	for {
		t := strings.TrimSpace(rest)
		if strings.HasPrefix(t, "--") {
			if i := strings.Index(t, "\n"); i >= 0 {
				rest = t[i+1:]
				continue
			}
			return ""
		}
		if strings.HasPrefix(t, "/*") {
			if i := strings.Index(t, "*/"); i >= 0 {
				rest = t[i+2:]
				continue
			}
			return t
		}
		return t
	}
}

// rawFirstKeyword returns the upper-cased first keyword after stripping
// leading comments and parens (e.g. "-- c\nSELECT ..." -> "SELECT").
func rawFirstKeyword(q string) string {
	rest := stripLeadingSQLComments(q)
	rest = strings.TrimLeft(rest, "( ")
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(strings.Trim(fields[0], ";()"))
}

// Insert inserts a document into the type. The returned id is the record's
// @rid (generated server-side) or the parsed integer/complex id present in the
// insert command result.
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := validateIdent(collection); err != nil {
		return adapter.InsertResult{}, fmt.Errorf("arcadedb: %w", err)
	}
	cols := make([]string, 0, len(doc))
	for k := range doc {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	var sb strings.Builder
	sb.WriteString("INSERT INTO " + collection + " SET ")
	params := make([]any, 0, len(cols))
	for i, k := range cols {
		if err := validateIdent(k); err != nil {
			return adapter.InsertResult{}, fmt.Errorf("arcadedb: invalid field %q", k)
		}
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(k + " = ?")
		params = append(params, doc[k])
	}
	rows, err := a.command(ctx, sb.String(), params)
	if err != nil {
		return adapter.InsertResult{}, err
	}
	id := extractID(rows)
	return adapter.InsertResult{Collection: collection, ID: id}, nil
}

// Update assigns new field values to documents matching the filter.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := validateIdent(filter.Collection); err != nil {
		return adapter.UpdateResult{}, fmt.Errorf("arcadedb: %w", err)
	}
	matched, err := a.countMatches(ctx, filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	where, whereParams, err := buildWhere(filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	cols := make([]string, 0, len(update))
	for k := range update {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	var sb strings.Builder
	sb.WriteString("UPDATE " + filter.Collection + " SET ")
	params := make([]any, 0, len(cols)+len(whereParams))
	for i, k := range cols {
		if err := validateIdent(k); err != nil {
			return adapter.UpdateResult{}, fmt.Errorf("arcadedb: invalid field %q", k)
		}
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(k + " = ?")
		params = append(params, update[k])
	}
	if where != "" {
		sb.WriteString(" WHERE " + where)
		params = append(params, whereParams...)
	}
	if _, err := a.command(ctx, sb.String(), params); err != nil {
		return adapter.UpdateResult{}, err
	}
	return adapter.UpdateResult{MatchedCount: matched, ModifiedCount: matched}, nil
}

// Delete removes documents matching the filter.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := validateIdent(filter.Collection); err != nil {
		return adapter.DeleteResult{}, fmt.Errorf("arcadedb: %w", err)
	}
	matched, err := a.countMatches(ctx, filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	where, params, err := buildWhere(filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	sb := "DELETE FROM " + filter.Collection
	if where != "" {
		sb += " WHERE " + where
	}
	if _, err := a.command(ctx, sb, params); err != nil {
		return adapter.DeleteResult{}, err
	}
	return adapter.DeleteResult{DeletedCount: matched}, nil
}

// countMatches returns how many documents match the filter via SELECT count(*).
func (a *Adapter) countMatches(ctx context.Context, filter adapter.Filter) (int64, error) {
	where, params, err := buildWhere(filter)
	if err != nil {
		return 0, err
	}
	sb := "SELECT count(*) AS count FROM " + filter.Collection
	if where != "" {
		sb += " WHERE " + where
	}
	rows, err := a.query(ctx, sb, params)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	switch c := rows[0]["count"].(type) {
	case float64:
		return int64(c), nil
	case int64:
		return c, nil
	case int:
		return int64(c), nil
	case json.Number:
		n, _ := c.Int64()
		return n, nil
	}
	return 0, nil
}

// RegisterTrigger is unsupported: ArcadeDB document surface has no hooks here.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return fmt.Errorf("%w: arcadedb document surface has no triggers", adapter.ErrUnsupported)
}

func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	return fmt.Errorf("%w: arcadedb document surface has no triggers", adapter.ErrUnsupported)
}

func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return fmt.Errorf("%w: arcadedb has no realtime change stream", adapter.ErrUnsupported)
}

func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, fmt.Errorf("%w: arcadedb has no realtime change stream", adapter.ErrUnsupported)
}

// buildSelect composes SELECT ... [WHERE ...] [ORDER BY ...] [LIMIT n] [SKIP m].
func buildSelect(collection string, f adapter.Filter) (string, []any, error) {
	sb := "SELECT FROM " + collection
	where, params, err := buildWhere(f)
	if err != nil {
		return "", nil, err
	}
	if where != "" {
		sb += " WHERE " + where
	}
	if len(f.OrderBy) > 0 {
		sb += " ORDER BY "
		for i, ob := range f.OrderBy {
			if err := validateIdent(ob.Field); err != nil {
				return "", nil, fmt.Errorf("arcadedb: invalid order field %q", ob.Field)
			}
			if i > 0 {
				sb += ", "
			}
			sb += ob.Field
			if ob.Desc {
				sb += " DESC"
			} else {
				sb += " ASC"
			}
		}
	}
	if f.Limit != nil {
		sb += " LIMIT " + itoa(*f.Limit)
	}
	if f.Offset != nil {
		sb += " SKIP " + itoa(*f.Offset)
	}
	return sb, params, nil
}

// buildWhere compiles universal conditions into SQL WHERE with bound params.
func buildWhere(f adapter.Filter) (string, []any, error) {
	if len(f.Conditions) == 0 {
		return "", nil, nil
	}
	var ops strings.Builder
	params := make([]any, 0, len(f.Conditions))
	for i, c := range f.Conditions {
		if err := validateIdent(c.Field); err != nil {
			return "", nil, fmt.Errorf("arcadedb: invalid filter field %q", c.Field)
		}
		if i > 0 {
			ops.WriteString(" AND ")
		}
		ops.WriteString(c.Field)
		var op string
		switch c.Operator {
		case adapter.OpEqual:
			op = "="
		case adapter.OpNotEqual:
			op = "<>"
		case adapter.OpGreaterThan:
			op = ">"
		case adapter.OpLessThan:
			op = "<"
		case adapter.OpGreaterEq:
			op = ">="
		case adapter.OpLessEq:
			op = "<="
		case adapter.OpContains:
			op = " CONTAINS "
			ops.WriteString(op + "?")
			params = append(params, c.Value)
			continue
		default:
			return "", nil, fmt.Errorf("arcadedb: unsupported operator %q", c.Operator)
		}
		ops.WriteString(op + " ?")
		params = append(params, c.Value)
	}
	return ops.String(), params, nil
}

// rowsToResult flattens decoded rows into the universal ResultSet, omitting
// ArcadeDB's @type/@cat metadata columns.
func rowsToResult(rows []map[string]any) adapter.ResultSet {
	var result adapter.ResultSet
	keys := map[string]bool{}
	var out []map[string]any
	for _, r := range rows {
		row := map[string]any{}
		for k, v := range r {
			if k == "@type" || k == "@cat" {
				continue
			}
			row[k] = v
			keys[k] = true
		}
		out = append(out, row)
	}
	cols := make([]string, 0, len(keys))
	for k := range keys {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	result.Columns = cols
	result.Rows = out
	return result
}

// extractID pulls a record id from an insert command result (the @rid field of
// the first affected record, else an index-based fallback).
func extractID(rows []map[string]any) any {
	if len(rows) == 0 {
		return nil
	}
	if v, ok := rows[0]["@rid"]; ok {
		return v
	}
	if v, ok := rows[0]["rid"]; ok {
		return v
	}
	return nil
}

// validateIdent rejects identifiers that could break out of a generated SQL
// statement, allowing only letters/digits/_/$/. (dot for navigated fields).
func validateIdent(s string) error {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '_' || r == '$' || r == '.') {
			return fmt.Errorf("invalid identifier %q", s)
		}
	}
	if s == "" {
		return fmt.Errorf("invalid identifier %q", s)
	}
	return nil
}

// itoa avoids importing strconv for this small helper.
func itoa(n int) string {
	return fmt.Sprint(n)
}

// goType maps a decoded JSON value to a coarse data type string.
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
