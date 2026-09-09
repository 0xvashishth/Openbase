-- Phase 10 (A3.2): auth hooks — invoke a project function on identity events.
-- Events: before-user-created (may reject / merge user_metadata),
-- after-user-created (best-effort), before-token-issued (may merge custom
-- claims into the access token). fail_open controls whether a hook runtime
-- failure blocks the auth flow (before-*) or is only logged.

CREATE TABLE IF NOT EXISTS project_auth_hooks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event           TEXT NOT NULL CHECK (event IN (
                        'before-user-created', 'after-user-created', 'before-token-issued'
                    )),
    function_id     UUID NOT NULL REFERENCES functions(id) ON DELETE CASCADE,
    fail_open       BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, event)
);

CREATE INDEX IF NOT EXISTS idx_project_auth_hooks_project ON project_auth_hooks(project_id);
