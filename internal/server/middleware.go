package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/metadata"
)

const bearerPrefix = "Bearer "

// requireAuth validates the Authorization bearer token and injects the user ID
// into the request context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ah := r.Header.Get("Authorization")
		if !strings.HasPrefix(ah, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token := strings.TrimPrefix(ah, bearerPrefix)
		claims, err := s.svc.Tokens.Parse(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		ctx := contextWithUserID(r.Context(), claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authorizeOrg returns 403 unless the authenticated user is a member of orgID.
func (s *Server) authorizeOrg(r *http.Request, orgID string) error {
	userID := userIDFromContext(r.Context())
	if userID == "" {
		return errForbidden
	}
	_, err := s.svc.Store.GetMembership(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return errForbidden
		}
		return err
	}
	return nil
}

func (s *Server) authorizeProjectOrg(r *http.Request, projectID, orgID string) error {
	if err := s.authorizeOrg(r, orgID); err != nil {
		return err
	}
	p, err := s.svc.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return errNotFound
		}
		return err
	}
	if p.OrganizationID != orgID {
		return errForbidden
	}
	return nil
}
