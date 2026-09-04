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

// Compile-time assertion that Adapter supports raw mongo-shell execution.
var _ adapter.RawQuerier = (*Adapter)(nil)

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

// ExecRaw executes raw mongo-shell style commands:
//
//	db.<coll>.find(<filter>[, {limit, skip, sort}])[.limit(n).skip(n).sort({...})]
//	db.<coll>.insertOne(<doc>) / insertMany([<docs>])
//	db.<coll>.updateOne/updateMany(<filter>, <update>)
//	db.<coll>.deleteOne/deleteMany(<filter>)
//	db.<coll>.countDocuments([<filter>]) / count([<filter>])
//	db.createCollection("<name>") / db.<coll>.drop()
//
// As an alternative, a JSON object with {"collection","op","filter","doc",
// "docs","update","limit","skip","sort"} is accepted. Reads return
// columns+rows capped at MaxRawRows; writes return affected/count/OK rows.
func (a *Adapter) ExecRaw(ctx context.Context, query string) (adapter.ResultSet, error) {
	if err := a.requireConnected(); err != nil {
		return adapter.ResultSet{}, err
	}
	cleaned := stripMongoComments(query)
	trimmed := strings.TrimSpace(cleaned)
	// Tolerate a single trailing semicolon from the editor.
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	if trimmed == "" {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: query is required")
	}
	if strings.HasPrefix(trimmed, "{") {
		return a.execRawJSON(ctx, trimmed)
	}
	coll, op, argsStr, chain, err := parseMongoShell(trimmed)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	args, err := splitTopLevelArgs(argsStr)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	return a.dispatchMongoOp(ctx, coll, op, args, chain)
}

func (a *Adapter) dispatchMongoOp(ctx context.Context, coll, op string, args []string, chain string) (adapter.ResultSet, error) {
	switch op {
	case "find", "findOne":
		return a.execFind(ctx, coll, args, chain, op == "findOne")
	case "insertOne":
		return a.execInsertOne(ctx, coll, args)
	case "insertMany":
		return a.execInsertMany(ctx, coll, args)
	case "updateOne", "updateMany":
		return a.execUpdate(ctx, coll, args, op == "updateMany")
	case "deleteOne", "deleteMany":
		return a.execDeleteRaw(ctx, coll, args, op == "deleteMany")
	case "countDocuments", "count", "count_documents":
		return a.execCount(ctx, coll, args)
	case "drop":
		if err := a.db.Collection(coll).Drop(ctx); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: drop: %w", err)
		}
		return okResult(), nil
	case "createCollection":
		// db.createCollection("name") parses coll as "createCollection".
		name := coll
		if len(args) > 0 {
			var nm string
			if err := bson.UnmarshalExtJSON([]byte(strings.TrimSpace(args[0])), false, &nm); err == nil {
				name = nm
			} else {
				name = strings.Trim(strings.TrimSpace(args[0]), `"'`)
			}
		}
		if name == "" {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: createCollection requires a name")
		}
		if err := a.db.CreateCollection(ctx, name); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: createCollection: %w", err)
		}
		return okResult(), nil
	default:
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: unsupported operation %q (want find/insertOne/insertMany/updateOne/updateMany/deleteOne/deleteMany/countDocuments/drop)", op)
	}
}

func (a *Adapter) execFind(ctx context.Context, coll string, args []string, chain string, one bool) (adapter.ResultSet, error) {
	filter := bson.M{}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		var f bson.M
		if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &f); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid filter JSON: %w", err)
		}
		if f == nil {
			f = bson.M{}
		}
		filter = f
	}
	var (
		limit    *int64
		skip     *int64
		sortSpec any
	)
	if len(args) > 1 && strings.TrimSpace(args[1]) != "" {
		var opts bson.M
		if err := bson.UnmarshalExtJSON([]byte(args[1]), false, &opts); err == nil && opts != nil {
			if v, ok := opts["limit"]; ok {
				if n, ok := toInt64(v); ok && n > 0 {
					limit = &n
				}
			}
			if v, ok := opts["skip"]; ok {
				if n, ok := toInt64(v); ok && n >= 0 {
					skip = &n
				}
			}
			if v, ok := opts["sort"]; ok {
				sortSpec = v
			}
		}
	}
	if chain != "" {
		if n, ok := parseChainInt(chain, "limit"); ok {
			limit = &n
		}
		if n, ok := parseChainInt(chain, "skip"); ok {
			skip = &n
		}
		if s := parseChainDoc(chain, "sort"); s != "" {
			var sd bson.M
			if err := bson.UnmarshalExtJSON([]byte(s), false, &sd); err == nil {
				sortSpec = sd
			}
		}
	}
	if one {
		n := int64(1)
		limit = &n
	}
	maxCap := int64(adapter.MaxRawRows)
	if limit == nil || *limit > maxCap {
		limit = &maxCap
	}
	findOpts := options.Find()
	if limit != nil {
		findOpts.SetLimit(*limit)
	}
	if skip != nil {
		findOpts.SetSkip(*skip)
	}
	if sortSpec != nil {
		findOpts.SetSort(sortSpec)
	}
	cur, err := a.db.Collection(coll).Find(ctx, filter, findOpts)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: find: %w", err)
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
		if len(result.Rows) >= adapter.MaxRawRows {
			break
		}
	}
	if result.Columns == nil {
		result.Columns = []string{}
	}
	if result.Rows == nil {
		result.Rows = []map[string]any{}
	}
	return result, cur.Err()
}

func (a *Adapter) execInsertOne(ctx context.Context, coll string, args []string) (adapter.ResultSet, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertOne requires a document")
	}
	var doc bson.M
	if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &doc); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid document JSON: %w", err)
	}
	res, err := a.db.Collection(coll).InsertOne(ctx, doc)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertOne: %w", err)
	}
	return adapter.ResultSet{
		Columns: []string{"inserted_id"},
		Rows:    []map[string]any{{"inserted_id": normalize(res.InsertedID)}},
	}, nil
}

func (a *Adapter) execInsertMany(ctx context.Context, coll string, args []string) (adapter.ResultSet, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertMany requires an array of documents")
	}
	var docs []any
	if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &docs); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid documents JSON: %w", err)
	}
	if len(docs) == 0 {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertMany requires at least one document")
	}
	res, err := a.db.Collection(coll).InsertMany(ctx, docs)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertMany: %w", err)
	}
	ids := make([]any, 0, len(res.InsertedIDs))
	for _, id := range res.InsertedIDs {
		ids = append(ids, normalize(id))
	}
	return adapter.ResultSet{
		Columns: []string{"inserted_count", "inserted_ids"},
		Rows:    []map[string]any{{"inserted_count": int64(len(ids)), "inserted_ids": ids}},
	}, nil
}

func (a *Adapter) execUpdate(ctx context.Context, coll string, args []string, many bool) (adapter.ResultSet, error) {
	if len(args) < 2 {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: update requires (filter, update)")
	}
	var filter bson.M
	if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &filter); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid filter JSON: %w", err)
	}
	if filter == nil {
		filter = bson.M{}
	}
	var update bson.M
	if err := bson.UnmarshalExtJSON([]byte(args[1]), false, &update); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid update JSON: %w", err)
	}
	if !hasDollarKey(update) {
		wrapped := bson.M{}
		for k, v := range update {
			wrapped[k] = v
		}
		update = bson.M{"$set": wrapped}
	}
	c := a.db.Collection(coll)
	var (
		matched, modified int64
		err               error
	)
	if many {
		var res *mongo.UpdateResult
		res, err = c.UpdateMany(ctx, filter, update)
		if err == nil {
			matched, modified = res.MatchedCount, res.ModifiedCount
		}
	} else {
		var res *mongo.UpdateResult
		res, err = c.UpdateOne(ctx, filter, update)
		if err == nil {
			matched, modified = res.MatchedCount, res.ModifiedCount
		}
	}
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: update: %w", err)
	}
	return adapter.ResultSet{
		Columns: []string{"matched_count", "modified_count"},
		Rows:    []map[string]any{{"matched_count": matched, "modified_count": modified}},
	}, nil
}

func (a *Adapter) execDeleteRaw(ctx context.Context, coll string, args []string, many bool) (adapter.ResultSet, error) {
	filter := bson.M{}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &filter); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid filter JSON: %w", err)
		}
		if filter == nil {
			filter = bson.M{}
		}
	}
	c := a.db.Collection(coll)
	var (
		deleted int64
		err     error
	)
	if many {
		var res *mongo.DeleteResult
		res, err = c.DeleteMany(ctx, filter)
		if err == nil {
			deleted = res.DeletedCount
		}
	} else {
		var res *mongo.DeleteResult
		res, err = c.DeleteOne(ctx, filter)
		if err == nil {
			deleted = res.DeletedCount
		}
	}
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: delete: %w", err)
	}
	return adapter.ResultSet{
		Columns: []string{"deleted_count"},
		Rows:    []map[string]any{{"deleted_count": deleted}},
	}, nil
}

func (a *Adapter) execCount(ctx context.Context, coll string, args []string) (adapter.ResultSet, error) {
	filter := bson.M{}
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		if err := bson.UnmarshalExtJSON([]byte(args[0]), false, &filter); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid filter JSON: %w", err)
		}
		if filter == nil {
			filter = bson.M{}
		}
	}
	n, err := a.db.Collection(coll).CountDocuments(ctx, filter)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: count: %w", err)
	}
	return adapter.ResultSet{
		Columns: []string{"count"},
		Rows:    []map[string]any{{"count": n}},
	}, nil
}

// execRawJSON handles {"collection","op","filter","doc","docs","update",
// "limit","skip","sort"} as an alternative to shell syntax.
func (a *Adapter) execRawJSON(ctx context.Context, raw string) (adapter.ResultSet, error) {
	// Parse via ExtJSON so $oid/$date keep working, then normalize to plain.
	var ext bson.M
	if err := bson.UnmarshalExtJSON([]byte(raw), false, &ext); err != nil {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid JSON (want db.<coll>.<op>(...) or {\"collection\",\"op\",...}): %w", err)
	}
	coll, _ := ext["collection"].(string)
	op, _ := ext["op"].(string)
	if coll == "" {
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: JSON raw requires \"collection\" (e.g. {\"collection\":\"users\",\"op\":\"find\",\"filter\":{}})")
	}
	if op == "" {
		op = "find"
	}
	toJSON := func(v any) string {
		if v == nil {
			return ""
		}
		b, err := bson.MarshalExtJSON(v, false, false)
		if err != nil {
			return ""
		}
		return string(b)
	}
	filterJSON := toJSON(ext["filter"])
	docJSON := toJSON(firstNonNil(ext["doc"], ext["document"]))
	updateJSON := toJSON(ext["update"])
	var args []string
	switch op {
	case "find", "findOne":
		if filterJSON != "" {
			args = append(args, filterJSON)
		}
		// Build options doc from limit/skip/sort when present.
		opts := bson.M{}
		if lim, ok := ext["limit"]; ok {
			opts["limit"] = lim
		}
		if sk, ok := ext["skip"]; ok {
			opts["skip"] = sk
		}
		if s, ok := ext["sort"]; ok {
			opts["sort"] = s
		}
		if len(opts) > 0 {
			if b, err := bson.MarshalExtJSON(opts, false, false); err == nil {
				if filterJSON == "" {
					args = append(args, "{}")
				}
				args = append(args, string(b))
			}
		}
		return a.dispatchMongoOp(ctx, coll, op, args, "")
	case "insertOne":
		if docJSON == "" {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertOne requires \"doc\"")
		}
		return a.dispatchMongoOp(ctx, coll, op, []string{docJSON}, "")
	case "insertMany":
		docsVal, ok := ext["docs"]
		if !ok || docsVal == nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: insertMany requires \"docs\" array")
		}
		b, err := bson.MarshalExtJSON(docsVal, false, false)
		if err != nil {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: invalid docs: %w", err)
		}
		return a.dispatchMongoOp(ctx, coll, op, []string{string(b)}, "")
	case "updateOne", "updateMany":
		if filterJSON == "" {
			filterJSON = "{}"
		}
		if updateJSON == "" {
			return adapter.ResultSet{}, fmt.Errorf("ferretdb: update requires \"update\"")
		}
		return a.dispatchMongoOp(ctx, coll, op, []string{filterJSON, updateJSON}, "")
	case "deleteOne", "deleteMany":
		if filterJSON == "" {
			filterJSON = "{}"
		}
		return a.dispatchMongoOp(ctx, coll, op, []string{filterJSON}, "")
	case "countDocuments", "count":
		if filterJSON == "" {
			filterJSON = "{}"
		}
		return a.dispatchMongoOp(ctx, coll, op, []string{filterJSON}, "")
	case "drop":
		return a.dispatchMongoOp(ctx, coll, op, nil, "")
	default:
		return adapter.ResultSet{}, fmt.Errorf("ferretdb: unsupported op %q", op)
	}
}

func okResult() adapter.ResultSet {
	return adapter.ResultSet{Columns: []string{"result"}, Rows: []map[string]any{{"result": "OK"}}}
}

func hasDollarKey(m bson.M) bool {
	for k := range m {
		if strings.HasPrefix(k, "$") {
			return true
		}
	}
	return false
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int32:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	default:
		return 0, false
	}
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// stripMongoComments removes // line comments outside strings and drops
// full-line comments, so editor samples with "// ..." run as-is.
func stripMongoComments(q string) string {
	lines := strings.Split(q, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") {
			continue
		}
		// Inline // outside quotes.
		idx := mongoCommentIndex(line)
		if idx >= 0 {
			line = line[:idx]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func mongoCommentIndex(s string) int {
	var inSingle, inDouble bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
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

// parseMongoShell parses "db.<coll>.<op>(<args>)[chain]" into parts. chain is
// the trailing ".limit(..).skip(..).sort(..)" suffix for find (may be "").
func parseMongoShell(q string) (coll, op, argsStr, chain string, err error) {
	t := strings.TrimSpace(q)
	if !strings.HasPrefix(t, "db.") {
		return "", "", "", "", fmt.Errorf("ferretdb: want db.<collection>.<op>(...) (e.g. db.users.find({}))")
	}
	rest := t[len("db."):]
	// Special-case db.createCollection("name").
	if strings.HasPrefix(rest, "createCollection") {
		open := strings.Index(rest, "(")
		if open < 0 {
			return "", "", "", "", fmt.Errorf("ferretdb: invalid createCollection syntax")
		}
		end, ok := matchingParen(rest, open)
		if !ok {
			return "", "", "", "", fmt.Errorf("ferretdb: invalid createCollection syntax")
		}
		return "createCollection", "createCollection", rest[open+1 : end], "", nil
	}
	dot := strings.Index(rest, ".")
	if dot < 0 {
		return "", "", "", "", fmt.Errorf("ferretdb: want db.<collection>.<op>(...)")
	}
	coll = rest[:dot]
	rest = rest[dot+1:]
	open := strings.Index(rest, "(")
	if open < 0 {
		// Bare "db.users.drop" without parens is also accepted as drop.
		if strings.TrimSpace(rest) == "drop" {
			return coll, "drop", "", "", nil
		}
		return "", "", "", "", fmt.Errorf("ferretdb: want db.<collection>.<op>(...)")
	}
	op = strings.TrimSpace(rest[:open])
	// Match the operation's own closing paren, so a chained
	// ".limit(10).skip(2)" suffix isn't swallowed into the arguments.
	end, ok := matchingParen(rest, open)
	if !ok {
		return "", "", "", "", fmt.Errorf("ferretdb: missing closing ')'")
	}
	argsStr = rest[open+1 : end]
	chain = strings.TrimSpace(rest[end+1:])
	chain = strings.TrimSuffix(chain, ";")
	if coll == "" || op == "" {
		return "", "", "", "", fmt.Errorf("ferretdb: invalid db.<collection>.<op>(...) syntax")
	}
	return coll, op, argsStr, chain, nil
}

// matchingParen returns the index of the ')' that closes the '(' at openIdx,
// ignoring parens inside single/double quoted strings.
func matchingParen(s string, openIdx int) (int, bool) {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '(' {
		return 0, false
	}
	depth := 0
	var inSingle, inDouble bool
	for i := openIdx; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
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
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// splitTopLevelArgs splits comma-separated JSON args at depth 0, respecting
// braces/brackets and single/double quoted strings.
func splitTopLevelArgs(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var (
		out                []string
		depthBraces        int
		depthBrack         int
		inSingle, inDouble bool
		start              = 0
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inSingle {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
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
		switch c {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '{':
			depthBraces++
		case '}':
			depthBraces--
			if depthBraces < 0 {
				return nil, fmt.Errorf("ferretdb: unbalanced '}' in arguments")
			}
		case '[':
			depthBrack++
		case ']':
			depthBrack--
			if depthBrack < 0 {
				return nil, fmt.Errorf("ferretdb: unbalanced ']' in arguments")
			}
		case ',':
			if depthBraces == 0 && depthBrack == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("ferretdb: unterminated string in arguments")
	}
	if depthBraces != 0 || depthBrack != 0 {
		return nil, fmt.Errorf("ferretdb: unbalanced braces/brackets in arguments")
	}
	out = append(out, s[start:])
	return out, nil
}

func parseChainInt(chain, name string) (int64, bool) {
	// Looks for .name(<digits>) in the chain suffix.
	needle := "." + name + "("
	idx := strings.Index(chain, needle)
	if idx < 0 {
		return 0, false
	}
	rest := chain[idx+len(needle):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return 0, false
	}
	var n int64
	if _, err := fmt.Sscanf(strings.TrimSpace(rest[:end]), "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

func parseChainDoc(chain, name string) string {
	needle := "." + name + "("
	idx := strings.Index(chain, needle)
	if idx < 0 {
		return ""
	}
	rest := chain[idx+len(needle):]
	// Find matching close paren accounting for braces.
	depth := 1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return rest[:i]
			}
		case '"', '\'':
			quote := rest[i]
			i++
			for i < len(rest) && rest[i] != quote {
				if rest[i] == '\\' {
					i++
				}
				i++
			}
		}
	}
	return ""
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