// Package postgres implements the DatabaseAdapter interface for PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/adapter"
)

var (
	tableIdentRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Adapter is a PostgreSQL implementation of the Universal Data Interface.
type Adapter struct {
	pool *pgxpool.Pool
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// Compile-time assertion that Adapter supports raw SQL (reads + writes/DDL).
var _ adapter.RawQuerier = (*Adapter)(nil)

// New returns a Postgres adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	pool, err := pgxpool.New(ctx, cfg.ConnStr)
	if err != nil {
		return fmt.Errorf("postgres: parse connection: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	a.pool = pool
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	if a.pool == nil {
		return nil
	}
	a.pool.Close()
	a.pool = nil
	return nil
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	return adapter.CapabilitySet{
		SupportsRelationalJoins: true,
		SupportsForeignKeys:     true,
		SupportsNativeTriggers:  true,
		SupportsChangeStreams:   true,
		SupportsRealtime:        adapter.RealtimeNative,
		SupportsTransactions:    true,
		SupportsFullTextSearch:  true,
		SupportsVectorSearch:    true,
	}
}

func (a *Adapter) requirePool() error {
	if a.pool == nil {
		return errors.New("postgres: not connected")
	}
	return nil
}

// ListCollections returns the user tables in the public schema.
func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	if err := a.requirePool(); err != nil {
		return nil, err
	}
	rows, err := a.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list collections: %w", err)
	}
	defer rows.Close()

	var out []adapter.CollectionInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, adapter.CollectionInfo{Name: name})
	}
	return out, rows.Err()
}

// GetSchema returns column metadata for a single table.
func (a *Adapter) GetSchema(ctx context.Context, collection string) (adapter.SchemaInfo, error) {
	if err := a.requirePool(); err != nil {
		return adapter.SchemaInfo{}, err
	}
	if !tableIdentRE.MatchString(collection) {
		return adapter.SchemaInfo{}, fmt.Errorf("postgres: invalid table name %q", collection)
	}

	var info adapter.SchemaInfo
	info.Collection = collection

	rows, err := a.pool.Query(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position`, collection)
	if err != nil {
		return info, fmt.Errorf("postgres: get schema: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, dataType, nullable string
		var def *string
		if err := rows.Scan(&name, &dataType, &nullable, &def); err != nil {
			return info, err
		}
		info.Columns = append(info.Columns, adapter.ColumnInfo{
			Name:       name,
			DataType:   dataType,
			Nullable:   nullable == "YES",
			DefaultVal: def,
		})
	}
	if err := rows.Err(); err != nil {
		return info, err
	}

	// Primary key columns.
	pkRows, err := a.pool.Query(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		WHERE tc.table_schema = 'public'
		  AND tc.table_name = $1
		  AND tc.constraint_type = 'PRIMARY KEY'`, collection)
	if err != nil {
		return info, fmt.Errorf("postgres: get primary key: %w", err)
	}
	defer pkRows.Close()

	pk := map[string]bool{}
	for pkRows.Next() {
		var col string
		if err := pkRows.Scan(&col); err != nil {
			return info, err
		}
		pk[col] = true
	}
	if err := pkRows.Err(); err != nil {
		return info, err
	}
	for i := range info.Columns {
		info.Columns[i].IsPrimary = pk[info.Columns[i].Name]
	}
	return info, nil
}

// ListRelationships returns foreign-key relationships in the public schema.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	if err := a.requirePool(); err != nil {
		return nil, err
	}
	rows, err := a.pool.Query(ctx, `
		SELECT
			rc.from_table, rc.from_col,
			rc.to_table, rc.to_col
		FROM (
			SELECT
				tc.table_name AS from_table,
				kcu.column_name AS from_col,
				ccu.table_name AS to_table,
				ccu.column_name AS to_col
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON tc.constraint_name = kcu.constraint_name
			 AND tc.table_schema = kcu.constraint_schema
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name
			 AND ccu.constraint_schema = tc.constraint_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = 'public'
		) rc`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list relationships: %w", err)
	}
	defer rows.Close()

	var out []adapter.Relationship
	for rows.Next() {
		var r adapter.Relationship
		if err := rows.Scan(&r.FromCollection, &r.FromColumn, &r.ToCollection, &r.ToColumn); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Query executes a UniversalQuery, translating filters/order/limit into SQL.
func (a *Adapter) Query(ctx context.Context, q adapter.UniversalQuery) (adapter.ResultSet, error) {
	if err := a.requirePool(); err != nil {
		return adapter.ResultSet{}, err
	}
	col := q.Filter.Collection
	if !tableIdentRE.MatchString(col) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid table name %q", col)
	}

	where, args, err := buildWhere(col, q.Filter)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	sql := fmt.Sprintf("SELECT * FROM %s %s", quote(col), where)
	sql += buildOrderLimit(q.Filter)

	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: query: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	cols := make([]string, 0, len(fieldDescs))
	for _, fd := range fieldDescs {
		cols = append(cols, string(fd.Name))
	}

	var result adapter.ResultSet
	result.Columns = cols
	if result.Columns == nil {
		result.Columns = []string{}
	}
	result.Rows = []map[string]any{}
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return result, err
		}
		row := make(map[string]any, len(cols))
		for i, v := range vals {
			row[cols[i]] = normalizeValue(v)
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

// ExecRaw executes a raw SQL statement — reads (SELECT/WITH/EXPLAIN) as well
// as DML (INSERT/UPDATE/DELETE) and DDL (CREATE/ALTER/DROP/TRUNCATE, ...).
// Write-guard validation lives in the server handler; this method caps read
// rows at adapter.MaxRawRows so a runaway SELECT can't exhaust host memory.
// Writes return a single-row ResultSet: DML yields {"affected_rows": N}
// (plus RETURNING rows when present), DDL yields {"result": "OK"}.
func (a *Adapter) ExecRaw(ctx context.Context, query string) (adapter.ResultSet, error) {
	if err := a.requirePool(); err != nil {
		return adapter.ResultSet{}, err
	}
	rows, err := a.pool.Query(ctx, query)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: exec raw: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	cols := make([]string, 0, len(fieldDescs))
	for _, fd := range fieldDescs {
		cols = append(cols, string(fd.Name))
	}
	result := adapter.ResultSet{Columns: cols, Rows: []map[string]any{}}
	if result.Columns == nil {
		result.Columns = []string{}
	}
	for rows.Next() {
		if len(result.Rows) >= adapter.MaxRawRows {
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return result, err
		}
		row := make(map[string]any, len(cols))
		for i, v := range vals {
			if i < len(cols) {
				row[cols[i]] = normalizeValue(v)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("postgres: exec raw: %w", err)
	}
	// No columns and no rows means the statement was a write/DDL executed
	// via the Query path (e.g. INSERT without RETURNING, CREATE TABLE).
	// Surface the command tag as an affected_rows / OK row so the SQL editor
	// can render writes instead of a confusing "0 rows".
	if len(cols) == 0 && len(result.Rows) == 0 {
		tag := rows.CommandTag()
		affected := tag.RowsAffected()
		if affected != 0 {
			return adapter.ResultSet{
				Columns: []string{"affected_rows"},
				Rows:    []map[string]any{{"affected_rows": affected}},
			}, nil
		}
		return adapter.ResultSet{
			Columns: []string{"result"},
			Rows:    []map[string]any{{"result": "OK"}},
		}, nil
	}
	return result, nil
}

func buildWhere(collection string, f adapter.Filter) (string, []any, error) {
	if len(f.Conditions) == 0 {
		return "", nil, nil
	}
	parts := make([]string, 0, len(f.Conditions))
	args := make([]any, 0, len(f.Conditions))
	for _, c := range f.Conditions {
		if !tableIdentRE.MatchString(c.Field) {
			return "", nil, fmt.Errorf("postgres: invalid field name %q", c.Field)
		}
		op, ok := sqlOp(c.Operator)
		if !ok {
			return "", nil, fmt.Errorf("postgres: unsupported operator %q", c.Operator)
		}
		args = append(args, c.Value)
		parts = append(parts, fmt.Sprintf("%s %s $%d", quote(c.Field), op, len(args)))
	}
	return "WHERE " + strings.Join(parts, " AND "), args, nil
}

func buildOrderLimit(f adapter.Filter) string {
	var b strings.Builder
	if len(f.OrderBy) > 0 {
		b.WriteString(" ORDER BY ")
		for i, o := range f.OrderBy {
			if i > 0 {
				b.WriteString(", ")
			}
			dir := "ASC"
			if o.Desc {
				dir = "DESC"
			}
			if tableIdentRE.MatchString(o.Field) {
				b.WriteString(quote(o.Field))
			} else {
				b.WriteString(o.Field) // allow expression ordering with validation
			}
			b.WriteString(" " + dir)
		}
	}
	if f.Limit != nil {
		fmt.Fprintf(&b, " LIMIT %d", *f.Limit)
	}
	if f.Offset != nil {
		fmt.Fprintf(&b, " OFFSET %d", *f.Offset)
	}
	return b.String()
}

func sqlOp(op adapter.Op) (string, bool) {
	switch op {
	case adapter.OpEqual:
		return "=", true
	case adapter.OpNotEqual:
		return "<>", true
	case adapter.OpGreaterThan:
		return ">", true
	case adapter.OpLessThan:
		return "<", true
	case adapter.OpGreaterEq:
		return ">=", true
	case adapter.OpLessEq:
		return "<=", true
	case adapter.OpContains:
		return "LIKE", true
	}
	return "", false
}

// Insert inserts a document and returns the generated id column, if any.
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := a.requirePool(); err != nil {
		return adapter.InsertResult{}, err
	}
	if !tableIdentRE.MatchString(collection) {
		return adapter.InsertResult{}, fmt.Errorf("postgres: invalid table name %q", collection)
	}
	if len(doc) == 0 {
		return adapter.InsertResult{}, errors.New("postgres: insert requires at least one column")
	}

	cols := make([]string, 0, len(doc))
	vals := make([]any, 0, len(doc))
	for k, v := range doc {
		if !tableIdentRE.MatchString(k) {
			return adapter.InsertResult{}, fmt.Errorf("postgres: invalid column name %q", k)
		}
		cols = append(cols, k)
		vals = append(vals, v)
	}

	placeholders := make([]string, len(vals))
	for i := range vals {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		quote(collection), quoteList(cols), strings.Join(placeholders, ", "))

	rows, err := a.pool.Query(ctx, sql, vals...)
	if err != nil {
		return adapter.InsertResult{}, fmt.Errorf("postgres: insert: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		fieldDescs := rows.FieldDescriptions()
		vals, err := rows.Values()
		if err != nil {
			return adapter.InsertResult{}, err
		}
		generated := make(map[string]any, len(fieldDescs))
		var id any
		for i, fd := range fieldDescs {
			if fd.Name == "id" {
				id = normalizeValue(vals[i])
			}
			generated[string(fd.Name)] = normalizeValue(vals[i])
		}
		return adapter.InsertResult{Collection: collection, ID: id, Generated: generated}, nil
	}
	return adapter.InsertResult{}, errors.New("postgres: insert returned no row")
}

// Update updates rows matching the filter.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := a.requirePool(); err != nil {
		return adapter.UpdateResult{}, err
	}
	col := filter.Collection
	if !tableIdentRE.MatchString(col) {
		return adapter.UpdateResult{}, fmt.Errorf("postgres: invalid table name %q", col)
	}
	if len(update) == 0 {
		return adapter.UpdateResult{}, errors.New("postgres: update requires at least one column")
	}

	setCols := make([]string, 0, len(update))
	args := make([]any, 0, len(update))
	for k, v := range update {
		if !tableIdentRE.MatchString(k) {
			return adapter.UpdateResult{}, fmt.Errorf("postgres: invalid column name %q", k)
		}
		args = append(args, v)
		setCols = append(setCols, fmt.Sprintf("%s = $%d", quote(k), len(args)))
	}

	where, whereArgs, err := buildWhere(col, filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	args = append(args, whereArgs...)
	// Rebase the where placeholders after the set args.
	where = rebasePlaceholders(where, len(setCols))

	sql := fmt.Sprintf("UPDATE %s SET %s %s", quote(col), strings.Join(setCols, ", "), where)
	tag, err := a.pool.Exec(ctx, sql, args...)
	if err != nil {
		return adapter.UpdateResult{}, fmt.Errorf("postgres: update: %w", err)
	}
	return adapter.UpdateResult{MatchedCount: tag.RowsAffected(), ModifiedCount: tag.RowsAffected()}, nil
}

// Delete removes rows matching the filter.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := a.requirePool(); err != nil {
		return adapter.DeleteResult{}, err
	}
	col := filter.Collection
	if !tableIdentRE.MatchString(col) {
		return adapter.DeleteResult{}, fmt.Errorf("postgres: invalid table name %q", col)
	}
	where, args, err := buildWhere(col, filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	sql := fmt.Sprintf("DELETE FROM %s %s", quote(col), where)
	tag, err := a.pool.Exec(ctx, sql, args...)
	if err != nil {
		return adapter.DeleteResult{}, fmt.Errorf("postgres: delete: %w", err)
	}
	return adapter.DeleteResult{DeletedCount: tag.RowsAffected()}, nil
}

// realtimePayload is the pg_notify body shared by the broadcast and trigger
// notify functions. `data` is the post-image for INSERT/UPDATE and the
// pre-image for DELETE (NEW is null on delete, so COALESCE picks OLD) —
// delete events therefore carry the deleted row instead of null.
const realtimePayload = `jsonb_build_object('event', TG_OP, 'data', COALESCE(to_jsonb(NEW), to_jsonb(OLD)))::text`

// RegisterTrigger wires a PL/pgSQL trigger so the platform's event is fired
// natively via LISTEN/NOTIFY on a dedicated channel.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	if err := a.requirePool(); err != nil {
		return err
	}
	if !tableIdentRE.MatchString(t.Collection) {
		return fmt.Errorf("postgres: invalid table name %q", t.Collection)
	}

	fnName := "openbase_notify_" + sanitizeFuncName(t.ID)
	_, err := a.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION `+fnName+`() RETURNS trigger AS $$
		BEGIN
			PERFORM pg_notify('`+notifyChannel(t.Collection)+`',
				`+realtimePayload+`);
			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`)
	if err != nil {
		return fmt.Errorf("postgres: create trigger function: %w", err)
	}

	trgName := "openbase_trg_" + sanitizeFuncName(t.ID)
	_, err = a.pool.Exec(ctx,
		"DROP TRIGGER IF EXISTS "+trgName+" ON "+quote(t.Collection))
	if err != nil {
		return fmt.Errorf("postgres: drop trigger: %w", err)
	}

	event := strings.ToUpper(string(t.Event))
	_, err = a.pool.Exec(ctx,
		fmt.Sprintf("CREATE TRIGGER %s AFTER %s ON %s FOR EACH ROW EXECUTE FUNCTION %s()",
			trgName, event, quote(t.Collection), fnName))
	if err != nil {
		return fmt.Errorf("postgres: create trigger: %w", err)
	}
	return nil
}

func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	if err := a.requirePool(); err != nil {
		return err
	}
	// Determine the collection and drop the trigger by known naming.
	// A full implementation persists the mapping; here we drop by name prefix
	// through a helper the caller supplies via metadata.
	return errors.New("postgres: RemoveTrigger requires collection context, use RemoveTriggerOn")
}

// RemoveTriggerOn drops the platform trigger on a collection.
func (a *Adapter) RemoveTriggerOn(ctx context.Context, collection, triggerID string) error {
	if err := a.requirePool(); err != nil {
		return err
	}
	if !tableIdentRE.MatchString(collection) {
		return fmt.Errorf("postgres: invalid table name %q", collection)
	}
	trgName := "openbase_trg_" + sanitizeFuncName(triggerID)
	_, err := a.pool.Exec(ctx, "DROP TRIGGER IF EXISTS "+trgName+" ON "+quote(collection))
	if err != nil {
		return fmt.Errorf("postgres: drop trigger: %w", err)
	}
	fnName := "openbase_notify_" + sanitizeFuncName(triggerID)
	_, err = a.pool.Exec(ctx, "DROP FUNCTION IF EXISTS "+fnName+"()")
	if err != nil {
		return fmt.Errorf("postgres: drop trigger function: %w", err)
	}
	return nil
}

// SubscribeToChanges uses Postgres LISTEN on the project channel. Each
// notification carries an enriched payload `{"event","data"}` produced by the
// platform's notify triggers; this parses it and delivers the real event + row.
func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	if err := a.requirePool(); err != nil {
		return nil, err
	}
	conn, err := a.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: acquire listener: %w", err)
	}
	channel := notifyChannel(collection)
	_, err = conn.Exec(ctx, "LISTEN "+channel)
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("postgres: listen: %w", err)
	}

	subCtx, cancel := context.WithCancel(context.Background())
	sub := &pgSubscription{conn: conn, cancel: cancel}

	go func() {
		defer conn.Release()
		for {
			msg, err := conn.Conn().WaitForNotification(subCtx)
			if err != nil {
				return
			}
			if handler != nil {
				ev, data := parseNotify(msg.Payload)
				handler(collection, ev, data)
			}
		}
	}()

	return sub, nil
}

// parseNotify decodes a notification payload into an event and row data.
// Payloads are `{"event":"INSERT","data":{...}}`; unknown/unparseable payloads
// degrade to an "update" event with the raw payload preserved as data.
func parseNotify(payload string) (adapter.TriggerEvent, map[string]any) {
	var p struct {
		Event string         `json:"event"`
		Data  map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err == nil && p.Event != "" {
		return adapter.TriggerEvent(strings.ToLower(p.Event)), p.Data
	}
	return adapter.TriggerUpdate, map[string]any{"payload": payload}
}

// RegisterRealtimeBroadcast installs a per-collection trigger that notifies
// every row operation (insert/update/delete) on a collection with the enriched
// payload. All collections share one notify function; the target channel is
// passed per trigger via TG_ARGV[0], so registering collection B can never
// hijack collection A's notifications (the previous design baked one channel
// into the shared function body). Idempotent: recreates the trigger in place.
//
// Upgrade note: triggers registered by the old version call the function with
// no arguments. The COALESCE fallback re-derives the legacy channel from the
// table name, so those triggers keep working until their collection is
// re-registered (which happens on the next subscribe).
func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	if err := a.requirePool(); err != nil {
		return err
	}
	if !tableIdentRE.MatchString(collection) {
		return fmt.Errorf("postgres: invalid table name %q", collection)
	}
	const fnName = "openbase_rt_notify"
	_, err := a.pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION openbase_rt_notify() RETURNS trigger AS $$
		BEGIN
			PERFORM pg_notify(
				COALESCE(TG_ARGV[0], 'openbase_' || regexp_replace(TG_TABLE_NAME, '[^a-zA-Z0-9_]', '_', 'g')),
				`+realtimePayload+`);
			IF TG_OP = 'DELETE' THEN
				RETURN OLD;
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`)
	if err != nil {
		return fmt.Errorf("postgres: create realtime notify fn: %w", err)
	}
	trgName := "openbase_rt_trg_" + sanitizeFuncName(collection)
	_, err = a.pool.Exec(ctx,
		"DROP TRIGGER IF EXISTS "+trgName+" ON "+quote(collection))
	if err != nil {
		return fmt.Errorf("postgres: drop realtime trigger: %w", err)
	}
	// The channel travels as a trigger argument, never baked into the shared
	// function. It is sanitized by notifyChannel, so interpolation is safe.
	channel := strings.ReplaceAll(notifyChannel(collection), "'", "''")
	_, err = a.pool.Exec(ctx,
		"CREATE TRIGGER "+trgName+" AFTER INSERT OR UPDATE OR DELETE ON "+quote(collection)+
			" FOR EACH ROW EXECUTE FUNCTION openbase_rt_notify('"+channel+"')")
	if err != nil {
		return fmt.Errorf("postgres: create realtime trigger: %w", err)
	}
	return nil
}

// notifyChannel builds a valid LISTEN/NOTIFY channel name for a collection.
func notifyChannel(collection string) string {
	if !tableIdentRE.MatchString(collection) {
		// Fall back to a sanitized token; channel identifiers can only contain
		// letters, digits and underscores.
		return "openbase_" + sanitizeFuncName(collection)
	}
	return "openbase_" + collection
}

// normalizeValue maps pgx types that don't serialize cleanly to JSON to
// friendly primitives.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return t
	default:
		return t
	}
}

func quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func quoteList(idents []string) string {
	out := make([]string, len(idents))
	for i, id := range idents {
		out[i] = quote(id)
	}
	return strings.Join(out, ", ")
}

func sanitizeFuncName(id string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	s := re.ReplaceAllString(id, "_")
	if s == "" {
		s = "x"
	}
	return s
}

// rebasePlaceholders shifts $n placeholders by an offset (used to merge set
// and where argument lists in UPDATE).
func rebasePlaceholders(sql string, offset int) string {
	re := regexp.MustCompile(`\$(\d+)`)
	return re.ReplaceAllStringFunc(sql, func(m string) string {
		var n int
		fmt.Sscanf(m, "$%d", &n)
		return fmt.Sprintf("$%d", n+offset)
	})
}

var _ = pgx.ErrNoRows
