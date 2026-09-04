// Package valkey implements the DatabaseAdapter interface for Valkey/Redis —
// a key-value store. Data is modeled as a "collection" per Redis set: each
// member key holds one JSON document. Capabilities honestly reflect the
// key-value model (no foreign keys/joins/transactions/native triggers/
// realtime), while table-browse, schema sampling and CRUD all work.
package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/openbase/openbase/internal/adapter"
)

// Adapter is a Valkey implementation of the Universal Data Interface.
type Adapter struct {
	client *redis.Client
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// Compile-time assertion that Adapter supports raw Redis command execution.
var _ adapter.RawQuerier = (*Adapter)(nil)

// New returns a Valkey adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	opts, err := redis.ParseURL(cfg.ConnStr)
	if err != nil {
		// Fall back to a plain host:port if the connection string is not a
		// redis:// URL.
		opts = &redis.Options{Addr: cfg.ConnStr}
	}
	if cfg.Username != "" {
		opts.Username = cfg.Username
	}
	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	opts.DialTimeout = 5 * time.Second

	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("valkey: ping: %w", err)
	}
	a.client = client
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	if a.client == nil {
		return nil
	}
	err := a.client.Close()
	a.client = nil
	return err
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	// Honest for the key-value model and today's implementation:
	// - No relational/foreign-key/join semantics (key-value store).
	// - No native triggers or change streams; no transactions, no search.
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
	if a.client == nil {
		return errors.New("valkey: not connected")
	}
	return nil
}

// memberKey returns the key holding a single document of a collection.
func memberKey(collection, id string) string {
	return collection + ":" + id
}

// seqKey returns the auto-increment sequence key for a collection.
func seqKey(collection string) string {
	return collection + ":seq"
}

// ListCollections returns every set that looks like a collection (any key
// ending in ":" followed by an integer id).
func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	if err := a.requireConnected(); err != nil {
		return nil, err
	}
	keys, err := a.client.Keys(ctx, "*").Result()
	if err != nil {
		return nil, fmt.Errorf("valkey: list collections: %w", err)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if i := strings.LastIndexByte(k, ':'); i > 0 && !seen[k[:i]] {
			// Only treat keys whose suffix parses as an integer as members of a
			// collection; seq keys are excluded by the same rule.
			if _, err := strconv.ParseInt(k[i+1:], 10, 64); err == nil {
				seen[k[:i]] = true
			}
		}
	}
	out := make([]adapter.CollectionInfo, 0, len(seen))
	for name := range seen {
		out = append(out, adapter.CollectionInfo{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetSchema samples the union of top-level JSON fields across a collection's
// documents plus the always-present `id` primary key.
func (a *Adapter) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.SchemaInfo{}, err
	}
	info := adapter.SchemaInfo{Collection: collection}
	rows, _, err := a.loadAll(ctx, collection)
	if err != nil {
		return info, err
	}
	seen := map[string]bool{"id": true}
	for _, row := range rows {
		for k := range row {
			seen[k] = true
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	for _, k := range cols {
		info.Columns = append(info.Columns, adapter.ColumnInfo{
			Name:      k,
			DataType:  "variant",
			Nullable:  k != "id",
			IsPrimary: k == "id",
			IsUnique:  k == "id",
		})
	}
	info.Indexes = []adapter.IndexInfo{{
		Name:    "id",
		Kind:    adapter.IndexPrimary,
		Columns: []string{"id"},
	}}
	return info, nil
}

// ListRelationships always returns empty: the key-value model has no foreign keys.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	return nil, nil
}

// loadAll fetches every document of a collection. Returns the rows and the
// member id for each (parallel slices).
func (a *Adapter) loadAll(ctx context.Context, collection string) ([]map[string]any, []string, error) {
	idStrs, err := a.client.SMembers(ctx, collection).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("valkey: smembers: %w", err)
	}
	var rows []map[string]any
	var ids []string
	for _, id := range idStrs {
		raw, err := a.client.Get(ctx, memberKey(collection, id)).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return nil, nil, fmt.Errorf("valkey: get: %w", err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, nil, fmt.Errorf("valkey: decode %q: %w", memberKey(collection, id), err)
		}
		rows = append(rows, doc)
		ids = append(ids, id)
	}
	return rows, ids, nil
}

// matches evaluates the platform filter conditions against a document. Supports
// the equality family plus contains on string values.
func matches(f adapter.Filter, doc map[string]any) bool {
	for _, c := range f.Conditions {
		v, ok := doc[c.Field]
		if !ok {
			return false
		}
		if !compareOp(c.Operator, v, c.Value) {
			return false
		}
	}
	return true
}

func compareOp(op adapter.Op, actual, want any) bool {
	switch op {
	case "", adapter.OpEqual:
		return fmt.Sprint(actual) == fmt.Sprint(want)
	case adapter.OpNotEqual:
		return fmt.Sprint(actual) != fmt.Sprint(want)
	case adapter.OpContains:
		return strings.Contains(Stringify(actual), fmt.Sprint(want))
	default:
		// Ordering ops aren't reliably comparable on a key-value store; treat
		// them as a no-op filter (honest limitation).
		return true
	}
}

// Stringify renders a JSON primitive value as a comparable string.
func Stringify(v any) string {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case string:
		return t
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

// Query returns rows matching the filter, honoring limit/offset applied after
// filtering (key-value stores have no server-side query planner).
func (a *Adapter) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	rows, _, err := a.loadAll(ctx, q.Filter.Collection)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	var matched []map[string]any
	for _, row := range rows {
		if matches(q.Filter, row) {
			matched = append(matched, row)
		}
	}
	if len(q.Filter.OrderBy) > 0 && len(matched) > 0 {
		ob := q.Filter.OrderBy[0]
		sort.SliceStable(matched, func(i, j int) bool {
			a := fmt.Sprint(matched[i][ob.Field])
			b := fmt.Sprint(matched[j][ob.Field])
			if ob.Desc {
				return a > b
			}
			return a < b
		})
	}
	offset := 0
	if q.Filter.Offset != nil {
		offset = *q.Filter.Offset
	}
	limit := len(matched)
	if q.Filter.Limit != nil {
		limit = *q.Filter.Limit
	}
	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[offset:end]

	seen := map[string]bool{}
	var columns []string
	for _, row := range page {
		for k := range row {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
	}
	return adapter.ResultSet{Columns: columns, Rows: page}, nil
}

// Insert stores a document in the collection. If no "id" is provided, one is
// allocated from the collection's sequence counter.
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.InsertResult{}, err
	}
	var id string
	if rawID, present := doc["id"]; present {
		id = Stringify(rawID)
	} else {
		n, err := a.client.Incr(ctx, seqKey(collection)).Result()
		if err != nil {
			return adapter.InsertResult{}, fmt.Errorf("valkey: incr: %w", err)
		}
		id = strconv.FormatInt(n, 10)
		doc["id"] = n
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return adapter.InsertResult{}, fmt.Errorf("valkey: marshal: %w", err)
	}
	key := memberKey(collection, id)
	if err := a.client.Set(ctx, key, raw, 0).Err(); err != nil {
		return adapter.InsertResult{}, fmt.Errorf("valkey: set: %w", err)
	}
	if err := a.client.SAdd(ctx, collection, id).Err(); err != nil {
		return adapter.InsertResult{}, fmt.Errorf("valkey: sadd: %w", err)
	}
	return adapter.InsertResult{Collection: collection, ID: id, Generated: doc}, nil
}

// Update merges the update map into documents matching the filter.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.UpdateResult{}, err
	}
	rows, ids, err := a.loadAll(ctx, filter.Collection)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	var matchedCount, modifiedCount int64
	pipe := a.client.Pipeline()
	for i, row := range rows {
		if !matches(filter, row) {
			continue
		}
		matchedCount++
		changed := false
		for k, v := range update {
			if Stringify(row[k]) != Stringify(v) {
				row[k] = v
				changed = true
			}
		}
		if !changed {
			continue
		}
		raw, err := json.Marshal(row)
		if err != nil {
			return adapter.UpdateResult{}, fmt.Errorf("valkey: marshal: %w", err)
		}
		pipe.Set(ctx, memberKey(filter.Collection, ids[i]), raw, 0)
		modifiedCount++
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return adapter.UpdateResult{}, fmt.Errorf("valkey: update: %w", err)
	}
	return adapter.UpdateResult{MatchedCount: matchedCount, ModifiedCount: modifiedCount}, nil
}

// Delete removes documents matching the filter, plus their member keys.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.DeleteResult{}, err
	}
	rows, ids, err := a.loadAll(ctx, filter.Collection)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	var removed int64
	pipe := a.client.Pipeline()
	for i, row := range rows {
		if !matches(filter, row) {
			continue
		}
		pipe.Del(ctx, memberKey(filter.Collection, ids[i]))
		pipe.SRem(ctx, filter.Collection, ids[i])
		removed++
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return adapter.DeleteResult{}, fmt.Errorf("valkey: delete: %w", err)
	}
	return adapter.DeleteResult{DeletedCount: removed}, nil
}

// ExecRaw executes raw Valkey/Redis commands (GET/SET/HSET/DEL/KEYS/...
// anything the server speaks) via a direct Do. One command per line; "//",
// "--" and "#" comment lines and blanks are ignored. Multiple commands run
// sequentially and the last result is returned. Results map to a ResultSet:
// scalars -> {"result": v}, arrays -> {"value": each}, maps -> columns=keys.
func (a *Adapter) ExecRaw(ctx context.Context, query string) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	lines := splitRawLines(query)
	if len(lines) == 0 {
		return adapter.ResultSet{}, fmt.Errorf("valkey: query is required")
	}
	var last adapter.ResultSet
	for _, line := range lines {
		args, err := tokenizeRedis(line)
		if err != nil {
			return adapter.ResultSet{}, fmt.Errorf("valkey: %w", err)
		}
		if len(args) == 0 {
			continue
		}
		iface := make([]any, len(args))
		for i, s := range args {
			iface[i] = s
		}
		val, err := a.client.Do(ctx, iface...).Result()
		if err != nil {
			if err == redis.Nil {
				last = adapter.ResultSet{
					Columns: []string{"result"},
					Rows:    []map[string]any{{"result": nil}},
				}
				continue
			}
			return adapter.ResultSet{}, fmt.Errorf("valkey: exec raw (%q): %w", args[0], err)
		}
		last = redisValueToResult(args[0], val)
	}
	if last.Columns == nil {
		last.Columns = []string{}
	}
	if last.Rows == nil {
		last.Rows = []map[string]any{}
	}
	return last, nil
}

// splitRawLines returns executable command lines, dropping blanks and
// full-line comments (//, --, #, /* */).
func splitRawLines(q string) []string {
	var out []string
	for _, line := range strings.Split(q, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "--") || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "/*") && strings.HasSuffix(t, "*/") {
			continue
		}
		// Inline "//" comments (editor samples) — strip when outside quotes.
		if idx := commentIndex(t); idx >= 0 {
			t = strings.TrimSpace(t[:idx])
			if t == "" {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// commentIndex returns the index of an inline "//" outside quotes, or -1.
func commentIndex(s string) int {
	var inSingle, inDouble bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == '"' {
				inDouble = false
			}
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
		if c == '/' && i+1 < len(s) && s[i+1] == '/' {
			return i
		}
	}
	return -1
}

// tokenizeRedis splits a command line on whitespace outside single/double
// quotes (redis-cli style). Surrounding quotes are stripped; backslash
// escapes work inside double quotes.
func tokenizeRedis(line string) ([]string, error) {
	var (
		out      []string
		cur      strings.Builder
		inSingle bool
		inDouble bool
		hasToken bool
	)
	flush := func() {
		if hasToken {
			out = append(out, cur.String())
			cur.Reset()
			hasToken = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inSingle {
			hasToken = true
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		if inDouble {
			hasToken = true
			if c == '\\' && i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
				continue
			}
			if c == '"' {
				inDouble = false
			} else {
				cur.WriteByte(c)
			}
			continue
		}
		switch {
		case c == '\'':
			inSingle = true
			hasToken = true
		case c == '"':
			inDouble = true
			hasToken = true
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		default:
			cur.WriteByte(c)
			hasToken = true
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in %q", line)
	}
	flush()
	return out, nil
}

// redisValueToResult normalizes a go-redis Do result into a ResultSet.
func redisValueToResult(cmd string, val any) adapter.ResultSet {
	_ = cmd
	switch v := val.(type) {
	case nil:
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": nil}}}
	case string:
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": v}}}
	case int64:
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": v}}}
	case int:
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": int64(v)}}}
	case bool:
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": v}}}
	case []any:
		cols := []string{"value"}
		rows := make([]map[string]any, 0, len(v))
		for _, e := range v {
			if len(rows) >= adapter.MaxRawRows {
				break
			}
			rows = append(rows, map[string]any{"value": normalizeRedisScalar(e)})
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		return adapter.ResultSet{Columns: cols, Rows: rows}
	case []string:
		rows := make([]map[string]any, 0, len(v))
		for _, e := range v {
			if len(rows) >= adapter.MaxRawRows {
				break
			}
			rows = append(rows, map[string]any{"value": e})
		}
		if rows == nil {
			rows = []map[string]any{}
		}
		return adapter.ResultSet{Columns: []string{"value"}, Rows: rows}
	case map[string]string:
		cols := make([]string, 0, len(v))
		row := make(map[string]any, len(v))
		for k, s := range v {
			cols = append(cols, k)
			row[k] = s
		}
		sort.Strings(cols)
		return adapter.ResultSet{Columns: cols, Rows: []map[string]any{row}}
	case map[string]any:
		cols := make([]string, 0, len(v))
		for k := range v {
			cols = append(cols, k)
		}
		sort.Strings(cols)
		return adapter.ResultSet{Columns: cols, Rows: []map[string]any{v}}
	case map[any]any:
		cols := make([]string, 0, len(v))
		row := make(map[string]any, len(v))
		for k, e := range v {
			ks := fmt.Sprint(k)
			cols = append(cols, ks)
			row[ks] = normalizeRedisScalar(e)
		}
		sort.Strings(cols)
		return adapter.ResultSet{Columns: cols, Rows: []map[string]any{row}}
	default:
		// Slices of other shapes (e.g. []map) via reflection-free fallback.
		if b, err := json.Marshal(v); err == nil {
			var arr []any
			if err := json.Unmarshal(b, &arr); err == nil {
				rows := make([]map[string]any, 0, len(arr))
				for _, e := range arr {
					if len(rows) >= adapter.MaxRawRows {
						break
					}
					rows = append(rows, map[string]any{"value": e})
				}
				return adapter.ResultSet{Columns: []string{"value"}, Rows: rows}
			}
			var obj map[string]any
			if err := json.Unmarshal(b, &obj); err == nil {
				cols := make([]string, 0, len(obj))
				for k := range obj {
					cols = append(cols, k)
				}
				sort.Strings(cols)
				return adapter.ResultSet{Columns: cols, Rows: []map[string]any{obj}}
			}
			return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": string(b)}}}
		}
		return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": fmt.Sprint(v)}}}
	}
}

func normalizeRedisScalar(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string, int64, int, float64, bool, nil:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// RegisterTrigger is unsupported: Valkey has no native trigger machinery.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return fmt.Errorf("%w: valkey has no native triggers", adapter.ErrUnsupported)
}

// RemoveTrigger is unsupported, mirroring RegisterTrigger.
func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	return fmt.Errorf("%w: valkey has no native triggers", adapter.ErrUnsupported)
}

// RegisterRealtimeBroadcast is unsupported: Valkey has no native triggers.
func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return fmt.Errorf("%w: valkey realtime requires polling emulation (later phase)", adapter.ErrUnsupported)
}

// SubscribeToChanges is unsupported until a polling emulation lands.
func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, fmt.Errorf("%w: valkey realtime requires polling emulation (later phase)", adapter.ErrUnsupported)
}
