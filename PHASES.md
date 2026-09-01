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

- [ ] Relationship diagram UI (React Flow or similar) showing tables, PK/FK lines — only rendered for adapters where `SupportsForeignKeys` is true
- [ ] Auto-generated REST endpoints per collection/table (CRUD), scoped by API key
- [ ] API key management UI (`api_keys` table from `SCHEMA.md`)
- [ ] (Stretch) GraphQL layer generated from the same schema introspection

**Done when:** a user can visually see their schema and hit a real REST endpoint from outside the platform using an API key.

## Phase 4 — Triggers + Runtime Functions
**Goal:** Visual trigger builder + sandboxed function execution.

- [ ] `TriggerDefinition` format finalized, `triggers` table wired up
- [ ] Native trigger implementation for Postgres (LISTEN/NOTIFY based)
- [ ] Polling-based trigger emulation for FerretDB
- [ ] Sandboxed function runtime (start with Node.js support, Python second)
- [ ] Visual trigger builder UI: pick collection → event (insert/update/delete) → action (function or webhook)

**Done when:** an insert into a table can trigger a user-authored function, for both adapters, with the UI honestly reflecting latency differences (native vs. polling).

## Phase 5 — Realtime Layer
**Goal:** Client SDK can subscribe to live data changes.

- [ ] `SubscribeToChanges` implemented for Postgres (via logical replication) and FerretDB (via polling, clearly labeled as near-realtime not instant)
- [ ] WebSocket gateway for client subscriptions
- [ ] Minimal client SDK (JS/TS) demonstrating a live-updating list

**Done when:** a browser demo shows a list updating live when a row changes, for at least the Postgres adapter.

## Phase 6 — BYODB Mode (mostly pulled forward into Phase 1)
**Goal:** Users can connect an existing database instead of provisioning one.

> Note: the core BYODB flow was deliberately pulled forward into Phase 1 (documented deviation) to make the
> Phase 1 dashboard table-browser testable. The remaining work below is incremental hardening.

- [x] Connection string input + auto-detection (`SCHEMA.md` §3) — delivered in Phase 1
- [x] Test-connection-before-save flow (ADAPTERS.md §5) — delivered in Phase 1
- [x] Encrypted credential storage (envelope-encrypted at rest, SCHEMA.md §2) — delivered in Phase 1
- [ ] Upgrade the envelope-encryption master key to a real vault/KMS/signer (currently an env-configured master key; design in `ARCHITECTURE.md` §2.4)
- [ ] All existing features (browser, schema explorer, triggers, realtime) work identically over a BYODB connection, gated by the same capability flags — automatically satisfied by the adapter gate, but re-verify after Phase 3/4/5 features land

**Done when:** a user pastes a connection string to their own existing Postgres or FerretDB instance and gets the full platform experience without provisioning anything.

## Phase 7 — Remaining Adapters + Polish
**Goal:** Round out the database options and harden what exists.

- [ ] Valkey adapter (key-value; capability-limited UI accordingly)
- [ ] ArcadeDB adapter (graph; relationship UI adapted for graph traversal rather than FK lines)
- [ ] Qdrant/Chroma adapter (vector; UI for embedding/similarity search rather than table rows)
- [ ] MySQL adapter (second relational engine)
- [ ] Load testing, security audit pass (especially the function sandbox and secrets vault)
- [ ] Documentation for third-party adapter contributions (formalize the checklist in `ADAPTERS.md` §4)

**Done when:** all six V1 target engines from `README.md` §4 are selectable, each with an honest, capability-flag-driven feature set.

---

## Explicitly out of scope for this roadmap (future, not V1)
- Cloud-hosted/managed offering
- Multi-tenant shared-cluster infrastructure
- Billing/pricing tiers
- Migration tooling between shared and dedicated tenancy (see architecture discussion — relevant once cloud hosting is built)
