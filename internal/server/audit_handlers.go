package server

import (
	"net/http"
	"strconv"

	"github.com/openbase/openbase/internal/metadata"
)

// listAuditEvents handles GET /v1/orgs/{orgID}/audit — admins+ read the
// org's audit log.
func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if _, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	events, err := s.svc.Store.ListAuditEvents(r.Context(), orgID, limit)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}
