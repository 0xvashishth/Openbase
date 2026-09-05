package server

import (
	"errors"
	"net/http"

	"github.com/openbase/openbase/internal/metadata"
)

// projectAndOrgRole loads a project and its owning org id, checking that the
// authenticated user has at least the minimum role in that org.
func (s *Server) projectAndOrgRole(r *http.Request, projectID string, min metadata.OrgRole) (*metadata.Project, *metadata.Organization, *metadata.Membership, error) {
	p, err := s.svc.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil, nil, errNotFound
		}
		return nil, nil, nil, err
	}
	return s.authorizeProjectOrgRole(r, projectID, p.OrganizationID, min)
}

// projectAndOrg is the member-minimum wrapper used by read-only handlers.
func (s *Server) projectAndOrg(r *http.Request, projectID string) (*metadata.Project, *metadata.Organization, error) {
	p, org, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleMember)
	return p, org, err
}

func (s *Server) getConnection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}

	conn, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no connection configured for this project")
			return
		}
		s.writeErr(w, err)
		return
	}
	// Never return encrypted credential bytes.
	conn.EncryptedConnString = nil
	conn.EncryptedUsername = nil
	conn.EncryptedPassword = nil
	writeJSON(w, http.StatusOK, conn)
}

// testConnectionRequest carries a BYODB connection string for a test before
// saving anything. See ADAPTERS.md §5.
type testConnectionRequest struct {
	ConnectionString string `json:"connection_string"`
}

type testConnectionResponse struct {
	Success bool   `json:"success"`
	Engine  string `json:"engine,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}

	var req testConnectionRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ConnectionString == "" {
		writeError(w, http.StatusBadRequest, "connection_string is required")
		return
	}

	if s.svc.AdapterFactory == nil {
		writeError(w, http.StatusNotImplemented, "adapter engine not configured")
		return
	}

	engine, err := s.svc.AdapterFactory.TestConnection(r.Context(), req.ConnectionString)
	if err != nil {
		writeJSON(w, http.StatusOK, testConnectionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, testConnectionResponse{
		Success: true,
		Engine:  string(engine),
	})
}
