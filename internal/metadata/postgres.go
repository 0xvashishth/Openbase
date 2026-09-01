package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("metadata: not found")

// ErrConflict is returned on unique-constraint violations (email, slug, etc).
var ErrConflict = errors.New("metadata: conflict")

// Postgres implements Store on top of the platform's own database.
type Postgres struct {
	pool *pgxpool.Pool
}

var _ Store = (*Postgres)(nil)

// NewPostgres returns a metadata store using the given pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func newID() string { return uuid.NewString() }

func newTime() time.Time { return time.Now().UTC() }

// ---- Users ----

func (s *Postgres) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = newID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = newTime()
	}
	u.UpdatedAt = u.CreatedAt
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, strings.ToLower(u.Email), u.PasswordHash, u.FullName, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	return nil
}

func (s *Postgres) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, created_at, updated_at
		FROM users WHERE email = $1`, strings.ToLower(email))
	return scanUser(row)
}

func (s *Postgres) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, full_name, created_at, updated_at
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ---- Organizations + memberships ----

func (s *Postgres) CreateOrganization(ctx context.Context, org *Organization, creatorID string, role OrgRole) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if org.ID == "" {
		org.ID = newID()
	}
	if org.CreatedAt.IsZero() {
		org.CreatedAt = newTime()
	}
	org.CreatedBy = creatorID
	_, err = tx.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		org.ID, org.Name, org.Slug, creatorID, org.CreatedAt)
	if err != nil {
		return mapError(err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)`,
		org.ID, creatorID, role, org.CreatedAt)
	if err != nil {
		return mapError(err)
	}

	return tx.Commit(ctx)
}

func (s *Postgres) GetOrganization(ctx context.Context, id string) (*Organization, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, slug, created_by, created_at
		FROM organizations WHERE id = $1`, id)
	var o Organization
	err := row.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedBy, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Postgres) ListOrganizationsForUser(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.created_by, o.created_at
		FROM organizations o
		JOIN organization_members m ON m.organization_id = o.id
		WHERE m.user_id = $1
		ORDER BY o.created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedBy, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Postgres) AddMember(ctx context.Context, m *Membership) error {
	if m.JoinedAt.IsZero() {
		m.JoinedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)`,
		m.OrganizationID, m.UserID, m.Role, m.JoinedAt)
	return mapError(err)
}

func (s *Postgres) ListMembers(ctx context.Context, orgID string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT organization_id, user_id, role, joined_at
		FROM organization_members WHERE organization_id = $1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.OrganizationID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Postgres) GetMembership(ctx context.Context, orgID, userID string) (*Membership, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT organization_id, user_id, role, joined_at
		FROM organization_members WHERE organization_id = $1 AND user_id = $2`, orgID, userID)
	var m Membership
	err := row.Scan(&m.OrganizationID, &m.UserID, &m.Role, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ---- Projects ----

func (s *Postgres) CreateProject(ctx context.Context, p *Project) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO projects (id, organization_id, name, slug, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.OrganizationID, p.Name, p.Slug, p.CreatedBy, p.CreatedAt)
	return mapError(err)
}

func (s *Postgres) GetProject(ctx context.Context, id string) (*Project, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, slug, created_by, created_at
		FROM projects WHERE id = $1`, id)
	var p Project
	err := row.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.CreatedBy, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Postgres) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, slug, created_by, created_at
		FROM projects WHERE organization_id = $1
		ORDER BY created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- Connections ----

func (s *Postgres) CreateConnection(ctx context.Context, c *Connection) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = newTime()
	}
	if c.Status == "" {
		c.Status = StatusPending
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO connections (
			id, project_id, mode, engine, container_id,
			encrypted_conn_string, encrypted_username, encrypted_password,
			encryption_key_id, status, last_checked_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID, c.ProjectID, c.Mode, c.Engine, c.ContainerID,
		c.EncryptedConnString, c.EncryptedUsername, c.EncryptedPassword,
		c.EncryptionKeyID, c.Status, c.LastCheckedAt, c.CreatedAt)
	return mapError(err)
}

func (s *Postgres) GetConnectionByProject(ctx context.Context, projectID string) (*Connection, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, mode, engine, container_id,
			encrypted_conn_string, encrypted_username, encrypted_password,
			encryption_key_id, status, last_checked_at, created_at
		FROM connections WHERE project_id = $1`, projectID)
	var c Connection
	err := row.Scan(&c.ID, &c.ProjectID, &c.Mode, &c.Engine, &c.ContainerID,
		&c.EncryptedConnString, &c.EncryptedUsername, &c.EncryptedPassword,
		&c.EncryptionKeyID, &c.Status, &c.LastCheckedAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateConnection overwrites the mutable fields of an existing connection,
// preserving its identity and creation time. Used by the BYODB upsert flow.
func (s *Postgres) UpdateConnection(ctx context.Context, c *Connection) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE connections SET
			mode = $2, engine = $3, container_id = $4,
			encrypted_conn_string = $5, encrypted_username = $6,
			encrypted_password = $7, encryption_key_id = $8,
			status = $9, last_checked_at = $10
		WHERE id = $1`,
		c.ID, c.Mode, c.Engine, c.ContainerID,
		c.EncryptedConnString, c.EncryptedUsername, c.EncryptedPassword,
		c.EncryptionKeyID, c.Status, c.LastCheckedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) UpdateConnectionStatus(ctx context.Context, id string, status ConnectionStatus) error {	now := newTime()
	tag, err := s.pool.Exec(ctx, `
		UPDATE connections SET status = $2, last_checked_at = $3 WHERE id = $1`,
		id, status, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- API keys ----

func (s *Postgres) CreateAPIKey(ctx context.Context, k *APIKey) error {
	if k.ID == "" {
		k.ID = newID()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = newTime()
	}
	if k.Scopes == nil {
		k.Scopes = []string{}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (id, project_id, name, key_hash, scopes, created_at, revoked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		k.ID, k.ProjectID, k.Name, k.KeyHash, k.Scopes, k.CreatedAt, k.RevokedAt)
	return mapError(err)
}

func (s *Postgres) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, key_hash, scopes, created_at, revoked_at
		FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, hash)
	var k APIKey
	err := row.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scopes, &k.CreatedAt, &k.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Postgres) ListAPIKeys(ctx context.Context, projectID string) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, key_hash, scopes, created_at, revoked_at
		FROM api_keys WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Scopes, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Postgres) RevokeAPIKey(ctx context.Context, id string) error {
	now := newTime()
	tag, err := s.pool.Exec(ctx, `UPDATE api_keys SET revoked_at = $2 WHERE id = $1`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// mapError translates driver errors into package-level sentinels, keeping the
// HTTP layer independent of Postgres specifics.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
