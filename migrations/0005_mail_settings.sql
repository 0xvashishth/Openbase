-- Phase 9.2: dashboard-managed mailer (BYOC SMTP).
-- mail_settings is a singleton row (id = 'default'): the operator's own
-- mail provider, configured from the dashboard Email page. The SMTP password
-- is envelope-encrypted (ciphertext + key id, rotation-aware like
-- connections); it is never returned by the API.
-- mail_log records every send attempt for the dashboard delivery log.

CREATE TABLE IF NOT EXISTS mail_settings (
    id                  TEXT PRIMARY KEY DEFAULT 'default',
    provider            TEXT NOT NULL DEFAULT 'log' CHECK (provider IN ('smtp', 'log')),
    smtp_host           TEXT NOT NULL DEFAULT '',
    smtp_port           INT NOT NULL DEFAULT 587,
    smtp_username       TEXT NOT NULL DEFAULT '',
    encrypted_password  BYTEA,
    encryption_key_id   TEXT,
    from_address        TEXT NOT NULL DEFAULT '',
    from_name           TEXT NOT NULL DEFAULT 'Openbase',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO mail_settings (id, provider)
VALUES ('default', 'log')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS mail_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    to_address  TEXT NOT NULL,
    template    TEXT NOT NULL,
    subject     TEXT NOT NULL,
    ok          BOOLEAN NOT NULL DEFAULT FALSE,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mail_log_time
    ON mail_log(created_at DESC);
