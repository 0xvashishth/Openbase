-- Openbase platform metadata schema. Mirror of SCHEMA.md §1 plus the
-- recommended indexes from SCHEMA.md §4. Applied by the embedded migrator.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    full_name       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organizations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT UNIQUE NOT NULL,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE IF NOT EXISTS projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE TABLE IF NOT EXISTS connections (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    mode                TEXT NOT NULL CHECK (mode IN ('provisioned', 'byodb')),
    engine              TEXT NOT NULL CHECK (engine IN (
                            'postgres', 'mysql', 'ferretdb', 'valkey',
                            'arcadedb', 'qdrant', 'chroma', 'other'
                        )),
    container_id        TEXT,
    encrypted_conn_string BYTEA,
    encrypted_username     BYTEA,
    encrypted_password     BYTEA,
    encryption_key_id      TEXT,
    status                  TEXT NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'connected', 'error')),
    last_checked_at         TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS triggers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    collection      TEXT NOT NULL,
    event           TEXT NOT NULL CHECK (event IN ('insert', 'update', 'delete')),
    action_type     TEXT NOT NULL CHECK (action_type IN ('function', 'webhook')),
    action_target   TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS functions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    runtime         TEXT NOT NULL CHECK (runtime IN ('node', 'python')),
    source_ref      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    key_hash        TEXT NOT NULL,
    scopes          TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(organization_id);
CREATE INDEX IF NOT EXISTS idx_connections_project ON connections(project_id);
CREATE INDEX IF NOT EXISTS idx_triggers_project ON triggers(project_id);
CREATE INDEX IF NOT EXISTS idx_org_members_user ON organization_members(user_id);