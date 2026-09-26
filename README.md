# Openbase

**An open-source, self-hosted backend platform that is not locked to one database engine.**

Most backend-as-a-service products pick a database for you. Openbase lets you pick — per project —
from a relational, document, key-value, graph or vector engine, and then gives you the same managed
layer on top of whichever one you chose: auth, an auto-generated API, a visual schema explorer,
triggers, functions, realtime, and role-based access control.

```
Operator  →  sign up  →  Organization  →  Project  →  pick a database  →  build
```

A project gets its database one of two ways:

- **Provisioned** — Openbase runs the engine for you in a dedicated container.
- **BYODB (Bring Your Own Database)** — you paste a connection string to a database you already run,
  and Openbase connects to it instead. Nothing is provisioned, nothing is redistributed.

On top of either mode you get one common surface:

| Capability | What it means today |
|---|---|
| **Multi-engine adapters** | Postgres, MySQL, FerretDB, Valkey, ArcadeDB, Qdrant behind one internal interface |
| **Honest capabilities** | Every adapter *declares* what it supports; the UI hides or explains what it can't do — it never fakes a feature |
| **Auth (operator)** | Accounts, organizations, roles, sessions + refresh tokens, TOTP MFA, email invites, audit log |
| **Auth (end users)** | Per-project user identity, ES256 signing keys + public JWKS, email/password, magic link, OTP, OAuth/OIDC with PKCE, MFA, auth hooks, admin/impersonation API |
| **Auto-generated REST API** | `GET/POST/PUT/DELETE /v1/api/{collection}` scoped by per-project API keys (`anon`, `authenticated`, `service_role`) |
| **Visual schema explorer** | React Flow ERD of tables and FK relationships, capability-gated per engine |
| **Triggers** | Visual builder: on insert/update/delete → webhook (HMAC-signed) or serverless function |
| **Functions** | Sandboxed Node.js runtime: process-group isolation, bounded output capture, no host env leakage |
| **Realtime** | WebSocket gateway at `GET /v1/realtime`; native change delivery for Postgres via `LISTEN`/`NOTIFY` |
| **SQL editor + query API** | Schema-aware SQL editor and a capability-gated raw query path |
| **JS SDK** | `@openbase/js` — `createClient(url, key)` → `auth` + `from(table)` + `channel`, with an SSR entry for Next.js |

**Scope of V1:** open source and self-hosted only. No cloud tier, no billing, no multi-tenant
infrastructure. That keeps the licensing story clean (see [`LICENSING_NOTES.md`](LICENSING_NOTES.md))
while the product gets proven. The table above is what ships today; [`PHASES.md`](PHASES.md) tracks
what is being built next, phase by phase, with live status.

---

## Supported engines

All chosen for permissive, redistribution-safe licenses — no SSPL, no source-available restrictions on
hosting them as part of a product.

| Category | Engine | License | Adapter status | Provisioned? |
|---|---|---|---|---|
| Relational | PostgreSQL | PostgreSQL License (BSD-style) | implemented | yes |
| Relational | MySQL Community | GPLv2 | implemented | BYODB only |
| Document (Mongo wire) | FerretDB (on Postgres) | Apache 2.0 | implemented | yes |
| Key-value / cache | Valkey (Redis fork) | BSD | implemented | BYODB only |
| Graph | ArcadeDB | Apache 2.0 | implemented (document-model surface) | BYODB only |
| Vector | Qdrant | Apache 2.0 | implemented | BYODB only |
| Vector | Chroma | Apache 2.0 | planned | — |

BYODB accepts a connection string to **any** engine the adapter layer supports, including ones the
platform never provisions (your own MongoDB Atlas, your own Redis, …) — in that mode Openbase is
simply a client, so redistribution licensing does not apply.

The exact per-engine feature matrix (joins, foreign keys, triggers, realtime, transactions, vector
search) lives in `ADAPTERS.md` §2 and is mirrored in each adapter's `Capabilities()`.

---

## Quick start

### With Docker (fastest)

Boots the platform API, its metadata Postgres and the web dashboard:

```bash
git clone https://github.com/openbase/openbase.git
cd openbase
make dev            # or: docker compose up --build
```

| Service | URL |
|---|---|
| Dashboard | http://localhost:3000 |
| API | http://localhost:8080 |
| Health / readiness / metrics | `/healthz`, `/readyz`, `/metrics` |

Then: create an account → create an organization → create a project → add a database under
**DB Source** (paste a connection string, or enable provisioned mode) → use the **Connect** tab for
your API key, REST endpoints and the realtime URL.

`docker compose` leaves provisioned mode **off** (the API container has no Docker socket). To turn it
on, mount the host Docker socket into the `api` service, install the Docker CLI in its image and set
`OPENBASE_PROVISIONER_ENABLED=true`. For plain local development, run the API on the host with that
variable set and any reachable Docker daemon.

### Without Docker for the whole stack

```bash
# 1) metadata Postgres
docker run -d --name openbase-meta -p 5432:5432 \
  -e POSTGRES_USER=openbase -e POSTGRES_PASSWORD=openbase -e POSTGRES_DB=openbase \
  postgres:16-alpine

# 2) platform API (applies migrations on startup)
OPENBASE_DATABASE_URL="postgres://openbase:openbase@localhost:5432/openbase?sslmode=disable" \
OPENBASE_ENCRYPTION_KEY="some-strong-local-key" \
go run ./cmd/server

# 3) dashboard
cd dashboard && npm install && npm run dev     # http://localhost:3000
```

---

## Development environment

### Prerequisites

| Tool | Version | Needed for |
|---|---|---|
| Go | 1.26+ | platform API, adapters, tests |
| Node | 22+ | dashboard and `@openbase/js` |
| Docker | any | metadata Postgres, test databases, provisioned mode |
| PostgreSQL | 16 | the platform's own metadata store (a container is fine) |

### Repository layout

```
cmd/server/          API entrypoint; wires config, migrations, adapters, runtime
internal/
  adapter/           Universal Data Interface + one package per engine
  engine/            Adapter factory + connection-string auto-detection
  server/            HTTP handlers, middleware (auth, API keys, RBAC, CORS)
  metadata/          Metadata store (SQL over the platform's own Postgres)
  projectauth/       End-user identity: passwords, tokens, providers, MFA
  auth/  apikey/  crypto/  secrets/  mfa/  sms/  mail/
  pool/  provision/  realtime/  triggers/  function/  testutil/
migrations/          0001…0013 — schema history, one file per change, applied on boot
dashboard/           Next.js 15 + React 19 + TypeScript + Tailwind web app
sdk/js/              @openbase/js — the published JavaScript SDK
```

### Configuration

Everything is environment-driven (`internal/config`). The essentials:

| Env var | Purpose | Default |
|---|---|---|
| `OPENBASE_ADDR` | API listen address | `:8080` |
| `OPENBASE_DATABASE_URL` | Platform metadata Postgres | local dev URL |
| `OPENBASE_JWT_SECRET` | Operator JWT signing secret | dev value (rejected when `ENV=production`) |
| `OPENBASE_ENCRYPTION_KEY` | Envelope-encryption master key for connection secrets | empty ⇒ secrets disabled |
| `OPENBASE_ENCRYPTION_KEY_ID` / `OPENBASE_ENCRYPTION_KEYS` | Key id + rotation registry | `openbase-master-key-v1` / empty |
| `OPENBASE_ALLOWED_ORIGINS` | CORS allow-list (comma-separated) | empty ⇒ any origin |
| `OPENBASE_PUBLIC_URL` | Externally reachable API origin advertised on the Connect tab | derived from `Host` |
| `OPENBASE_PROVISIONER_ENABLED` | Enable provisioned DB mode (needs a Docker daemon) | `false` |
| `OPENBASE_ALLOW_PRIVATE_WEBHOOKS` | Allow webhook triggers to target private/LAN addresses | `false` |
| `NEXT_PUBLIC_OPENBASE_API_URL` | Dashboard → API base URL (browser) | `http://localhost:8080` |

The full table, the two connection directions (inbound **DB Source** vs outbound **Connect**), the
org/project endpoint reference with minimum roles, and adapter-pooling rules are in
[`DEVELOPMENT.md`](DEVELOPMENT.md).

### Make targets

| Command | What it does |
|---|---|
| `make dev` / `make up` / `make down` | Boot / start detached / stop the compose stack |
| `make build` | Build `bin/openbase-server` |
| `make test` | `go test ./...` |
| `make test-short` | `go test -short ./...` |
| `make vet` | `go vet ./...` |
| `make lint` | `go vet` + dashboard typecheck + SDK typecheck |
| `make clean` | Remove `bin/` |

### Tests

```bash
go test ./...                          # Go: real Postgres in Docker, skips if unavailable
cd dashboard && npm test               # vitest
cd dashboard && npm run typecheck
cd sdk/js && npm test && npm run typecheck
```

The dashboard has no ESLint configuration yet, so `npm run lint` there opens Next's interactive
setup prompt — use `tsc --noEmit` (wired into `make lint`) until one is added.

There are no database mocks for the DB layer: Go tests spin up a real Postgres via `internal/testutil`
and skip gracefully without Docker. The deliberate exceptions use in-process fakes so those adapters
stay covered even without Docker — `miniredis` for Valkey, `go-sqlmock` for MySQL, and `httptest`
fakes for Qdrant and ArcadeDB.

---

## Architecture in one paragraph

The platform has its own Postgres metadata store (users, orgs, projects, connections, API keys,
triggers, functions, audit events) and never stores credentials in plaintext: every connection secret
is envelope-encrypted with a per-value key wrapped by a master key, and a `KeyProvider` registry makes
key rotation possible. Every data-path feature talks to the **Universal Data Interface**
(`internal/adapter`), never to a driver directly; each engine ships a thin adapter that translates
the platform's query IR into native syntax and reports an honest `CapabilitySet`, which the API and
dashboard both read before rendering a feature. Project connections are pooled (one live adapter per
connection, idle-evicted after 5 minutes) and any code path that repoints a project must invalidate
its pooled adapters. Full detail: [`ARCHITECTURE.md`](ARCHITECTURE.md) and
[`ADAPTERS.md`](ADAPTERS.md).

---

## Documentation

| File | Read it for |
|---|---|
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | Tech stack decisions, deployment model, where end-user identity lives (§2.8) |
| [`ADAPTERS.md`](ADAPTERS.md) | **The adapter spec** — universal interface, capability flags, trigger tiers, adding a new engine (§4) |
| [`SCHEMA.md`](SCHEMA.md) | The platform's own metadata schema, encryption notes, connection-string auto-detection, RBAC role matrix |
| [`DEVELOPMENT.md`](DEVELOPMENT.md) | Running the stack, config, endpoint reference, adapter pooling, test setup |
| [`DESIGN.md`](DESIGN.md) | Dashboard design system: tokens, typography, components, do's and don'ts |
| [`PHASES.md`](PHASES.md) | The build roadmap — per-phase deliverables with live status |
| [`LICENSING_NOTES.md`](LICENSING_NOTES.md) | Why these engines were chosen and what is safe to bundle |
| [`sdk/js/README.md`](sdk/js/README.md) | `@openbase/js` usage and its own development commands |

Start with `ADAPTERS.md` if you are touching anything cross-database, and `PHASES.md` if you want to
know what is planned next.

---

## Contributing

Contributions are welcome — adapters, dashboard work, docs, bug reports.

**Before you start**

1. Open an issue (or a draft PR) describing the problem, so we can agree on scope before you build it.
   For anything that changes the adapter interface, capability flags, or the metadata schema, wait
   for a quick sign-off — those are the load-bearing contracts of the platform.

**Ground rules for code**

- **Go through the adapter.** Feature code must not import a database driver. If the interface in
  `ADAPTERS.md` doesn't expose what you need, extend the interface (and the optional interfaces like
  `RawQuerier`) instead of branching on an engine in a handler.
- **Capabilities stay honest.** If a feature genuinely doesn't work on an engine, declare
  `false`/`none` in `Capabilities()` and return `ErrUnsupported`, and let the UI hide or explain the
  gap. A missing button beats a visible button that silently does nothing — and never gate a control
  on a capability an adapter doesn't actually report.
- **Tests go with the change.** Go tests live next to the code as `*_test.go`; dashboard and SDK use
  vitest. Prefer real databases in Docker; use an in-process fake only where a live engine is
  genuinely unavailable, and say so in a comment.
- **Keep the two directions separate.** Inbound (Openbase → your database) is operator-authenticated;
  outbound (your app → Openbase) is API-key or end-user-token authenticated. Don't blur them.

**Adding a new engine adapter**

1. Check the license first (`LICENSING_NOTES.md`).
2. Work through the checklist in `ADAPTERS.md` §4: implement `internal/adapter/<engine>`, register
   it with the factory, add connection-string auto-detection, fill in `Capabilities()` from the
   matrix, add tests, and add a BYODB test connection path.

**Local checks before you open a PR**

```bash
make lint        # go vet + dashboard typecheck + SDK typecheck
make test        # go test ./...
cd dashboard && npm test
cd sdk/js && npm test
```

**Style**

Follow the surrounding code. Go is `gofmt`-clean with doc comments on exported symbols; TypeScript
matches the existing strict style. Keep comments about *why*, not *what* — the codebase already
documents a lot of non-obvious decisions inline, and that is a good habit to keep.

**Reporting bugs**

Include the Openbase commit, the engine and connection mode, the API/dashboard versions, and the
minimal reproduction. Security issues: do not open a public issue — contact the maintainers privately.

---

## License

The dashboard and SDK packages are published under Apache-2.0. No top-level `LICENSE` file has been
added to the repository yet; until it is, treat the code as all-rights-reserved and ask before
redistributing.

For the licenses of the database engines themselves — and why Openbase bundles FerretDB instead of
MongoDB, and Valkey instead of Redis — see [`LICENSING_NOTES.md`](LICENSING_NOTES.md).
