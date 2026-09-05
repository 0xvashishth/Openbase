package server

import (
	"errors"
	"fmt"
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

// authorizeOrgRole returns the caller's membership if they meet the minimum
// role requirement for the organization. Returns 403 if not a member or role
// is insufficient. The membership is returned so handlers can access the role
// without a second lookup.
func (s *Server) authorizeOrgRole(r *http.Request, orgID string, min metadata.OrgRole) (*metadata.Membership, error) {
	userID := userIDFromContext(r.Context())
	if userID == "" {
		return nil, errForbidden
	}
	m, err := s.svc.Store.GetMembership(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, errForbidden
		}
		return nil, err
	}
	if !m.Role.AtLeast(min) {
		return nil, fmt.Errorf("%w: %s role required", errForbidden, min)
	}
	return m, nil
}

// authorizeOrg returns 403 unless the authenticated user is a member of orgID.
// Kept for backward compatibility with ~30 existing call sites that only need
// "member or above".
func (s *Server) authorizeOrg(r *http.Request, orgID string) error {
	_, err := s.authorizeOrgRole(r, orgID, metadata.RoleMember)
	return err
}

// authorizeProjectOrgRole is the project-scoped equivalent: it checks org
// membership with a minimum role, then verifies the project belongs to that
// org. Returns the project, org, and membership so handlers don't need
// additional lookups.
func (s *Server) authorizeProjectOrgRole(r *http.Request, projectID, orgID string, min metadata.OrgRole) (*metadata.Project, *metadata.Organization, *metadata.Membership, error) {
	m, err := s.authorizeOrgRole(r, orgID, min)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := s.svc.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil, nil, errNotFound
		}
		return nil, nil, nil, err
	}
	if p.OrganizationID != orgID {
		return nil, nil, nil, errForbidden
	}
	org, err := s.svc.Store.GetOrganization(r.Context(), orgID)
	if err != nil {
		return nil, nil, nil, err
	}
	return p, org, m, nil
}

// authorizeProjectOrg kept for compatibility; requires member role.
func (s *Server) authorizeProjectOrg(r *http.Request, projectID, orgID string) error {
	_, _, _, err := s.authorizeProjectOrgRole(r, projectID, orgID, metadata.RoleMember)
	return err
}

// forbiddenf writes a 403 with a message that includes the required role,
// so the dashboard can surface a real reason instead of "forbidden".
func (s *Server) forbiddenf(w http.ResponseWriter, format string, args ...any) {
	writeError(w, http.StatusForbidden, fmt.Sprintf(format, args...))
}
