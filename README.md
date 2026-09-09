# Open Multi-Database Backend Platform

## 1. Vision

An open-source, self-hosted backend platform (Firebase/Supabase/Appwrite category) that does **not** lock users into a single database engine. A user creates an account → creates an **Organization** → creates a **Project** → and inside a Project chooses:

- **Provisioned mode** — the platform spins up a database instance for them (Postgres, FerretDB, Valkey, etc.), or
- **Bring-Your-Own-Database (BYODB) mode** — the user supplies an existing connection string + credentials, and the platform connects to it instead of hosting anything.

On top of whichever database is chosen, the platform provides a common layer of services:

- Authentication & user management
- Auto-generated REST/GraphQL APIs
- Visual schema/relationship explorer (tables, foreign keys, primary keys — like Supabase's table editor)
- Triggers (visual, database-agnostic where possible)
- Serverless/runtime functions
- Realtime data sync (where the underlying engine supports it)
- Role-based access control per project

**V1 scope is intentionally narrow**: open source, self-hosted only, no cloud-hosted/billing layer. This sidesteps the multi-tenant infrastructure question and the SSPL/licensing minefield (see `LICENSING_NOTES.md`) until the product itself is proven.

## 2. Core Design Principle: Database-Agnostic by Adapter

Every feature (auth, triggers, realtime, API generation, visualization) talks to an internal **Universal Data Interface**, never to a specific database driver directly. Each supported database ships as a thin **Adapter** that implements that interface and **declares its own capabilities** (e.g. "supports native realtime: yes/no", "supports triggers: yes/via polling/no"). See `ADAPTERS.md` for full detail — this is the most important document in this set, read it first when implementing anything cross-database.

## 3. Repository / Doc Map

| File | Purpose |
|---|---|
| `README.md` | This file — vision, scope, roadmap |
| `ARCHITECTURE.md` | Tech stack choices and system architecture |
| `ADAPTERS.md` | The database abstraction layer spec (adapter pattern, capability flags) |
| `SCHEMA.md` | Platform's own metadata database schema (users, orgs, projects, connections) |
| `LICENSING_NOTES.md` | Which databases are safe to bundle/redistribute, and why |
| `PHASES.md` | Phase-by-phase build roadmap with concrete deliverables per phase |
| `SDK_PLAN.md` | Detailed plan for the end-user auth system + `@openbase/js` npm SDK (expands `PHASES.md` Phase 10 + §12.7) |
| `DEVELOPMENT.md` | How to run the API + dashboard locally, config, and tests |
| `dashboard/` | Next.js + TypeScript + Tailwind web dashboard |

## 4. Supported Databases (V1 target list)

All chosen for **permissive, redistribution-safe licenses** — no SSPL, no BSL restrictions on hosting as part of a product.

| Category | Engine | License | Adapter |
|---|---|---|---|
| Relational | PostgreSQL | PostgreSQL License (BSD-style) | ✅ implemented |
| Relational | MySQL Community | GPLv2 | ✅ implemented |
| Document (Mongo-compatible) | FerretDB (on Postgres) | Apache 2.0 | ✅ implemented |
| Key-Value / Cache | Valkey (Redis fork) | BSD | ✅ implemented |
| Graph | ArcadeDB | Apache 2.0 | ✅ implemented (document-model surface) |
| Vector (AI/embeddings) | Qdrant or Chroma | Apache 2.0 | ✅ Qdrant implemented (Chroma planned) |

BYODB mode additionally accepts connection strings to **any** database the adapter layer supports, including ones the platform doesn't provision itself (e.g. a user's existing real MongoDB Atlas cluster) — the platform just connects, it doesn't redistribute that engine, so licensing concerns don't apply there.

## 5. High-Level Roadmap (see PHASES.md for detail)

1. **Phase 0** — Platform metadata service, auth, org/project CRUD (complete)
2. **Phase 1** — Adapter interface + first adapter (Postgres), provisioned-mode creation, table browser (complete)
3. **Phase 2** — Second adapter (FerretDB) to prove the abstraction generalizes (complete)
4. **Phase 3** — Visual schema explorer + auto-generated REST API + API keys (complete)
5. **Phase 4** — Triggers + runtime functions (complete)
6. **Phase 5** — Realtime layer (complete)
7. **Phase 6** — BYODB hardening (core flow pulled forward into Phase 1) (complete)
8. **Phase 7** — Remaining adapters (Valkey ✅, MySQL ✅, Qdrant ✅, ArcadeDB ✅, Chroma) + polish (in progress — Valkey, MySQL, Qdrant, ArcadeDB, function-sandbox hardening done)

**Part II — Supabase parity (Phases 8–18, planned).** Phases 0–7 built the part Supabase doesn't
have: a capability-honest six-engine adapter layer. Part II closes the gap on what Supabase does
have, audited feature-by-feature against its published catalogue. In dependency order: **8** remediation
(live defects, RBAC/scope enforcement, connection pooling), **9** platform accounts & orgs (sessions,
email, MFA, invites, audit log), **10** end-user auth per project (the GoTrue equivalent — the largest
structural gap), **11** authorization/RLS equivalent (native pushdown for Postgres+MySQL,
platform-side enforcement for the rest), **12** data-API parity + client SDKs, **13** table editor &
DDL, **14** storage, **15** functions v2 + cron + queues, **16** realtime v2 (broadcast, presence,
polling tier), **17** observability & advisors, **18** backups, migrations, branching, CLI.
See `PHASES.md` Part II for the per-step plan and the full gap inventory.

> **Status:** Phases 0–6 are **complete** (API + metadata store + auth + encryption + Next.js
> dashboard; Postgres + FerretDB adapters with provisioned + BYODB modes; a React Flow schema
> explorer diagramming tables & FK relationships, capability-gated; per-project API keys powering an
> auto-generated REST API `GET/POST/PUT/DELETE /v1/api/{collection}`; a visual trigger builder +
> sandboxed Node function runtime, where table changes fire webhooks/functions via native
> Postgres LISTEN/NOTIFY; a realtime WebSocket gateway `GET /v1/realtime` with a browser
> live-updating-list SDK/demo; and BYODB hardening — multi-key envelope encryption with key
> rotation (`encryption_key_id`-keyed decrypt) plus honest capability gating for triggers/realtime).
> Phase 7 progress: a Valkey key-value adapter (`redis://` auto-detect, collections as JSON sets,
> honest capabilities — no FKs/joins/triggers/realtime, all `ErrUnsupported`); a MySQL adapter
> (`mysql://` auto-detect, native CRUD/schema/joins/transactions/full-text, with trigger/realtime
> delivery honestly gated off until a queue-table + polling tier lands); a Qdrant vector adapter
> (`http(s)://…:6333` auto-detect, points+payloads exposed as rows/columns, `SupportsVectorSearch`
> honest with no joins/triggers/realtime); an ArcadeDB adapter (`http(s)://…:2480`/`bolt://`
> auto-detect, document-model: types as collections + full CRUD/schema/filtering via its SQL-style
> REST API, `SupportsRelationalJoins:false` — graph/traversal surface is a documented follow-up);
> and a function-sandbox
> security audit (process-group isolation so descendant processes die on timeout, plus bounded
> stdout/stderr capture so a runaway function can't exhaust host memory).
> External clients can hit their project's generated REST endpoints with an
> API key and attach webhook/function automation to data changes. See `PHASES.md` for the live
> checklist.
