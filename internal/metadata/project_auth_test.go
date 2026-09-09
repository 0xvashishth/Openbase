package metadata

import (
	"context"
	"errors"
	"testing"
	"time"
)

func setupProject(t *testing.T, s Store) string {
	t.Helper()
	ctx := context.Background()
	u := &User{Email: "owner-" + newID() + "@example.com", PasswordHash: "h"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	org := &Organization{Name: "Acme", Slug: "acme-" + newID()[:8]}
	if err := s.CreateOrganization(ctx, org, u.ID, RoleOwner); err != nil {
		t.Fatal(err)
	}
	p := &Project{OrganizationID: org.ID, Name: "P", Slug: "p-" + newID()[:8], CreatedBy: u.ID}
	if err := s.CreateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func strptr(s string) *string { return &s }

func TestProjectUserLifecycle(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)

	u := &ProjectUser{ProjectID: pID, Email: strptr("end@example.com"), PasswordHash: "ph",
		UserMetadata: map[string]any{"plan": "pro"}}
	if err := s.CreateProjectUser(ctx, u); err != nil {
		t.Fatalf("CreateProjectUser: %v", err)
	}
	if u.ID == "" {
		t.Fatal("id should be auto-generated")
	}

	// Duplicate email in the same project conflicts…
	dup := &ProjectUser{ProjectID: pID, Email: strptr("end@example.com")}
	if err := s.CreateProjectUser(ctx, dup); !IsConflict(err) {
		t.Fatalf("duplicate email: expected conflict, got %v", err)
	}
	// …but the same email in another project is fine.
	other := setupProject(t, s)
	cross := &ProjectUser{ProjectID: other, Email: strptr("end@example.com")}
	if err := s.CreateProjectUser(ctx, cross); err != nil {
		t.Fatalf("cross-project email must not conflict: %v", err)
	}

	byEmail, err := s.GetProjectUserByEmail(ctx, pID, "end@example.com")
	if err != nil {
		t.Fatalf("GetProjectUserByEmail: %v", err)
	}
	if byEmail.ID != u.ID {
		t.Fatal("id mismatch")
	}
	if byEmail.UserMetadata["plan"] != "pro" {
		t.Fatalf("metadata lost: %v", byEmail.UserMetadata)
	}

	byID, err := s.GetProjectUser(ctx, pID, u.ID)
	if err != nil {
		t.Fatalf("GetProjectUser: %v", err)
	}
	if byID.Banned(time.Now()) {
		t.Fatal("fresh user must not be banned")
	}

	// Ban + confirm + metadata update.
	future := time.Now().Add(time.Hour)
	byID.BannedUntil = &future
	now := time.Now()
	byID.EmailConfirmedAt = &now
	byID.UserMetadata = map[string]any{"plan": "team"}
	if err := s.UpdateProjectUser(ctx, byID); err != nil {
		t.Fatalf("UpdateProjectUser: %v", err)
	}
	got, _ := s.GetProjectUser(ctx, pID, u.ID)
	if !got.Banned(time.Now()) || got.EmailConfirmedAt == nil {
		t.Fatal("ban/confirm not persisted")
	}

	// List + search.
	_ = s.CreateProjectUser(ctx, &ProjectUser{ProjectID: pID, Email: strptr("other@example.com")})
	all, err := s.ListProjectUsers(ctx, pID, "", 50, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %d, %v", len(all), err)
	}
	found, err := s.ListProjectUsers(ctx, pID, "other@", 50, 0)
	if err != nil || len(found) != 1 {
		t.Fatalf("search = %d, %v", len(found), err)
	}

	// Scoped reads: wrong project misses.
	if _, err := s.GetProjectUser(ctx, other, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project read should miss, got %v", err)
	}

	if err := s.DeleteProjectUser(ctx, pID, u.ID); err != nil {
		t.Fatalf("DeleteProjectUser: %v", err)
	}
	if _, err := s.GetProjectUser(ctx, pID, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted user should miss, got %v", err)
	}
}

func TestProjectUserAnonymousAndPhone(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)

	// Anonymous users carry neither email nor phone; NULLs never conflict.
	for range 2 {
		if err := s.CreateProjectUser(ctx, &ProjectUser{ProjectID: pID, IsAnonymous: true}); err != nil {
			t.Fatalf("anon create: %v", err)
		}
	}
	if err := s.CreateProjectUser(ctx, &ProjectUser{ProjectID: pID, Phone: strptr("+15550001")}); err != nil {
		t.Fatalf("phone create: %v", err)
	}
	if err := s.CreateProjectUser(ctx, &ProjectUser{ProjectID: pID, Phone: strptr("+15550001")}); !IsConflict(err) {
		t.Fatalf("duplicate phone: expected conflict, got %v", err)
	}
	byPhone, err := s.GetProjectUserByPhone(ctx, pID, "+15550001")
	if err != nil || byPhone.Phone == nil {
		t.Fatalf("GetProjectUserByPhone: %v", err)
	}
}

func TestProjectSessionRotationAndReuse(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)
	u := &ProjectUser{ProjectID: pID, Email: strptr("s@example.com")}
	if err := s.CreateProjectUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	ses := &ProjectSession{ProjectID: pID, UserID: u.ID, RefreshHash: "hash-1",
		UserAgent: "jest", IP: "127.0.0.1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateProjectSession(ctx, ses); err != nil {
		t.Fatalf("CreateProjectSession: %v", err)
	}

	// Happy-path rotation: old revoked, new live.
	next := &ProjectSession{RefreshHash: "hash-2", UserAgent: "jest",
		ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.RotateProjectSession(ctx, "hash-1", next); err != nil {
		t.Fatalf("RotateProjectSession: %v", err)
	}
	old, err := s.GetProjectSessionByHash(ctx, "hash-1")
	if err != nil || !old.Revoked() {
		t.Fatalf("old session must be revoked: %+v %v", old, err)
	}
	live, err := s.GetProjectSessionByHash(ctx, "hash-2")
	if err != nil || live.Revoked() {
		t.Fatalf("new session must be live: %+v %v", live, err)
	}

	// Reuse of the rotated token burns the chain.
	retry := &ProjectSession{RefreshHash: "hash-3", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.RotateProjectSession(ctx, "hash-1", retry); !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("reuse: expected ErrRefreshReuse, got %v", err)
	}
	burned, err := s.GetProjectSessionByHash(ctx, "hash-2")
	if err != nil || !burned.Revoked() {
		t.Fatal("chain must be revoked after reuse")
	}

	// Unknown hash.
	if err := s.RotateProjectSession(ctx, "nope", &ProjectSession{RefreshHash: "x", ExpiresAt: time.Now().Add(time.Hour)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown hash: expected ErrNotFound, got %v", err)
	}

	// Revoke-all + list.
	u2 := &ProjectUser{ProjectID: pID, Email: strptr("t@example.com")}
	if err := s.CreateProjectUser(ctx, u2); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"a", "b"} {
		if err := s.CreateProjectSession(ctx, &ProjectSession{ProjectID: pID, UserID: u2.ID,
			RefreshHash: h, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := s.ListProjectSessions(ctx, pID, u2.ID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("list sessions = %d, %v", len(listed), err)
	}
	if err := s.RevokeProjectUserSessions(ctx, pID, u2.ID); err != nil {
		t.Fatal(err)
	}
	listed, _ = s.ListProjectSessions(ctx, pID, u2.ID)
	for _, x := range listed {
		if !x.Revoked() {
			t.Fatal("all sessions must be revoked")
		}
	}
}

func TestProjectAuthSettingsMFAIdentitiesKeys(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)

	// Settings auto-create with sane defaults.
	st, err := s.GetOrCreateProjectAuthSettings(ctx, pID)
	if err != nil {
		t.Fatalf("GetOrCreateProjectAuthSettings: %v", err)
	}
	if st.PasswordMinLength != 8 || !st.MFAEnabled {
		t.Fatalf("bad defaults: %+v", st)
	}
	st.SiteURL = "https://app.example.com"
	st.RedirectAllowList = []string{"https://app.example.com/**"}
	if err := s.UpdateProjectAuthSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	st2, _ := s.GetOrCreateProjectAuthSettings(ctx, pID)
	if st2.SiteURL != "https://app.example.com" || len(st2.RedirectAllowList) != 1 {
		t.Fatalf("settings not persisted: %+v", st2)
	}

	// Identities.
	u := &ProjectUser{ProjectID: pID, Email: strptr("o@example.com")}
	if err := s.CreateProjectUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	idn := &ProjectIdentity{ProjectID: pID, UserID: u.ID, Provider: "github",
		ProviderUID: "123", IdentityData: map[string]any{"login": "octo"}}
	if err := s.UpsertProjectIdentity(ctx, idn); err != nil {
		t.Fatal(err)
	}
	// Re-upsert refreshes instead of conflicting.
	if err := s.UpsertProjectIdentity(ctx, &ProjectIdentity{ProjectID: pID, UserID: u.ID,
		Provider: "github", ProviderUID: "123"}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	linked, err := s.ListProjectIdentities(ctx, pID, u.ID)
	if err != nil || len(linked) != 1 || linked[0].ProviderUID != "123" {
		t.Fatalf("identities: %+v %v", linked, err)
	}

	// MFA factor + single-use challenge.
	f := &ProjectMFAFactor{ProjectID: pID, UserID: u.ID, FriendlyName: "phone",
		SecretEncrypted: []byte("enc"), EncryptionKeyID: "k1"}
	if err := s.CreateMFAFactor(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMFAFactor(ctx, pID, f.ID)
	if err != nil || got.Status != "unverified" {
		t.Fatalf("factor: %+v %v", got, err)
	}
	ch := &ProjectMFAChallenge{FactorID: f.ID, ChallengeOTPHash: "oph",
		ExpiresAt: time.Now().Add(5 * time.Minute)}
	if err := s.CreateMFAChallenge(ctx, ch); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkMFAChallengeVerified(ctx, ch.ID); err != nil {
		t.Fatal(err)
	}
	// Double-spend loses.
	if err := s.MarkMFAChallengeVerified(ctx, ch.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double verify: expected ErrNotFound, got %v", err)
	}
	if err := s.VerifyMFAFactor(ctx, pID, f.ID); err != nil {
		t.Fatal(err)
	}
	factors, _ := s.ListMFAFactors(ctx, pID, u.ID)
	if len(factors) != 1 || factors[0].Status != "verified" {
		t.Fatalf("factors: %+v", factors)
	}
	if err := s.DeleteMFAFactor(ctx, pID, f.ID); err != nil {
		t.Fatal(err)
	}

	// Signing keys.
	k := &ProjectSigningKey{ProjectID: pID, KID: "kid_1", Alg: "ES256",
		PublicJWK: map[string]any{"kty": "EC"}, PrivateEncrypted: []byte("enc"), EncryptionKeyID: "k1"}
	if err := s.CreateProjectSigningKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	keys, err := s.ListProjectSigningKeys(ctx, pID)
	if err != nil || len(keys) != 1 || keys[0].PublicJWK["kty"] != "EC" {
		t.Fatalf("keys: %+v %v", keys, err)
	}
	if err := s.DeleteProjectSigningKey(ctx, pID, k.ID); err != nil {
		t.Fatal(err)
	}
	if keys, _ := s.ListProjectSigningKeys(ctx, pID); len(keys) != 0 {
		t.Fatal("key must be deleted")
	}
}

func TestProjectAuthCodesSingleUse(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)
	u := &ProjectUser{ProjectID: pID, Email: strptr("code@example.com")}
	if err := s.CreateProjectUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	c := &ProjectAuthCode{ProjectID: pID, UserID: u.ID, Kind: AuthCodeRecovery,
		TokenHash: "hash-recovery-1", ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateProjectAuthCode(ctx, c); err != nil {
		t.Fatalf("CreateProjectAuthCode: %v", err)
	}
	if bad := (&ProjectAuthCode{ProjectID: pID, UserID: u.ID, Kind: "bogus",
		TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour)}); s.CreateProjectAuthCode(ctx, bad) == nil {
		t.Fatal("unknown kind must fail")
	}

	got, err := s.GetProjectAuthCodeByHash(ctx, "hash-recovery-1")
	if err != nil || got.Kind != AuthCodeRecovery {
		t.Fatalf("GetProjectAuthCodeByHash: %+v %v", got, err)
	}
	if err := s.MarkProjectAuthCodeUsed(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	// Consumed: invisible + second consume misses.
	if _, err := s.GetProjectAuthCodeByHash(ctx, "hash-recovery-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("used code must miss, got %v", err)
	}
	if err := s.MarkProjectAuthCodeUsed(ctx, got.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double consume must miss, got %v", err)
	}

	// Expired codes behave as not-found.
	exp := &ProjectAuthCode{ProjectID: pID, UserID: u.ID, Kind: AuthCodeOTPEmail,
		TokenHash: "hash-exp", ExpiresAt: time.Now().Add(-time.Minute)}
	if err := s.CreateProjectAuthCode(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProjectAuthCodeByHash(ctx, "hash-exp"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired code must miss, got %v", err)
	}

	// Bulk revoke by kind, then all.
	for _, h := range []string{"k1", "k2"} {
		if err := s.CreateProjectAuthCode(ctx, &ProjectAuthCode{ProjectID: pID, UserID: u.ID,
			Kind: AuthCodeVerifyEmail, TokenHash: h, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RevokeProjectAuthCodesForUser(ctx, pID, u.ID, AuthCodeVerifyEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProjectAuthCodeByHash(ctx, "k1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("revoked code must miss")
	}
}

func TestProjectAuthProvidersAndOAuthStates(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)

	// Provider upsert / get / list / delete.
	p := &ProjectAuthProvider{ProjectID: pID, Provider: "github", Enabled: true,
		ClientID: "cid", ClientSecretEncrypted: []byte("enc"), EncryptionKeyID: "k1",
		Config: map[string]any{"scopes": "read:user"}}
	if err := s.UpsertProjectAuthProvider(ctx, p); err != nil {
		t.Fatalf("UpsertProjectAuthProvider: %v", err)
	}
	got, err := s.GetProjectAuthProvider(ctx, pID, "github")
	if err != nil || got.ClientID != "cid" || got.Config["scopes"] != "read:user" {
		t.Fatalf("GetProjectAuthProvider: %+v %v", got, err)
	}
	p.ClientID = "cid2"
	if err := s.UpsertProjectAuthProvider(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetProjectAuthProvider(ctx, pID, "github")
	if got.ClientID != "cid2" {
		t.Fatal("upsert must replace")
	}
	listed, err := s.ListProjectAuthProviders(ctx, pID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list providers: %d %v", len(listed), err)
	}
	if err := s.DeleteProjectAuthProvider(ctx, pID, "github"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProjectAuthProvider(ctx, pID, "github"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted provider must miss, got %v", err)
	}

	// OAuth states: create → attach → consume once.
	st := &ProjectOAuthState{ProjectID: pID, Provider: "github", StateHash: "st-hash-1",
		RedirectTo: "https://app.example.com/cb", CodeChallenge: "verifier-abc",
		CodeChallengeMethod: "plain", ExpiresAt: time.Now().Add(10 * time.Minute)}
	if err := s.CreateOAuthState(ctx, st); err != nil {
		t.Fatalf("CreateOAuthState: %v", err)
	}
	u := &ProjectUser{ProjectID: pID, Email: strptr("oauth@example.com")}
	if err := s.CreateProjectUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachOAuthCode(ctx, st.ID, u.ID, "code-hash-1"); err != nil {
		t.Fatalf("AttachOAuthCode: %v", err)
	}
	// Second attach loses (already has a code).
	if err := s.AttachOAuthCode(ctx, st.ID, u.ID, "code-hash-2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double attach must miss, got %v", err)
	}
	consumed, err := s.ConsumeOAuthCode(ctx, "code-hash-1")
	if err != nil || consumed.UserID == nil || *consumed.UserID != u.ID {
		t.Fatalf("ConsumeOAuthCode: %+v %v", consumed, err)
	}
	if _, err := s.ConsumeOAuthCode(ctx, "code-hash-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double consume must miss, got %v", err)
	}
	if _, err := s.GetOAuthStateByHash(ctx, "st-hash-1"); err != nil {
		t.Fatalf("used state stays visible for replay distinction: %v", err)
	}
}

func TestProjectAuthHooksCRUD(t *testing.T) {
	s := setup(t)
	ctx := context.Background()
	pID := setupProject(t, s)

	if _, err := s.GetProjectAuthHook(ctx, pID, HookBeforeUserCreated); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unconfigured hook must miss, got %v", err)
	}
	fn := &Function{ProjectID: pID, Name: "hook-fn", Runtime: "node"}
	if err := s.CreateFunction(ctx, fn); err != nil {
		t.Fatal(err)
	}
	fn2 := &Function{ProjectID: pID, Name: "hook-fn-2", Runtime: "node"}
	if err := s.CreateFunction(ctx, fn2); err != nil {
		t.Fatal(err)
	}
	h := &ProjectAuthHook{ProjectID: pID, Event: HookBeforeUserCreated, FunctionID: fn.ID}
	if err := s.UpsertProjectAuthHook(ctx, h); err != nil {
		t.Fatalf("UpsertProjectAuthHook: %v", err)
	}
	got, err := s.GetProjectAuthHook(ctx, pID, HookBeforeUserCreated)
	if err != nil || got.FunctionID != h.FunctionID || got.FailOpen {
		t.Fatalf("GetProjectAuthHook: %+v %v", got, err)
	}
	// Replace (one row per event).
	h2 := &ProjectAuthHook{ProjectID: pID, Event: HookBeforeUserCreated, FunctionID: fn2.ID, FailOpen: true}
	if err := s.UpsertProjectAuthHook(ctx, h2); err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListProjectAuthHooks(ctx, pID)
	if err != nil || len(listed) != 1 || !listed[0].FailOpen {
		t.Fatalf("list hooks: %+v %v", listed, err)
	}
	if err := s.UpsertProjectAuthHook(ctx, &ProjectAuthHook{ProjectID: pID, Event: "bogus", FunctionID: fn.ID}); err == nil {
		t.Fatal("unknown event must fail")
	}
	if err := s.DeleteProjectAuthHook(ctx, pID, HookBeforeUserCreated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProjectAuthHook(ctx, pID, HookBeforeUserCreated); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted hook must miss")
	}
}
