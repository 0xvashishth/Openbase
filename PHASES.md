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
- [ ] "Provisioned" mode: creating a project spins up a dedicated Postgres container, connection stored (encrypted) in `connections` table

**Status (in progress):** Adapter interface + Postgres adapter (CRUD, schema, PK/FK introspection,
native triggers via LISTEN/NOTIFY, realtime subscribe, capability flags) are done and tested against a
real Postgres in Docker. A **BYODB path for Phase 1 is already working end-to-end**: save-connection
(test-before-save, auto-detect engine, envelope-encrypt credentials → SCHEMA.md §2), plus `GET
/collections`, `GET /collections/{name}` (schema) and `POST /query` (rows + filters) endpoints all route
through the adapter (`internal/engine`, `internal/secrets`, `internal/server/data_handlers.go`). The
dashboard's **Tables** tab browses a connected database's tables and rows. What remains is **provisioned
mode** (spinning up the dedicated container) — see the next points below.

**Remaining work (next, one at a time):**
1. Add `ProvisionerInterface` (ARCHITECTURE.md §2.7) behind Docker; project creation in `provisioned`
   mode calls it to launch a dedicated Postgres container, stores its connection (encrypted, like BYODB)
   and marks it `connected`.
2. Wire the "Database type" picker (provisioned / BYODB) into the dashboard project-creation flow
   (`CreateProjectForm`) and the `POST /projects` + `POST /projects/{id}/connections` handlers.
3. Add a `GET /projects/{id}/connections` status-refresh + `DELETE` (disconnect) endpoint so a misconfigured
   BYODB string can be replaced without a fresh project.
4. (Test) Provisioning tests boot a real container via Docker and assert the table browser works over it.

**Done when:** a user creates a project, gets a provisioned Postgres instance, and can view/query its tables through the dashboard.

## Phase 2 — Second Adapter (FerretDB) — Prove the Abstraction Actually Generalizes
**Goal:** This is the real test of whether the adapter pattern was designed correctly. If adding FerretDB requires touching feature code outside the adapter itself, the interface needs rework before adding more engines.

- [ ] Implement FerretDB adapter against the same `DatabaseAdapter` interface
- [ ] Fill in its `Capabilities()` honestly (see matrix in `ADAPTERS.md` §2)
- [ ] Dashboard's table browser must work unmodified against both adapters
- [ ] Database picker in project creation flow (dropdown: Postgres / FerretDB for now)

**Done when:** the exact same dashboard code browses both a Postgres-backed and a FerretDB-backed project, with document-model differences handled gracefully (no crashing on missing foreign key info, etc.)

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

## Phase 6 — BYODB Mode
**Goal:** Users can connect an existing database instead of provisioning one.

- [ ] Connection string input + auto-detection (`SCHEMA.md` §3)
- [ ] Test-connection-before-save flow
- [ ] Encrypted credential storage wired to real vault/KMS (not the earlier basic encryption)
- [ ] All existing features (browser, schema explorer, triggers, realtime) work identically over a BYODB connection, gated by the same capability flags

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
