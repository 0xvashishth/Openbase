package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/metadata"
)

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	var req createOrgRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}

	org := &metadata.Organization{Name: req.Name, Slug: req.Slug}
	err := s.svc.Store.CreateOrganization(r.Context(), org, userIDFromContext(r.Context()), metadata.RoleOwner)
	if err != nil {
		if metadata.IsConflict(err) {
			writeError(w, http.StatusConflict, "slug already in use")
			return
		}
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.svc.Store.ListOrganizationsForUser(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (s *Server) getOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if err := s.authorizeOrg(r, orgID); err != nil {
		s.writeErr(w, err)
		return
	}
	org, err := s.svc.Store.GetOrganization(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

type createProjectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if err := s.authorizeOrg(r, orgID); err != nil {
		s.writeErr(w, err)
		return
	}

	var req createProjectRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.TrimSpace(req.Slug)
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}

	p := &metadata.Project{
		OrganizationID: orgID,
		Name:           req.Name,
		Slug:           req.Slug,
		CreatedBy:      userIDFromContext(r.Context()),
	}
	if err := s.svc.Store.CreateProject(r.Context(), p); err != nil {
		if metadata.IsConflict(err) {
			writeError(w, http.StatusConflict, "slug already in use in this org")
			return
		}
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if err := s.authorizeOrg(r, orgID); err != nil {
		s.writeErr(w, err)
		return
	}
	projects, err := s.svc.Store.ListProjects(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}
