# Database Abstraction Layer — Adapter Spec

This is the most important document for whoever implements the core engine. Every feature in the platform (auth, API generation, triggers, realtime, visualization) must go through this interface — **never** talk to a specific database driver directly from a feature module.

## 1. The Universal Data Interface (conceptual)

Every adapter implements the same interface, roughly:

```go
type DatabaseAdapter interface {
    Connect(config ConnectionConfig) error
    Disconnect() error

    // Schema
    ListCollections() ([]CollectionInfo, error)
    GetSchema(collection string) (SchemaInfo, error)
    ListRelationships() ([]Relationship, error) // PK/FK info, if applicable

    // CRUD
    Query(q UniversalQuery) (ResultSet, error)
    Insert(collection string, doc map[string]any) (InsertResult, error)
    Update(collection string, filter Filter, update map[string]any) (UpdateResult, error)
    Delete(collection string, filter Filter) (DeleteResult, error)

    // Triggers
    RegisterTrigger(t TriggerDefinition) error
    RemoveTrigger(triggerID string) error

    // Realtime
    SubscribeToChanges(collection string, handler ChangeHandler) (Subscription, error)

    // Capability introspection — see below
    Capabilities() CapabilitySet
}
```

`UniversalQuery` and `Filter` are the platform's own intermediate representation (think: a simplified query AST — collection, filters, sort, limit, joins-if-supported) that each adapter translates into native syntax (SQL for Postgres/MySQL, MongoDB query documents for FerretDB, etc.).

## 2. Capability Flags — the core of honest abstraction

Not every database can do everything. Rather than faking support or silently failing, every adapter must declare what it supports:

```go
type CapabilitySet struct {
    SupportsRelationalJoins   bool
    SupportsForeignKeys       bool
    SupportsNativeTriggers    bool   // e.g. Postgres triggers
    SupportsChangeStreams     bool   // e.g. Mongo/FerretDB change streams
    SupportsRealtime          RealtimeMode // "native" | "polling" | "none"
    SupportsTransactions      bool
    SupportsFullTextSearch    bool
    SupportsVectorSearch      bool
}
```

The dashboard and API layer **query this before rendering a feature**. Example: if a user picks Valkey (key-value store) as their project's database, the "Foreign Keys" tab in the schema visualizer simply doesn't render, and the UI explains why, rather than showing an empty/broken table view.

### Reference capability matrix (V1 target adapters)

| Engine | Joins | Foreign Keys | Native Triggers | Realtime | Transactions | Vector Search |
|---|---|---|---|---|---|---|
| PostgreSQL | Yes | Yes | Yes | Native (logical replication) | Yes | Yes (pgvector) |
| MySQL | Yes | Yes | Yes | Polling / binlog-based | Yes | No |
| FerretDB (on Postgres) | Limited | No (document model) | Via polling | Polling (no native change streams yet) | Limited | No |
| Valkey | No | No | No | Native (pub/sub) | Limited | No |
| ArcadeDB (graph) | N/A (graph traversal instead) | N/A | Limited | Polling | Yes | Yes |
| Qdrant / Chroma (vector) | No | No | No | No | No | Yes (native) |

This table should live in code as the actual `Capabilities()` return values, not just documentation — keep them in sync.

## 3. Triggers: the gnarliest part

Three tiers of implementation per adapter, in order of preference:

1. **Native trigger support** (Postgres: real `CREATE TRIGGER` + `LISTEN/NOTIFY`, or Postgres logical replication for the realtime path).
2. **Emulated via polling** — for databases with no native hook (e.g. FerretDB today), the adapter runs an internal poll loop checking for changes on a defined interval, and fires the platform-level trigger. This has latency tradeoffs the UI should surface (e.g. "changes detected within ~2s" rather than instant).
3. **Unsupported** — the capability flag is `false`, and the trigger builder UI disables that database as a target.

The visual trigger builder in the dashboard is built once against the universal `TriggerDefinition` format; only the adapter's internal implementation of `RegisterTrigger()` differs per engine.

## 4. Adding a new adapter — checklist

When adding support for a new database engine, an implementer should:

1. Implement `DatabaseAdapter` interface fully (stub unsupported methods to return a clear "unsupported by this engine" error, never a silent no-op).
2. Fill out `Capabilities()` honestly.
3. Add a row to the capability matrix in this doc.
4. Add connection-string auto-detection pattern (see `SCHEMA.md` §3) so BYODB mode can auto-identify the engine from the URL scheme.
5. Add adapter-specific tests against a real instance of that database (via Docker) — never mock the underlying DB in adapter tests, since the whole point of the adapter is faithfully translating to real native behavior.

## 5. BYODB (Bring Your Own Database) connection flow

1. User pastes a connection string (and optionally separate username/password if not embedded in the string).
2. Platform parses the URL scheme (`postgres://`, `mongodb://`, `redis://`, etc.) to auto-select the correct adapter.
3. Platform attempts a test connection immediately using that adapter, read-only introspection call (`ListCollections()`), before saving anything.
4. On success, credentials are encrypted and stored (see `SCHEMA.md` §2 `connections` table); the project is now backed by that adapter exactly as if it were provisioned in-platform — same capability-flag-driven feature set applies.
5. On failure, surface the adapter's actual connection error to the user rather than a generic message — connection failures are one of the most common support burdens for platforms like this.
