-- Phase 2 (8.6): Org settings, project settings, member management

-- updated_at for organizations and projects (renames, slug changes)
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE projects      ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Owner-count guard index: fast COUNT where role='owner'
CREATE INDEX IF NOT EXISTS idx_org_members_owner
    ON organization_members(organization_id) WHERE role = 'owner';

-- Invites (token-based, no mailer needed yet; works for self-hosted)
CREATE TABLE IF NOT EXISTS organization_invites (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email               TEXT NOT NULL,
    role                TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    token_hash          TEXT NOT NULL,
    invited_by          UUID NOT NULL REFERENCES users(id),
    expires_at          TIMESTAMPTZ NOT NULL,
    accepted_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, email, accepted_at)  -- one pending per email
);

CREATE INDEX IF NOT EXISTS idx_org_invites_token ON organization_invites(token_hash);
CREATE INDEX IF NOT EXISTS idx_org_invites_org ON organization_invites(organization_id);

-- Audit trail (seam for 9.8; minimal columns for the events we emit now)
CREATE TABLE IF NOT EXISTS audit_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id   UUID NOT NULL REFERENCES users(id),
    organization_id UUID REFERENCES organizations(id),
    project_id      UUID REFERENCES projects(id),
    action          TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT,
    metadata        JSONB,
    ip              INET,
    user_agent      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_events(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_project ON audit_events(project_id, created_at DESC);