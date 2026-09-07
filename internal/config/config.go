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
	// EncryptionKeyID names the current encryption key stamped on new rows.
	// Defaults to "openbase-master-key-v1".
	EncryptionKeyID string
	// EncryptionKeys is the rotation registry: previously-used key ids mapped
	// to their secrets, so rows encrypted under a rotated key stay readable.
	// The current key is always available for writes and is added automatically.
	EncryptionKeys map[string]string

	// AllowedOrigins for CORS, comma-separated. Empty means allow any origin
	// (development default).
	AllowedOrigins []string

	// PublicBaseURL is the externally-reachable origin of this API, e.g.
	// "https://api.example.com". Advertised to clients by the connect-info
	// endpoint. Empty (default) derives it per-request from Host /
	// X-Forwarded-* headers, which is correct for local dev and simple proxies.
	PublicBaseURL string

	// ProvisioningEnabled toggles "provisioned" database creation. Off by
	// default so the API boots without Docker; enable with
	// OPENBASE_PROVISIONER_ENABLED=true when Docker is available.
	ProvisioningEnabled bool

	// AllowPrivateWebhooks disables the SSRF guard on webhook trigger
	// destinations so self-hosters can target LAN/internal URLs. Default
	// false: webhooks may only target public http(s) hosts. Enable with
	// OPENBASE_ALLOW_PRIVATE_WEBHOOKS=true.
	AllowPrivateWebhooks bool
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
		EncryptionKeyID: envOr("OPENBASE_ENCRYPTION_KEY_ID", "openbase-master-key-v1"),
		EncryptionKeys:  parseKeyMap(os.Getenv("OPENBASE_ENCRYPTION_KEYS")),
		AllowedOrigins:  splitCSV(os.Getenv("OPENBASE_ALLOWED_ORIGINS")),
		PublicBaseURL:   strings.TrimRight(os.Getenv("OPENBASE_PUBLIC_URL"), "/"),
		ProvisioningEnabled: os.Getenv("OPENBASE_PROVISIONER_ENABLED") == "true",
		AllowPrivateWebhooks: os.Getenv("OPENBASE_ALLOW_PRIVATE_WEBHOOKS") == "true",
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

// parseKeyMap parses a rotation registry of the form "id=secret,id=secret".
// Values are trimmed; malformed entries (no '=') are skipped. An empty input
// yields nil (no historical keys).
func parseKeyMap(s string) map[string]string {
	if s == "" {
		return nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		if id != "" {
			out[id] = parts[1]
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}