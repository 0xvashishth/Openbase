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
| MySQL ³ | Yes | Yes | `false` until delivery wired | `none` (delivery not wired) | Yes | No |
| FerretDB (on Postgres) ¹ | Limited | No (document model) | Via polling | Polling (no native change streams yet) | Limited | No |
| Valkey ² | No | No | No | `none` (keyspace notifs not implemented) | No | No |
| ArcadeDB (graph) ⁵ | No (doc-model; graph traversal is a follow-up) | No | No | `none` | No | No |
| Qdrant / Chroma (vector) ⁴ | No | No | No | No | No | Yes (native) |

¹ The shipped FerretDB adapter currently returns `Realtime:none` in `Capabilities()`; the polling-based
emulation tier (ADAPTERS.md §3, tier 2) arrives with the triggers/realtime phases, at which point the
flag flips to `polling` and this row becomes accurate in code, not just intent.

² The shipped Valkey adapter uses Redis sets of JSON docs as collections. It currently returns
`Realtime:none` — a Redis keyspace-notifications change stream is a candidate future tier, but has not
been implemented, so the honest value is `none`, not the `pub/sub` intent above.

³ The shipped MySQL adapter (CRUD/schema/joins/transactions/full-text all native) does not yet wire the
platform's trigger/realtime *delivery*. MySQL has native triggers, but without a queue-table + polling
change-stream tier a created trigger would fire into a void, so the adapter honestly returns
`SupportsNativeTriggers:false` and `Realtime:none`, and `RegisterTrigger`/`SubscribeToChanges`
return `ErrUnsupported`. The flags flip when that tier lands.

⁴ The shipped Qdrant adapter exposes each point (id + vector + JSON payload) as a universal row and
honestly reports `SupportsVectorSearch: true` (Qdrant is a vector engine) with no joins/FKs/triggers/
realtime. The universal query IR currently speaks equality filters over payloads, so a dedicated
vector/similarity-search method is a documented follow-up; until then `Query` filters payloads.

⁵ The shipped ArcadeDB adapter uses its **document-model** surface (per scoping decision), not the
graph model: types are collections, documents are rows, served over ArcadeDB's SQL-style REST API
(`POST /api/v1/query|command/{db}`). It reports honest capabilities — no relational joins/foreign
keys/transactions/triggers/realtime (`AllErrUnsupported`, `Realtime:none`) — because graph
traversal and edge `ListRelationships` are a documented follow-up; until then the matrix above
reflects what is shipped, not the graph-model intent.

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
5. Implement the optional `RawQuerier` interface (see §6) so the SQL editor can run the engine's own language.
6. Add adapter-specific tests against a real instance of that database (via Docker) — never mock the underlying DB in adapter tests, since the whole point of the adapter is faithfully translating to real native behavior.

## 5. BYODB (Bring Your Own Database) connection flow

1. User pastes a connection string (and optionally separate username/password if not embedded in the string).
2. Platform parses the URL scheme (`postgres://`, `mongodb://`, `redis://`, etc.) to auto-select the correct adapter.
3. Platform attempts a test connection immediately using that adapter, read-only introspection call (`ListCollections()`), before saving anything.
4. On success, credentials are encrypted and stored (see `SCHEMA.md` §2 `connections` table); the project is now backed by that adapter exactly as if it were provisioned in-platform — same capability-flag-driven feature set applies.
5. On failure, surface the adapter's actual connection error to the user rather than a generic message — connection failures are one of the most common support burdens for platforms like this.

## 6. `RawQuerier` — the SQL editor's escape hatch

The universal query IR deliberately covers only what every engine can express (collection, equality/comparison filters, sort, limit). The SQL editor needs the opposite: the engine's *full* native language, reads **and** writes. That is the optional `RawQuerier` interface:

```go
type RawQuerier interface {
    ExecRaw(ctx context.Context, query string) (ResultSet, error)
}
```

Rules every implementation follows:

- **Reads and writes both run.** DDL and DML are not blocked — this is the user's own database and the editor is the tool for changing it. The server-side guard only rejects empty/oversized input and stacked statements (`SELECT 1; DROP TABLE x`), so one Run executes exactly one statement.
- **Results are normalized into `ResultSet`.** Row-returning statements fill `Columns`/`Rows` capped at `MaxRawRows` (200). Writes report what happened as a single row: `affected_rows` (plus `insert_id` on MySQL, `matched_count`/`modified_count`/`deleted_count`/`inserted_id` on FerretDB) or `{"result": "OK"}` for DDL, so the editor never shows a confusing empty grid after a successful write.
- **Errors surface verbatim.** The engine's own message reaches the user; nothing is swallowed or rewritten.
- **Timeouts come from the context.** The server wraps every call in a 15s deadline.

Per-engine language (as shipped):

| Engine | Raw language accepted by `ExecRaw` |
|---|---|
| PostgreSQL | SQL: `SELECT`/`WITH`/`EXPLAIN` plus `INSERT`/`UPDATE`/`DELETE` (incl. `RETURNING`) and DDL |
| MySQL | SQL: reads (`SELECT`/`WITH`/`EXPLAIN`/`SHOW`/`DESCRIBE`) routed to Query, everything else to Exec |
| ArcadeDB | ArcadeDB SQL: `SELECT` via `POST /api/v1/query`, writes/DDL via `POST /api/v1/command` |
| FerretDB | mongo-shell: `db.<coll>.find/insertOne/insertMany/updateOne/updateMany/deleteOne/deleteMany/countDocuments/drop`, `db.createCollection`, `.limit().skip().sort()` chains, plus a `{"collection","op",…}` JSON form |
| Valkey | Redis commands (`GET`/`SET`/`HSET`/`DEL`/`KEYS`/…), one per line, redis-cli-style quoting; the last result is returned |
| Qdrant | `SCROLL`/`SEARCH`/`UPSERT`/`DELETE`/`CREATE`/`DROP <collection> [json]`, plus a `{"collection","op",…}` JSON form |

An adapter that genuinely has no raw language simply doesn't implement the interface, and the server answers `400 raw queries are not supported for this engine` rather than faking execution.

**Plumbing note:** the engine package's `Conn` wrapper must forward `ExecRaw` to the underlying adapter. It is the type the server actually holds, so a missing forwarder makes the server's `adapter.RawQuerier` type-assertion fail for *every* engine — which is exactly how the editor once reported "raw SQL is only supported for postgres and mysql" even on Postgres. The same rule applies to `pooledAdapter` (§7): every wrapper in the chain must forward the optional interfaces, because a type assertion cannot see through an embedded one.

## 7. Adapter pooling — one live adapter per project

The server keeps one connected adapter per project connection in a keyed cache (`internal/pool`, wired in `internal/server/adapter_pool.go`) rather than dialing per request. Before this, every data request paid a full TCP + TLS + auth + `Ping` handshake and then closed the pool on the way out, so `GET /v1/api/*` — the hottest route in the system — did a fresh connection setup and teardown each time, and `GET /projects/{id}/schema` ran its N+1 introspection on a permanently cold connection.

What an adapter implementer needs to know:

- **Adapters are shared across concurrent requests.** Every currently-shipped adapter is safe for this because its underlying client already is (`pgxpool.Pool`, `*sql.DB`, `mongo.Client`, `redis.Client`, `http.Client` are all documented as concurrency-safe). An adapter that holds a single non-shareable session — a bare `net.Conn`, or any client with per-connection server-side state — must not be pooled; say so in review rather than assuming.
- **`Disconnect` on a pooled adapter is a no-op.** The pool owns the lifecycle. Handlers still write `defer a.Disconnect(ctx)` and that is correct: real disposal happens on idle eviction, invalidation, or server shutdown.
- **Optional interfaces must be forwarded explicitly** by the wrapper (see the `RawQuerier` forwarder and the assertions next to it). This is the one real hazard the wrapper introduces, and it fails for all engines at once, so `pooled_adapter_test.go` asserts each forwarded capability.
- **Long-lived subscriptions stay out of the pool.** Realtime and the trigger runtime dial their own adapters on purpose: a Postgres `LISTEN` needs a dedicated session, and its lifetime is the subscription's, not a request's.

Invalidation is by project, on every event that can repoint a project at different data: saving or replacing a connection, and deleting one. `saveConnection` reuses the connection row id when it overwrites credentials, so the pool key also carries a credential *generation* — without it, a rewrite would keep serving an adapter still dialed at the previous database. The memoized primary-key cache (§`resolveRowKey`) is invalidated on the same events, since a new target can have a different schema.

