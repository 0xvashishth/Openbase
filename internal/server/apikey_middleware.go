package server

import (
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/apikey"
)

// requireAPIKey authenticates external requests via an API key. On success it
// injects the project ID into the request context so handlers can scope work
// to the correct project.
//
// Only the SHA-256 hash of a key is ever stored (SCHEMA.md §1 `api_keys.key_hash`,
// internal/apikey), so a lookup by hash is the authentication — the plaintext is
// never compared or stored.
func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ah := r.Header.Get("Authorization")
		if !strings.HasPrefix(ah, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}
		raw := strings.TrimPrefix(ah, bearerPrefix)
		normalized, err := apikey.Normalize(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid API key format")
			return
		}
		hash := apikey.Hash(normalized)

		k, err := s.svc.Store.GetAPIKeyByHash(r.Context(), hash)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}
		if k.RevokedAt != nil {
			writeError(w, http.StatusUnauthorized, "API key has been revoked")
			return
		}

		ctx := contextWithProjectID(r.Context(), k.ProjectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
