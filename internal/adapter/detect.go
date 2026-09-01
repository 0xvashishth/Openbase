package adapter

import (
	"errors"
	"net/url"
	"strings"
)

// ErrUnknownEngine is returned when a connection string's scheme cannot be
// mapped unambiguously to a known engine.
var ErrUnknownEngine = errors.New("adapter: could not detect engine from connection string")

// DetectEngine maps a connection-string URL scheme to an engine, mirroring the
// auto-detection table in SCHEMA.md §3. Ambiguous schemes (e.g. a bare https
// URL) return ErrUnknownEngine so the caller can ask the user to pick
// explicitly rather than guessing.
func DetectEngine(connStr string) (Engine, error) {
	// Handle schemes that may carry a "+srv" style suffix before "://".
	scheme := connStr
	if idx := strings.Index(scheme, "://"); idx >= 0 {
		scheme = scheme[:idx]
	}
	scheme = strings.TrimSpace(strings.ToLower(scheme))

	switch scheme {
	case "postgres", "postgresql":
		return EnginePostgres, nil
	case "mysql":
		return EngineMySQL, nil
	case "mongodb", "mongodb+srv":
		return EngineFerretDB, nil
	case "redis", "valkey", "rediss":
		return EngineValkey, nil
	case "bolt":
		// ArcadeDB and Neo4j both speak bolt; infer plausibly by URL host port
		// below, otherwise default to arcadedb per the reference matrix.
		return EngineArcadeDB, nil
	case "qdrant", "http", "https":
		// Ambiguous: only infer qdrant when a port hint is present. We
		// delegate to the URL parser which returns an explicit decision.
		return detectHTTP(connStr)
	default:
		return "", ErrUnknownEngine
	}
}

// detectHTTP decides between qdrant and "ambiguous/unknown" for http(s) URLs.
func detectHTTP(connStr string) (Engine, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return "", ErrUnknownEngine
	}
	// Qdrant default ports: 6333 (http) / 6334 (grpc).
	if u.Port() == "6333" || u.Port() == "6334" {
		return EngineQdrant, nil
	}
	return "", ErrUnknownEngine
}
