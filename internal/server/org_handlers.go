package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

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

type updateOrgRequest struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`
}

// updateOrg handles PATCH /v1/orgs/{orgID} - admin+ can rename/slug
func (s *Server) updateOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req updateOrgRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
	}
	if req.Slug != nil {
		trimmed := strings.TrimSpace(*req.Slug)
		req.Slug = &trimmed
		if *req.Slug == "" {
			writeError(w, http.StatusBadRequest, "slug cannot be empty")
			return
		}
	}
	if req.Name == nil && req.Slug == nil {
		writeError(w, http.StatusBadRequest, "name or slug is required")
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
	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.Slug != nil {
		org.Slug = *req.Slug
	}

	if err := s.svc.Store.UpdateOrganization(r.Context(), org); err != nil {
		if metadata.IsConflict(err) {
			writeError(w, http.StatusConflict, "slug already in use")
			return
		}
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orgView{Organization: org, Role: m.Role})
}

// deleteOrg handles DELETE /v1/orgs/{orgID} - owner only, requires slug confirmation
func (s *Server) deleteOrg(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	_, err := s.authorizeOrgRole(r, orgID, metadata.RoleOwner)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req struct {
		Slug string `json:"slug"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug confirmation is required")
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
	if org.Slug != req.Slug {
		writeError(w, http.StatusBadRequest, "slug confirmation does not match")
		return
	}

	// Check for existing projects
	projectCount, err := s.svc.Store.CountProjects(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if projectCount > 0 {
		writeError(w, http.StatusConflict, "organization has projects; delete them first")
		return
	}

	if err := s.svc.Store.DeleteOrganization(r.Context(), orgID); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ---- Member management ----

type memberView struct {
	*metadata.Membership
	Email    string `json:"email"`
	FullName string `json:"full_name,omitempty"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateMemberRequest struct {
	Role string `json:"role"`
}

type transferOwnershipRequest struct {
	UserID     string `json:"user_id"`
	DemoteSelf bool   `json:"demote_self,omitempty"`
}

// listMembers handles GET /v1/orgs/{orgID}/members - member+ can list
func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleMember)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	members, err := s.svc.Store.ListMembersWithUsers(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]memberView, 0, len(members))
	for _, mb := range members {
		out = append(out, memberView{
			Membership: &metadata.Membership{
				OrganizationID: mb.UserID,
				UserID:         mb.UserID,
				Role:           mb.Role,
				JoinedAt:       mb.JoinedAt,
			},
			Email:    mb.Email,
			FullName: mb.FullName,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"members": out,
		"role":    m.Role,
	})
}

// addMember handles POST /v1/orgs/{orgID}/members - admin+ can add existing users
func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req addMemberRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	role := metadata.OrgRole(req.Role)
	if role != metadata.RoleOwner && role != metadata.RoleAdmin && role != metadata.RoleMember {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return
	}
	if role == metadata.RoleOwner && m.Role != metadata.RoleOwner {
		writeError(w, http.StatusForbidden, "only owners can grant owner role")
		return
	}

	// Look up user by email
	user, err := s.svc.Store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no Openbase account uses that email")
			return
		}
		s.writeErr(w, err)
		return
	}

	// Check if already a member
	existing, err := s.svc.Store.GetMembership(r.Context(), orgID, user.ID)
	if err == nil && existing != nil {
		writeError(w, http.StatusConflict, "user is already a member of this organization")
		return
	}

	if err := s.svc.Store.AddMember(r.Context(), &metadata.Membership{
		OrganizationID: orgID,
		UserID:         user.ID,
		Role:           role,
		JoinedAt:       time.Now().UTC(),
	}); err != nil {
		s.writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    role,
	})
}

// updateMemberRole handles PATCH /v1/orgs/{orgID}/members/{userID} - admin+ can change roles
func (s *Server) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	userID := r.PathValue("userID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req updateMemberRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}
	newRole := metadata.OrgRole(req.Role)
	if newRole != metadata.RoleOwner && newRole != metadata.RoleAdmin && newRole != metadata.RoleMember {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return
	}

	// Get current membership
	target, err := s.svc.Store.GetMembership(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		s.writeErr(w, err)
		return
	}

	// Self-role changes: owner cannot demote themselves; admin cannot change own role
	actorID := userIDFromContext(r.Context())
	if target.UserID == actorID {
		if m.Role != metadata.RoleOwner {
			writeError(w, http.StatusForbidden, "only owners can change their own role (use transfer ownership)")
			return
		}
		if newRole != metadata.RoleOwner {
			writeError(w, http.StatusForbidden, "owners cannot demote themselves; transfer ownership first")
			return
		}
	}

	// Admin granting/removing owner
	if newRole == metadata.RoleOwner && m.Role != metadata.RoleOwner {
		writeError(w, http.StatusForbidden, "only owners can grant owner role")
		return
	}
	if target.Role == metadata.RoleOwner && m.Role != metadata.RoleOwner {
		writeError(w, http.StatusForbidden, "only owners can modify owner roles")
		return
	}

	// Last-owner guard
	ownerCount, err := s.svc.Store.CountOwners(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if target.Role == metadata.RoleOwner && newRole != metadata.RoleOwner && ownerCount <= 1 {
		writeError(w, http.StatusConflict, "organization must have at least one owner")
		return
	}

	if err := s.svc.Store.UpdateMemberRole(r.Context(), orgID, userID, newRole); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": newRole})
}

// removeMember handles DELETE /v1/orgs/{orgID}/members/{userID} - admin+ can remove, self can leave
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	userID := r.PathValue("userID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	actorID := userIDFromContext(r.Context())

	// Self-removal = leave organization
	if userID == actorID {
		// Owner cannot leave unless they transfer ownership first
		if m.Role == metadata.RoleOwner {
			writeError(w, http.StatusForbidden, "owners cannot leave; transfer ownership first")
			return
		}
		if err := s.svc.Store.RemoveMember(r.Context(), orgID, userID); err != nil {
			s.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"left": true})
		return
	}

	// Removing another member
	target, err := s.svc.Store.GetMembership(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "member not found")
			return
		}
		s.writeErr(w, err)
		return
	}

	// Admin removing owner
	if target.Role == metadata.RoleOwner && m.Role != metadata.RoleOwner {
		writeError(w, http.StatusForbidden, "only owners can remove owners")
		return
	}

	// Last-owner guard
	ownerCount, err := s.svc.Store.CountOwners(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if target.Role == metadata.RoleOwner && ownerCount <= 1 {
		writeError(w, http.StatusConflict, "organization must have at least one owner")
		return
	}

	if err := s.svc.Store.RemoveMember(r.Context(), orgID, userID); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// transferOwnership handles POST /v1/orgs/{orgID}/transfer-ownership - owner only
func (s *Server) transferOwnership(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	_, err := s.authorizeOrgRole(r, orgID, metadata.RoleOwner)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req transferOwnershipRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	actorID := userIDFromContext(r.Context())
	if req.UserID == actorID {
		writeError(w, http.StatusBadRequest, "cannot transfer ownership to yourself")
		return
	}

	// Verify target is a member
	_ , err = s.svc.Store.GetMembership(r.Context(), orgID, req.UserID)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeError(w, http.StatusNotFound, "target user is not a member of this organization")
			return
		}
		s.writeErr(w, err)
		return
	}

	if err := s.svc.Store.TransferOwnership(r.Context(), orgID, actorID, req.UserID, req.DemoteSelf); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transferred_to": req.UserID,
		"demoted_self":   req.DemoteSelf,
	})
}

type updateProjectRequest struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`
}

// updateProject handles PATCH /v1/projects/{projectID} - admin+ can rename/slug
func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	_, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	var req updateProjectRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
		if *req.Name == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
	}
	if req.Slug != nil {
		trimmed := strings.TrimSpace(*req.Slug)
		req.Slug = &trimmed
		if *req.Slug == "" {
			writeError(w, http.StatusBadRequest, "slug cannot be empty")
			return
		}
	}
	if req.Name == nil && req.Slug == nil {
		writeError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	p, err := s.svc.Store.GetProject(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Slug != nil {
		p.Slug = *req.Slug
	}
	if err := s.svc.Store.UpdateProject(r.Context(), p); err != nil {
		if metadata.IsConflict(err) {
			writeError(w, http.StatusConflict, "slug already in use in this org")
			return
		}
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// deleteProject handles DELETE /v1/projects/{projectID} - owner only
// Cascade delete: destroy provisioned container, stop triggers, revoke keys, invalidate pool, delete project row
func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	_, org, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleOwner)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	// Get connection to check for provisioned container
	conn, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	if err != nil && !errors.Is(err, metadata.ErrNotFound) {
		s.writeErr(w, err)
		return
	}

	// If provisioned, destroy the container first
	if conn != nil && conn.Mode == metadata.ModeProvisioned && conn.ContainerID != nil && *conn.ContainerID != "" && s.svc.Provisioner != nil {
		if err := s.svc.Provisioner.Destroy(r.Context(), projectID); err != nil {
			s.writeErr(w, err)
			return
		}
	}

	// Stop triggers for this project
	if s.svc.TriggerService != nil {
		s.svc.TriggerService.StopProject(projectID)
	}

	// Invalidate adapter pool and pkCache
	s.invalidateProjectAdapters(projectID)

	// Delete project (cascades to connections, api_keys, triggers, functions via DB)
	if err := s.svc.Store.DeleteProject(r.Context(), projectID); err != nil {
		s.writeErr(w, err)
		return
	}

	// Audit log (Phase 9.8 will wire a real sink; for now log via slog)
	s.svc.Log.Info("project deleted", "org", org.ID, "project", projectID, "actor", userIDFromContext(r.Context()))

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}