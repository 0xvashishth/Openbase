package server

import (
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/apikey"
	"github.com/openbase/openbase/internal/projectauth"
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
		// End-user JWTs (Phase 10) authenticate the data plane and /auth/v1/*
		// refresh-adjacent calls alongside API keys. Anything that is not an
		// ob_ key takes the JWT path: the unverified pid claim routes the
		// lookup, and verification (signature + project binding) decides.
		if !strings.HasPrefix(raw, "ob_") {
			s.serveWithEndUserJWT(w, r, raw, next)
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

		// Phase 10 (A1.1): propagate the key role so the data path (and Phase
		// 11 policies) can distinguish anon from service_role. Pre-role keys
		// read back as service_role via the migration default.
		role := k.Role
		if role == "" {
			role = "service_role"
		}
		ctx := contextWithKeyRole(contextWithProjectID(r.Context(), k.ProjectID), role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// serveWithEndUserJWT authenticates via a per-project user access token.
// The pid claim is a routing hint only — VerifyFor checks the signature
// against that project's keys and the project binding before trusting it.
func (s *Server) serveWithEndUserJWT(w http.ResponseWriter, r *http.Request, raw string, next http.Handler) {
	pid, err := projectauth.UnverifiedProjectID(raw)
	if err != nil || pid == "" {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}
	claims, err := s.projectAuthManager(r).VerifyFor(r.Context(), pid, raw)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}
	ctx := contextWithEndUserID(contextWithProjectID(r.Context(), claims.ProjectID), claims.UserID)
	next.ServeHTTP(w, r.WithContext(ctx))
}
