# Phase-Wise Build Roadmap

Each phase should be independently shippable/demoable. Don't start Phase N+1 until Phase N's adapter/feature actually works end-to-end against a real running database, not mocked.

## Phase 0 — Platform Foundation
**Goal:** Auth + org/project management works, backed by the platform's own Postgres metadata DB. No adapter engine yet.

- [x] Metadata DB schema from `SCHEMA.md` §1, migrations set up
- [x] User signup/login (email+password to start; OAuth can come later)
- [x] Organization CRUD + membership/roles
- [x] Project CRUD scoped to an organization
- [x] Basic dashboard shell (Next.js) with login → org list → project list navigation
- [x] `docker-compose.yml` that boots: platform API + metadata Postgres + dashboard

**Status (done):** Go API (`cmd/server`) + Postgres metadata store (`internal/metadata`, embedded migrations) +
JWT auth (`internal/auth`) + envelope encryption (`internal/crypto`) + CORS. Dashboard (`dashboard/`, Next.js 15 +
TypeScript + Tailwind) provides signup/login, org list/create, project list/create, and a per-project
Overview / Connection page (BYODB test-connection preview against the real API). Full API tests
(`internal/server`, `internal/metadata`, `migrations`) run against a real Postgres in Docker.

**Done when:** a user can sign up, create an org, create a project, and see it in the dashboard. No real "database service" functionality yet.

## Phase 1 — Adapter Engine + First Adapter (Postgres)
**Goal:** Prove the abstraction layer with the simplest, most capable adapter first.

- [x] Implement `DatabaseAdapter` interface (see `ADAPTERS.md` §1)
- [x] Implement Postgres adapter fully (CRUD, schema introspection, capabilities)
- [x] Basic table/data browser in dashboard (list collections, view rows) — no visual relationship diagram yet, just raw table view
- [x] "Provisioned" mode: creating a project spins up a dedicated Postgres container, connection stored (encrypted) in `connections` table

**Status (complete ✅):** Adapter interface + Postgres adapter (CRUD, schema, PK/FK introspection,
native triggers via LISTEN/NOTIFY, realtime subscribe, capability flags) tested against a real Postgres
in Docker. Both connection paths work end-to-end from the dashboard:
- **BYODB**: save-connection (test-before-save, auto-detect engine, envelope-encrypt credentials →
  SCHEMA.md §2), `GET /collections`, `GET /collections/{name}` (schema) and `POST /query` (rows +
  filters) all route through the adapter.
- **Provisioned**: `internal/provision` (ARCHITECTURE.md §2.7) runs a Docker-backed `Compose`
  provisioner; saving a `mode: provisioned` connection spawns a dedicated Postgres container, stores
  its generated credentials encrypted with a `container_id`, and marks it `connected`. `DELETE /v1/
  projects/{id}/connections` tears the container down (verified in an E2E test: provisioning →
  browse → delete → container gone). Overwriting a provisioned connection destroys the old container
  so nothing leaks. Provisioning is opt-in via `OPENBASE_PROVISIONER_ENABLED=true`.
- Dashboard: **Connection** tab lets you create a new provisioned DB or attach an existing one, and
  shows the current connection with a Remove action; **Tables** tab browses it.

**Done when:** a user creates a project, gets a provisioned Postgres instance, and can view/query its tables through the dashboard. — *met, verified in `internal/server/provision_test.go` and a Docker-backed E2E smoke.*

## Phase 2 — Second Adapter (FerretDB) — Prove the Abstraction Actually Generalizes
**Goal:** This is the real test of whether the adapter pattern was designed correctly. If adding FerretDB requires touching feature code outside the adapter itself, the interface needs rework before adding more engines.

- [x] Implement FerretDB adapter against the same `DatabaseAdapter` interface
- [x] Fill in its `Capabilities()` honestly (see matrix in `ADAPTERS.md` §2)
- [x] Dashboard's table browser must work unmodified against both adapters
- [x] Database picker (dropdown: Postgres / FerretDB) in the connection flow

**Status (complete ✅):** A full FerretDB adapter (`internal/adapter/ferretdb`) speaks the Mongo wire
protocol via `go.mongodb.org/mongo-driver/v2` to `ghcr.io/ferretdb/ferretdb:2.7.0` (backed by the
DocumentDB-enabled Postgres image). It implements CRUD, schema introspection (field-union sampling +
`_id` primary, index listing), filtered/sorted/paginated query, and BSON→JSON-friendly normalization;
unsupported methods (triggers, change streams, relationships) return the platform's explicit
`ErrUnsupported` and `Capabilities()` declares `Realtime:none` for now (polling emulation ships with
Phase 4/5, per ADAPTERS.md §2 matrix). Provisioning also handles FerretDB: `internal/provision`
spins a whole group (private network + DocumentDB backend + FerretDB container) and `Destroy` tears
it down — the stored `container_id` carries a `ferret|` prefix to signal a group. Verified by:
adapter tests against a real FerretDB (CRUD/introspection/capabilities), a provision-package
round-trip teardown test, and a server-level E2E (provision FerretDB via API → browse endpoints →
DELETE → connection gone). Dashboard Connection panel gained an engine picker and the table browser
runs against both engines unchanged. Full Go suite + dashboard build green.

**Done when:** the exact same dashboard code browses both a Postgres-backed and a FerretDB-backed project, with document-model differences handled gracefully (no crashing on missing foreign key info, etc.) — *met.*

## Phase 3 — Visual Schema Explorer + Auto-Generated API
**Goal:** The Supabase-style "see your tables and how they relate" feature, plus a REST API generated from the schema.

- [x] Relationship diagram UI (React Flow) showing tables, PK/FK lines — only rendered for adapters where `SupportsForeignKeys` is true
- [x] Auto-generated REST endpoints per collection/table (CRUD), scoped by API key
- [x] API key management UI (`api_keys` table from `SCHEMA.md`)
- [ ] (Stretch) GraphQL layer generated from the same schema introspection

**Status (complete ✅):**
- **API keys**: `internal/apikey` (already present) wired to real handlers + middleware.
  `POST /v1/projects/{id}/api-keys` returns the plaintext `ob_...` key once; `GET` lists
  (never the hash/plaintext), `DELETE /{keyID}` revokes. Scopes default to read+write.
- **Auto-generated REST API** (external access, authenticated by API key):
  `GET /v1/api/tables`, `GET /v1/api/{collection}` (with `?limit`/`?offset`/`?order_by=&order=`),
  `GET /v1/api/{collection}/_schema`, `POST /v1/api/{collection}`, `PUT/DELETE /v1/api/{collection}/{id}`.
  The project is resolved from the key itself (no project ID in the URL). Rejected/invalid keys → 401.
- **Schema explorer**: new `GET /v1/projects/{id}/schema` returns all collections + schemas +
  FK relationships + honest `capabilities`. Dashboard gains a **Schema** tab using React Flow
  (`@xyflow/react`) rendering each table as a node (PK/UQ markers, data types) and FK lines as
  edges. The relationship diagram is **capability-gated**: engines without `SupportsForeignKeys`
  (e.g. FerretDB) show the tables without edges and a clear note — no broken rendering.
- **API Keys tab** in the dashboard: create (one-time plaintext), list, revoke, plus an
  "Using the API" reference.
- **Done when:** a user can visually see their schema and hit a real REST endpoint from outside
  the platform using an API key — *met.* Backend CRUD/schema/key tests verified against real
  Postgres in Docker (`internal/server/apikey_test.go`, `internal/server/data_test.go`);
  the dashboard typechecks and `npm run build` passes.

## Phase 4 — Triggers + Runtime Functions
**Goal:** Visual trigger builder + sandboxed function execution.

- [x] `TriggerDefinition` format finalized, `triggers` table wired up
- [x] Native trigger implementation for Postgres (LISTEN/NOTIFY based)
- [ ] Polling-based trigger emulation for FerretDB
- [x] Sandboxed function runtime (start with Node.js support; Python scaffolded but not yet a runner)
- [x] Visual trigger builder UI: pick collection → event (insert/update/delete) → action (function or webhook)

**Status (complete ✅, with one explicit deferral):**
- **Metadata + API**: `trigger`/`function` types and CRUD wired in `internal/metadata`
  (`scopes` check on `triggers.event`/`action_type` and `functions.runtime`). Dashboard-facing
  handlers: `GET/POST /v1/projects/{id}/triggers`, `PUT/DELETE .../triggers/{triggerID}`,
  `GET/POST .../functions`, `GET/DELETE .../functions/{fnID}` — all org-authorization gated.
- **Trigger runtime** (`internal/triggers`): `Service` registers native DB triggers and
  subscriptions per project via the adapter (re-registers on create/update/delete and on
  (re)connect), and a `Dispatcher` evaluates each event against enabled triggers for the
  collection/event, routing to an action.
- **Actions**:
  - **Webhook** (`EndpointAction`): POSTs `{trigger_id, project_id, collection, event, data}`
    to the target URL.
  - **Function** (`FunctionAction`): runs the referenced user function via the sandbox.
  - A `DispatchGroup` routes by `action_type`; unsupported/missing actions fail loudly (never silent).
- **Function sandbox** (`internal/function`, Node.js runner): writes the user module + wrapper to
  0600 temp files, executes via `node --max-old-space-size=128 --disallow-code-generation-from-strings`
  with a 5s timeout, JSON event on stdin, JSON result on stdout, `OB_FUNCTION_PATH` env var.
  Pure-subprocess, so unit-testable without Docker. (Python is accepted at the API/schema level
  but there is no Python runner yet — the FunctionAction returns an error.)
- **Postgres wiring**: `RegisterTrigger` recreates a `openbase_notify_<id>` PL/pgSQL trigger that
  `pg_notify`s `row_to_json(NEW)`; `SubscribeToChanges` `LISTEN`s and feeds the dispatcher. The
  engine `Conn` forwards register/subscribe/disconnect for the runtime.
- **Verified end-to-end** (`internal/server/trigger_e2e_test.go`): connect real Postgres → create a
  webhook trigger via the API → insert a row through the auto-generated REST API → the webhook is
  invoked with the expected payload (native LISTEN/NOTIFY delivery). Unit tests cover dispatch
  targeting, disabled/mismatch skipping, and the Node runner (success, timeout, bad source,
  unknown runtime). Dashboard `npm run build` passes.
- **Deferred (explicit)**: FerretDB polling-based trigger emulation. The FerretDB adapter honestly
  returns `ErrUnsupported` for register/subscribe and `Capabilities().SupportsNativeTriggers=false`,
  so the runtime skips it; trigger UI/API remain fully functional for Postgres. Because the model
  is capability-driven, FerretDB triggers need no feature-code changes to land later.

**Done when:** an insert into a table can trigger a user-authored function, for both adapters, with the UI honestly reflecting latency differences (native vs. polling). — *Postgres fully met; FerretDB polling emulation explicitly deferred to a follow-up (per the honest-capability rule, no fake support).*

## Phase 5 — Realtime Layer
**Goal:** Client SDK can subscribe to live data changes.

- [x] `SubscribeToChanges` implemented for Postgres (native LISTEN/NOTIFY) — delivered; FerretDB (polling, clearly labeled as near-realtime not instant) explicitly deferred to a follow-up (honest-capability rule)
- [x] WebSocket gateway for client subscriptions (`GET /v1/realtime`, API-key authed)
- [x] Minimal client SDK (JS/TS) + live-updating list demo in the dashboard

**Done when:** a browser demo shows a list updating live when a row changes, for at least the Postgres adapter. — *Met: the dashboard "Realtime" tab subscribes over WebSocket and a live-updating list reflects REST inserts for Postgres; `RegisterRealtimeBroadcast` returns `ErrUnsupported` for non-native engines.*

## Phase 6 — BYODB Mode (mostly pulled forward into Phase 1)
**Goal:** Users can connect an existing database instead of provisioning one.

> Note: the core BYODB flow was deliberately pulled forward into Phase 1 (documented deviation) to make the
> Phase 1 dashboard table-browser testable. The remaining work below is incremental hardening.

- [x] Connection string input + auto-detection (`SCHEMA.md` §3) — delivered in Phase 1
- [x] Test-connection-before-save flow (ADAPTERS.md §5) — delivered in Phase 1
- [x] Encrypted credential storage (envelope-encrypted at rest, SCHEMA.md §2) — delivered in Phase 1
- [x] Upgrade the envelope-encryption master key to a vault/KMS/signer — delivered as a `KeyProvider` abstraction + multi-key `MasterKeyProvider` (local in-process key by default; rotation via new current key while keeping old keys for decryption; a KMS/Vault provider slots into the `KeyProvider` interface). Design reference: `ARCHITECTURE.md` §2.6 (was mis-cited as §2.4). New env: `OPENBASE_ENCRYPTION_KEY_ID`, `OPENBASE_ENCRYPTION_KEYS`
- [x] All existing features (browser, schema explorer, triggers, realtime) work identically over a BYODB connection, gated by the same capability flags — verified; dashboard Triggers/Realtime tabs now show an honest unsupported state (not a runtime failure) for engines that lack native trigger/realtime support

**Done when:** a user pastes a connection string to their own existing Postgres or FerretDB instance and gets the full platform experience without provisioning anything. — *Postgres: fully met for every feature; FerretDB: honestly capability-gated (triggers + realtime unavailable), matching its adapter capabilities.*

## Phase 7 — Remaining Adapters + Polish
**Goal:** Round out the database options and harden what exists.

- [x] Valkey adapter (key-value; capability-limited UI accordingly) — implemented (`internal/adapter/valkey`): collections modeled as Redis sets of JSON docs; CRUD, schema sampling, collection listing; honest capabilities (no FKs/joins/triggers/realtime). Auto-detected from `redis://`/`valkey://`/`rediss://`
- [x] ArcadeDB adapter (graph; relationship UI adapted for graph traversal rather than FK lines) — implemented (`internal/adapter/arcadedb`) as a **document-model surface** (per scoping decision): ArcadeDB document types are universal collections, documents are rows, via ArcadeDB's SQL-style HTTP REST API (`POST /api/v1/query|command/{db}`). Full CRUD, schema union-sampling (with the `@rid` as the primary key), equality/comparison/order/limit/skip filtering via bound params (injection-safe identifier validation + `?` placeholders). Honest capabilities: no joins/FKs/triggers/realtime/transactions (`ErrUnsupported`); graph/traversal surface and `ListRelationships` edges are a documented follow-up. Auto-detected from `http(s)://…:2480`/`2481` (also `bolt://`); BYODB only (not provisioned). Tested with `httptest` (no Docker).
- [x] Qdrant adapter (vector) — implemented (`internal/adapter/qdrant`): speaks Qdrant's HTTP REST API; each point (id + vector + JSON payload) surfaces as a universal row, payload fields become columns, schema union-samples payloads + reads vector size. Honest capabilities: `SupportsVectorSearch: true`, no joins/FKs/triggers/realtime (`ErrUnsupported`). Auto-detected from `http(s)://…:6333`/`6334`; BYODB only (not provisioned). A dedicated similarity-search method in the universal IR is a follow-up. Tested with `httptest` (no Docker).
- [ ] Chroma adapter (second vector engine; folds into the same vector-IR follow-up) — deferred
- [x] MySQL adapter (second relational engine) — implemented (`internal/adapter/mysql`): full CRUD, schema introspection, relationships/joins, transactions, full-text; auto-detected from `mysql://`. Honest gating: the platform's trigger/realtime *delivery* (queue-table + polling change stream) is not yet wired, so `SupportsNativeTriggers`/`SupportsRealtime` are `false` and `RegisterTrigger`/`SubscribeToChanges` return `ErrUnsupported` (no silent no-op). Tested with `go-sqlmock` (no live MySQL / no Docker); Docker-backed E2E deferred to a follow-up.
- [x] Load testing, security audit pass — function sandbox hardened: process-group isolation (descendant processes killed on timeout, no orphans) + bounded stdout/stderr (`limitedBuffer`) so a runaway function can't exhaust host memory
- [ ] Documentation for third-party adapter contributions (formalize the checklist in `ADAPTERS.md` §4)

**Done when:** all six V1 target engines from `README.md` §4 are selectable, each with an honest, capability-flag-driven feature set.

---

# Part II — Supabase Parity Roadmap (Phases 8+)

Phases 0–7 built the thing Supabase *doesn't* have: a capability-honest, six-engine adapter layer.
Part II closes the gap on what Supabase *does* have. The inventory below was taken against
Supabase's published feature catalogue (79 features) and audited line-by-line against this codebase.

## Ground rules for Part II

1. **Capability-driven, never faked.** Every feature declares which engines it supports via
   `CapabilitySet` (extended as needed). A feature that can't work on an engine shows an honest
   unsupported state — the same rule that governed Phases 1–7.
2. **Supabase can push work into Postgres; we usually can't.** RLS, PostgREST, `pg_graphql`,
   `pg_cron`, `pgvector`, `pg_net` are Postgres extensions. For 5 of our 6 engines the equivalent
   must live in the *platform* layer. Where a native pushdown exists (Postgres/MySQL) we use it and
   say so; otherwise we enforce in the API tier and say that too.
3. **Two-tier enforcement is a first-class concept.** Tier A = native pushdown (in-database,
   uncircumventable). Tier B = platform-side (enforced in the data path, bypassable by anyone with
   direct DB credentials). The UI must always state which tier is active for a given project.
4. **No new feature ships on a broken foundation.** Phase 8 is remediation only — it adds no
   user-visible capability, and it is a hard prerequisite for Phases 11–16.
5. Each phase still has to be independently demoable against a real running database.

## Parity gap inventory

Legend: ✅ have · ⚠️ partial/thin · ❌ absent · 🚫 out of scope for self-hosted V1.

### A. Auth — the single largest structural gap

Supabase's `auth.users` is the *end user of the customer's app*. Openbase's `users` table is the
*platform operator*. There is currently **no end-user identity concept anywhere in the codebase**,
which is why there is also no RLS, no `auth.uid()`, and no per-user data authorization.

| Supabase | Openbase today | Phase |
|---|---|---|
| Email login (end users) | ❌ no end-user directory at all | 10 |
| Magic links / passwordless | ❌ | 10 |
| Phone / SMS OTP | ❌ | 10 |
| Social login (GitHub, Google, Apple…) | ❌ | 10 |
| Custom OAuth2/OIDC providers | ❌ | 10 |
| SSO with SAML | ❌ | 10 (stretch) |
| Anonymous sign-in | ❌ | 10 |
| MFA (TOTP) | ❌ | 9 (operators), 10 (end users) |
| Web3 / wallet auth | ❌ | 🚫 not V1 |
| Captcha protection | ❌ | 10 |
| Email templates | ❌ no mailer of any kind | 9 |
| JWT signing keys (asymmetric, JWKS) | ❌ HS256 shared secret only | 10 |
| Third-party auth (trust external JWTs) | ❌ | 11 |
| Auth hooks (function on signup/token) | ❌ | 10 |
| Server-side auth helpers | ❌ | 12 (SDK) |
| Refresh tokens / sessions / sign-out | ❌ one opaque 24 h token, unrevocable | 9 |
| Password reset / email verification | ❌ | 9 |
| User impersonation | ❌ | 10 |
| RBAC | ⚠️ roles stored, **never compared** — a `member` can drop the DB | 8 |
| Authorization via RLS | ❌ nothing; an API key is unconditional full CRUD | 11 |

### B. Data API

| Supabase | Openbase today | Phase |
|---|---|---|
| PostgREST REST API | ⚠️ 6 hand-written routes, `limit/offset/order_by` only | 12 |
| Filter operators (`eq/in/gt/like/or/…`) | ❌ none on the public API | 12 |
| Column projection (`select=`) | ❌ always `SELECT *` | 12 |
| Embedded resources (FK joins) | ❌ | 12 |
| Exact/estimated count, `Content-Range` | ❌ | 12 |
| Upsert / bulk insert / PATCH | ❌ | 12 |
| RPC (call DB functions) | ❌ | 12 |
| OpenAPI spec | ❌ | 12 |
| `pg_graphql` GraphQL API | ❌ (Phase 3 stretch, never started) | 12 |
| Client libraries (JS/Python/Flutter/Swift) | ⚠️ a 113-line WS-only helper in the dashboard | 12 |
| **Live defect** | `PUT`/`DELETE /v1/api/{c}/{id}` hard-code `_id` → **broken on Postgres/MySQL**, untested | 8 |

### C. Database & Studio

| Supabase | Openbase today | Phase |
|---|---|---|
| Table editor: insert/edit/delete rows | ❌ browser is **strictly read-only** | 13 |
| Cell editor, JSON viewer, row drawer | ❌ | 13 |
| Create/alter/drop table & column UI | ❌ no DDL API and no DDL UI | 13 |
| Index manager, enums, views | ❌ | 13 |
| Foreign Key Selector | ❌ | 13 |
| CSV import / export | ❌ | 13 |
| Multi-condition filter / multi-sort / count | ⚠️ **one** condition, hand-typed field, no total count, fixed 25/page | 13 |
| SQL Editor | ✅ CodeMirror + history, reads **and** writes/DDL on every engine in its own language — no Cmd+Enter, no snippets, no autocomplete, no EXPLAIN, no export | 13 |
| Visual Schema Designer (editable) | ⚠️ read-only ERD, layout not persisted | 13 |
| Postgres Extensions page | ❌ | 13 |
| Postgres Roles page | ❌ | 13 |
| Database Webhooks | ✅ trigger→webhook exists — no signing, no retries, no delivery log, **no SSRF guard** | 8 |
| Policy Templates | ❌ | 11 |
| Security & Performance Advisor | ❌ | 17 |
| Declarative schemas / migrations / branching | ❌ | 18 |
| Database backups + PITR | ❌ **provisioned containers have no volume — `docker rm -f` is unrecoverable** | 18 |
| Read replicas, SSL enforcement, network restrictions | ❌ | 18 |
| Supavisor / dedicated poolers | ❌ **worse than absent: a brand-new pool is dialed and closed per HTTP request** | 8 |
| Foreign Data Wrappers, OrioleDB, Vault, Queues | ❌ | 18 / 🚫 |
| Cron | ❌ | 15 |
| Vector database | ⚠️ Qdrant adapter + a *static* `SupportsVectorSearch:true` on Postgres with **no vector query path in the IR** | 18 |
| Automatic embeddings, AI integrations | ❌ | 🚫 not V1 |

### D. Storage — 100 % absent

No bucket table, no multipart handling, no signed URLs, no backend. Zero code.

| Supabase | Phase |
|---|---|
| File storage (buckets/objects, policies) | 14 |
| Resumable (TUS) uploads | 14 |
| Image transformations | 14 |
| S3-compatible protocol access | 14 |
| CDN / Smart CDN | 🚫 cloud-only |
| Analytics/Vector Buckets (Iceberg) | 🚫 not V1 |

### E. Functions

| Supabase | Openbase today | Phase |
|---|---|---|
| HTTP-invokable functions | ❌ **trigger-invoked only**; no invoke endpoint exists | 15 |
| Deploy / update / version | ❌ create + delete only; a function is immutable once created | 15 |
| Secrets / env vars | ❌ **worse: the sandbox inherits `OPENBASE_JWT_SECRET` and `OPENBASE_ENCRYPTION_KEY`** | 8 |
| Logs / invocation history | ❌ stdout captured then discarded | 15 |
| Dependencies (npm/pip) | ❌ stdlib only, no `node_modules` | 15 |
| Multi-language | ⚠️ Node only; `python` is accepted by the API then fails at runtime | 15 |
| Real sandbox isolation | ⚠️ subprocess as the API's own uid, full network + FS | 15 |
| **Live defect** | the shipped `alpine` image has **no Node**, so functions cannot execute in production | 8 |
| Regional invocation, persistent S3 mounts | 🚫 cloud-only |

### F. Realtime

| Supabase | Openbase today | Phase |
|---|---|---|
| Postgres Changes | ✅ Postgres only, collection-granularity | — |
| Event-type + row filters (`event`, `filter=`) | ❌ you always get insert+update+delete, whole table | 16 |
| `old_record` on update/delete | ❌ `to_jsonb(NEW)` only → **DELETE delivers `data: null`** | 8 |
| Broadcast (client↔client) | ❌ | 16 |
| Presence | ❌ | 16 |
| Broadcast/Presence authorization | ❌ any valid key may subscribe to anything | 16 |
| Broadcast replay / `since` cursor | ❌ overflow is **silently dropped** at 256 msgs | 16 |
| Per-table replication toggle | ❌ | 16 |
| **Live defect** | one shared `openbase_rt_notify()` with the channel baked into its body → **a second subscribed collection hijacks the first** | 8 |
| **Live defect** | browsers cannot send an `Authorization` header; the shipped SDK accepts `apiKey` and never uses it | 8 |

### G. Platform & Ops

| Supabase | Openbase today | Phase |
|---|---|---|
| Logs & Analytics, Logs Explorer | ❌ stdout `slog`; request log has no status code | 17 |
| Reports & Metrics | ❌ no `/metrics`, no counters | 17 |
| Log Drains | ❌ | 17 |
| Health checks | ❌ **no `/healthz`** — the dashboard fakes it by timing `GET /v1/me` | 8 |
| Audit log | ❌ nothing records who ran DDL or minted a key | 9 |
| Management API | ⚠️ the dashboard API is undocumented and unversioned in practice | 18 |
| CLI | ❌ | 18 |
| MCP server / AI assistant | ❌ | 18 (stretch) |
| Terraform provider | ❌ | 18 (stretch) |
| Org members / invites / settings UI | ❌ two `EmptyState` stubs against **nonexistent endpoints** | 8 / 9 |
| Project rename / delete / transfer | ❌ "Danger zone" card is inert text | 8 |
| Custom domains, PrivateLink, SOC2, billing | 🚫 cloud-only |

## Phase 8 — Remediation: make what already ships actually true
**Goal:** zero new user-visible features. Fix the seven live defects the audit found, enforce the
authorization model that is currently decorative, and put a connection pool in the data path.
**Every one of Phases 11–16 depends on this.** Do not start them first.

**8.1 — Primary-key resolution on the public data API.**
`apiUpdateRow`/`apiDeleteRow` hard-code `{Field: "_id"}` (`internal/server/public_api_handlers.go:235`,
`:263`), which compiles to `WHERE "_id" = $1` and errors on Postgres and MySQL. Resolve the PK from
`GetSchema(collection).PrimaryKey` with a per-connection cache; 400 with a clear message when a
collection has no single-column PK. Add `PUT`/`DELETE` cases to `internal/server/apikey_test.go` for
Postgres *and* FerretDB — the bug is live precisely because no test covers these verbs.

**8.2 — Postgres realtime: per-collection notify + full payload.**
`RegisterRealtimeBroadcast` (`internal/adapter/postgres/adapter.go:641`) creates one globally named
`openbase_rt_notify()` with the channel string baked into the function body, so registering a second
collection rewrites the function and every table then notifies the last-registered channel. Fix:
pass the channel via `TG_ARGV[0]` (single shared function, per-trigger argument). Change
`to_jsonb(NEW)` → `record`/`old_record` built from `COALESCE(NEW, OLD)` so DELETE carries data. Add a
two-collection E2E test asserting no cross-talk, plus a DELETE-payload assertion.

**8.3 — Adapter connection pooling.**
Today every data request runs `pgxpool.New` + `Ping` and closes the pool on the way out
(`connectProjectForAPIKey` → `Factory.ConnectForProject` → `defer Disconnect`). One TCP+TLS+auth
handshake per API call, and `GET /projects/{id}/schema` does N+1 introspection on a cold pool.
Build `internal/pool`: a keyed cache (`connection.id` + credential generation) of live adapters,
idle eviction, health re-dial, max-per-project cap, and explicit invalidation on
save/delete-connection and on key rotation. Also add `GET /v1/projects/{projectID}` so the dashboard
stops fetching every project to resolve one, and index `api_keys.key_hash` (see 8.5) so auth stops
being a seq scan. Ship with a before/after benchmark in the PR.

**8.4 — Enforce RBAC.** ✅ *shipped*
`authorizeOrg` (`internal/server/middleware.go:34`) only asserted that a membership row exists; the
`role` column was never compared anywhere outside its own creation, so a `member` could delete the
project's database, mint API keys and run arbitrary DDL. Now: `OrgRole.rank()`/`AtLeast()`,
`authorizeOrgRole(min)` / `projectAndOrgRole(min)` applied to every mutating route, the matrix
documented in `SCHEMA.md` §5, and 403 bodies that name the required role. `rbac_test.go` walks the
matrix; `dashboard/lib/permissions.test.ts` is a parity check so UI gating cannot drift from it.

**8.5 — API keys: real scopes, real hygiene.**
Scopes are stored, echoed and never consulted (`internal/server/apikey_handlers.go:88`), so a
"read" key can DELETE. Enforce scopes in `requireAPIKey`; add `UNIQUE` + index on `key_hash`,
`last_used_at`, optional `expires_at`, and a stored display prefix (`ob_abc…`) so the UI can
identify a key it can never show again. Migration `0002_*.sql`.

**8.6 — Org/project lifecycle endpoints.** ✅ *shipped*
`Store.AddMember`/`ListMembers` existed with no route, and the dashboard shipped
`/orgs/[orgId]/members`, `/orgs/[orgId]/settings` and the project "Danger zone" as stubs whose copy
claimed the feature was "available via the API" — it was not. Now: `PATCH`/`DELETE /v1/orgs/{id}`,
full member CRUD, `POST /v1/orgs/{id}/transfer-ownership`, and `PATCH`/`DELETE /v1/projects/{id}`
with an ordered cascade (destroy the provisioned container first and abort on failure, stop
triggers, invalidate the pooled adapter, then let `ON DELETE CASCADE` clear connections, keys,
triggers and functions). All three pages are real, with `OrgSettingsPanel`, `MembersPanel` and
`ProjectSettingsPanel`. Deleting an org refuses with 409 while projects exist rather than cascading
across N containers mid-transaction.

Two things deliberately left for later: adding a member requires an existing account (there is no
mailer until 9.2, so an unknown address returns 404 instead of pretending an invite was sent), and
`organization_invites` exists in migration `0003` but has no routes yet — that is 9.7.

**8.7 — Abuse controls + operability.**
Add `GET /healthz` (liveness) and `GET /readyz` (metadata DB + migration state), a Prometheus
`/metrics` endpoint, a request-id middleware that also logs status code and bytes, per-IP rate
limiting on `/v1/auth/*` with lockout after N failures, and per-API-key quotas. Point the
dashboard's `ApiStatus` at `/healthz` instead of timing `GET /v1/me`. Add a container healthcheck
for the `api` service in `docker-compose.yml`.

**8.8 — Function sandbox: stop leaking platform secrets, and ship a runtime that works.**
`internal/function/runner.go:104` passes `os.Environ()` through, so untrusted user code reads
`OPENBASE_JWT_SECRET`, `OPENBASE_ENCRYPTION_KEY` and `OPENBASE_DATABASE_URL`. Switch to a
deny-by-default env allow-list. Then fix the packaging: the runtime image is `alpine:3.20` with only
the Go binary, so no `node` exists and function triggers silently fail in production — either add
Node to the image or refuse to register function-actions when no runtime is detected (honest-capability
rule). Add a per-project concurrency cap.

**8.9 — Webhook hardening.**
`EndpointAction` (`internal/triggers/service.go:100`) POSTs with no signature, no timeout of its own
and no destination validation, so `action_target` can be `http://169.254.169.254/…`. Add an HMAC
`X-Openbase-Signature` (per-project secret), a request timeout, an SSRF deny-list (loopback,
link-local, RFC1918 — configurable for self-host), retry with exponential backoff, and a
`webhook_deliveries` table + delivery log in the Triggers UI.

**8.10 — Dashboard debt that blocks everything after it.**
Collapse the five duplicated definitions of the project tool list (`ProjectSidebar.tsx`, `nav.ts`,
`Breadcrumbs.tsx` ×2, the Overview tile grid, `GlobalSearch.tsx`) into one source of truth — adding
a tab currently means editing five files. Add a toast system and adopt the already-written-but-unused
`ConfirmDialog` in place of the four `window.confirm` calls. Add `error.tsx` + `not-found.tsx`. Fix
the auth pages: they hardcode `bg-slate-50`/`bg-white` so they ignore dark mode, and use
`bg-brand-600`/`text-brand-600` classes that **are not defined in `tailwind.config.ts`** and emit
nothing. Add Cmd/Ctrl+Enter to the SQL editor, copy-to-clipboard on every key/URL/snippet.
Introduce SWR or react-query — every panel currently refetches from scratch with zero caching, and
`GlobalSearch` re-fans-out `listOrgs`+`listProjects` on every ⌘K.

**Done when:** `go test ./...` and the dashboard suite are green with new negative-authorization,
PK-resolution, multi-collection-realtime and DELETE-payload tests; a `member` provably cannot
destroy a project; a read-scoped key provably cannot write; `/healthz` and `/metrics` respond; and
the data path reuses pooled connections (measured).

## Phase 9 — Platform account & organization management
**Goal:** the operator-facing account system stops being a 110-line stub. Prerequisite for org
collaboration and for the audit trail that Phase 11+ security features imply.

**9.1 — Sessions and refresh tokens.** `sessions` table (id, user, refresh-token hash, user agent,
IP, `expires_at`, `revoked_at`). Short-lived access token (15 min) + rotating refresh token with
reuse detection. `POST /v1/auth/refresh`, `POST /v1/auth/logout`, `POST /v1/auth/logout-all`.
Today a stolen JWT is valid for its full 24 h and deleting the user does not invalidate it.

**9.2 — Mailer abstraction.** `internal/mail` with an SMTP sender, a dev/log sink, and templated
HTML+text messages. There is currently no mail capability anywhere in the repo, which is why 9.3,
9.7 and most of Phase 10 are blocked.

**9.3 — Credential lifecycle.** Email verification, forgot/reset password (single-use tokens with
expiry), change password (re-auth + revoke other sessions), change email (confirm both addresses).
Fix the timing oracle in login — the "no such user" branch skips bcrypt entirely.

**9.4 — Account settings UI.** `/account`: profile (`full_name` is currently write-once at
registration), email, password, sessions/devices list with revoke, delete account.

**9.5 — Dashboard SSO.** GitHub + Google OAuth for *platform* login, `identities` table, account
linking. SAML behind a build flag (stretch).

**9.6 — MFA for operators.** TOTP enrolment + recovery codes + step-up challenge on destructive
actions.

**9.7 — Org collaboration.** *Partially shipped in 8.6:* members list with an inline role editor,
transfer ownership (with optional self-demotion), leave org, and the last-owner guard are all live,
and adding an existing account by email works. What remains is the part that needs a mailer:
email invites with an accept flow. Migration `0003` already ships
`organization_invites (org_id, email, role, token_hash, invited_by, expires_at, accepted_at)`, so
this is `POST /v1/orgs/{id}/invites` returning a one-time link plus
`POST /v1/invites/{token}/accept` — usable out-of-band for self-hosters even before 9.2 lands, with
mail becoming just another delivery channel for the same token.

**9.8 — Audit log.** `audit_events` (actor, org, project, action, target, IP, metadata, ts) written
for login, key mint/revoke, connection change, **every raw SQL execution**, DDL, member/role change,
project delete. `GET /v1/orgs/{id}/audit` + a filterable UI. Nothing today records that a member ran
`DROP TABLE` through the SQL editor.

*Seam already in place from 8.6:* `server.AuditSink` is an interface on `Services` with a no-op
default, and every org/project mutation already calls `Record(actor, org, project, action, target,
metadata)`. Migration `0003` creates the `audit_events` table. This phase is now a writer
implementation plus the read endpoint and UI — the ~15 call sites exist, which is the part that is
expensive to retrofit.

**Done when:** a user can sign up, verify their email, reset a forgotten password, enrol TOTP,
invite a colleague as `admin`, see both sessions listed, revoke one, and read the audit trail of all
of it.

## Phase 10 — End-user authentication per project (the GoTrue equivalent)
**Goal:** the customer's *application users* become a first-class concept. This is the largest single
gap and the hard prerequisite for Phase 11 (there is nothing to authorize against until it exists).

**10.0 — Decide where end users live (do this first, write it into `ARCHITECTURE.md`).**
Two options:
- **(a) In the project's own database** (`openbase_auth` schema). Matches Supabase, lets native RLS
  join against `auth.users`, and survives Openbase being uninstalled. Only possible for engines with
  DDL + relational semantics: Postgres, MySQL.
- **(b) In the platform metadata DB**, keyed by `project_id`. Works for all six engines, keeps a
  BYODB user's database untouched, but makes native RLS pushdown impossible (the identity is not
  visible to the engine).

**Recommendation: (b) as the default, (a) as an opt-in "native auth" mode for Postgres/MySQL**,
declared through a new `SupportsNativeAuth` capability. This is the honest multi-engine answer, and
it is exactly the Tier A/Tier B split from ground rule 3. Getting this wrong is expensive to undo —
settle it before writing code.

**10.1 — Identity model.** `project_users`, `project_identities` (provider + provider_uid),
`project_sessions`, `project_auth_settings`, `project_mfa_factors`. Per-project **asymmetric** JWT
signing keys (ES256/RS256) + `GET /v1/projects/{id}/.well-known/jwks.json`. Openbase currently signs
everything with one shared HS256 secret, which cannot be given to a third party to verify.

**10.2 — Public auth endpoints** (project-scoped, resolved from the anon key):
`POST /v1/auth/v1/signup`, `/token?grant_type=password`, `/token?grant_type=refresh_token`,
`/logout`, `GET/PUT /user`, `/recover`, `/verify`, `/magiclink`, `/otp`.

**10.3 — Providers, in order:** email+password → magic link → OAuth (GitHub, Google, then a generic
OIDC connector that covers "custom identity providers") → phone/SMS OTP behind a pluggable
`SMSProvider` → anonymous sign-in.

**10.4 — Policy and abuse surface.** Redirect-URL allow-list, per-project email templates,
per-project rate limits, captcha hook (hCaptcha/Turnstile), password strength policy, session
timeout (idle + absolute), leaked-password check (optional, offline list).

**10.5 — MFA for end users.** TOTP enrol/challenge/verify, `aal1`/`aal2` in the token claims.

**10.6 — Auth hooks.** Invoke a project function on `before-user-created`, `after-user-created`,
`before-token-issued` (custom claims) — this is what makes RBAC-inside-the-app possible.

**10.7 — Authentication section in the dashboard.** New top-level project nav group:
- **Users** — table with search, create/invite, delete, reset password, ban, confirm email,
  view identities/sessions, and *impersonate* (issues a scoped short-lived token; gated to `admin`+
  and always audit-logged).
- **Providers** — enable/configure per provider with redirect-URI copy blocks.
- **Email templates** — editable with variable reference and preview.
- **URL configuration**, **Rate limits**, **Sessions**, **Signing keys** (view/rotate JWKS).

**Done when:** an external app can sign a user up, receive a verified email, sign in with GitHub,
refresh its token, and have that token verified against the project's public JWKS — and the operator
can see, invite, ban and impersonate that user from the dashboard.

## Phase 11 — Authorization: the RLS equivalent
**Goal:** stop an API key from being unconditional full CRUD on the whole database. This is the
feature that makes Openbase usable from a browser at all, and the one where the multi-engine design
forces a genuinely different solution from Supabase's.

Depends on: Phase 8 (pooling, scopes, RBAC), Phase 10 (an identity to authorize).

**11.1 — Key roles.** Split the single `ob_` key into `anon`, `authenticated` (implicit — carried by
an end-user JWT) and `service_role`, mirroring Supabase's mental model. The data API accepts either
an API key or an end-user JWT from 10.1; the JWT's claims become the authorization context. Keep
`service_role` bypass explicit, loudly labelled, and never shipped to a browser.

**11.2 — Policy model.** `policies` table: project, collection, action (`select|insert|update|delete`),
role, and an expression AST (JSON, not raw SQL — it must be translatable to two backends). Vocabulary:
`auth.uid()`, `auth.role()`, `auth.jwt() -> claim`, row column references, literals, `and/or/not`,
comparison and `in`. A small, deliberately restricted language; not arbitrary code.

**11.3 — Two-tier enforcement.**
- **Tier A, native pushdown (Postgres, MySQL 8+):** compile the AST to `CREATE POLICY` /
  `CREATE VIEW` + `SET LOCAL request.jwt.claims`, and enable `ROW LEVEL SECURITY` on the table. Now
  uncircumventable, even for a direct psql connection. New capability flag: `SupportsRowSecurity`.
- **Tier B, platform-side (FerretDB, ArcadeDB, Qdrant, Valkey):** the evaluator injects the policy
  predicate into `adapter.Filter` before every read, and validates the candidate document against the
  predicate before every write. Enforced in the data path only.
- The UI must state the active tier per collection. Tier B without a badge that says "enforced by
  Openbase, not by your database" would be dishonest.

**11.4 — Default-deny and ergonomics.** A per-collection `security_enabled` switch that defaults to
**on for new projects** (Supabase's hard-won lesson); an unmissable warning in the Table editor and
Advisor when a collection is exposed with no policy; policy templates ("user owns row via
`user_id`", "read-only for anon", "team membership via join", "public read / owner write"); a
"test as user" simulator that runs a query under a chosen uid and shows the compiled predicate.

**11.5 — One evaluator, three consumers.** The same package authorizes the Data API (Phase 12),
Realtime subscriptions and channels (Phase 16), and Storage objects (Phase 14). Write it as a
standalone `internal/authz` with no HTTP dependency.

**11.6 — Third-party auth.** Accept JWTs from an external issuer (Auth0/Clerk/Firebase) by
configuring issuer + JWKS URL per project, mapping claims into the same authorization context.

**Done when:** two end users of the same project, using the *same* anon key, provably see only their
own rows — on Postgres via native RLS and on FerretDB via platform-side enforcement — and the UI
correctly reports which mechanism is protecting each.

## Phase 12 — Data API parity + client SDKs
**Goal:** the auto-generated API becomes something you can actually build an app against, and
there's a library to call it with.

Depends on: Phase 11 (every filter must run inside the policy predicate, not around it).

**12.1 — Query grammar.** PostgREST-compatible query strings on `GET /v1/api/{collection}`:
`?col=eq.value`, `neq/gt/gte/lt/lte/like/ilike/in/is/not`, `or=(a.eq.1,b.gt.2)`, `select=a,b`,
`order=col.desc.nullslast`, `limit/offset`, `Range` header, `Prefer: count=exact` → `Content-Range`.
Parse into the existing `UniversalQuery` IR so all six adapters inherit it; return a clear 400 for
operators an engine can't express rather than silently ignoring them.

**12.2 — Embedded resources.** `select=*,orders(*)` FK-following, gated on `SupportsForeignKeys`
(Postgres/MySQL today). One query per level with an IN-batch, not N+1.

**12.3 — Write semantics.** Bulk insert (array body), upsert via
`Prefer: resolution=merge-duplicates`, `PATCH` for partial update, `Prefer: return=representation|minimal`
(today `Insert` unconditionally echoes every column of the new row back), and column masking driven
by policies.

**12.4 — RPC.** `POST /v1/api/rpc/{name}` to call a database function, capability-gated
(`SupportsStoredProcedures`), with argument binding and a typed signature read from introspection.

**12.5 — OpenAPI + docs UI.** Generate an OpenAPI 3.1 document per project from the existing
introspection, serve it at `GET /v1/api/` (as PostgREST does), and add an **API Docs** tab: per-table
endpoints with copy-ready `curl` / JS / Python snippets — folding in the in-flight `ConnectPanel`
work rather than duplicating it.

**12.6 — GraphQL.** The Phase 3 stretch goal, finally: schema generated from the same introspection,
`POST /v1/graphql`, connection-style pagination, mutations, and policy enforcement through the Phase
11 evaluator. Capability-gated to engines with FK metadata. GraphiQL in the dashboard.

**12.7 — Client SDKs.** `@openbase/js` as a real npm package (auth + data query builder + realtime +
storage + functions), replacing `dashboard/lib/realtime.ts` — the dashboard becomes its first
consumer, which is the only way the SDK stays honest. Then `openbase-py`. Publish a
server-side-auth helper (cookie/session handling for Next.js and friends).

**Done when:** `openbase.from('orders').select('*,customer(name)').eq('status','open').limit(20)`
works from a browser against a Postgres project, returns only policy-permitted rows, and the same SDK
call degrades with an explicit error (not a wrong answer) against Qdrant.

## Phase 13 — Table editor, DDL and Studio parity
**Goal:** the dashboard stops being a read-only viewer. This is the biggest *UI* phase, and the one
users will notice most.

Depends on: Phase 8.10 (toasts, confirm dialogs, data-fetch caching), Phase 12.1 (the grid's filter
UI should compile to the same grammar the API exposes).

**13.1 — Universal DDL in the adapter interface.** New optional interface `SchemaMutator`:
`CreateCollection`, `DropCollection`, `RenameCollection`, `AddColumn`, `AlterColumn`, `DropColumn`,
`CreateIndex`, `DropIndex`, `AddForeignKey`, `DropForeignKey` + `SupportsDDL` capability. Implement
for Postgres and MySQL; FerretDB gets collection-level operations only; Valkey/Qdrant/ArcadeDB
declare it unsupported. Every generated statement goes through identifier validation and is
audit-logged (9.8).

**13.2 — Row editing.** Insert-row panel built from `ColumnInfo` (types, defaults, nullability are
already fetched and currently never rendered), inline cell edit with optimistic update + rollback,
row detail drawer, delete/duplicate row, multi-select with bulk delete, a JSON/JSONB cell editor,
and a foreign-key selector that searches the referenced table. The public REST API already supports
per-row writes; the dashboard has simply never called them.

**13.3 — Grid upgrades.** Total row count (`count=exact` from 12.1), page-size selector, jump-to-page,
multi-column sort, a real filter builder (column dropdown, type-aware operators, `IS NULL`, `IN`,
AND/OR groups) replacing today's single hand-typed condition, column show/hide/reorder/resize with
persisted per-table view state, and keyboard grid navigation. Today: one condition, fixed 25/page,
and the "n rows" badge counts only the current page.

**13.4 — Import/export.** CSV/JSON import wizard (upload → column mapping → type coercion preview →
dry-run → commit with row-level error report) and export of a table or a query result to CSV/JSON,
including clipboard copy.

**13.5 — Schema management UI.** New table dialog, column editor, rename/drop with dependency
warnings, index manager, enum manager, view + materialized-view manager.

**13.6 — SQL Editor v2.** Cmd/Ctrl+Enter (8.10), query tabs, **server-stored** snippets with sharing
inside the org (today: `localStorage`, per-browser, undiscoverable), schema-aware autocomplete,
`EXPLAIN`/`EXPLAIN ANALYZE` with a plan visualiser, formatting, result export, result pagination
beyond the current 200-row server cap, statement cancellation, opt-in multi-statement execution
wrapped in a transaction (the stacked-statement guard stays on by default), and an explicit
confirmation for destructive DDL.

**13.7 — Database section pages.** Extensions (list/enable/disable — Postgres only), Roles &
grants, Publications/replication, Database functions, *native* DB triggers (distinct from the
platform triggers in Phase 4, and currently invisible in the UI), and Indexes.

**13.8 — Visual Schema Designer.** Promote the read-only ERD to an editor: create tables, columns and
FK relationships from the canvas, persist node layout per project (it currently re-grids on every
filter keystroke), and export PNG/SVG.

**Done when:** a user can create a table, add and type its columns, define a foreign key, insert and
edit rows, import a CSV, and export the result — never leaving the dashboard and never writing SQL.

## Phase 14 — Storage
**Goal:** files. Currently 0 % implemented — no table, no route, no backend, no UI.

Depends on: Phase 11 (object access must be policy-driven from day one, not bolted on).

**14.1 — Backend abstraction.** `internal/storage` with a `Backend` interface (`Put`, `Get`, `Stat`,
`List`, `Delete`, `Copy`, `Move`, `SignedURL`) and two implementations: local filesystem (the
self-host default) and S3-compatible (MinIO / R2 / S3). Same adapter-pattern discipline as databases.

**14.2 — Metadata + model.** `buckets` (project, name, public flag, allowed MIME types, size limit)
and `objects` (bucket, path, size, mime, checksum, owner = a `project_users` id, metadata JSONB,
timestamps), plus a `folder/` path convention.

**14.3 — REST surface.** `POST /v1/storage/{bucket}/{path}` (single + multipart),
`GET`, `HEAD`, `DELETE`, list, move, copy, plus `POST .../sign` for time-limited download and
upload URLs. Range requests and correct caching headers.

**14.4 — Storage policies.** Reuse the Phase 11 evaluator over object path, bucket, owner and JWT
claims. Templates: "public read", "owner-only", "authenticated upload to own folder".

**14.5 — Advanced.** Resumable uploads (TUS protocol), image transformations
(`?width=&height=&quality=&format=`) with an on-disk derivative cache, and an S3-compatible gateway
so external tooling can talk to Openbase storage directly.

**14.6 — Storage UI.** Bucket list + create (public/private, limits), file browser with
breadcrumbs, drag-and-drop upload with progress, preview for images/text/PDF, rename/move/delete,
bulk select, copy URL / create signed URL, and a per-bucket policy editor.

**Done when:** a user creates a private bucket, an end user uploads an avatar through the SDK,
another end user provably cannot read it, and the owner sees it rendered in the dashboard browser.

## Phase 15 — Functions v2, scheduling and queues
**Goal:** functions become a product rather than a trigger side-effect, and Openbase gets the
engine-agnostic answer to `pg_cron`.

**15.1 — HTTP invocation.** `POST /v1/functions/{name}` authenticated by API key *or* end-user JWT,
sync and fire-and-forget modes, JSON in/out, per-function CORS config, and the end user's identity
passed into the event so a function can act on their behalf. There is currently **no way to invoke a
function except by mutating a Postgres table**.

**15.2 — Lifecycle.** `PUT /v1/projects/{id}/functions/{fnID}` (a function is immutable today),
version history with rollback, a CodeMirror editor replacing the plain `<textarea>`, a source viewer
(`api.getFunction` already exists and is never called), and a **Test invoke** panel with a sample
event and the response/logs inline.

**15.3 — Secrets and config.** Per-function encrypted secrets (reusing `internal/crypto`), an
explicit env allow-list building on 8.8, timeout and memory overrides per function, and an import
map / dependency manifest.

**15.4 — Logs and reliability.** Persist `function_invocations` (status, duration, memory, truncated
stdout/stderr — today the buffer is discarded unless the run errors), a log tail UI, retries with
backoff and a dead-letter queue for trigger-invoked runs, and per-function metrics.

**15.5 — Real isolation.** Deliver what `ARCHITECTURE.md` §2.4 actually specifies: gVisor-sandboxed
containers (or Firecracker), CPU quota, a network egress policy, a read-only root FS, and a
dependency install step (`npm install` / `pip install`) with a build cache. Today it is a subprocess
running as the API server's own uid with full network and filesystem access.

**15.6 — Python runtime.** Finish the scaffold: `python` is accepted by the API and the DB CHECK but
`Run` rejects it, so a Python function can be created and then always fails. Multi-language was
called out in `ARCHITECTURE.md` §2.4 as a deliberate differentiator over Supabase's TS-only functions.

**15.7 — Scheduled jobs (cron).** A platform scheduler + `scheduled_jobs` table invoking functions or
webhooks on a cron expression, with run history, manual run, pause, and overlap policy. Because it
lives in the platform rather than in `pg_cron`, **this works for all six engines** — a genuine
advantage worth documenting.

**15.8 — Durable queues.** A metadata-DB-backed queue (enqueue/dequeue/ack/retry/DLQ, visibility
timeout) consumable by functions, exposed over the API and in the SDK. The Supabase Queues
equivalent, again engine-independent.

**Done when:** a function can be edited in the dashboard, invoked over HTTP with an end-user token,
scheduled every five minutes, and its logs and failures read back — on a project backed by any engine.

## Phase 16 — Realtime v2
**Goal:** close the Phase 4/5 deferrals and reach feature parity on channels.

Depends on: Phase 8.2 (the notify collision), Phase 11 (channel authorization).

**16.1 — Browser-usable authentication.** A browser cannot set an `Authorization` header on
`new WebSocket()`, so the gateway is currently unauthenticatable from the web — the shipped SDK takes
an `apiKey` argument and never sends it. Fix with a short-lived ticket endpoint
(`POST /v1/realtime/ticket` → single-use token in the query string) or a WS subprotocol carrying the
token. Also tighten `OriginPatterns` (currently `"*"`) and wire `Hub.store` (currently `nil`, so
project validation is a no-op).

**16.2 — Postgres Changes v2.** Per-subscription event-type selection (`INSERT`/`UPDATE`/`DELETE` —
today you always get all three), row filters reusing the Phase 12.1 grammar, `old_record` on
update/delete, a per-table replication toggle, and a move from trigger-based `pg_notify` to **logical
decoding** (`pgoutput` replication slot), which removes the DDL-on-user-tables side effects and the
8 KB `pg_notify` payload ceiling.

**16.3 — Broadcast.** Client→client channels with server-side fan-out, an HTTP publish endpoint, and
optional "broadcast from the database" via trigger.

**16.4 — Presence.** Join/leave tracking, per-connection state, diff-based sync.

**16.5 — Authorization.** Channel and postgres-changes subscriptions run through the Phase 11
evaluator, so a subscriber only receives rows they could have `SELECT`ed. Today any valid key may
subscribe to any collection in the project.

**16.6 — Delivery guarantees.** Heartbeat/ack, message ids, a `since` cursor for replay, and an
explicit slow-consumer signal — the hub currently drops silently once the 256-message buffer fills.

**16.7 — The polling tier (closes the Phase 4 and 5 deferrals).** A generic change-detection tier for
engines with no native change feed: FerretDB (`_id`/updated-at cursor or change streams if the
DocumentDB backend gains them), MySQL (queue table + poll, per the Phase 7 note), ArcadeDB, Qdrant,
Valkey (keyspace notifications). `RealtimeMode: "polling"` finally becomes real. Every surface must
label it "near-realtime (~N s)" — the honest-capability rule, and the acceptance criterion the
original Phase 4 spec set.

**16.8 — Realtime inspector.** Replace the demo panel with a real tool: active channels, subscriber
counts, a live message log with timestamps and payload inspector, event-type and filter controls, and
a disconnect button. Fix the demo's dead-end where a pre-existing API key makes it unusable.

**Done when:** two browser clients exchange broadcast messages and see each other's presence; a
FerretDB project receives labelled near-realtime row changes; and an unauthorized subscriber receives
nothing.

## Phase 17 — Observability, reports and advisors
**Goal:** operators can see what the platform is doing. Today: `slog` to stdout, and a request log
line that doesn't even include the status code.

**17.1 — Log pipeline.** Structured events (request id from 8.7, actor, project, status, bytes,
duration, error) into a partitioned `logs` table with retention, plus per-service streams: API, Auth,
Data API, Realtime, Functions, and the database's own logs where reachable.

**17.2 — Logs Explorer UI.** Filterable, time-ranged, tail-following viewer with saved queries and a
per-service selector.

**17.3 — Metrics and Reports.** Prometheus `/metrics` (8.7) plus a Reports page: request volume,
p50/p95/p99 latency, error rate, active DB connections, database and storage size, function
invocations and failures, realtime peak connections, API-key usage by key.

**17.4 — Query performance.** `pg_stat_statements` where available (MySQL: performance schema),
slow-query list, and per-endpoint timing.

**17.5 — Security Advisor.** Automated checks with fix links: collections exposed with no policy, RLS
disabled, `service_role` key used from a browser origin, default JWT secret in use, keys never
rotated or unused for 90 days, public buckets, `sslmode=disable` on a BYODB connection, provisioned
containers without a volume, functions with wide env access.

**17.6 — Performance Advisor.** Missing FK indexes, unused indexes, sequential scans on large tables,
tables without a primary key (which also breaks 12.3 and 8.1), N+1 patterns detected in API logs.

**17.7 — Log drains.** Export to webhook, S3, Datadog, Loki/Grafana or Sentry, with retention config.

**Done when:** an operator can answer "what broke, for whom, and when" from the dashboard alone, and
the Security Advisor flags a table exposed without a policy before a user finds out the hard way.

## Phase 18 — Ops, developer workflow and the last adapter gaps
**Goal:** the lifecycle work that makes the platform safe to run in production, plus the Phase 7
leftovers.

**18.1 — Backups and restore.** This is the most dangerous current gap: provisioned containers are
created **with no volume**, so `DELETE /connections` (`docker rm -f`) destroys the data
irrecoverably. Add persistent volumes, then per-engine scheduled logical backups (`pg_dump`,
`mysqldump`, `mongodump`, Valkey RDB, Qdrant snapshot, ArcadeDB backup) to a Phase 14 storage
backend, with retention, integrity verification, a restore flow, download, and a backups UI.
PITR for Postgres (WAL archiving) as a follow-up.

**18.2 — Migrations for user databases.** A migration ledger inside the project database,
`POST /v1/projects/{id}/migrations` (apply/rollback/status), a schema-diff generator that emits a
migration from two schema snapshots, and seed-data support. Today the only path to schema change is
hand-typed single-statement SQL with no history.

**18.3 — Branching.** Clone a project's schema (optionally with data) into a branch project, diff it
against the parent, and merge back as a migration. Capability-gated to engines with DDL support.

**18.4 — Declarative schema.** Schema-as-files with a plan/apply loop, so a repo is the source of
truth.

**18.5 — `openbase` CLI.** `login`, `link`, `db pull/push/diff/reset`, `migration new/up`,
`functions new/deploy/serve`, `gen types` (TypeScript types from introspection), `logs`, `start`
(local stack). This is the single biggest developer-experience multiplier on the list.

**18.6 — Management API + Terraform.** Version and document the platform API as a stable contract
(OpenAPI), with PATs separate from user JWTs. A Terraform provider on top of it (stretch).

**18.7 — Connection and network settings.** SSL/CA certificate upload for BYODB (today the BYODB
form is a single password field and provisioned DBs run `sslmode=disable`), broken-out
host/port/db/user fields, `statement_timeout` and other DB settings, IP allow-lists on provisioned
databases, a pooled connection endpoint for external clients (the Supavisor equivalent, built on the
8.3 pool), read replicas (stretch), and a Kubernetes `ProvisionerInterface` implementation to prove
the abstraction from `ARCHITECTURE.md` §2.7.

**18.8 — Multiple databases per project.** Drop the `UNIQUE` constraint on `connections.project_id`
(`SCHEMA.md` §1 already anticipates this) so a project can, for example, pair Postgres with Qdrant —
something Supabase structurally cannot do, and the clearest expression of Openbase's premise.

**18.9 — Finish the Phase 7 leftovers.**
- Chroma adapter (the last V1 engine).
- **Vector search in the universal IR** — `VectorQuery`/`Search` on the adapter interface, so
  `SupportsVectorSearch` becomes actionable instead of a static `true` on Postgres that nothing can
  use, and so Qdrant similarity search is reachable without raw commands. Includes a `pgvector`
  probe rather than an assumption, and an embeddings/similarity UI.
- ArcadeDB graph traversal surface + `ListRelationships` edges, with a graph view in the schema
  explorer.
- Live-engine integration tests for MySQL, Qdrant and ArcadeDB (currently `sqlmock`/`httptest` only).
- Third-party adapter contribution guide (`ADAPTERS.md` §4 checklist, still open).

**18.10 — MCP server + AI assistant** (stretch). An MCP server exposing schema, logs and SQL to AI
tooling, and a natural-language → SQL/policy assistant in the dashboard.

**Done when:** a project can be backed up and restored, its schema evolved through versioned
migrations from a CLI, and all six V1 engines pass live integration tests.

---

## Suggested execution order

Phases 8 → 9 → 10 → 11 are a dependency chain and should be done in order: remediation, then
operator identity, then end-user identity, then authorization. Nothing built on top is trustworthy
until 11 lands.

After 11, these can proceed in parallel by area:
- **Data path:** 12 → 16
- **Studio UI:** 13
- **New services:** 14, 15
- **Cross-cutting:** 17, then 18

Two items are worth pulling forward out of order because they are cheap and currently dangerous:
**8.1, 8.2, 8.8 and 8.9** (live defects and a secret leak) and **18.1's persistent volumes** (silent
data loss on connection removal).

---

## Explicitly out of scope for this roadmap (future, not V1)
- Cloud-hosted/managed offering
- Multi-tenant shared-cluster infrastructure
- Billing/pricing tiers
- Migration tooling between shared and dedicated tenancy (see architecture discussion — relevant once cloud hosting is built)

### Supabase features deliberately excluded from Part II

Evaluated and rejected for a self-hosted V1, with the reason:

| Supabase feature | Why not |
|---|---|
| CDN / Smart CDN | An edge network is a hosting concern; self-hosters put their own CDN in front. |
| Custom domains | Same — a reverse-proxy concern, not a platform feature. |
| PrivateLink, Network restrictions (cloud) | Cloud networking. The provisioned-DB IP allow-list in 18.7 is the self-host analogue. |
| Regional invocations, Persistent S3 mounts for functions | Multi-region infrastructure. |
| SOC 2 / HIPAA compliance | Certification of a hosted service, not of software. |
| OrioleDB | A Postgres storage-engine bet; nothing in Openbase's layer depends on it. |
| Foreign Data Wrappers | Postgres-specific, and multi-database-per-project (18.8) covers the actual use case in an engine-neutral way. |
| Vault (secrets inside Postgres) | Openbase already has envelope encryption with a pluggable `KeyProvider` (`ARCHITECTURE.md` §2.6); a second in-database secret store would duplicate it. |
| Supabase Pipelines, Analytics/Vector Buckets (Iceberg) | Analytics/lakehouse product surface, far outside a backend-as-a-service V1. |
| Automatic embeddings, AI integrations (OpenAI/HF) | Depends on 18.9's vector IR first; revisit afterwards. |
| Web3 / wallet authentication | Niche relative to everything else in Phase 10. |
| Client libraries for Flutter / Swift | After `@openbase/js` and `openbase-py` prove the API contract (12.7). |

