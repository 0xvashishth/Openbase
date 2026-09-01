package apikey

import (
	"strings"
	"testing"
)

func TestNewKeyFormat(t *testing.T) {
	plain, hash, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !strings.HasPrefix(plain, prefix) {
		t.Fatalf("plaintext key should start with %q, got %q", prefix, plain)
	}
	if len(plain) <= len(prefix) {
		t.Fatal("plaintext key should have random suffix")
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if hash == Hash(plain+"x") {
		t.Fatal("hash should be specific to the key")
	}
	if strings.Contains(plain, "=") || strings.Contains(plain, "+") || strings.Contains(plain, "/") {
		t.Fatal("raw base64url should not contain padding or unsafe chars")
	}
}

func TestHashDeterministic(t *testing.T) {
	k := "ob_testkey123"
	if Hash(k) != Hash(k) {
		t.Fatal("hash should be deterministic")
	}
	if len(Hash(k)) != 64 {
		t.Fatalf("sha256 hex should be 64 chars, got %d", len(Hash(k)))
	}
}

func TestNormalize(t *testing.T) {
	if _, err := Normalize(" bad-key "); err == nil {
		t.Fatal("normalize should reject keys without the prefix")
	}
	got, err := Normalize("  ob_validkey  ")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != "ob_validkey" {
		t.Fatalf("Normalize = %q, want trimmed key", got)
	}
}

func TestNewKeysUnique(t *testing.T) {
	k1, _, _ := New()
	k2, _, _ := New()
	if k1 == k2 {
		t.Fatal("generated keys should be unique")
	}
}