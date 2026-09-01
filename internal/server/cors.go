package server

import (
	"net/http"
	"strings"
)

// CORS wraps handlers with permissive-by-default cross-origin headers for the
// dashboard. When AllowedOrigins is non-empty, only matching origins get CORS
// headers; otherwise any origin is allowed (development convenience).
type CORS struct {
	AllowedOrigins []string
}

// Middleware returns an http.Handler wrapping next with CORS handling.
func (c *CORS) Middleware(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range c.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = true
	}
	wildcard := len(allowed) == 0

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (wildcard || allowed[strings.TrimRight(origin, "/")]) {
			// Never reflect untrusted origins as wildcard with credentials.
			if wildcard {
				// The API uses Bearer tokens rather than cookies, so the
				// browser's credential mode is irrelevant; wildcard is safe.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}