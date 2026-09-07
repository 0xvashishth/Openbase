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
//
// The key is read from the `Authorization: Bearer ob_...` header, with a
// `?apiKey=` query-parameter fallback. The fallback exists because browsers
// cannot set request headers on `new WebSocket()` — without it the realtime
// gateway is unauthenticatable from the web. Request logging records only
// URL.Path (see withRequestID), so the query value never lands in platform logs.
func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := ""
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, bearerPrefix) {
			raw = strings.TrimPrefix(ah, bearerPrefix)
		} else {
			raw = r.URL.Query().Get("apiKey")
		}
		if raw == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}
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
		// Per-key quota (Phase 8.7): bound data-plane abuse per credential.
		if !s.checkAPIKeyQuota(w, r, hash) {
			return
		}

		ctx := contextWithProjectID(r.Context(), k.ProjectID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
