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
| `OPENBASE_ENCRYPTION_KEY` | Envelope-encryption master key for connection secrets | empty ⇒ secrets disabled |
| `OPENBASE_ALLOWED_ORIGINS` | CORS allow-list (comma-separated); empty = any origin | empty |
| `NEXT_PUBLIC_OPENBASE_API_URL` | Dashboard → API base URL (browser) | `http://localhost:8080` |

## Tests

All Go tests hit a **real Postgres in Docker** (`internal/testutil` spins one up and
skips gracefully when Docker is unavailable). No mocks for the DB layer (ADAPTERS.md §4).

```bash
go test ./...     # full suite
make test-short   # -short variant
```

Dashboard: `cd dashboard && npm run build`.
