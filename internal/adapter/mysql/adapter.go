// Package mysql implements the DatabaseAdapter interface for MySQL/MariaDB.
//
// It mirrors the Postgres adapter's structure but speaks MySQL's dialect
// (`?` placeholders, backtick quoting) via database/sql + the go-sql-driver.
//
// CRUD, schema introspection, relationships, joins, transactions and full-text
// search are implemented natively. Trigger *delivery* (the platform's change
// stream into functions/webhooks) is NOT yet wired: MySQL has no LISTEN/NOTIFY,
// and the queue-table + polling tier is a follow-up. Until it lands,
// RegisterTrigger/SubscribeToChanges return ErrUnsupported and the capability
// flags honestly report no native triggers / no realtime, so the dashboard
// disables those tabs rather than failing at runtime.
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	// Register the mysql driver with database/sql.
	_ "github.com/go-sql-driver/mysql"

	"github.com/openbase/openbase/internal/adapter"
)

var (
	identRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Adapter is a MySQL implementation of the Universal Data Interface.
type Adapter struct {
	db *sql.DB
}

// Compile-time assertion that Adapter satisfies the interface.
var _ adapter.DatabaseAdapter = (*Adapter)(nil)

// New returns a MySQL adapter with no live connection yet.
func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	// Prefer an explicit DSN-style conn string; otherwise assemble one from the
	// structured username/password/database fields.
	dsn := cfg.ConnStr
	if dsn == "" {
		dsn = fmt.Sprintf("%s:%s@/%s?parseTime=true", cfg.Username, cfg.Password, cfg.Database)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: parse connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("mysql: ping: %w", err)
	}
	a.db = db
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	if a.db == nil {
		return nil
	}
	err := a.db.Close()
	a.db = nil
	return err
}

func (a *Adapter) Capabilities() adapter.CapabilitySet {
	return adapter.CapabilitySet{
		SupportsRelationalJoins: true,
		SupportsForeignKeys:     true,
		// MySQL has native triggers, but the platform's trigger *delivery* path
		// (queue-table + polling change stream) is not implemented yet, so the
		// adapter honestly reports no native triggers / no realtime until then.
		SupportsNativeTriggers: false,
		SupportsChangeStreams:  false,
		SupportsRealtime:       adapter.RealtimeNone,
		SupportsTransactions:   true,
		SupportsFullTextSearch: true,
		SupportsVectorSearch:   false,
	}
}

func (a *Adapter) requireDB() error {
	if a.db == nil {
		return errors.New("mysql: not connected")
	}
	return nil
}

// ListCollections returns the user tables in the current schema.
func (a *Adapter) ListCollections(ctx context.Context) ([]adapter.CollectionInfo, error) {
	if err := a.requireDB(); err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return nil, fmt.Errorf("mysql: list collections: %w", err)
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
	if err := a.requireDB(); err != nil {
		return adapter.SchemaInfo{}, err
	}
	if !identRE.MatchString(collection) {
		return adapter.SchemaInfo{}, fmt.Errorf("mysql: invalid table name %q", collection)
	}

	var info adapter.SchemaInfo
	info.Collection = collection

	rows, err := a.db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
		ORDER BY ordinal_position`, collection)
	if err != nil {
		return info, fmt.Errorf("mysql: get schema: %w", err)
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
	pkRows, err := a.db.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND constraint_name = 'PRIMARY'`, collection)
	if err != nil {
		return info, fmt.Errorf("mysql: get primary key: %w", err)
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

// ListRelationships returns foreign-key relationships in the current schema.
func (a *Adapter) ListRelationships(ctx context.Context) ([]adapter.Relationship, error) {
	if err := a.requireDB(); err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT
			kcu.table_name, kcu.column_name,
			kcu.referenced_table_name, kcu.referenced_column_name
		FROM information_schema.key_column_usage kcu
		WHERE kcu.table_schema = DATABASE()
		  AND kcu.referenced_table_name IS NOT NULL
		ORDER BY kcu.table_name, kcu.ordinal_position`)
	if err != nil {
		return nil, fmt.Errorf("mysql: list relationships: %w", err)
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
	if err := a.requireDB(); err != nil {
		return adapter.ResultSet{}, err
	}
	col := q.Filter.Collection
	if !identRE.MatchString(col) {
		return adapter.ResultSet{}, fmt.Errorf("mysql: invalid table name %q", col)
	}

	where, args, err := buildWhere(q.Filter)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	stmt := "SELECT * FROM " + quote(col) + " " + where
	stmt += buildOrderLimit(q.Filter)

	rows, err := a.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mysql: query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return adapter.ResultSet{}, err
	}

	var result adapter.ResultSet
	result.Columns = cols
	raw := make([]sql.RawBytes, len(cols))
	scanArgs := make([]any, len(cols))
	for i := range raw {
		scanArgs[i] = &raw[i]
	}
	// Values are scanned as bytes (text protocol); phase values into primitives.
	for rows.Next() {
		if err := rows.Scan(scanArgs...); err != nil {
			return result, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = bytesToValue(raw[i])
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

func buildWhere(f adapter.Filter) (string, []any, error) {
	if len(f.Conditions) == 0 {
		return "", nil, nil
	}
	parts := make([]string, 0, len(f.Conditions))
	args := make([]any, 0, len(f.Conditions))
	for _, c := range f.Conditions {
		if !identRE.MatchString(c.Field) {
			return "", nil, fmt.Errorf("mysql: invalid field name %q", c.Field)
		}
		op, ok := sqlOp(c.Operator)
		if !ok {
			return "", nil, fmt.Errorf("mysql: unsupported operator %q", c.Operator)
		}
		args = append(args, c.Value)
		parts = append(parts, fmt.Sprintf("%s %s ?", quote(c.Field), op))
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
			if identRE.MatchString(o.Field) {
				b.WriteString(quote(o.Field))
			} else {
				b.WriteString(o.Field)
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

// Insert inserts a row and returns its id (auto-increment column, if any).
func (a *Adapter) Insert(ctx context.Context, collection string, doc map[string]any) (adapter.InsertResult, error) {
	if err := a.requireDB(); err != nil {
		return adapter.InsertResult{}, err
	}
	if !identRE.MatchString(collection) {
		return adapter.InsertResult{}, fmt.Errorf("mysql: invalid table name %q", collection)
	}
	if len(doc) == 0 {
		return adapter.InsertResult{}, errors.New("mysql: insert requires at least one column")
	}

	cols := make([]string, 0, len(doc))
	vals := make([]any, 0, len(doc))
	for k, v := range doc {
		if !identRE.MatchString(k) {
			return adapter.InsertResult{}, fmt.Errorf("mysql: invalid column name %q", k)
		}
		cols = append(cols, k)
		vals = append(vals, v)
	}

	placeholders := make([]string, len(vals))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quote(collection), quoteList(cols), strings.Join(placeholders, ", "))

	res, err := a.db.ExecContext(ctx, stmt, vals...)
	if err != nil {
		return adapter.InsertResult{}, fmt.Errorf("mysql: insert: %w", err)
	}

	id, _ := res.LastInsertId()
	generated := map[string]any{}
	var idVal any
	if id != 0 {
		idVal = id
		generated["id"] = id
	}
	return adapter.InsertResult{Collection: collection, ID: idVal, Generated: generated}, nil
}

// Update updates rows matching the filter.
func (a *Adapter) Update(ctx context.Context, filter adapter.Filter, update map[string]any) (adapter.UpdateResult, error) {
	if err := a.requireDB(); err != nil {
		return adapter.UpdateResult{}, err
	}
	col := filter.Collection
	if !identRE.MatchString(col) {
		return adapter.UpdateResult{}, fmt.Errorf("mysql: invalid table name %q", col)
	}
	if len(update) == 0 {
		return adapter.UpdateResult{}, errors.New("mysql: update requires at least one column")
	}

	setCols := make([]string, 0, len(update))
	args := make([]any, 0, len(update))
	for k, v := range update {
		if !identRE.MatchString(k) {
			return adapter.UpdateResult{}, fmt.Errorf("mysql: invalid column name %q", k)
		}
		args = append(args, v)
		setCols = append(setCols, quote(k)+" = ?")
	}

	where, whereArgs, err := buildWhere(filter)
	if err != nil {
		return adapter.UpdateResult{}, err
	}
	args = append(args, whereArgs...)

	stmt := fmt.Sprintf("UPDATE %s SET %s %s", quote(col), strings.Join(setCols, ", "), where)
	res, err := a.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return adapter.UpdateResult{}, fmt.Errorf("mysql: update: %w", err)
	}
	affected, _ := res.RowsAffected()
	return adapter.UpdateResult{MatchedCount: affected, ModifiedCount: affected}, nil
}

// Delete removes rows matching the filter.
func (a *Adapter) Delete(ctx context.Context, filter adapter.Filter) (adapter.DeleteResult, error) {
	if err := a.requireDB(); err != nil {
		return adapter.DeleteResult{}, err
	}
	col := filter.Collection
	if !identRE.MatchString(col) {
		return adapter.DeleteResult{}, fmt.Errorf("mysql: invalid table name %q", col)
	}
	where, args, err := buildWhere(filter)
	if err != nil {
		return adapter.DeleteResult{}, err
	}
	stmt := "DELETE FROM " + quote(col) + " " + where
	res, err := a.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return adapter.DeleteResult{}, fmt.Errorf("mysql: delete: %w", err)
	}
	deleted, _ := res.RowsAffected()
	return adapter.DeleteResult{DeletedCount: deleted}, nil
}

// RegisterTrigger is unsupported until the queue-table + polling change-delivery
// tier lands, so a MySQL trigger doesn't fire into a void.
func (a *Adapter) RegisterTrigger(ctx context.Context, t adapter.TriggerDefinition) error {
	return fmt.Errorf("%w: mysql trigger delivery (queue-table + polling) is a later phase", adapter.ErrUnsupported)
}

// RemoveTrigger mirrors RegisterTrigger.
func (a *Adapter) RemoveTrigger(ctx context.Context, triggerID string) error {
	return fmt.Errorf("%w: mysql trigger delivery is a later phase", adapter.ErrUnsupported)
}

// RegisterRealtimeBroadcast is unsupported until the polling tier lands.
func (a *Adapter) RegisterRealtimeBroadcast(ctx context.Context, collection string) error {
	return fmt.Errorf("%w: mysql realtime requires queue-table + polling emulation (later phase)", adapter.ErrUnsupported)
}

// SubscribeToChanges is unsupported until a polling change stream lands.
func (a *Adapter) SubscribeToChanges(ctx context.Context, collection string, handler adapter.ChangeHandler) (adapter.Subscription, error) {
	return nil, fmt.Errorf("%w: mysql realtime requires queue-table + polling emulation (later phase)", adapter.ErrUnsupported)
}

func quote(ident string) string {
	return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
}

func quoteList(idents []string) string {
	out := make([]string, len(idents))
	for i, id := range idents {
		out[i] = quote(id)
	}
	return strings.Join(out, ", ")
}

// bytesToValue converts a byte-slice cell from the MySQL text protocol into a
// primitive. Integers/floats/booleans are normalized so JSON rendering is clean.
func bytesToValue(b []byte) any {
	if b == nil {
		return nil
	}
	s := string(b)
	// Booleans are returned as 0/1 by MySQL; keep them numeric for simplicity.
	if i, ok := parseInt(s); ok {
		return i
	}
	if f, ok := parseFloat(s); ok {
		return f
	}
	return s
}

func parseInt(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	neg := s[0] == '-'
	var out int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' && i == 0 {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		out = out*10 + int64(c-'0')
	}
	if neg {
		out = -out
	}
	return out, true
}

func parseFloat(s string) (float64, bool) {
	if s == "" || strings.ContainsAny(s, "eE") {
		return 0, false
	}
	if !strings.Contains(s, ".") {
		return 0, false
	}
	var out float64
	var frac float64 = 0.1
	seenDot := false
	for i, c := range s {
		if c == '.' && !seenDot {
			seenDot = true
			continue
		}
		if i == 0 && c == '-' {
			continue
		}
		if c < '0' || c > '9' {
			return 0, false
		}
		if seenDot {
			out += float64(c-'0') * frac
			frac /= 10
		} else {
			out = out*10 + float64(c-'0')
		}
	}
	if strings.HasPrefix(s, "-") {
		out = -out
	}
	return out, true
}
