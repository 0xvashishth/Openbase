-- Phase 10 (A0.2): end-user identity per project (the GoTrue equivalent).
-- Per ARCHITECTURE.md §2.8, app users live in the platform metadata DB keyed
-- by project_id — never inside the project's own database. Only refresh-token
-- SHA-256 hashes and encrypted private keys are stored; a leaked database
-- never yields usable sessions or signing keys.

-- People who sign into a customer's app (one row per project membership).
CREATE TABLE IF NOT EXISTS project_users (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    email               TEXT,
    phone               TEXT,
    password_hash       TEXT NOT NULL DEFAULT '',
    email_confirmed_at  TIMESTAMPTZ,
    phone_confirmed_at  TIMESTAMPTZ,
    banned_until        TIMESTAMPTZ,
    is_anonymous        BOOLEAN NOT NULL DEFAULT false,
    user_metadata       JSONB NOT NULL DEFAULT '{}',
    app_metadata        JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (email IS NOT NULL OR phone IS NOT NULL OR is_anonymous),
    UNIQUE (project_id, email),
    UNIQUE (project_id, phone)
);

-- OAuth / OIDC / phone identities linked to a project user.
CREATE TABLE IF NOT EXISTS project_identities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES project_users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    provider_uid    TEXT NOT NULL,
    identity_data   JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, provider, provider_uid)
);

-- Opaque rotating refresh sessions (mirrors operator `sessions`, Phase 9.1).
CREATE TABLE IF NOT EXISTS project_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES project_users(id) ON DELETE CASCADE,
    refresh_hash    TEXT NOT NULL UNIQUE,
    user_agent      TEXT NOT NULL DEFAULT '',
    ip              TEXT NOT NULL DEFAULT '',
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_project_users_project ON project_users(project_id);
CREATE INDEX IF NOT EXISTS idx_project_users_email ON project_users(project_id, email) WHERE email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_project_identities_user ON project_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_project_sessions_user ON project_sessions(project_id, user_id);
CREATE INDEX IF NOT EXISTS idx_project_sessions_refresh_hash ON project_sessions(refresh_hash);

-- Per-project auth configuration (providers, redirect allow-list, policies).
CREATE TABLE IF NOT EXISTS project_auth_settings (
    project_id              UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    site_url                TEXT NOT NULL DEFAULT '',
    redirect_allow_list     TEXT[] NOT NULL DEFAULT '{}',
    password_min_length     INT NOT NULL DEFAULT 8,
    session_idle_timeout_s  INT NOT NULL DEFAULT 2592000,
    session_absolute_timeout_s INT NOT NULL DEFAULT 31536000,
    mfa_enabled             BOOLEAN NOT NULL DEFAULT true,
    providers               JSONB NOT NULL DEFAULT '{}',
    mail_templates          JSONB NOT NULL DEFAULT '{}',
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- TOTP factors + challenges for end-user MFA (aal1 -> aal2 step-up).
CREATE TABLE IF NOT EXISTS project_mfa_factors (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES project_users(id) ON DELETE CASCADE,
    factor_type     TEXT NOT NULL DEFAULT 'totp' CHECK (factor_type IN ('totp')),
    friendly_name   TEXT NOT NULL DEFAULT '',
    secret_encrypted BYTEA NOT NULL,
    encryption_key_id TEXT,
    status          TEXT NOT NULL DEFAULT 'unverified' CHECK (status IN ('unverified', 'verified')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, user_id, friendly_name)
);

CREATE TABLE IF NOT EXISTS project_mfa_challenges (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    factor_id       UUID NOT NULL REFERENCES project_mfa_factors(id) ON DELETE CASCADE,
    challenge_otp_hash TEXT NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    verified_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-project asymmetric signing keys (ES256). Only the public JWK is ever
-- served (JWKS endpoint); the private key is envelope-encrypted at rest.
CREATE TABLE IF NOT EXISTS project_signing_keys (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kid                 TEXT NOT NULL,
    alg                 TEXT NOT NULL DEFAULT 'ES256' CHECK (alg IN ('ES256', 'RS256')),
    public_jwk          JSONB NOT NULL,
    private_encrypted   BYTEA NOT NULL,
    encryption_key_id   TEXT,
    rotated_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, kid)
);

CREATE INDEX IF NOT EXISTS idx_project_signing_keys_project ON project_signing_keys(project_id);
