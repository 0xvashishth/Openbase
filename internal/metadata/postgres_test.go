package metadata

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
	"github.com/openbase/openbase/migrations"
)

func setup(t *testing.T) Store {
	t.Helper()
	pg := testutil.StartPostgres(t)
	t.Logf("using test postgres at %s", pg.DSN)

	pool, err := pgxpool.New(context.Background(), pg.DSN)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrations.Apply(context.Background(), pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return NewPostgres(pool)
}

func TestUserLifecycle(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	if err := s.CreateUser(ctx, &User{Email: "a@example.com", PasswordHash: "hash", FullName: "A"}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Duplicate email must conflict.
	if err := s.CreateUser(ctx, &User{Email: "A@example.com", PasswordHash: "h2"}); !IsConflict(err) {
		t.Fatalf("duplicate email: expected conflict, got %v", err)
	}

	got, err := s.GetUserByEmail(ctx, "a@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID == "" {
		t.Fatal("user id should be auto-generated")
	}
	if got.Email != "a@example.com" {
		t.Fatalf("email not normalized/lowercased: %q", got.Email)
	}

	byID, err := s.GetUserByID(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID.FullName != "A" {
		t.Fatalf("FullName = %q", byID.FullName)
	}

	if _, err := s.GetUserByID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user should be ErrNotFound, got %v", err)
	}
}

func TestOrganizationAndProjectLifecycle(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	u := &User{Email: "owner@example.com", PasswordHash: "h"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	org := &Organization{Name: "Acme", Slug: "acme"}
	if err := s.CreateOrganization(ctx, org, u.ID, RoleOwner); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.ID == "" {
		t.Fatal("org id should be generated")
	}
	if org.CreatedBy != u.ID {
		t.Fatalf("org.CreatedBy = %q, want %q", org.CreatedBy, u.ID)
	}

	// Creator should be member + owner.
	mems, err := s.ListMembers(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mems) != 1 || mems[0].Role != RoleOwner || mems[0].UserID != u.ID {
		t.Fatalf("unexpected members: %+v", mems)
	}

	orgs, err := s.ListOrganizationsForUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Fatalf("unexpected org list: %+v", orgs)
	}

	// Duplicate org slug conflicts.
	if err := s.CreateOrganization(ctx, &Organization{Name: "Acme2", Slug: "acme"}, u.ID, RoleOwner); !IsConflict(err) {
		t.Fatalf("duplicate slug: expected conflict, got %v", err)
	}

	p := &Project{OrganizationID: org.ID, Name: "Web", Slug: "web", CreatedBy: u.ID}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("project id should be generated")
	}

	projects, err := s.ListProjects(ctx, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].ID != p.ID {
		t.Fatalf("unexpected projects: %+v", projects)
	}

	// Duplicate project slug within org conflicts.
	if err := s.CreateProject(ctx, &Project{OrganizationID: org.ID, Name: "Web2", Slug: "web", CreatedBy: u.ID}); !IsConflict(err) {
		t.Fatalf("duplicate project slug: expected conflict, got %v", err)
	}

	// Non-member cannot see org.
	other := &User{Email: "other@example.com", PasswordHash: "h"}
	_ = s.CreateUser(ctx, other)
	orgsOther, err := s.ListOrganizationsForUser(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orgsOther) != 0 {
		t.Fatalf("non-member should see no orgs, got %+v", orgsOther)
	}
}

func TestConnectionLifecycle(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	u := &User{Email: "conn@example.com", PasswordHash: "h"}
	_ = s.CreateUser(ctx, u)
	org := &Organization{Name: "O", Slug: "o1"}
	_ = s.CreateOrganization(ctx, org, u.ID, RoleOwner)
	p := &Project{OrganizationID: org.ID, Name: "P", Slug: "p1", CreatedBy: u.ID}
	_ = s.CreateProject(ctx, p)

	c := &Connection{
		ProjectID:           p.ID,
		Mode:                ModeProvisioned,
		Engine:              "postgres",
		EncryptedConnString: []byte("ciphertext"),
		Status:              StatusPending,
	}
	if err := s.CreateConnection(ctx, c); err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}

	got, err := s.GetConnectionByProject(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetConnectionByProject: %v", err)
	}
	if got.Engine != "postgres" || string(got.EncryptedConnString) != "ciphertext" {
		t.Fatalf("unexpected connection: %+v", got)
	}

	// One connection per project enforced (UNIQUE on project_id).
	if err := s.CreateConnection(ctx, &Connection{ProjectID: p.ID, Mode: ModeBYODB, Engine: "mysql"}); !IsConflict(err) {
		t.Fatalf("second connection for same project: expected conflict, got %v", err)
	}

	if err := s.UpdateConnectionStatus(ctx, c.ID, StatusConnected); err != nil {
		t.Fatalf("UpdateConnectionStatus: %v", err)
	}
	got2, _ := s.GetConnectionByProject(ctx, p.ID)
	if got2.Status != StatusConnected || got2.LastCheckedAt == nil {
		t.Fatalf("status not updated: %+v", got2)
	}

	// UpdateConnection overwrites mutable fields, preserving identity.
	err = s.UpdateConnection(ctx, &Connection{
		ID:                    c.ID,
		ProjectID:             p.ID,
		Mode:                  ModeBYODB,
		Engine:                "mysql",
		EncryptedConnString:   []byte("new-cipher"),
		Status:                StatusPending,
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	got3, _ := s.GetConnectionByProject(ctx, p.ID)
	if got3.ID != c.ID || got3.CreatedAt != got.CreatedAt {
		t.Fatalf("UpdateConnection changed identity: %+v", got3)
	}
	if got3.Engine != "mysql" || string(got3.EncryptedConnString) != "new-cipher" {
		t.Fatalf("UpdateConnection did not apply fields: %+v", got3)
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	u := &User{Email: "keys@example.com", PasswordHash: "h"}
	_ = s.CreateUser(ctx, u)
	org := &Organization{Name: "O", Slug: "o2"}
	_ = s.CreateOrganization(ctx, org, u.ID, RoleOwner)
	p := &Project{OrganizationID: org.ID, Name: "P", Slug: "p2", CreatedBy: u.ID}
	_ = s.CreateProject(ctx, p)

	k := &APIKey{ProjectID: p.ID, Name: "prod", KeyHash: "sha256hash", Scopes: []string{"read"}}
	if err := s.CreateAPIKey(ctx, k); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := s.GetAPIKeyByHash(ctx, "sha256hash")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if got.Name != "prod" || len(got.Scopes) != 1 {
		t.Fatalf("unexpected key: %+v", got)
	}

	list, err := s.ListAPIKeys(ctx, p.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListAPIKeys: n=%d err=%v", len(list), err)
	}

	if err := s.RevokeAPIKey(ctx, k.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := s.GetAPIKeyByHash(ctx, "sha256hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked key should not resolve, got %v", err)
	}
}