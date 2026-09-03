// Package adapter defines the Universal Data Interface that every supported
// database engine implements. Feature modules (auth, API generation, triggers,
// realtime, visualization) MUST only ever talk to this interface and never
// directly to a specific database driver. See ADAPTERS.md for the full spec.
package adapter

import (
	"context"
	"errors"
)

// ErrUnsupported is returned by adapters when a feature is not supported by
// the underlying engine. It must never be a silent no-op: implementers stub
// unsupported methods to return this error wrapped with context.
var ErrUnsupported = errors.New("adapter: operation not supported by this engine")

// CapabilitySet declares what a database engine can do. The dashboard and API
// layer query this before rendering a feature so the UI can honestly reflect
// the engine's limits rather than showing broken/empty views.
type CapabilitySet struct {
	SupportsRelationalJoins bool        `json:"supports_relational_joins"`
	SupportsForeignKeys     bool        `json:"supports_foreign_keys"`
	SupportsNativeTriggers  bool        `json:"supports_native_triggers"`
	SupportsChangeStreams   bool        `json:"supports_change_streams"`
	SupportsRealtime        RealtimeMode `json:"supports_realtime"`
	SupportsTransactions    bool        `json:"supports_transactions"`
	SupportsFullTextSearch  bool        `json:"supports_full_text_search"`
	SupportsVectorSearch    bool        `json:"supports_vector_search"`
}

// RealtimeMode describes how (if at all) an engine delivers live changes.
type RealtimeMode string

const (
	RealtimeNative  RealtimeMode = "native"
	RealtimePolling RealtimeMode = "polling"
	RealtimeNone    RealtimeMode = "none"
)

// Engine identifies a supported database engine. Values mirror the `engine`
// CHECK constraint in SCHEMA.md §1 `connections` table.
type Engine string

const (
	EnginePostgres Engine = "postgres"
	EngineMySQL    Engine = "mysql"
	EngineFerretDB Engine = "ferretdb"
	EngineValkey   Engine = "valkey"
	EngineArcadeDB Engine = "arcadedb"
	EngineQdrant   Engine = "qdrant"
	EngineChroma   Engine = "chroma"
	EngineOther    Engine = "other"
)

// IndexKind mirrors the SQL-level concept of a primary/unique/index key.
type IndexKind string

const (
	IndexPrimary IndexKind = "primary"
	IndexUnique  IndexKind = "unique"
	IndexNormal  IndexKind = "index"
)

// CollectionInfo describes a collection (SQL table or document collection).
type CollectionInfo struct {
	Name    string       `json:"name"`
	Columns []ColumnInfo `json:"columns,omitempty"`
}

// ColumnInfo describes a single column/field in a collection.
type ColumnInfo struct {
	Name       string  `json:"name"`
	DataType   string  `json:"data_type"`
	Nullable   bool    `json:"nullable"`
	IsPrimary  bool    `json:"is_primary"`
	IsUnique   bool    `json:"is_unique"`
	DefaultVal *string `json:"default_val,omitempty"`
}

// Relationship describes a foreign-key relationship between collections.
type Relationship struct {
	FromCollection string `json:"from_collection"`
	FromColumn     string `json:"from_column"`
	ToCollection   string `json:"to_collection"`
	ToColumn       string `json:"to_column"`
}

// Classification of a native data type for cross-engine rendering.
type SchemaInfo struct {
	Collection string      `json:"collection"`
	Columns    []ColumnInfo `json:"columns"`
	Indexes    []IndexInfo  `json:"indexes"`
}

type IndexInfo struct {
	Name    string    `json:"name"`
	Kind    IndexKind `json:"kind"`
	Columns []string  `json:"columns"`
}

// Filter is the platform's intermediate representation of a selection
// predicate. Currently a minimal equality-filter; each adapter translates it
// into native syntax.
type Filter struct {
	Collection string
	Conditions []Condition
	OrderBy    []OrderBy
	Limit      *int
	Offset     *int
}

// Condition is a single equality predicate on one field.
type Condition struct {
	Field    string
	Operator Op
	Value    any
}

// Op is a comparison operator within a condition.
type Op string

const (
	OpEqual       Op = "eq"
	OpNotEqual    Op = "neq"
	OpGreaterThan Op = "gt"
	OpLessThan    Op = "lt"
	OpGreaterEq   Op = "gte"
	OpLessEq      Op = "lte"
	OpContains    Op = "contains"
)

// OrderBy describes a sort directive.
type OrderBy struct {
	Field string
	Desc  bool
}

// UniversalQuery is the platform's read request, translated per-adapter.
type UniversalQuery struct {
	Filter Filter
}

// ResultSet is the normalized shape returned by Query.
type ResultSet struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// MaxRawRows caps raw SQL responses so a runaway SELECT can't OOM the host.
const MaxRawRows = 200

// RawQuerier is an optional interface for SQL engines that support
// read-only raw query execution (SQL editor). Engines that don't speak SQL
// simply don't implement it; the server reports an honest error.
type RawQuerier interface {
	ExecRaw(ctx context.Context, query string) (ResultSet, error)
}

// InsertResult reports what an Insert produced.
type InsertResult struct {
	Collection string
	ID         any
	Generated  map[string]any
}

// UpdateResult reports how a mutation affected rows.
type UpdateResult struct {
	MatchedCount  int64
	ModifiedCount int64
}

// DeleteResult reports how many rows were removed.
type DeleteResult struct {
	DeletedCount int64
}

// TriggerEvent is the kind of change that fires a trigger.
type TriggerEvent string

const (
	TriggerInsert TriggerEvent = "insert"
	TriggerUpdate TriggerEvent = "update"
	TriggerDelete TriggerEvent = "delete"
)

// TriggerDefinition describes a trigger in the platform-neutral format.
// The store persists this in the `triggers` table (SCHEMA.md §1); only the
// adapter's internal implementation differs per engine.
type TriggerDefinition struct {
	ID           string
	ProjectID    string
	Name         string
	Collection   string
	Event        TriggerEvent
	ActionType   string // "function" | "webhook"
	ActionTarget string
}

// ChangeHandler is invoked by realtime subscriptions when data changes.
type ChangeHandler func(Collection string, event TriggerEvent, record map[string]any)

// Subscription is a live handle returned by SubscribeToChanges.
type Subscription interface {
	Close() error
}

// ConnectionConfig carries whatever an adapter needs to establish a
// connection. Sensitive fields are populated from the (decrypted) credentials
// store at runtime, never persisted in plaintext.
type ConnectionConfig struct {
	Engine   Engine
	ConnStr  string
	Username string
	Password string
	Database string
}

// DatabaseAdapter is the Universal Data Interface. Every engine adapter
// implements this fully.
type DatabaseAdapter interface {
	// Connect establishes a live connection. Returns an error carrying the
	// underlying engine's message on failure so BYODB flows can surface it.
	Connect(ctx context.Context, cfg ConnectionConfig) error
	Disconnect(ctx context.Context) error

	// Schema introspection.
	ListCollections(ctx context.Context) ([]CollectionInfo, error)
	GetSchema(ctx context.Context, collection string) (SchemaInfo, error)
	ListRelationships(ctx context.Context) ([]Relationship, error)

	// CRUD.
	Query(ctx context.Context, q UniversalQuery) (ResultSet, error)
	Insert(ctx context.Context, collection string, doc map[string]any) (InsertResult, error)
	Update(ctx context.Context, filter Filter, update map[string]any) (UpdateResult, error)
	Delete(ctx context.Context, filter Filter) (DeleteResult, error)

	// Triggers.
	RegisterTrigger(ctx context.Context, t TriggerDefinition) error
	RemoveTrigger(ctx context.Context, triggerID string) error

	// Realtime.
	// RegisterRealtimeBroadcast installs a native trigger that notifies change
	// events for every row operation (insert/update/delete) on a collection, so
	// SubscribeToChanges delivers live row data. Adapters without native
	// triggers return ErrUnsupported (polling emulation replaced this in the
	// future). It is idempotent.
	RegisterRealtimeBroadcast(ctx context.Context, collection string) error
	SubscribeToChanges(ctx context.Context, collection string, handler ChangeHandler) (Subscription, error)

	// Capability introspection.
	Capabilities() CapabilitySet
}
