package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrRefreshReuse is returned when a revoked/rotated refresh token is
// presented again: the chain is compromised, so the whole chain is revoked.
var ErrRefreshReuse = errors.New("metadata: refresh token reuse detected")

// ProjectUser is one end-user of a customer's app (Phase 10). Email and phone
// are nullable: anonymous users have neither (is_anonymous), and each is
// unique per project when present.
type ProjectUser struct {
	ID               string         `json:"id"`
	ProjectID        string         `json:"project_id"`
	Email            *string        `json:"email,omitempty"`
	Phone            *string        `json:"phone,omitempty"`
	PasswordHash     string         `json:"-"`
	EmailConfirmedAt *time.Time     `json:"email_confirmed_at,omitempty"`
	PhoneConfirmedAt *time.Time     `json:"phone_confirmed_at,omitempty"`
	BannedUntil      *time.Time     `json:"banned_until,omitempty"`
	IsAnonymous      bool           `json:"is_anonymous"`
	UserMetadata     map[string]any `json:"user_metadata,omitempty"`
	AppMetadata      map[string]any `json:"app_metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Banned reports whether the user is currently banned.
func (u *ProjectUser) Banned(now time.Time) bool {
	return u.BannedUntil != nil && u.BannedUntil.After(now)
}

// ProjectIdentity links an external provider identity to a project user.
type ProjectIdentity struct {
	ID           string         `json:"id"`
	ProjectID    string         `json:"project_id"`
	UserID       string         `json:"user_id"`
	Provider     string         `json:"provider"`
	ProviderUID  string         `json:"provider_uid"`
	IdentityData map[string]any `json:"identity_data,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ProjectSession is one end-user refresh session. Only the SHA-256 hash is
// stored — a leaked database never yields usable refresh tokens.
type ProjectSession struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	UserID      string     `json:"user_id"`
	RefreshHash string     `json:"-"`
	UserAgent   string     `json:"user_agent"`
	IP          string     `json:"ip"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Revoked reports whether the session was explicitly revoked (incl. rotation).
func (s *ProjectSession) Revoked() bool { return s.RevokedAt != nil }

// Expired reports whether the refresh window has passed.
func (s *ProjectSession) Expired(now time.Time) bool { return !s.ExpiresAt.After(now) }

// ProjectAuthSettings is the per-project auth configuration row.
type ProjectAuthSettings struct {
	ProjectID              string         `json:"project_id"`
	SiteURL                string         `json:"site_url"`
	RedirectAllowList      []string       `json:"redirect_allow_list"`
	PasswordMinLength      int            `json:"password_min_length"`
	SessionIdleTimeoutS    int            `json:"session_idle_timeout_s"`
	SessionAbsoluteTimeoutS int           `json:"session_absolute_timeout_s"`
	MFAEnabled             bool           `json:"mfa_enabled"`
	Providers              map[string]any `json:"providers,omitempty"`
	MailTemplates          map[string]any `json:"mail_templates,omitempty"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// ProjectMFAFactor is one enrolled TOTP factor.
type ProjectMFAFactor struct {
	ID              string    `json:"id"`
	ProjectID       string    `json:"project_id"`
	UserID          string    `json:"user_id"`
	FactorType      string    `json:"factor_type"`
	FriendlyName    string    `json:"friendly_name"`
	SecretEncrypted []byte    `json:"-"`
	EncryptionKeyID string    `json:"-"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// ProjectMFAChallenge is one TOTP challenge (single-use, expiring).
type ProjectMFAChallenge struct {
	ID               string     `json:"id"`
	FactorID         string     `json:"factor_id"`
	ChallengeOTPHash string     `json:"-"`
	ExpiresAt        time.Time  `json:"expires_at"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// ProjectSigningKey is one asymmetric project signing key. The private half
// is envelope-encrypted; only the public JWK is ever served.
type ProjectSigningKey struct {
	ID                string         `json:"id"`
	ProjectID         string         `json:"project_id"`
	KID               string         `json:"kid"`
	Alg               string         `json:"alg"`
	PublicJWK         map[string]any `json:"public_jwk"`
	PrivateEncrypted  []byte         `json:"-"`
	EncryptionKeyID   string         `json:"-"`
	RotatedAt         *time.Time     `json:"rotated_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
}

// ProjectAuthCode is a single-use verification code (hash stored only).
type ProjectAuthCode struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	UserID    string     `json:"user_id"`
	Kind      string     `json:"kind"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Auth code kinds (mirror the 0010 CHECK constraint).
const (
	AuthCodeVerifyEmail = "verify_email"
	AuthCodeRecovery    = "recovery"
	AuthCodeMagicLink   = "magiclink"
	AuthCodeOTPEmail    = "otp_email"
	AuthCodeOTPPhone    = "otp_phone"
	AuthCodePhoneChange = "phone_change"
	AuthCodeEmailChange = "email_change"
)

// ValidAuthCodeKind reports whether kind is a known project_auth_codes kind.
func ValidAuthCodeKind(kind string) bool {
	switch kind {
	case AuthCodeVerifyEmail, AuthCodeRecovery, AuthCodeMagicLink,
		AuthCodeOTPEmail, AuthCodeOTPPhone, AuthCodePhoneChange, AuthCodeEmailChange:
		return true
	}
	return false
}

// ---- helpers ----

func marshalJSONB(v any) ([]byte, error) {
	if v == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(v)
}

func unmarshalJSONBMap(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

func unmarshalJSONBStrings(raw []byte) []string {
	var out []string
	if len(raw) == 0 {
		return []string{}
	}
	// TEXT[] arrives via pgx as []string already in most paths; this covers
	// the JSONB-encoded fallback.
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	return out
}

var _ = unmarshalJSONBStrings

// scanProjectUser maps a project_users row in canonical column order.
func scanProjectUser(row pgx.Row) (*ProjectUser, error) {
	var u ProjectUser
	var userMeta, appMeta []byte
	err := row.Scan(&u.ID, &u.ProjectID, &u.Email, &u.Phone, &u.PasswordHash,
		&u.EmailConfirmedAt, &u.PhoneConfirmedAt, &u.BannedUntil, &u.IsAnonymous,
		&userMeta, &appMeta, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.UserMetadata = unmarshalJSONBMap(userMeta)
	u.AppMetadata = unmarshalJSONBMap(appMeta)
	return &u, nil
}

const projectUserColumns = `id, project_id, email, phone, password_hash,
	email_confirmed_at, phone_confirmed_at, banned_until, is_anonymous,
	user_metadata, app_metadata, created_at, updated_at`

// CreateProjectUser inserts an end user. Email/phone uniqueness (when
// present) is enforced by the 0008 UNIQUE constraints.
func (s *Postgres) CreateProjectUser(ctx context.Context, u *ProjectUser) error {
	if u.ProjectID == "" {
		return errors.New("metadata: project_id is required")
	}
	if u.ID == "" {
		u.ID = newID()
	}
	now := newTime()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	userMeta, err := marshalJSONB(u.UserMetadata)
	if err != nil {
		return err
	}
	appMeta, err := marshalJSONB(u.AppMetadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO project_users (id, project_id, email, phone, password_hash,
			email_confirmed_at, phone_confirmed_at, banned_until, is_anonymous,
			user_metadata, app_metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		u.ID, u.ProjectID, u.Email, u.Phone, u.PasswordHash,
		u.EmailConfirmedAt, u.PhoneConfirmedAt, u.BannedUntil, u.IsAnonymous,
		userMeta, appMeta, u.CreatedAt, u.UpdatedAt)
	return mapError(err)
}

// GetProjectUser fetches one user, scoped to its project.
func (s *Postgres) GetProjectUser(ctx context.Context, projectID, userID string) (*ProjectUser, error) {
	return scanProjectUser(s.pool.QueryRow(ctx, `
		SELECT `+projectUserColumns+`
		FROM project_users WHERE project_id = $1 AND id = $2`, projectID, userID))
}

// GetProjectUserByEmail fetches by (lowercased-by-caller) email.
func (s *Postgres) GetProjectUserByEmail(ctx context.Context, projectID, email string) (*ProjectUser, error) {
	return scanProjectUser(s.pool.QueryRow(ctx, `
		SELECT `+projectUserColumns+`
		FROM project_users WHERE project_id = $1 AND email = $2`, projectID, email))
}

// GetProjectUserByPhone fetches by phone.
func (s *Postgres) GetProjectUserByPhone(ctx context.Context, projectID, phone string) (*ProjectUser, error) {
	return scanProjectUser(s.pool.QueryRow(ctx, `
		SELECT `+projectUserColumns+`
		FROM project_users WHERE project_id = $1 AND phone = $2`, projectID, phone))
}

// ListProjectUsers lists users newest-last with optional email search.
func (s *Postgres) ListProjectUsers(ctx context.Context, projectID, search string, limit, offset int) ([]ProjectUser, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	if search == "" {
		rows, err = s.pool.Query(ctx, `
			SELECT `+projectUserColumns+`
			FROM project_users WHERE project_id = $1
			ORDER BY created_at LIMIT $2 OFFSET $3`, projectID, limit, offset)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT `+projectUserColumns+`
			FROM project_users WHERE project_id = $1 AND email ILIKE '%' || $2 || '%'
			ORDER BY created_at LIMIT $3 OFFSET $4`, projectID, search, limit, offset)
	}
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []ProjectUser{}
	for rows.Next() {
		u, err := scanProjectUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdateProjectUser writes mutable fields (confirmations, ban, metadata,
// password hash, anonymous promotion). Email/phone changes go through the
// verify-both-addresses flow, not this method.
func (s *Postgres) UpdateProjectUser(ctx context.Context, u *ProjectUser) error {
	userMeta, err := marshalJSONB(u.UserMetadata)
	if err != nil {
		return err
	}
	appMeta, err := marshalJSONB(u.AppMetadata)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_users SET password_hash = $1, email_confirmed_at = $2,
			phone_confirmed_at = $3, banned_until = $4, is_anonymous = $5,
			user_metadata = $6, app_metadata = $7, updated_at = now()
		WHERE id = $8 AND project_id = $9`,
		u.PasswordHash, u.EmailConfirmedAt, u.PhoneConfirmedAt, u.BannedUntil,
		u.IsAnonymous, userMeta, appMeta, u.ID, u.ProjectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProjectUser removes a user; sessions/identities/factors cascade.
func (s *Postgres) DeleteProjectUser(ctx context.Context, projectID, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_users WHERE id = $1 AND project_id = $2`, userID, projectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertProjectIdentity links (or refreshes) a provider identity.
func (s *Postgres) UpsertProjectIdentity(ctx context.Context, idn *ProjectIdentity) error {
	if idn.ID == "" {
		idn.ID = newID()
	}
	if idn.CreatedAt.IsZero() {
		idn.CreatedAt = newTime()
	}
	data, err := marshalJSONB(idn.IdentityData)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO project_identities (id, project_id, user_id, provider, provider_uid, identity_data, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (project_id, provider, provider_uid)
		DO UPDATE SET user_id = EXCLUDED.user_id, identity_data = EXCLUDED.identity_data`,
		idn.ID, idn.ProjectID, idn.UserID, idn.Provider, idn.ProviderUID, data, idn.CreatedAt)
	return mapError(err)
}

// ListProjectIdentities returns a user's linked identities.
func (s *Postgres) ListProjectIdentities(ctx context.Context, projectID, userID string) ([]ProjectIdentity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, user_id, provider, provider_uid, identity_data, created_at
		FROM project_identities WHERE project_id = $1 AND user_id = $2 ORDER BY created_at`,
		projectID, userID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []ProjectIdentity{}
	for rows.Next() {
		var idn ProjectIdentity
		var data []byte
		if err := rows.Scan(&idn.ID, &idn.ProjectID, &idn.UserID, &idn.Provider,
			&idn.ProviderUID, &data, &idn.CreatedAt); err != nil {
			return nil, err
		}
		idn.IdentityData = unmarshalJSONBMap(data)
		out = append(out, idn)
	}
	return out, rows.Err()
}

// CreateProjectSession stores a refresh session (hash only, never plaintext).
func (s *Postgres) CreateProjectSession(ctx context.Context, ses *ProjectSession) error {
	if ses.ProjectID == "" || ses.UserID == "" || ses.RefreshHash == "" || ses.ExpiresAt.IsZero() {
		return errors.New("metadata: project session fields incomplete")
	}
	if ses.ID == "" {
		ses.ID = newID()
	}
	if ses.CreatedAt.IsZero() {
		ses.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_sessions (id, project_id, user_id, refresh_hash, user_agent, ip, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		ses.ID, ses.ProjectID, ses.UserID, ses.RefreshHash, ses.UserAgent, ses.IP, ses.ExpiresAt, ses.CreatedAt)
	return mapError(err)
}

func scanProjectSession(row pgx.Row) (*ProjectSession, error) {
	var ses ProjectSession
	err := row.Scan(&ses.ID, &ses.ProjectID, &ses.UserID, &ses.RefreshHash,
		&ses.UserAgent, &ses.IP, &ses.ExpiresAt, &ses.RevokedAt, &ses.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ses, nil
}

const projectSessionColumns = `id, project_id, user_id, refresh_hash, user_agent, ip, expires_at, revoked_at, created_at`

// GetProjectSessionByHash looks up a session including revoked ones (callers
// need the revoked row for reuse detection).
func (s *Postgres) GetProjectSessionByHash(ctx context.Context, hash string) (*ProjectSession, error) {
	return scanProjectSession(s.pool.QueryRow(ctx, `
		SELECT `+projectSessionColumns+`
		FROM project_sessions WHERE refresh_hash = $1`, hash))
}

// RevokeProjectSession revokes one session by ID.
func (s *Postgres) RevokeProjectSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE project_sessions SET revoked_at = now() WHERE id = $1`, id)
	return mapError(err)
}

// RevokeProjectUserSessions revokes every live session for a user in a project.
func (s *Postgres) RevokeProjectUserSessions(ctx context.Context, projectID, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE project_sessions SET revoked_at = now()
		WHERE project_id = $1 AND user_id = $2 AND revoked_at IS NULL`, projectID, userID)
	return mapError(err)
}

// ListProjectSessions lists a user's sessions, newest first.
func (s *Postgres) ListProjectSessions(ctx context.Context, projectID, userID string) ([]ProjectSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+projectSessionColumns+`
		FROM project_sessions WHERE project_id = $1 AND user_id = $2 ORDER BY created_at DESC`,
		projectID, userID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []ProjectSession{}
	for rows.Next() {
		ses, err := scanProjectSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ses)
	}
	return out, rows.Err()
}

// RotateProjectSession atomically rotates a refresh session: the old hash is
// revoked and the new session inserted in one transaction. If the presented
// hash belongs to an already-rotated (revoked) session, the whole chain is
// revoked and ErrRefreshReuse is returned — a stolen refresh token is only
// useful once, and using it burns the attacker's copy too.
func (s *Postgres) RotateProjectSession(ctx context.Context, oldHash string, next *ProjectSession) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var cur ProjectSession
	err = tx.QueryRow(ctx, `
		SELECT `+projectSessionColumns+`
		FROM project_sessions WHERE refresh_hash = $1 FOR UPDATE`, oldHash).
		Scan(&cur.ID, &cur.ProjectID, &cur.UserID, &cur.RefreshHash,
			&cur.UserAgent, &cur.IP, &cur.ExpiresAt, &cur.RevokedAt, &cur.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if cur.Revoked() {
		// Reuse: burn the chain.
		if _, err := tx.Exec(ctx, `
			UPDATE project_sessions SET revoked_at = now()
			WHERE project_id = $1 AND user_id = $2 AND revoked_at IS NULL`,
			cur.ProjectID, cur.UserID); err != nil {
			return err
		}
		_ = tx.Commit(ctx)
		return ErrRefreshReuse
	}
	if cur.Expired(time.Now()) {
		return errors.New("metadata: refresh token expired")
	}
	if _, err := tx.Exec(ctx,
		`UPDATE project_sessions SET revoked_at = now() WHERE id = $1`, cur.ID); err != nil {
		return err
	}
	if next.ID == "" {
		next.ID = newID()
	}
	next.ProjectID = cur.ProjectID
	next.UserID = cur.UserID
	if next.CreatedAt.IsZero() {
		next.CreatedAt = newTime()
	}
	if next.RefreshHash == "" || next.ExpiresAt.IsZero() {
		return errors.New("metadata: replacement session fields incomplete")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO project_sessions (id, project_id, user_id, refresh_hash, user_agent, ip, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		next.ID, next.ProjectID, next.UserID, next.RefreshHash,
		next.UserAgent, next.IP, next.ExpiresAt, next.CreatedAt); err != nil {
		return mapError(err)
	}
	return tx.Commit(ctx)
}

// DefaultProjectAuthSettings returns the settings row, creating it on first read.
func (s *Postgres) GetOrCreateProjectAuthSettings(ctx context.Context, projectID string) (*ProjectAuthSettings, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_auth_settings (project_id) VALUES ($1) ON CONFLICT (project_id) DO NOTHING`, projectID)
	if err != nil {
		return nil, mapError(err)
	}
	var st ProjectAuthSettings
	var providers, templates []byte
	err = s.pool.QueryRow(ctx, `
		SELECT project_id, site_url, redirect_allow_list, password_min_length,
			session_idle_timeout_s, session_absolute_timeout_s, mfa_enabled,
			providers, mail_templates, updated_at
		FROM project_auth_settings WHERE project_id = $1`, projectID).
		Scan(&st.ProjectID, &st.SiteURL, &st.RedirectAllowList, &st.PasswordMinLength,
			&st.SessionIdleTimeoutS, &st.SessionAbsoluteTimeoutS, &st.MFAEnabled,
			&providers, &templates, &st.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	st.Providers = unmarshalJSONBMap(providers)
	st.MailTemplates = unmarshalJSONBMap(templates)
	return &st, nil
}

// UpdateProjectAuthSettings writes the settings row (must exist; read first).
func (s *Postgres) UpdateProjectAuthSettings(ctx context.Context, st *ProjectAuthSettings) error {
	providers, err := marshalJSONB(st.Providers)
	if err != nil {
		return err
	}
	templates, err := marshalJSONB(st.MailTemplates)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_auth_settings SET site_url = $1, redirect_allow_list = $2,
			password_min_length = $3, session_idle_timeout_s = $4,
			session_absolute_timeout_s = $5, mfa_enabled = $6,
			providers = $7, mail_templates = $8, updated_at = now()
		WHERE project_id = $9`,
		st.SiteURL, st.RedirectAllowList, st.PasswordMinLength,
		st.SessionIdleTimeoutS, st.SessionAbsoluteTimeoutS, st.MFAEnabled,
		providers, templates, st.ProjectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateMFAFactor enrolls a TOTP factor (status starts unverified).
func (s *Postgres) CreateMFAFactor(ctx context.Context, f *ProjectMFAFactor) error {
	if f.ID == "" {
		f.ID = newID()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = newTime()
	}
	if f.FactorType == "" {
		f.FactorType = "totp"
	}
	if f.Status == "" {
		f.Status = "unverified"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_mfa_factors (id, project_id, user_id, factor_type, friendly_name,
			secret_encrypted, encryption_key_id, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		f.ID, f.ProjectID, f.UserID, f.FactorType, f.FriendlyName,
		f.SecretEncrypted, f.EncryptionKeyID, f.Status, f.CreatedAt)
	return mapError(err)
}

// GetMFAFactor fetches one factor scoped to its project.
func (s *Postgres) GetMFAFactor(ctx context.Context, projectID, factorID string) (*ProjectMFAFactor, error) {
	var f ProjectMFAFactor
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, user_id, factor_type, friendly_name,
			secret_encrypted, encryption_key_id, status, created_at
		FROM project_mfa_factors WHERE id = $1 AND project_id = $2`, factorID, projectID).
		Scan(&f.ID, &f.ProjectID, &f.UserID, &f.FactorType, &f.FriendlyName,
			&f.SecretEncrypted, &f.EncryptionKeyID, &f.Status, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ListMFAFactors returns a user's factors.
func (s *Postgres) ListMFAFactors(ctx context.Context, projectID, userID string) ([]ProjectMFAFactor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, user_id, factor_type, friendly_name,
			secret_encrypted, encryption_key_id, status, created_at
		FROM project_mfa_factors WHERE project_id = $1 AND user_id = $2 ORDER BY created_at`,
		projectID, userID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []ProjectMFAFactor{}
	for rows.Next() {
		var f ProjectMFAFactor
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.UserID, &f.FactorType, &f.FriendlyName,
			&f.SecretEncrypted, &f.EncryptionKeyID, &f.Status, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// VerifyMFAFactor marks a factor verified after a successful challenge.
func (s *Postgres) VerifyMFAFactor(ctx context.Context, projectID, factorID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_mfa_factors SET status = 'verified'
		WHERE id = $1 AND project_id = $2`, factorID, projectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMFAFactor unenrolls a factor; challenges cascade.
func (s *Postgres) DeleteMFAFactor(ctx context.Context, projectID, factorID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_mfa_factors WHERE id = $1 AND project_id = $2`, factorID, projectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateMFAChallenge stores a challenge (OTP hash only, single-use).
func (s *Postgres) CreateMFAChallenge(ctx context.Context, c *ProjectMFAChallenge) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_mfa_challenges (id, factor_id, challenge_otp_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5)`,
		c.ID, c.FactorID, c.ChallengeOTPHash, c.ExpiresAt, c.CreatedAt)
	return mapError(err)
}

// GetMFAChallenge fetches a challenge by ID.
func (s *Postgres) GetMFAChallenge(ctx context.Context, challengeID string) (*ProjectMFAChallenge, error) {
	var c ProjectMFAChallenge
	err := s.pool.QueryRow(ctx, `
		SELECT id, factor_id, challenge_otp_hash, expires_at, verified_at, created_at
		FROM project_mfa_challenges WHERE id = $1`, challengeID).
		Scan(&c.ID, &c.FactorID, &c.ChallengeOTPHash, &c.ExpiresAt, &c.VerifiedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// MarkMFAChallengeVerified consumes a challenge exactly once: the UPDATE only
// matches unverified, unexpired rows, so a concurrent verify loses the race
// instead of double-spending the OTP.
func (s *Postgres) MarkMFAChallengeVerified(ctx context.Context, challengeID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_mfa_challenges SET verified_at = now()
		WHERE id = $1 AND verified_at IS NULL AND expires_at > now()`, challengeID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateProjectSigningKey stores a key row (private half envelope-encrypted
// by the caller via the secrets provider).
func (s *Postgres) CreateProjectSigningKey(ctx context.Context, k *ProjectSigningKey) error {
	if k.ID == "" {
		k.ID = newID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = newTime()
	}
	if k.Alg == "" {
		k.Alg = "ES256"
	}
	jwk, err := marshalJSONB(k.PublicJWK)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO project_signing_keys (id, project_id, kid, alg, public_jwk,
			private_encrypted, encryption_key_id, rotated_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		k.ID, k.ProjectID, k.KID, k.Alg, jwk,
		k.PrivateEncrypted, k.EncryptionKeyID, k.RotatedAt, k.CreatedAt)
	return mapError(err)
}

// ListProjectSigningKeys returns a project's keys, newest first.
func (s *Postgres) ListProjectSigningKeys(ctx context.Context, projectID string) ([]ProjectSigningKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, kid, alg, public_jwk,
			private_encrypted, encryption_key_id, rotated_at, created_at
		FROM project_signing_keys WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, mapError(err)
	}
	defer rows.Close()
	out := []ProjectSigningKey{}
	for rows.Next() {
		var k ProjectSigningKey
		var jwk []byte
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.KID, &k.Alg, &jwk,
			&k.PrivateEncrypted, &k.EncryptionKeyID, &k.RotatedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		k.PublicJWK = unmarshalJSONBMap(jwk)
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteProjectSigningKey removes a retired key row. Callers must refuse to
// delete the active kid (the projectauth KeySet owns that decision).
func (s *Postgres) DeleteProjectSigningKey(ctx context.Context, projectID, keyID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_signing_keys WHERE id = $1 AND project_id = $2`, keyID, projectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateProjectAuthCode stores a single-use code (hash only).
func (s *Postgres) CreateProjectAuthCode(ctx context.Context, c *ProjectAuthCode) error {
	if c.ProjectID == "" || c.UserID == "" || c.TokenHash == "" || c.ExpiresAt.IsZero() {
		return errors.New("metadata: auth code fields incomplete")
	}
	if !ValidAuthCodeKind(c.Kind) {
		return errors.New("metadata: unknown auth code kind")
	}
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO project_auth_codes (id, project_id, user_id, kind, token_hash, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.ProjectID, c.UserID, c.Kind, c.TokenHash, c.ExpiresAt, c.CreatedAt)
	return mapError(err)
}

// GetProjectAuthCodeByHash fetches a live (unused, unexpired) code. Used codes
// and expired codes behave as not-found so callers cannot distinguish them.
func (s *Postgres) GetProjectAuthCodeByHash(ctx context.Context, hash string) (*ProjectAuthCode, error) {
	var c ProjectAuthCode
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, user_id, kind, token_hash, expires_at, used_at, created_at
		FROM project_auth_codes
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()`, hash).
		Scan(&c.ID, &c.ProjectID, &c.UserID, &c.Kind, &c.TokenHash, &c.ExpiresAt, &c.UsedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// MarkProjectAuthCodeUsed consumes a code exactly once (concurrent consumers
// race; only one UPDATE matches).
func (s *Postgres) MarkProjectAuthCodeUsed(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_auth_codes SET used_at = now()
		WHERE id = $1 AND used_at IS NULL AND expires_at > now()`, id)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeProjectAuthCodesForUser invalidates all live codes of a kind (or all
// kinds when kind == "") — e.g. password change burns recovery codes.
func (s *Postgres) RevokeProjectAuthCodesForUser(ctx context.Context, projectID, userID, kind string) error {
	if kind == "" {
		_, err := s.pool.Exec(ctx, `
			UPDATE project_auth_codes SET used_at = now()
			WHERE project_id = $1 AND user_id = $2 AND used_at IS NULL`, projectID, userID)
		return mapError(err)
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE project_auth_codes SET used_at = now()
		WHERE project_id = $1 AND user_id = $2 AND kind = $3 AND used_at IS NULL`,
		projectID, userID, kind)
	return mapError(err)
}

// MarkProjectSigningKeyRotated stamps rotated_at when a newer key becomes active.
// Retired rows stay verifiable until deleted.
func (s *Postgres) MarkProjectSigningKeyRotated(ctx context.Context, projectID, keyID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE project_signing_keys SET rotated_at = now()
		WHERE id = $1 AND project_id = $2`, keyID, projectID)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
