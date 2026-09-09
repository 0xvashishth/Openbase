package mfa

import (
	"strings"
	"testing"
	"time"
)

func TestRFC6238Vector(t *testing.T) {
	// RFC 6238 Appendix B, SHA1, T0=0 X=30: secret "12345678901234567890"
	// has base32 form GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	// T=59 → counter 1 → 287082 (last 6 of 94287082).
	got, err := CodeAt(secret, time.Unix(59, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if got != "287082" {
		t.Fatalf("RFC vector: got %s, want 287082", got)
	}
	// T=1111111109 → 081804; T=2000000000 → 279037.
	for ts, want := range map[int64]string{1111111109: "081804", 2000000000: "279037"} {
		got, err := CodeAt(secret, time.Unix(ts, 0).UTC())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("T=%d: got %s, want %s", ts, got, want)
		}
	}
}

func TestVerifyWindow(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cur, _ := CodeAt(secret, now)
	if !Verify(secret, cur, now) {
		t.Fatal("current code must verify")
	}
	prev, _ := CodeAt(secret, now.Add(-Period*time.Second))
	if !Verify(secret, prev, now) {
		t.Fatal("adjacent step must verify (skew)")
	}
	if Verify(secret, "000000", now) && cur == "000000" {
		t.Fatal("unreachable")
	}
	far, _ := CodeAt(secret, now.Add(-10*Period*time.Second))
	if Verify(secret, far, now) {
		t.Fatal("far-past code must not verify")
	}
	if Verify(secret, "12ab56", now) {
		t.Fatal("non-numeric must not verify")
	}
	if Verify(secret, "12345", now) {
		t.Fatal("short code must not verify")
	}
}

func TestURIAndSecretValidation(t *testing.T) {
	secret, _ := GenerateSecret()
	u := URI(secret, "a@example.com", "MyApp")
	if !strings.HasPrefix(u, "otpauth://totp/") || !strings.Contains(u, "secret=") {
		t.Fatalf("bad URI: %s", u)
	}
	if err := ValidateSecret(secret); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecret("!!!"); err == nil {
		t.Fatal("expected base32 rejection")
	}
}
