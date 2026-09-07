package metadata

import (
	"context"
	"errors"
	"time"
)

// Session is one operator login session: an opaque rotating refresh token
// (stored hashed) with device metadata for the 9.4 sessions list.
type Session struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	RefreshHash string    `json:"-"`
	UserAgent   string    `json:"user_agent"`
	IP          string    `json:"ip"`
	ExpiresAt   time.Time `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Revoked reports whether the session was explicitly revoked.
func (s *Session) Revoked() bool { return s.RevokedAt != nil }

// Expired reports whether the refresh window has passed.
func (s *Session) Expired(now time.Time) bool { return !s.ExpiresAt.After(now) }

// CreateSession creates a new session for a user, storing only the SHA-256
// hash of the refresh token. The plaintext token never touches the database.
func (s *Postgres) CreateSession(ctx context.Context, ses *Session) error {
	if ses.RefreshHash == "" || ses.UserID == "" || ses.ExpiresAt.IsZero() {
		return errors.New("auth: session fields incomplete")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, refresh_hash, user_agent, ip, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		ses.ID, ses.UserID, ses.RefreshHash, ses.UserAgent, ses.IP, ses.ExpiresAt, newTime())
	return mapError(err)
}

// GetSessionByHash looks up a session by its stored refresh-token hash.
func (s *Postgres) GetSessionByHash(ctx context.Context, hash string) (*Session, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, refresh_hash, user_agent, ip, expires_at, revoked_at, created_at
		FROM sessions WHERE refresh_hash = $1`, hash)
	var sess Session
	var revokedAt time.Time
	if err := row.Scan(&sess.ID, &sess.UserID, &sess.RefreshHash, &sess.UserAgent, &sess.IP, &sess.ExpiresAt, &revokedAt, &sess.CreatedAt); err != nil {
		return nil, mapError(err)
	}
	if !revokedAt.IsZero() {
		sess.RevokedAt = &revokedAt
	}
	return &sess, nil
}

// RevokeSession explicitly revokes a session by ID.
func (s *Postgres) RevokeSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now() WHERE id = $1`, id)
	return mapError(err)
}

// RevokeUserSessions revokes all active sessions for a given user.
func (s *Postgres) RevokeUserSessions(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return mapError(err)
}

// ListSessions lists all sessions for a user, newest first.
func (s *Postgres) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, refresh_hash, user_agent, ip, expires_at, revoked_at, created_at
		FROM sessions WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var sess Session
		var revokedAt time.Time
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.RefreshHash, &sess.UserAgent, &sess.IP, &sess.ExpiresAt, &revokedAt, &sess.CreatedAt); err != nil {
			return nil, mapError(err)
		}
		if !revokedAt.IsZero() {
			sess.RevokedAt = &revokedAt
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}