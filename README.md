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
| `DEVELOPMENT.md` | How to run the API + dashboard locally, config, and tests |
| `dashboard/` | Next.js + TypeScript + Tailwind web dashboard |

## 4. Supported Databases (V1 target list)

All chosen for **permissive, redistribution-safe licenses** — no SSPL, no BSL restrictions on hosting as part of a product.

| Category | Engine | License |
|---|---|---|
| Relational | PostgreSQL | PostgreSQL License (BSD-style) |
| Relational | MySQL Community | GPLv2 |
| Document (Mongo-compatible) | FerretDB (on Postgres) | Apache 2.0 |
| Key-Value / Cache | Valkey (Redis fork) | BSD |
| Graph | ArcadeDB | Apache 2.0 |
| Vector (AI/embeddings) | Qdrant or Chroma | Apache 2.0 |

BYODB mode additionally accepts connection strings to **any** database the adapter layer supports, including ones the platform doesn't provision itself (e.g. a user's existing real MongoDB Atlas cluster) — the platform just connects, it doesn't redistribute that engine, so licensing concerns don't apply there.

## 5. High-Level Roadmap (see PHASES.md for detail)

1. **Phase 0** — Platform metadata service, auth, org/project CRUD, single adapter (Postgres) only
2. **Phase 1** — Adapter interface hardening + second adapter (FerretDB) to prove the abstraction
3. **Phase 2** — Visual schema explorer + auto-generated REST API
4. **Phase 3** — Triggers + runtime functions
5. **Phase 4** — Realtime layer
6. **Phase 5** — BYODB connection mode
7. **Phase 6** — Remaining adapters (Valkey, graph, vector) + polish

> **Status:** Phase 0 is **complete** (API + metadata store + auth + encryption + Next.js dashboard).
> Phase 1 is **complete**: Postgres adapter, provisioned-mode DB creation (Docker-backed), a working
> BYODB path, and a dashboard table browser. Both connection modes are verified end-to-end. Next up is
> Phase 2 (FerretDB adapter to prove the abstraction generalizes). See `PHASES.md` for the live checklist.
