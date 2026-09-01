// Package config loads runtime configuration from the environment with sane
// defaults suitable for local development and docker-compose.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Config bundles all runtime settings.
type Config struct {
	// HTTP server.
	Addr            string
	ShutdownTimeout time.Duration

	// Platform metadata database (the platform's own Postgres).
	DatabaseURL string

	// Auth / tokens.
	JWTSecret  string
	JWTIssuer  string
	JWTAudience string
	TokenTTL   time.Duration

	// Secrets. V1 uses an envelope-encryption key configured here; a real
	// KMS/vault integration is scheduled for a later phase (PHASES.md §6).
	EncryptionKey string

	// AllowedOrigins for CORS, comma-separated. Empty means allow any origin
	// (development default).
	AllowedOrigins []string
}

// Default returns configuration populated from the environment.
func Default() (*Config, error) {
	c := &Config{
		Addr:            envOr("OPENBASE_ADDR", ":8080"),
		ShutdownTimeout: 10 * time.Second,
		DatabaseURL:     envOr("OPENBASE_DATABASE_URL", "postgres://openbase:openbase@localhost:5432/openbase?sslmode=disable"),
		JWTSecret:       envOr("OPENBASE_JWT_SECRET", "dev-only-secret-change-me"),
		JWTIssuer:       envOr("OPENBASE_JWT_ISSUER", "openbase"),
		JWTAudience:     envOr("OPENBASE_JWT_AUDIENCE", "openbase-dashboard"),
		TokenTTL:        24 * time.Hour,
		EncryptionKey:   envOr("OPENBASE_ENCRYPTION_KEY", ""),
		AllowedOrigins:  splitCSV(os.Getenv("OPENBASE_ALLOWED_ORIGINS")),
	}

	if c.JWTSecret == "dev-only-secret-change-me" && os.Getenv("ENV") == "production" {
		return nil, errors.New("config: OPENBASE_JWT_SECRET must be set in production")
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}