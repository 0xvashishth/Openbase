package server

import (
	"errors"
	"net/http"

	"github.com/openbase/openbase/internal/metadata"
)

// projectAndOrg loads a project and its owning org id, checking that the
// authenticated user is a member of that org.
func (s *Server) projectAndOrg(r *http.Request, projectID string) (*metadata.Project, *metadata.Organization, error) {
	p, err := s.svc.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil, errNotFound
		}
		return nil, nil, err
	}
	if err := s.authorizeOrg(r, p.OrganizationID); err != nil {
		return nil, nil, err
	}
	org, err := s.svc.Store.GetOrganization(r.Context(), p.OrganizationID)
	if err != nil {
		return nil, nil, err
	}
	return p, org, nil
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
	Success bool                `json:"success"`
	Engine  string              `json:"engine,omitempty"`
	Message string              `json:"message,omitempty"`
}

func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
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
