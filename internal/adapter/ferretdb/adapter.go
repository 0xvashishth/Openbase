// Package ferretdb implements the DatabaseAdapter interface for FerretDB — a
// MongoDB-wire-compatible document database that stores data in Postgres.
// Documents are schemaless, so schema introspection samples stored documents
// rather than reporting a fixed table shape, and capabilities honestly reflect
// the document model (no foreign keys, no joins, no native triggers/realtime).
package ferretdb

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/openbase/openbase/internal/adapter"
)

// Adapter is a FerretDB implementation of the Universal Data Interface.
type Adapter struct {
	client *mongo.Client
	db     *mongo.Database
	dbName string
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// New returns a FerretDB adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	opts := options.Client().ApplyURI(cfg.ConnStr)
	if cfg.Username != "" || cfg.Password != "" {
		opts.SetAuth(options.Credential{Username: cfg.Username, Password: cfg.Password})
	}
	// Favor a short server-selection timeout so a dead endpoint fails fast in
	// the BYODB test flow instead of hanging on the driver default (30s+).
	opts.SetServerSelectionTimeout(10 * time.Second)

	client, err := mongo.Connect(opts)
	if err != nil {
		return fmt.Errorf("ferretdb: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(ctx)
		return fmt.Errorf("ferretdb: ping: %w", err)
	}

	a.client = client
	a.dbName = dbName(cfg)
	a.db = client.Database(a.dbName)
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	if a.client == nil {
		return nil
	}
	err := a.client.Disconnect(ctx)
	a.client = nil
	a.db = nil
	return err
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	// Honest for FerretDB's document model and today's implementation:
	// - No foreign keys / joins (document model).
	// - No native triggers or change streams; realtime arrives via a polling
	//   emulation in a later phase, so it is "none" until that exists.
	// - No transactions, no vector/full-text search primitives to rely on.
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
	if a.client == nil || a.db == nil {
		return errors.New("ferretdb: not connected")
	}
	return nil
}

func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	if err := a.requireConnected(); err != nil {
		return nil, err
	}
	names, err := a.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("ferretdb: list collections: %w", err)
	}
	out := make([]adapter.CollectionInfo, 0, len(names))
	for _, n := range names {
		out = append(out, adapter.CollectionInfo{Name: n})
	}
	return out, nil
}

// GetSchema reports the current shape of a collection. Because documents are
// schemaless this is a sample of the union of top-level fields plus the
// always-present `_id` primary key, prefixed with a "sampled" data type.
func (a *Adapter) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.SchemaInfo{}, err
	}
	info := adapter.SchemaInfo{Collection: collection}

	// Union of keys across a sample of documents.
	cur, err := a.db.Collection(collection).Find(ctx, bson.D{})
	if err != nil {
		return info, fmt.Errorf("ferretdb: get schema: %w", err)
	}
	defer cur.Close(ctx)

	seen := map[string]bool{}
	limit := 200
	for cur.Next(ctx) {
		var doc map[string]any
		if err := cur.Decode(&doc); err != nil {
			return info, err
		}
		for k := range doc {
			seen[k] = true
		}
		limit--
		if limit <= 0 {
			break
		}
	}
	if err := cur.Err(); err != nil {
		return info, err
	}

	if !seen["_id"] {
		seen["_id"] = true
	}
	for k := range seen {
		info.Columns = append(info.Columns, adapter.ColumnInfo{
			Name:      k,
			DataType:  "variant",
			Nullable:  true,
			IsPrimary: k == "_id",
			IsUnique:  k == "_id",
		})
	}

	// Indexes: FerretDB always maintains the unique `_id` index. Query it;
	// if the command is unsupported degrade gracefully to the _id assumption.
	if idx, err := a.listIndexes(ctx, collection); err == nil {
		info.Indexes = idx
	} else if !seen["_id"] {
		info.Indexes = []adapter.IndexInfo{{
			Name:    "_id_",
			Kind:    adapter.IndexPrimary,
			Columns: []string{"_id"},
		}}
	}
	return info, nil
}

func (a *Adapter) listIndexes(ctx context.Context, collection string) ([]adapter.IndexInfo, error) {
	cur, err := a.db.Collection(collection).Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []adapter.IndexInfo
	for cur.Next(ctx) {
		var raw bson.Raw
		if err := cur.Decode(&raw); err != nil {
			continue
		}
		name := raw.Lookup("name").StringValue()
		els, err := raw.Lookup("key").Document().Elements()
		if err != nil {
			continue
		}
		cols := make([]string, 0, 1)
		for _, el := range els {
			cols = append(cols, el.Key())
		}
		kind := adapter.IndexNormal
		if name == "_id_" || (len(cols) == 1 && cols[0] == "_id") {
			kind = adapter.IndexPrimary
		}
		out = append(out, adapter.IndexInfo{Name: name, Kind: kind, Columns: cols})
	}
	return out, cur.Err()
}

// ListRelationships always returns empty: the document model has no foreign keys.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	return nil, nil
}

// Query translates a UniversalQuery into a MongoDB find command. The ResultSet's
// columns are the union of keys across the returned documents.
func (a *Adapter) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	filter, err := buildFilter(q.Filter)
	if err != nil {
		return adapter.ResultSet{}, err
	}

	findOpts := options.Find()
	if len(q.Filter.OrderBy) > 0 {
		sort := make(bson.D, 0, len(q.Filter.OrderBy))
		for _, o := range q.Filter.OrderBy {
			dir := 1
			if o.Desc {
				dir = -1
			}
			sort = append(sort, bson.E{Key: o.Field, Value: dir})
		}
		findOpts.SetSort(sort)
	}
	if q.Filter.Limit != nil {
		findOpts.SetLimit(int64(*q.Filter.Limit))
	}
	if q.Filter.Offset != nil {
		findOpts.SetSkip(int64(*q.Filter.Offset))
	}

	cur, err := a.db.Collection(q.Filter.Collection).Find(ctx, filter, findOpts)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: query: %w", err)
	}
	defer cur.Close(ctx)

	var result adapter.ResultSet
	seen := map[string]bool{}
	for cur.Next(ctx) {
		var doc map[string]any
		if err := cur.Decode(&doc); err != nil {
			return result, err
		}
		row := normalizeMap(doc)
		for k := range row {
			if !seen[k] {
				seen[k] = true
				result.Columns = append(result.Columns, k)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return result, cur.Err()
}

// buildFilter converts platform Conditions into a MongoDB query document.
func buildFilter(f adapter.Filter) (bson.M, error) {
	if len(f.Conditions) == 0 {
		return bson.M{}, nil
	}
	filter := make(bson.M, len(f.Conditions))
	for _, c := range f.Conditions {
		if c.Field == "" {
			return nil, errors.New("ferretdb: empty filter field")
		}
		switch c.Operator {
		case "", adapter.OpEqual:
			filter[c.Field] = c.Value
		case adapter.OpNotEqual:
			filter[c.Field] = bson.M{"$ne": c.Value}
		case adapter.OpGreaterThan:
			filter[c.Field] = bson.M{"$gt": c.Value}
		case adapter.OpLessThan:
			filter[c.Field] = bson.M{"$lt": c.Value}
		case adapter.OpGreaterEq:
			filter[c.Field] = bson.M{"$gte": c.Value}
		case adapter.OpLessEq:
			filter[c.Field] = bson.M{"$lte": c.Value}
		case adapter.OpContains:
			pattern := ".*" + regexp.QuoteMeta(fmt.Sprint(c.Value)) + ".*"
			filter[c.Field] = bson.M{"$regex": bson.Regex{Pattern: pattern}}
		default:
			return nil, fmt.Errorf("ferretdb: unsupported operator %q", c.Operator)
		}
	}
	return filter, nil
}

// Insert inserts a document and returns the generated `_id`.
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.InsertResult{}, err
	}
	res, err := a.db.Collection(collection).InsertOne(ctx, doc)
	if err != nil {
		return adapter.InsertResult{}, fmt.Errorf("ferretdb: insert: %w", err)
	}
	id := doc["_id"]
	if res.InsertedID != nil {
		id = normalize(res.InsertedID)
	}
	return adapter.InsertResult{
		Collection: collection,
		ID:         id,
		Generated:  normalizeMap(doc),
	}, nil
}

// Update updates documents matching the filter.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.UpdateResult{}, err
	}
	m, err := buildFilter(filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	res, err := a.db.Collection(filter.Collection).UpdateMany(ctx, m, bson.M{"$set": update})
	if err != nil {
		return adapter.UpdateResult{}, fmt.Errorf("ferretdb: update: %w", err)
	}
	return adapter.UpdateResult{MatchedCount: res.MatchedCount, ModifiedCount: res.ModifiedCount}, nil
}

// Delete removes documents matching the filter.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.DeleteResult{}, err
	}
	m, err := buildFilter(filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	res, err := a.db.Collection(filter.Collection).DeleteMany(ctx, m)
	if err != nil {
		return adapter.DeleteResult{}, fmt.Errorf("ferretdb: delete: %w", err)
	}
	return adapter.DeleteResult{DeletedCount: res.DeletedCount}, nil
}

// RegisterTrigger is unsupported: FerretDB has no native trigger machinery.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return fmt.Errorf("%w: ferretdb has no native triggers (polling emulation is a later phase)", adapter.ErrUnsupported)
}

// RemoveTrigger is unsupported, mirroring RegisterTrigger.
func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	return fmt.Errorf("%w: ferretdb has no native triggers", adapter.ErrUnsupported)
}

// RegisterRealtimeBroadcast is unsupported: FerretDB has no native triggers.
func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return fmt.Errorf("%w: ferretdb realtime requires polling emulation (later phase)", adapter.ErrUnsupported)
}

// SubscribeToChanges is unsupported until a polling emulation lands (Phase 5).
func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, fmt.Errorf("%w: ferretdb realtime requires polling emulation (later phase)", adapter.ErrUnsupported)
}

// dbName determines the target database from config or the connection string.
func dbName(cfg adapter.ConnectionConfig) string {
	if cfg.Database != "" {
		return cfg.Database
	}
	// CGI-decoded path of "mongodb://host:port/name" or "mongodb+srv://.../name".
	uri := cfg.ConnStr
	if i := strings.Index(uri, "://"); i >= 0 {
		rest := uri[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			if p := rest[j+1:]; p != "" {
				if k := strings.IndexByte(p, '?'); k >= 0 {
					p = p[:k]
				}
				return p
			}
		}
	}
	return "test"
}

// normalize converts BSON values into JSON-friendly primitives, recursively.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return normalizeMap(t)
	case bson.M:
		return normalizeMap(map[string]any(t))
	case bson.A:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalize(val)
		}
		return out
	case bson.ObjectID:
		return t.Hex()
	case bson.Decimal128:
		return t.String()
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	default:
		return v
	}
}

// normalizeMap converts each value of a decoded document in place.
func normalizeMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = normalize(v)
	}
	return out
}