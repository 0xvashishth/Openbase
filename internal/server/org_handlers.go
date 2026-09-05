package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/metadata"
)

// orgView embeds the caller's role so the dashboard can gate UI without
// a second round-trip. All org-scoped responses include it.
type orgView struct {
	*metadata.Organization
	Role metadata.OrgRole `json:"role"`
}

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
	writeJSON(w, http.StatusCreated, orgView{Organization: org, Role: metadata.RoleOwner})
}

func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.svc.Store.ListOrganizationsForUser(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	userID := userIDFromContext(r.Context())
	out := make([]orgView, 0, len(orgs))
	for _, o := range orgs {
		// We already have the membership from the join in ListOrganizationsForUser,
		// but the current query doesn't select role. Re-fetch via GetMembership
		// (cheap PK lookup) to get the role for each org.
		m, err := s.svc.Store.GetMembership(r.Context(), o.ID, userID)
		role := metadata.RoleMember
		if err == nil {
			role = m.Role
		}
		out = append(out, orgView{Organization: &o, Role: role})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleMember)
	if err != nil {
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
	writeJSON(w, http.StatusOK, orgView{Organization: org, Role: m.Role})
}

type createProjectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	// Creating a project requires admin or owner (matrix: admin+)
	_, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		if err == errForbidden {
			s.forbiddenf(w, "admin role required to create projects")
		} else {
			s.writeErr(w, err)
		}
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

// getProject resolves one project by id, checking org membership. Without it
// the dashboard had to list every project in an org and scan for the one it
// already had the id of.
func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, _, err := s.projectAndOrg(r, r.PathValue("projectID"))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
