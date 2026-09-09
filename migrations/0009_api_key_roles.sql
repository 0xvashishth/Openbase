-- Phase 10 (A1.1): split the single ob_ key into anon / service_role,
-- mirroring Supabase's mental model. The role is resolved server-side from
-- the key hash — it is never encoded in the key itself.
--
-- Existing keys predate roles and were unconditional full CRUD: they become
-- service_role, preserving current behavior. New browser-safe keys are minted
-- as anon (gated by Phase 11 policies once they land; until then the role is
-- recorded and propagated, not yet enforced, and the data path says so).

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'service_role';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_role_check'
    ) THEN
        ALTER TABLE api_keys
            ADD CONSTRAINT api_keys_role_check
            CHECK (role IN ('anon', 'service_role'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_api_keys_project_role ON api_keys(project_id, role);
