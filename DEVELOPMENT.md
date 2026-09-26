# Development

Quick guide to running the Openbase stack locally.

## Prerequisites

- Go 1.26+
- Node 22+ (for the dashboard)
- Docker (metadata DB + provisioned/test databases)

## Fastest path: docker compose

Boots the platform API, its metadata Postgres, and the dashboard together:

```bash
make dev          # or: docker compose up --build
# API:      http://localhost:8080
# Dashboard: http://localhost:3000
```

## Local, without containers for the whole stack

Run the metadata DB, then the API, then the dashboard:

```bash
# 1) metadata Postgres (docker)
docker run -d --name openbase-meta -p 5432:5432 \
  -e POSTGRES_USER=openbase -e POSTGRES_PASSWORD=openbase -e POSTGRES_DB=openbase \
  postgres:16-alpine

# 2) platform API
OPENBASE_DATABASE_URL="postgres://openbase:openbase@localhost:5432/openbase?sslmode=disable" \
OPENBASE_ENCRYPTION_KEY="some-strong-local-key" \
go run ./cmd/server
# listens on :8080, applies DB migrations on startup

# 3) dashboard (separate terminal)
cd dashboard
npm install
npm run dev        # http://localhost:3000
```

## Configuration

All config is env-driven (`internal/config`). The most important:

| Env var | Purpose | Default |
|---|---|---|
| `OPENBASE_ADDR` | API listen address | `:8080` |
| `OPENBASE_DATABASE_URL` | Platform metadata Postgres | `postgres://openbase:openbase@localhost:5432/openbase?sslmode=disable` |
| `OPENBASE_JWT_SECRET` | JWT signing secret | dev default (reject in production) |
| `OPENBASE_JWT_ISSUER` / `OPENBASE_JWT_AUDIENCE` | Operator token `iss` / `aud` claims | `openbase` / `openbase-dashboard` |
| `OPENBASE_ENCRYPTION_KEY` | Envelope-encryption master key (current key) for connection secrets | empty ⇒ secrets disabled |
| `OPENBASE_ENCRYPTION_KEY_ID` | Name/id of the current encryption key, stamped on new rows | `openbase-master-key-v1` |
| `OPENBASE_ENCRYPTION_KEYS` | Key-rotation registry of historical keys, `id=secret,id=secret`, kept decodable | empty |
| `OPENBASE_ALLOWED_ORIGINS` | CORS allow-list (comma-separated); empty = any origin | empty |
| `OPENBASE_PUBLIC_URL` | Externally-reachable API origin advertised on the Connect tab; empty derives it from `Host`/`X-Forwarded-*` | empty |
| `OPENBASE_PROVISIONER_ENABLED` | Enable "provisioned" DB mode (needs a Docker daemon) | `false` |
| `OPENBASE_ALLOW_PRIVATE_WEBHOOKS` | Allow webhook triggers to target loopback/private/LAN addresses; default is a public-http(s)-only SSRF deny-list | `false` |
| `ENV` | Set to `production` to refuse the dev-default `OPENBASE_JWT_SECRET` | empty |
| `NEXT_PUBLIC_OPENBASE_API_URL` | Dashboard → API base URL (browser) | `http://localhost:8080` |
| `OPENBASE_API_INTERNAL_URL` | Dashboard → API base URL (server-side proxy / rewrites) | `http://localhost:8080` |

## Two connection directions

The dashboard separates them into two project tabs, and it is worth keeping the
distinction straight when working on either:

| Direction | Tab | Auth | Endpoints |
|---|---|---|---|
| Openbase → your database (inbound) | **DB Source** | dashboard JWT + org membership | `/v1/projects/{id}/connections` |
| Your app → Openbase (outbound) | **Connect** | project API key (`Authorization: Bearer ob_…`) | `/v1/api/*`, `/v1/realtime` |

`GET /v1/projects/{id}/connect-info` backs the Connect tab: it returns the public
API URL, the realtime URL, the literal endpoint paths, the connected engine's
capabilities and a count of active API keys. It never returns database
credentials — provisioned credentials stay encrypted server-side, and BYODB
strings belong to the user's own provider (SCHEMA.md §2). Behind a reverse
proxy, set `OPENBASE_PUBLIC_URL` so the snippets it renders are copy-pasteable.

## Org, member and project management endpoints

All dashboard-authenticated (JWT). The **min role** column is enforced by
`authorizeOrgRole` / `projectAndOrgRole`; the full matrix lives in SCHEMA.md §5.

| Method | Path | Min role | Notes |
|---|---|---|---|
| `GET` | `/v1/orgs` | — | Each row carries the caller's `role` |
| `POST` | `/v1/orgs` | — | Creator becomes owner |
| `GET` | `/v1/orgs/{orgID}` | member | Includes `role`, so the UI needs no `/me` round-trip |
| `PATCH` | `/v1/orgs/{orgID}` | admin | `{name?, slug?}`; duplicate slug → 409 |
| `DELETE` | `/v1/orgs/{orgID}` | owner | Body `{slug}` must match; 409 while any project exists |
| `GET` | `/v1/orgs/{orgID}/members` | member | `[]OrgMember` with email + full_name, ordered by role rank |
| `POST` | `/v1/orgs/{orgID}/members` | admin | `{email, role}`; 404 unknown account, 409 duplicate, 403 if a non-owner grants owner |
| `PATCH` | `/v1/orgs/{orgID}/members/{userID}` | admin | `{role}`; last-owner demote → 409; owner transitions owner-only |
| `DELETE` | `/v1/orgs/{orgID}/members/{userID}` | member (self) / admin (others) | Self = leave; last owner → 409 |
| `POST` | `/v1/orgs/{orgID}/transfer-ownership` | owner | `{user_id, demote_self?}` in one transaction |
| `GET` | `/v1/orgs/{orgID}/invites` | admin | Pending invites issued by this org |
| `POST` | `/v1/orgs/{orgID}/invites` | admin | `{email, role}`; mails an accept link via `internal/mail` (Phase 9.7) |
| `DELETE` | `/v1/orgs/{orgID}/invites/{inviteID}` | admin | Revoke a pending invite |
| `POST` | `/v1/invites/accept` | — | Public (token-authed): joins the caller to the org as the invited role |
| `GET` | `/v1/orgs/{orgID}/audit` | admin | Audit trail for the org |
| `POST` | `/v1/orgs/{orgID}/projects` | admin | |
| `GET` | `/v1/orgs/{orgID}/projects` | member | |
| `GET` | `/v1/projects/{projectID}` | member | Single-project resolver |
| `PATCH` | `/v1/projects/{projectID}` | admin | `{name?, slug?}`; slug unique per org → 409 |
| `DELETE` | `/v1/projects/{projectID}` | owner | Destroys the provisioned container first, then cascades |

Two conventions worth knowing before adding endpoints here:

- **403 bodies name the required role.** `authorizeOrgRole` wraps `errForbidden`
  with `"%s role required"`, and `writeErr` preserves that message, so the
  dashboard can render a real reason. Do not flatten it back to `"forbidden"`.
- **A blocked invariant is 409, not 403.** The last-owner guard and the
  "org still has projects" check are state conflicts: the caller *has* the
  permission. Reserve 403 for genuine permission failures.

Adding a member directly requires an existing account, so `POST /v1/orgs/{orgID}/members`
returns 404 for an address nobody has registered with. For an unknown email use the invite
flow instead (`POST /v1/orgs/{orgID}/invites` → the invitee accepts at `GET /invite` with
the mailed token → `POST /v1/invites/accept`), which is wired to the dashboard-managed
BYOC SMTP mailer in `internal/mail` (Phase 9.2/9.7).

## Adapter pooling

Project database connections are pooled: one live adapter per project
connection, shared across requests, idle-evicted after 5 minutes
(`internal/server/adapter_pool.go`, on top of the generic `internal/pool`). The
practical consequences when working on handlers:

- `connectProject` / `connectProjectForAPIKey` return a **shared** adapter whose
  `Disconnect` is a no-op. Keep writing `defer a.Disconnect(ctx)` — it stays
  correct and costs nothing.
- Any code path that changes where a project points must call
  `invalidateProjectAdapters(projectID)`. `saveConnection`, `deleteConnection`
  and `deleteProject` already do.
- Optional adapter interfaces (today `adapter.RawQuerier`) must be forwarded by
  `pooledAdapter` explicitly; a type assertion cannot see through an embedded
  interface. See ADAPTERS.md §7.
- Realtime and the trigger runtime deliberately dial their own adapters, because
  a Postgres `LISTEN` needs its own session for the life of the subscription.

`server.New` therefore returns a `server.Handler` (an `http.Handler` plus
`Close()`); call `Close()` on shutdown so pooled sessions are released.

## Tests

All Go tests hit a **real Postgres in Docker** (`internal/testutil` spins one up and
skips gracefully when Docker is unavailable). No mocks for the DB layer (ADAPTERS.md §4).

The **Valkey adapter** is the one deliberate exception: its tests use
[`miniredis`](https://github.com/alicebob/miniredis) (an in-process Redis-protocol server),
so the key-value adapter is fully covered even without a Docker daemon or an external Redis.

The **MySQL adapter** is covered at the logic/SQL-generation level with
[`go-sqlmock`](https://github.com/DATA-DOG/go-sqlmock) (verifies the exact SQL, placeholders and
identifier validation the adapter emits) — this needs no live MySQL and no Docker. Live
integration testing against a real MySQL/MariaDB remains a documented follow-up.

The **Qdrant adapter** is exercised against an in-process [`httptest`](https://pkg.go.dev/net/http/httptest)
fake of the Qdrant REST API (an in-memory point store), so the HTTP client adapter is fully covered
without a live Qdrant or Docker. Live integration against a real Qdrant remains a follow-up.

The **ArcadeDB adapter** follows the same pattern: an [`httptest`](https://pkg.go.dev/net/http/httptest)
fake parses the adapter's emitted SQL-style commands over an in-memory document store, so the
document-model adapter is fully covered without a live ArcadeDB or Docker. Live integration against
a real ArcadeDB (and graph-traversal follow-up) remains a follow-up.

```bash
go test ./...     # full suite (provisioning tests need the Docker daemon and skip if absent)
make test-short   # -short variant
```

RBAC and lifecycle behaviour is covered by `internal/server/rbac_test.go` (the
matrix as a table), plus `org_settings_test.go`, `members_test.go` and
`project_settings_test.go`. The cascade test uses a stub provisioner and asserts
`Destroy` received the **container** id, that all four child tables are emptied,
and that a previously valid API key then 401s on `/v1/api/*`. A second stub whose
`Destroy` always fails pins the ordering guarantee: the project must survive.

Dashboard: `cd dashboard && npm test` (vitest) and `npm run build`.
`dashboard/lib/permissions.test.ts` is a line-by-line parity check against the Go
role matrix — if you change one, that test tells you to change the other.

### Provisioned databases from docker compose

`docker compose up` leaves provisioned mode off (the API container has no Docker
CLI/socket). To enable it in the compose stack you'd mount the host Docker socket
into the `api` service, add the Docker CLI to its image, and set
`OPENBASE_PROVISIONER_ENABLED=true`. See `docker-compose.yml`. For plain local
development, run the API on the host with the env var set and Docker any reachable
daemon.
