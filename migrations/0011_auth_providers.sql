-- Phase 10 (A2): per-project identity provider credentials + OAuth state.
--
-- Provider credentials (OAuth client secrets, SMS keys) live here
-- envelope-encrypted — never in the settings JSONB and never in logs.
-- The `provider` key covers oauth drivers (github, google, oidc) and the sms
-- driver (config.driver selects log vs twilio).

CREATE TABLE IF NOT EXISTS project_auth_providers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    client_id           TEXT NOT NULL DEFAULT '',
    client_secret_encrypted BYTEA NOT NULL DEFAULT ''::bytea,
    encryption_key_id   TEXT NOT NULL DEFAULT '',
    config              JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, provider)
);

-- OAuth login states: CSRF protection + PKCE challenge storage. On callback
-- the row gains a one-time auth_code (hashed) bound to the resolved user;
-- the client exchanges it at POST /auth/v1/token?grant_type=pkce.
CREATE TABLE IF NOT EXISTS project_oauth_states (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id              UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider                TEXT NOT NULL,
    state_hash              TEXT NOT NULL UNIQUE,
    redirect_to             TEXT NOT NULL DEFAULT '',
    code_challenge          TEXT NOT NULL DEFAULT '',
    code_challenge_method   TEXT NOT NULL DEFAULT 'plain',
    auth_code_hash          TEXT,
    user_id                 UUID REFERENCES project_users(id) ON DELETE CASCADE,
    expires_at              TIMESTAMPTZ NOT NULL,
    used_at                 TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_project_oauth_states_hash ON project_oauth_states(state_hash);
CREATE INDEX IF NOT EXISTS idx_project_auth_providers_project ON project_auth_providers(project_id);
