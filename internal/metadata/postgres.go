package metadata

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

// Ping verifies the metadata database is reachable. Used by GET /readyz.
func (s *Postgres) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// AppliedMigrations returns the versions recorded in schema_migrations.
// Used by GET /readyz to detect a running API whose schema lags the binary.
func (s *Postgres) AppliedMigrations(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
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

// DeleteConnection removes a connection row by id. Missing rows are treated as
// success so the endpoint is idempotent.
func (s *Postgres) DeleteConnection(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	return mapError(err)
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
	if k.Role == "" {
		k.Role = APIKeyRoleDefault
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO api_keys (id, project_id, name, key_hash, role, scopes, created_at, revoked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		k.ID, k.ProjectID, k.Name, k.KeyHash, k.Role, k.Scopes, k.CreatedAt, k.RevokedAt)
	return mapError(err)
}

func (s *Postgres) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, key_hash, role, scopes, created_at, revoked_at
		FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, hash)
	var k APIKey
	err := row.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Role, &k.Scopes, &k.CreatedAt, &k.RevokedAt)
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
		SELECT id, project_id, name, key_hash, role, scopes, created_at, revoked_at
		FROM api_keys WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.Name, &k.KeyHash, &k.Role, &k.Scopes, &k.CreatedAt, &k.RevokedAt); err != nil {
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

// ---- Triggers ----

func (s *Postgres) CreateTrigger(ctx context.Context, t *Trigger) error {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO triggers (id, project_id, name, collection, event, action_type, action_target, enabled, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		t.ID, t.ProjectID, t.Name, t.Collection, string(t.Event),
		string(t.ActionType), t.ActionTarget, t.Enabled, t.CreatedAt)
	return mapError(err)
}

func (s *Postgres) GetTrigger(ctx context.Context, projectID, id string) (*Trigger, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, collection, event, action_type, action_target, enabled, created_at
		FROM triggers WHERE id = $1 AND project_id = $2`, id, projectID)
	var t Trigger
	var ev, at string
	err := row.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Collection, &ev, &at, &t.ActionTarget, &t.Enabled, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Event = TriggerEvent(ev)
	t.ActionType = TriggerActionType(at)
	return &t, nil
}

func (s *Postgres) ListTriggers(ctx context.Context, projectID string) ([]Trigger, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, collection, event, action_type, action_target, enabled, created_at
		FROM triggers WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Trigger
	for rows.Next() {
		var t Trigger
		var ev, at string
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Collection, &ev, &at, &t.ActionTarget, &t.Enabled, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Event = TriggerEvent(ev)
		t.ActionType = TriggerActionType(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Postgres) UpdateTrigger(ctx context.Context, t *Trigger) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE triggers SET name=$3, collection=$4, event=$5, action_type=$6, action_target=$7, enabled=$8
		WHERE id=$1 AND project_id=$2`,
		t.ID, t.ProjectID, t.Name, t.Collection, string(t.Event),
		string(t.ActionType), t.ActionTarget, t.Enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) DeleteTrigger(ctx context.Context, projectID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM triggers WHERE id=$1 AND project_id=$2`, id, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Functions ----

func (s *Postgres) CreateFunction(ctx context.Context, f *Function) error {
	if f.ID == "" {
		f.ID = newID()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = newTime()
	}
	if f.Source == "" {
		f.Source = "// your handler here\nexports.handler = async (event) => { return { received: event } };"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO functions (id, project_id, name, runtime, source_ref, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		f.ID, f.ProjectID, f.Name, string(f.Runtime), f.Source, f.CreatedAt)
	return mapError(err)
}

func (s *Postgres) GetFunction(ctx context.Context, projectID, id string) (*Function, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, project_id, name, runtime, source_ref, created_at
		FROM functions WHERE id = $1 AND project_id = $2`, id, projectID)
	var f Function
	var rt string
	err := row.Scan(&f.ID, &f.ProjectID, &f.Name, &rt, &f.Source, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f.Runtime = FunctionRuntime(rt)
	return &f, nil
}

func (s *Postgres) ListFunctions(ctx context.Context, projectID string) ([]Function, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, name, runtime, source_ref, created_at
		FROM functions WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Function
	for rows.Next() {
		var f Function
		var rt string
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.Name, &rt, &f.Source, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Runtime = FunctionRuntime(rt)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteFunction(ctx context.Context, projectID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM functions WHERE id=$1 AND project_id=$2`, id, projectID)
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

// ---- Organizations ----

func (s *Postgres) UpdateOrganization(ctx context.Context, org *Organization) error {
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = newTime()
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE organizations SET name = $2, slug = $3, updated_at = $4 WHERE id = $1`,
		org.ID, org.Name, org.Slug, org.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) DeleteOrganization(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) CountProjects(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE organization_id = $1`, orgID).Scan(&count)
	return count, err
}

// ---- Members ----

func (s *Postgres) ListMembersWithUsers(ctx context.Context, orgID string) ([]OrgMember, error) {
	// Order by role RANK, not the role string: alphabetically 'owner' sorts
	// above 'member' above 'admin', which would put admins last.
	// password_hash is never selected.
	rows, err := s.pool.Query(ctx, `
		SELECT m.user_id, u.email, u.full_name, m.role, m.joined_at
		FROM organization_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.organization_id = $1
		ORDER BY CASE m.role
			WHEN 'owner'  THEN 3
			WHEN 'admin'  THEN 2
			WHEN 'member' THEN 1
			ELSE 0
		END DESC, m.joined_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OrgMember
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.FullName, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Postgres) UpdateMemberRole(ctx context.Context, orgID, userID string, role OrgRole) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE organization_members SET role = $3 WHERE organization_id = $1 AND user_id = $2`,
		orgID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) RemoveMember(ctx context.Context, orgID, userID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`,
		orgID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) CountOwners(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organization_members WHERE organization_id = $1 AND role = 'owner'`, orgID).Scan(&count)
	return count, err
}

func (s *Postgres) TransferOwnership(ctx context.Context, orgID, fromUserID, toUserID string, demoteFrom bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify fromUserID is currently an owner
	var fromRole string
	err = tx.QueryRow(ctx, `
		SELECT role FROM organization_members WHERE organization_id = $1 AND user_id = $2`,
		orgID, fromUserID).Scan(&fromRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if fromRole != "owner" {
		return fmt.Errorf("user %s is not an owner", fromUserID)
	}

	// Verify toUserID is a member
	var toExists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2)`,
		orgID, toUserID).Scan(&toExists)
	if err != nil {
		return err
	}
	if !toExists {
		return ErrNotFound
	}

	// Promote toUserID to owner
	_, err = tx.Exec(ctx, `
		UPDATE organization_members SET role = 'owner' WHERE organization_id = $1 AND user_id = $2`,
		orgID, toUserID)
	if err != nil {
		return err
	}

	// Demote fromUserID if requested
	if demoteFrom {
		_, err = tx.Exec(ctx, `
			UPDATE organization_members SET role = 'admin' WHERE organization_id = $1 AND user_id = $2`,
			orgID, fromUserID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ---- Invites ----

func (s *Postgres) CreateInvite(ctx context.Context, i *Invite) error {
	if i.ID == "" {
		i.ID = newID()
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO organization_invites (id, organization_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		i.ID, i.OrganizationID, i.Email, i.Role, i.TokenHash, i.InvitedBy, i.ExpiresAt, i.CreatedAt)
	return mapError(err)
}

func (s *Postgres) GetInviteByToken(ctx context.Context, tokenHash string) (*Invite, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, email, role, token_hash, invited_by, expires_at, accepted_at, created_at
		FROM organization_invites WHERE token_hash = $1`, tokenHash)
	var i Invite
	err := row.Scan(&i.ID, &i.OrganizationID, &i.Email, &i.Role, &i.TokenHash, &i.InvitedBy, &i.ExpiresAt, &i.AcceptedAt, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *Postgres) ListInvites(ctx context.Context, orgID string) ([]Invite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, email, role, token_hash, invited_by, expires_at, accepted_at, created_at
		FROM organization_invites WHERE organization_id = $1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Invite
	for rows.Next() {
		var i Invite
		if err := rows.Scan(&i.ID, &i.OrganizationID, &i.Email, &i.Role, &i.TokenHash, &i.InvitedBy, &i.ExpiresAt, &i.AcceptedAt, &i.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Postgres) RevokeInvite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM organization_invites WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) AcceptInvite(ctx context.Context, tokenHash string, userID string) (*Membership, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	invite, err := s.GetInviteByToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if invite.AcceptedAt != nil {
		return nil, fmt.Errorf("invite already accepted")
	}
	if time.Now().After(invite.ExpiresAt) {
		return nil, fmt.Errorf("invite expired")
	}

	// Check if user is already a member
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id = $1 AND user_id = $2)`,
		invite.OrganizationID, userID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		// Mark invite as accepted but don't create duplicate membership
		now := newTime()
		_, err = tx.Exec(ctx, `UPDATE organization_invites SET accepted_at = $1 WHERE id = $2`, now, invite.ID)
		if err != nil {
			return nil, err
		}
		invite.AcceptedAt = &now
		return &Membership{
			OrganizationID: invite.OrganizationID,
			UserID:         userID,
			Role:           invite.Role,
			JoinedAt:       now,
		}, tx.Commit(ctx)
	}

	now := newTime()
	_, err = tx.Exec(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)`,
		invite.OrganizationID, userID, invite.Role, now)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `UPDATE organization_invites SET accepted_at = $1 WHERE id = $2`, now, invite.ID)
	if err != nil {
		return nil, err
	}

	return &Membership{
		OrganizationID: invite.OrganizationID,
		UserID:         userID,
		Role:           invite.Role,
		JoinedAt:       now,
	}, tx.Commit(ctx)
}

func (s *Postgres) CleanupExpiredInvites(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM organization_invites WHERE expires_at < now() AND accepted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ---- Projects ----

func (s *Postgres) UpdateProject(ctx context.Context, p *Project) error {
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = newTime()
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE projects SET name = $2, slug = $3, updated_at = $4 WHERE id = $1`,
		p.ID, p.Name, p.Slug, p.UpdatedAt)
	if err != nil {
		return mapError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Postgres) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- Audit ----

func (s *Postgres) AppendAuditEvent(ctx context.Context, e *AuditEvent) error {
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = newTime()
	}
	metadataJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}
	// Empty UUID/IP strings become NULL: several callers record actions that
	// have no org/project (or ip), and a silent audit loss is worse than a
	// NULL column. Previously these writes failed (logged, never surfaced).
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_events (id, actor_user_id, actor_key_id, organization_id, project_id, action, target_type, target_id, metadata, ip, user_agent, created_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6, $7, $8, $9, NULLIF($10, '')::inet, $11, $12)`,
		e.ID, e.ActorUserID, e.ActorKeyID, e.OrganizationID, e.ProjectID, e.Action, e.TargetType, e.TargetID, metadataJSON, e.IP, e.UserAgent, e.CreatedAt)
	return err
}

// ---- Webhook deliveries (Phase 8.9) ----

// GetOrCreateWebhookSecret returns the project's HMAC signing secret,
// generating a random one on first use.
func (s *Postgres) GetOrCreateWebhookSecret(ctx context.Context, projectID string) (string, error) {
	var secret *string
	err := s.pool.QueryRow(ctx, `SELECT webhook_secret FROM projects WHERE id = $1`, projectID).Scan(&secret)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if secret != nil && *secret != "" {
		return *secret, nil
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	gen := hex.EncodeToString(b[:])
	if _, err := s.pool.Exec(ctx, `UPDATE projects SET webhook_secret = $1 WHERE id = $2`, gen, projectID); err != nil {
		return "", err
	}
	return gen, nil
}

// RecordWebhookDelivery persists one delivery outcome and enforces a 30-day
// retention window per project.
func (s *Postgres) RecordWebhookDelivery(ctx context.Context, d *WebhookDelivery) error {
	if d.ID == "" {
		d.ID = newID()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = newTime()
	}
	var triggerID *string
	if d.TriggerID != "" {
		triggerID = &d.TriggerID
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries
		    (id, project_id, trigger_id, target_url, collection, event,
		     attempts, status_code, ok, error, duration_ms, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		d.ID, d.ProjectID, triggerID, d.TargetURL, d.Collection, d.Event,
		d.Attempts, d.StatusCode, d.OK, d.Error, d.DurationMs, d.CreatedAt)
	if err != nil {
		return mapError(err)
	}
	_, _ = s.pool.Exec(ctx, `
		DELETE FROM webhook_deliveries
		WHERE project_id = $1 AND created_at < now() - interval '30 days'`, d.ProjectID)
	return nil
}

// ListWebhookDeliveries returns recent deliveries, newest first, optionally
// filtered to one trigger. Limit is clamped to [1,200].
func (s *Postgres) ListWebhookDeliveries(ctx context.Context, projectID string, triggerID string, limit int) ([]WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows pgx.Rows
	var err error
	if triggerID != "" {
		rows, err = s.pool.Query(ctx, `
			SELECT id, project_id, trigger_id, target_url, collection, event,
			       attempts, status_code, ok, error, duration_ms, created_at
			FROM webhook_deliveries
			WHERE project_id = $1 AND trigger_id = $2
			ORDER BY created_at DESC LIMIT $3`, projectID, triggerID, limit)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT id, project_id, trigger_id, target_url, collection, event,
			       attempts, status_code, ok, error, duration_ms, created_at
			FROM webhook_deliveries
			WHERE project_id = $1
			ORDER BY created_at DESC LIMIT $2`, projectID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebhookDelivery{}
	for rows.Next() {
		var d WebhookDelivery
		var triggerID *string
		if err := rows.Scan(&d.ID, &d.ProjectID, &triggerID, &d.TargetURL,
			&d.Collection, &d.Event, &d.Attempts, &d.StatusCode, &d.OK,
			&d.Error, &d.DurationMs, &d.CreatedAt); err != nil {
			return nil, err
		}
		if triggerID != nil {
			d.TriggerID = *triggerID
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ---- Mail settings + log (Phase 9.2) ----

// GetMailSettings returns the singleton mail_settings row. Migration 0005
// seeds provider 'log', so this always finds a row on migrated databases.
func (s *Postgres) GetMailSettings(ctx context.Context) (*MailSettings, error) {
	var m MailSettings
	err := s.pool.QueryRow(ctx, `
		SELECT provider, smtp_host, smtp_port, smtp_username,
		       encrypted_password, COALESCE(encryption_key_id, ''), from_address, from_name
		FROM mail_settings WHERE id = 'default'`).Scan(
		&m.Provider, &m.SMTPHost, &m.SMTPPort, &m.SMTPUsername,
		&m.EncryptedPassword, &m.EncryptionKeyID, &m.FromAddress, &m.FromName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &MailSettings{Provider: "log", SMTPPort: 587, FromName: "Openbase"}, nil
		}
		return nil, err
	}
	return &m, nil
}

// UpdateMailSettings upserts the singleton row.
func (s *Postgres) UpdateMailSettings(ctx context.Context, m *MailSettings) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mail_settings
		    (id, provider, smtp_host, smtp_port, smtp_username,
		     encrypted_password, encryption_key_id, from_address, from_name, updated_at)
		VALUES ('default', $1,$2,$3,$4,$5,$6,$7,$8, now())
		ON CONFLICT (id) DO UPDATE SET
		    provider = EXCLUDED.provider, smtp_host = EXCLUDED.smtp_host,
		    smtp_port = EXCLUDED.smtp_port, smtp_username = EXCLUDED.smtp_username,
		    encrypted_password = EXCLUDED.encrypted_password,
		    encryption_key_id = EXCLUDED.encryption_key_id,
		    from_address = EXCLUDED.from_address, from_name = EXCLUDED.from_name,
		    updated_at = now()`,
		m.Provider, m.SMTPHost, m.SMTPPort, m.SMTPUsername,
		m.EncryptedPassword, m.EncryptionKeyID, m.FromAddress, m.FromName)
	return mapError(err)
}

// RecordMailLog persists one send attempt and enforces a 30-day retention
// window.
func (s *Postgres) RecordMailLog(ctx context.Context, e *MailLog) error {
	if e.ID == "" {
		e.ID = newID()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = newTime()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mail_log (id, to_address, template, subject, ok, error, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.ID, e.ToAddress, e.Template, e.Subject, e.OK, e.Error, e.CreatedAt)
	if err != nil {
		return mapError(err)
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM mail_log WHERE created_at < now() - interval '30 days'`)
	return nil
}

// ListMailLog returns recent send attempts, newest first. Limit is clamped
// to [1,200].
func (s *Postgres) ListMailLog(ctx context.Context, limit int) ([]MailLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, to_address, template, subject, ok, error, created_at
		FROM mail_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MailLog{}
	for rows.Next() {
		var e MailLog
		if err := rows.Scan(&e.ID, &e.ToAddress, &e.Template, &e.Subject, &e.OK, &e.Error, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
