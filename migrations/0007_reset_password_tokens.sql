-- Phase 9.3: password reset tokens. Single-use, hashed, expiring.
-- Only the SHA-256 hash is stored (never the plaintext token).

CREATE TABLE IF NOT EXISTS reset_password_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_hash    TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reset_tokens_hash ON reset_password_tokens(refresh_hash);
CREATE INDEX IF NOT EXISTS idx_reset_tokens_user ON reset_password_tokens(user_id);
