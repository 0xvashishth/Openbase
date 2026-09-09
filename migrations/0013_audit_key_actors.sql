-- Phase 10 (A3.3): service-role API keys act without a user row, so the
-- audit trail needs a key actor alongside the user actor. actor_user_id
-- becomes nullable (user actions still always set it); key actions set
-- actor_key_id to service_role:<key-id> instead.

ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS actor_key_id TEXT;
ALTER TABLE audit_events ALTER COLUMN actor_user_id DROP NOT NULL;
