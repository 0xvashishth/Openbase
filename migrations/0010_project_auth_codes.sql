-- Phase 10 (A1): single-use verification codes for end-user flows —
-- email verification, password recovery, magic links, OTP. Only the SHA-256
-- hash is stored; the plaintext code is delivered via the mailer (or SMS
-- provider) and never touches the database.

CREATE TABLE IF NOT EXISTS project_auth_codes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES project_users(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN (
                        'verify_email', 'recovery', 'magiclink',
                        'otp_email', 'otp_phone', 'phone_change', 'email_change'
                    )),
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_project_auth_codes_hash ON project_auth_codes(token_hash);
CREATE INDEX IF NOT EXISTS idx_project_auth_codes_user ON project_auth_codes(project_id, user_id);
