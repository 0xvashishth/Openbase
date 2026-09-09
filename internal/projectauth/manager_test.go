package projectauth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/secrets"
	"github.com/openbase/openbase/internal/testutil"
	"github.com/openbase/openbase/migrations"
)

func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	pg := testutil.StartPostgres(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := metadata.NewPostgres(pool)

	// A project row to hang keys off (org + operator user first).
	op := &metadata.User{Email: "op-" + t.Name() + "@example.com", PasswordHash: "h"}
	if err := store.CreateUser(ctx, op); err != nil {
		t.Fatal(err)
	}
	org := &metadata.Organization{Name: "O", Slug: "o-" + t.Name()}
	if err := store.CreateOrganization(ctx, org, op.ID, metadata.RoleOwner); err != nil {
		t.Fatal(err)
	}
	proj := &metadata.Project{OrganizationID: org.ID, Name: "P", Slug: "p-" + t.Name(), CreatedBy: op.ID}
	if err := store.CreateProject(ctx, proj); err != nil {
		t.Fatal(err)
	}
	sec, err := secrets.New("test-master-secret-with-enough-entropy", "test-key-v1")
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(store, sec, "https://api.example.com"), proj.ID
}

func TestManagerBootstrapIssueVerify(t *testing.T) {
	m, pID := testManager(t)
	ctx := context.Background()

	ks, err := m.EnsureKeySet(ctx, pID)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.EnsureKeySet(ctx, pID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ActiveKID() != ks.ActiveKID() {
		t.Fatal("second ensure must reuse the persisted key")
	}

	signed, _, err := m.IssueFor(ctx, pID, "user_1", RoleAuthenticated, AAL1, 0)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.VerifyFor(ctx, pID, signed)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != "user_1" || claims.ProjectID != pID {
		t.Fatalf("claims: %+v", claims)
	}
}

func TestManagerRotate(t *testing.T) {
	m, pID := testManager(t)
	ctx := context.Background()

	old, _, err := m.IssueFor(ctx, pID, "u1", "", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := m.RotateFor(ctx, pID)
	if err != nil {
		t.Fatal(err)
	}
	if jwk.Kid == "" {
		t.Fatal("rotated JWK must carry a kid")
	}
	if _, err := m.VerifyFor(ctx, pID, old); err != nil {
		t.Fatalf("old token must verify: %v", err)
	}
	fresh, _, err := m.IssueFor(ctx, pID, "u1", "", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyFor(ctx, pID, fresh); err != nil {
		t.Fatal(err)
	}
	keys, err := m.PublicJWKS(ctx, pID)
	if err != nil || len(keys) != 2 {
		t.Fatalf("JWKS must hold both keys: %d %v", len(keys), err)
	}
}

func TestManagerVerifyUnknownProject(t *testing.T) {
	m, _ := testManager(t)
	if _, err := m.VerifyFor(context.Background(), "00000000-0000-0000-0000-000000000000", "x.y.z"); err == nil {
		t.Fatal("expected error for keyless project")
	}
}
