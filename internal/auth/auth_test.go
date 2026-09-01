package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || strings.Contains(hash, "s3cret") {
		t.Fatalf("hash should not be empty or contain plaintext, got %q", hash)
	}
	if !VerifyPassword(hash, "s3cret") {
		t.Fatal("VerifyPassword should accept the correct password")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("VerifyPassword should reject a wrong password")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	m := NewTokenManager(strings.Repeat("a", 32), "aud", "iss", time.Hour)
	tok, exp, err := m.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if exp.Before(time.Now()) {
		t.Fatal("expiry should be in the future")
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("claims.UserID = %q, want %q", claims.UserID, "user-1")
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "aud" {
		t.Fatalf("audience claim = %v, want [aud]", claims.Audience)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	m := NewTokenManager(strings.Repeat("a", 32), "aud", "iss", time.Hour)
	other := NewTokenManager(strings.Repeat("b", 32), "aud", "iss", time.Hour)
	tok, _, err := m.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("Parse with wrong secret should fail")
	}
}

func TestParseRejectsWrongAudience(t *testing.T) {
	m := NewTokenManager(strings.Repeat("a", 32), "right-aud", "iss", time.Hour)
	tok, _, err := m.Issue("user-1")
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokenManager(strings.Repeat("a", 32), "wrong-aud", "iss", time.Hour)
	if _, err := other.Parse(tok); err == nil {
		t.Fatal("Parse with wrong audience should fail")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	m := NewTokenManager(strings.Repeat("a", 32), "aud", "iss", time.Hour)
	if _, err := m.Parse("not.a.token"); err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestRandomHexDeterministicNonEmpty(t *testing.T) {
	a, err := RandomHex(16)
	if err != nil {
		t.Fatalf("RandomHex: %v", err)
	}
	b, err := RandomHex(16)
	if err != nil {
		t.Fatalf("RandomHex: %v", err)
	}
	if a == "" || a == b {
		t.Fatal("random hex should be non-empty and vary")
	}
	if len(a) < 20 {
		t.Fatalf("random hex too short: %q", a)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") {
		t.Fatal("equal strings should compare equal")
	}
	if ConstantTimeEqual("abc", "abd") {
		t.Fatal("different strings should not compare equal")
	}
}

var _ = errors.Is