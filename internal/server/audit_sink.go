package server

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/openbase/openbase/internal/metadata"
)

// PostgresAuditSink persists audit events to the metadata store.
// Wire it in place of NoopAuditSink in Services.AuditSink.
type PostgresAuditSink struct {
	Store metadata.Store
	Log   *slog.Logger
}

// Record converts the audit event into a metadata.AuditEvent and persists it.
// Recording failures are logged, never propagated — handlers must not fail
// user-visible actions because the audit log is unavailable.
func (a *PostgresAuditSink) Record(ctx context.Context, actorUserID, orgID, projectID string, action, targetType string, targetID any, meta map[string]any) error {
	if a == nil || a.Store == nil {
		return nil
	}
	tid := ""
	switch v := targetID.(type) {
	case string:
		tid = v
	default:
		if b, err := json.Marshal(v); err == nil {
			tid = string(b)
		}
	}
	e := &metadata.AuditEvent{
		ActorUserID:    actorUserID,
		OrganizationID: orgID,
		ProjectID:      projectID,
		Action:         action,
		TargetType:     targetType,
		TargetID:       tid,
		Metadata:       meta,
	}
	if err := a.Store.AppendAuditEvent(ctx, e); err != nil {
		if a.Log != nil {
			a.Log.Warn("audit: record failed", "action", action, "actor", actorUserID, "err", err)
		}
		return err
	}
	return nil
}
