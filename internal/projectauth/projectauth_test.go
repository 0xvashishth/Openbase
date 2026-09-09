package projectauth

import (
	"testing"
	"time"

	"github.com/openbase/openbase/internal/auth"
)

func TestIssueVerifyRoundTrip(t *testing.T) {
	ks, err := NewKeySet("proj_123", "https://api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	signed, _, err := ks.Issue("user_1", RoleAuthenticated, AAL1, 0) // default TTL
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ks.Verify(signed)
	if err != nil {
		t.Fatalf("verify own token: %v", err)
	}
	if claims.UserID != "user_1" || claims.ProjectID != "proj_123" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
	if claims.Role != RoleAuthenticated || claims.AAL != AAL1 {
		t.Fatalf("role/aal mismatch: %+v", claims)
	}
	if claims.Issuer != "https://api.example.com/auth/v1/proj_123" {
		t.Fatalf("issuer mismatch: %q", claims.Issuer)
	}
	// AAL2 + anonymous role survive the round trip.
	signed2, _, err := ks.Issue("anon_1", RoleAnonymous, AAL2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := ks.Verify(signed2)
	if err != nil {
		t.Fatal(err)
	}
	if c2.Role != RoleAnonymous || c2.AAL != AAL2 {
		t.Fatalf("role/aal mismatch: %+v", c2)
	}
}

func TestVerifyRejectsForeignProject(t *testing.T) {
	a, _ := NewKeySet("proj_A", "")
	b, _ := NewKeySet("proj_B", "")
	signed, _, err := a.Issue("u1", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Same key material, wrong project binding: import A's public key into B
	// and confirm the project check (not just the signature) rejects it.
	if err := b.ImportPublic(a.ActiveJWK()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Verify(signed); err == nil {
		t.Fatal("expected cross-project rejection")
	}
}

func TestVerifyRejectsTamperedAndExpired(t *testing.T) {
	ks, _ := NewKeySet("proj_1", "")
	signed, _, _ := ks.Issue("u1", "", "", 0)
	tampered := signed[:len(signed)-2] + "xx"
	if _, err := ks.Verify(tampered); err == nil {
		t.Fatal("expected tamper rejection")
	}
	expired, _, err := ks.Issue("u1", "", "", time.Nanosecond) // already past by verify time
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ks.Verify(expired); err == nil {
		t.Fatal("expected expiry rejection")
	}
	if _, err := ks.Verify("not.a.token"); err == nil {
		t.Fatal("expected malformed rejection")
	}
	// Operator HS256 tokens must never verify as end-user tokens.
	op := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "aud", "iss", time.Hour)
	opTok, _, _ := op.Issue("operator_1")
	if _, err := ks.Verify(opTok); err == nil {
		t.Fatal("expected operator-token rejection")
	}
}

func TestRotateKeepsOldTokensVerifiable(t *testing.T) {
	ks, _ := NewKeySet("proj_1", "")
	old, _, _ := ks.Issue("u1", "", "", 0)
	jwk, err := ks.Rotate()
	if err != nil {
		t.Fatal(err)
	}
	if jwk.Kid != ks.ActiveKID() {
		t.Fatal("rotated JWK must be the active key")
	}
	if _, err := ks.Verify(old); err != nil {
		t.Fatalf("old token must verify after rotation: %v", err)
	}
	fresh, _, _ := ks.Issue("u1", "", "", 0)
	if _, err := ks.Verify(fresh); err != nil {
		t.Fatalf("new token must verify: %v", err)
	}
	// Forgetting a retired kid invalidates its tokens; active is protected.
	var oldKID string
	for _, j := range ks.JWKS() {
		if j.Kid != ks.ActiveKID() {
			oldKID = j.Kid
		}
	}
	if oldKID == "" {
		t.Fatal("expected a retired kid")
	}
	if err := ks.Forget(oldKID); err != nil {
		t.Fatal(err)
	}
	if _, err := ks.Verify(old); err == nil {
		t.Fatal("expected forgotten-kid rejection")
	}
	if err := ks.Forget(ks.ActiveKID()); err == nil {
		t.Fatal("expected active-kid protection")
	}
}

func TestJWKExportParseRoundTrip(t *testing.T) {
	ks, _ := NewKeySet("proj_1", "")
	jwk := ks.ActiveJWK()
	if jwk.Kty != "EC" || jwk.Crv != "P-256" || jwk.Alg != "ES256" {
		t.Fatalf("bad JWK header fields: %+v", jwk)
	}
	pub, err := ParsePublicJWK(jwk)
	if err != nil {
		t.Fatal(err)
	}
	if pub.X.Cmp(ks.private[ks.ActiveKID()].PublicKey.X) != 0 {
		t.Fatal("JWK round trip changed the key")
	}
	bad := jwk
	bad.Crv = "P-384"
	if _, err := ParsePublicJWK(bad); err == nil {
		t.Fatal("expected curve rejection")
	}
	bad = jwk
	bad.X = "!!!"
	if _, err := ParsePublicJWK(bad); err == nil {
		t.Fatal("expected base64 rejection")
	}
}

func TestPasswordHashingViaOperatorScheme(t *testing.T) {
	// End-user passwords reuse the bcrypt scheme from internal/auth (one
	// scheme to audit, one cost to tune).
	hash, err := auth.HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.VerifyPassword(hash, "correct horse") {
		t.Fatal("hash must verify")
	}
	if auth.VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password must not verify")
	}
}
