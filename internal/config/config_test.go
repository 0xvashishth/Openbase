package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	os.Setenv("OPENBASE_ADDR", "")
	os.Setenv("OPENBASE_JWT_SECRET", "")
	os.Setenv("OPENBASE_DATABASE_URL", "")
	os.Setenv("ENV", "")
	c, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if c.Addr != ":8080" {
		t.Fatalf("default addr = %q", c.Addr)
	}
	if c.DatabaseURL == "" {
		t.Fatal("database url should default to something")
	}
	if c.TokenTTL != 24*time.Hour {
		t.Fatalf("default token TTL = %v", c.TokenTTL)
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("OPENBASE_ADDR", ":9999")
	os.Setenv("OPENBASE_JWT_SECRET", "custom-secret")
	defer func() {
		os.Unsetenv("OPENBASE_ADDR")
		os.Unsetenv("OPENBASE_JWT_SECRET")
	}()

	c, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if c.Addr != ":9999" {
		t.Fatalf("addr = %q, want :9999", c.Addr)
	}
	if c.JWTSecret != "custom-secret" {
		t.Fatalf("jwt secret = %q", c.JWTSecret)
	}
}

func TestProductionRequiresRealSecret(t *testing.T) {
	os.Setenv("ENV", "production")
	os.Setenv("OPENBASE_JWT_SECRET", "dev-only-secret-change-me")
	defer func() {
		os.Unsetenv("ENV")
		os.Unsetenv("OPENBASE_JWT_SECRET")
	}()
	if _, err := Default(); err == nil {
		t.Fatal("production mode with default secret should error")
	}
}

func TestProductionAllowsCustomSecret(t *testing.T) {
	os.Setenv("ENV", "production")
	os.Setenv("OPENBASE_JWT_SECRET", "a-real-production-secret")
	defer func() {
		os.Unsetenv("ENV")
		os.Unsetenv("OPENBASE_JWT_SECRET")
	}()
	if _, err := Default(); err != nil {
		t.Fatalf("custom production secret should be accepted: %v", err)
	}
}