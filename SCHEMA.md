# Platform Metadata Schema

This is the schema for the platform's **own** internal Postgres database — the one that tracks users, organizations, projects, and connection configs. This is separate and independent from whatever database each individual *project* uses for its actual application data.

## 1. Core tables

```sql
-- Users of the platform itself
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    full_name       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Organizations
CREATE TABLE organizations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT UNIQUE NOT NULL,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Many-to-many: users <-> organizations, with a role
CREATE TABLE organization_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

-- Projects belong to an organization
CREATE TABLE projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

-- Each project has exactly one primary database connection (v1);
-- multiple connections per project can be supported later by dropping the UNIQUE constraint.
CREATE TABLE connections (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    mode                TEXT NOT NULL CHECK (mode IN ('provisioned', 'byodb')),
    engine              TEXT NOT NULL CHECK (engine IN (
                            'postgres', 'mysql', 'ferretdb', 'valkey',
                            'arcadedb', 'qdrant', 'chroma', 'other'
                        )),
    -- for provisioned mode: internal container reference
    container_id        TEXT,
    -- for byodb mode: encrypted connection details (never store plaintext)
    encrypted_conn_string BYTEA,
    encrypted_username     BYTEA,
    encrypted_password     BYTEA,
    encryption_key_id      TEXT,   -- reference to which KMS/vault key encrypted this row
    status                  TEXT NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'connected', 'error')),
    last_checked_at         TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Triggers configured per project (definition only; execution is handled by the adapter engine)
CREATE TABLE triggers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    collection      TEXT NOT NULL,
    event           TEXT NOT NULL CHECK (event IN ('insert', 'update', 'delete')),
    action_type     TEXT NOT NULL CHECK (action_type IN ('function', 'webhook')),
    action_target   TEXT NOT NULL, -- function ID or webhook URL
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Runtime functions
CREATE TABLE functions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    runtime         TEXT NOT NULL CHECK (runtime IN ('node', 'python')),
    source_ref      TEXT NOT NULL, -- path/reference to stored source code
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-project API keys for external app access
CREATE TABLE api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    key_hash        TEXT NOT NULL,
    scopes          TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);
```

## 2. Encryption notes for `connections`

- `encrypted_conn_string`, `encrypted_username`, `encrypted_password` are encrypted application-side before insert (envelope encryption), not relying on Postgres-level encryption alone.
- `encryption_key_id` lets you rotate keys without needing to decrypt/re-encrypt every row in one pass.
- Never log these fields, even encrypted, in application logs.

## 3. Connection-string auto-detection (used by BYODB flow)

Mapping used to auto-select the adapter from a pasted connection string, referenced in `ADAPTERS.md` §5:

| URL scheme prefix | Detected engine |
|---|---|
| `postgres://`, `postgresql://` | postgres |
| `mysql://` | mysql |
| `mongodb://`, `mongodb+srv://` | ferretdb (Mongo-wire-compatible adapter) |
| `redis://`, `valkey://` | valkey |
| `bolt://` (or ArcadeDB-specific scheme) | arcadedb |
| `qdrant://`, `http(s)://` + Qdrant port convention | qdrant |

If the scheme is ambiguous (e.g. a bare `https://` for an HTTP-API-based database), fall back to asking the user to explicitly pick the engine from a dropdown rather than guessing.

## 4. Indexes worth adding early

```sql
CREATE INDEX idx_projects_org ON projects(organization_id);
CREATE INDEX idx_connections_project ON connections(project_id);
CREATE INDEX idx_triggers_project ON triggers(project_id);
CREATE INDEX idx_org_members_user ON organization_members(user_id);
```
