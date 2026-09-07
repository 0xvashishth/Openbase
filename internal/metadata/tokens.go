package metadata

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/base64"
	"time"
)

// ResetPasswordToken is a single-use token for password reset. The plaintext
// token never touches the database — only the SHA-256 hash + expiry is stored.
type ResetPasswordToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// CreateResetPasswordToken creates a single-use token for a user and returns
// the plaintext token (to be emailed). Storage only keeps the hash + expiry.
func (s *Postgres) CreateResetPasswordToken(ctx context.Context, userID string, ttl time.Duration) (plaintext string, rt *ResetPasswordToken, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, mapError(err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(sum[:])
	expiresAt := newTime().Add(ttl)
	rt = &ResetPasswordToken{
		ID:        newID(),
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO reset_password_tokens (id, user_id, refresh_hash, expires_at, used_at)
		VALUES ($1,$2,$3,$4,$5)`,
		rt.ID, userID, hash, expiresAt, nil)
	return plaintext, rt, mapError(err)
}

// GetResetPasswordToken looks up a reset token by its hash. Returns nil if
// the token is expired or already used.
func (s *Postgres) GetResetPasswordToken(ctx context.Context, hash string) (*ResetPasswordToken, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, user_id, refresh_hash, expires_at, used_at
		FROM reset_password_tokens WHERE refresh_hash = $1`, hash)
	var rt ResetPasswordToken
	var usedAt time.Time
	if err := row.Scan(&rt.ID, &rt.UserID, &rt.ExpiresAt, &usedAt); err != nil {
		return nil, mapError(err)
	}
	if !usedAt.IsZero() {
		return nil, nil // already used
	}
	if rt.ExpiresAt.Before(newTime()) {
		return nil, nil // expired
	}
	return &rt, nil
}

// MarkResetPasswordTokenUsed marks a token as used at the given time.
func (s *Postgres) MarkResetPasswordTokenUsed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE reset_password_tokens SET used_at = now() WHERE id = $1`, id)
	return mapError(err)
}
// SetUserPassword updates the password hash for a user.
func (s *Postgres) SetUserPassword(ctx context.Context, userID, passwordHash string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`,
		userID, passwordHash)
	return mapError(err)
}

// UpdateUserProfile updates mutable profile fields (currently only full_name).
func (s *Postgres) UpdateUserProfile(ctx context.Context, userID, fullName string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET full_name = $2, updated_at = now() WHERE id = $1`,
		userID, fullName)
	return mapError(err)
}
