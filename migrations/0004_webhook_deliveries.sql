-- Phase 8.9: webhook hardening support.
-- webhook_secret on projects feeds the per-project HMAC signature
-- (X-Openbase-Signature) so receivers can verify deliveries.
-- webhook_deliveries records every webhook attempt for the Triggers UI log.

ALTER TABLE projects ADD COLUMN IF NOT EXISTS webhook_secret TEXT;

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    trigger_id  UUID REFERENCES triggers(id) ON DELETE SET NULL,
    target_url  TEXT NOT NULL,
    collection  TEXT NOT NULL,
    event       TEXT NOT NULL,
    attempts    INT NOT NULL DEFAULT 1,
    status_code INT,
    ok          BOOLEAN NOT NULL DEFAULT FALSE,
    error       TEXT,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_project_time
    ON webhook_deliveries(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_trigger_time
    ON webhook_deliveries(trigger_id, created_at DESC);
