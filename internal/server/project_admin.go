package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/mail"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/projectauth"
)

// Admin API for end users (Phase 10, A3.3). Every route requires a
// service_role key — enforced by requireServiceRole, never by convention.
// All mutations are audit-logged with the key ID as actor; impersonation in
// particular is unusable without leaving a trail.

// requireServiceRole wraps API-key-authenticated handlers, refusing anything
// but service_role keys with an explicit 403.
func (s *Server) requireServiceRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if keyRoleFromContext(r.Context()) != metadata.APIKeyRoleService {
			writeError(w, http.StatusForbidden, "service_role key required (never use it in a browser)")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// adminAudit records an admin action. Actor is the service key ID (the audit
// schema's key-actor column — service keys have no user row). The org is
// resolved from the project so the event shows in the org trail. Audit write
// failures fail the request: a silent admin action is worse than a failed one.
func (s *Server) adminAudit(r *http.Request, projectID, action, targetID string, meta map[string]any) error {
	orgID := ""
	if p, err := s.svc.Store.GetProject(r.Context(), projectID); err == nil {
		orgID = p.OrganizationID
	}
	return s.svc.Store.AppendAuditEvent(r.Context(), &metadata.AuditEvent{
		ActorKeyID:     "service_role:" + keyIDFromContext(r.Context()),
		OrganizationID: orgID,
		ProjectID:      projectID,
		Action:         action,
		TargetType:     "project_user",
		TargetID:       targetID,
		Metadata:       meta,
	})
}

func parseAdminPaging(r *http.Request) (page, perPage int) {
	page, perPage = 1, 50
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 && v <= 200 {
		perPage = v
	}
	return page, perPage
}

// adminListUsers searches project users (service_role only).
func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	page, perPage := parseAdminPaging(r)
	users, err := s.svc.Store.ListProjectUsers(r.Context(), projectID,
		r.URL.Query().Get("search"), perPage, (page-1)*perPage)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]projectUserView, 0, len(users))
	for i := range users {
		out = append(out, projectUserToView(&users[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "page": page, "per_page": perPage})
}

type adminCreateUserRequest struct {
	Email         string         `json:"email"`
	Phone         string         `json:"phone"`
	Password      string         `json:"password"`
	EmailConfirm  bool           `json:"email_confirm"`
	PhoneConfirm  bool           `json:"phone_confirm"`
	UserMetadata  map[string]any `json:"user_metadata"`
}

// adminCreateUser creates a user bypassing verification (service_role only).
func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req adminCreateUserRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	phone := strings.TrimSpace(req.Phone)
	if email == "" && phone == "" {
		writeError(w, http.StatusBadRequest, "email or phone is required")
		return
	}
	if email != "" && !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "invalid email")
		return
	}
	u := &metadata.ProjectUser{ProjectID: projectID, UserMetadata: req.UserMetadata}
	if email != "" {
		u.Email = &email
		if req.EmailConfirm {
			now := time.Now()
			u.EmailConfirmedAt = &now
		}
	}
	if phone != "" {
		u.Phone = &phone
		if req.PhoneConfirm {
			now := time.Now()
			u.PhoneConfirmedAt = &now
		}
	}
	if req.Password != "" {
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		u.PasswordHash = hash
	}
	if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.adminAudit(r, projectID, "admin.create_user", u.ID, map[string]any{"email": strOrEmpty(u.Email)}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": projectUserToView(u)})
}

// writeStoreErr maps store misses to 404 (writeErr only knows server
// sentinels; a metadata.ErrNotFound would otherwise become a 500).
func (s *Server) writeStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	s.writeErr(w, err)
}

func (s *Server) adminGetUser(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, r.PathValue("userID"))
	if err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": projectUserToView(u)})
}

type adminUpdateUserRequest struct {
	Email        *string        `json:"email"`
	Phone        *string        `json:"phone"`
	Password     *string        `json:"password"`
	BanDuration  *string        `json:"ban_duration"` // "none", "24h", "720h" or RFC3339 timestamp
	UserMetadata map[string]any `json:"user_metadata"`
	AppMetadata  map[string]any `json:"app_metadata"`
}

// adminUpdateUser edits any user field (service_role only). Password rotation
// burns sessions and codes, like the self-serve flow.
func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	userID := r.PathValue("userID")
	var req adminUpdateUserRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeStoreErr(w, err)
		return
	}
	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if !strings.Contains(email, "@") {
			writeError(w, http.StatusBadRequest, "invalid email")
			return
		}
		u.Email = &email
	}
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		u.Phone = &phone
	}
	if req.Password != nil {
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		u.PasswordHash = hash
		_ = s.svc.Store.RevokeProjectAuthCodesForUser(r.Context(), projectID, userID, "")
		_ = s.svc.Store.RevokeProjectUserSessions(r.Context(), projectID, userID)
	}
	if req.BanDuration != nil {
		switch d := strings.TrimSpace(*req.BanDuration); {
		case d == "" || d == "none":
			u.BannedUntil = nil
		case strings.Contains(d, "T"):
			ts, err := time.Parse(time.RFC3339, d)
			if err != nil {
				writeError(w, http.StatusBadRequest, "ban_duration must be none, a Go duration, or RFC3339")
				return
			}
			u.BannedUntil = &ts
		default:
			dur, err := time.ParseDuration(d)
			if err != nil {
				writeError(w, http.StatusBadRequest, "ban_duration must be none, a Go duration, or RFC3339")
				return
			}
			ts := time.Now().Add(dur)
			u.BannedUntil = &ts
		}
	}
	if req.UserMetadata != nil {
		u.UserMetadata = req.UserMetadata
	}
	if req.AppMetadata != nil {
		u.AppMetadata = req.AppMetadata
	}
	if err := s.svc.Store.UpdateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.adminAudit(r, projectID, "admin.update_user", u.ID, nil); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": projectUserToView(u)})
}

func (s *Server) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	userID := r.PathValue("userID")
	if err := s.svc.Store.DeleteProjectUser(r.Context(), projectID, userID); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	if err := s.adminAudit(r, projectID, "admin.delete_user", userID, nil); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type adminInviteRequest struct {
	Email      string         `json:"email"`
	RedirectTo string         `json:"redirect_to"`
	Data       map[string]any `json:"data"`
}

// adminInviteUser creates an unconfirmed user and sends the verification mail.
func (s *Server) adminInviteUser(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req adminInviteRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	u, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email)
	if err != nil {
		u = &metadata.ProjectUser{ProjectID: projectID, Email: &email, UserMetadata: req.Data}
		if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
			s.writeErr(w, err)
			return
		}
	}
	s.sendProjectCode(r, projectID, u, metadata.AuthCodeVerifyEmail, mail.TemplateVerifyEmail, "24 hours")
	if err := s.adminAudit(r, projectID, "admin.invite_user", u.ID, map[string]any{"email": email}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": projectUserToView(u)})
}

type adminGenerateLinkRequest struct {
	Type     string `json:"type"` // signup | magiclink | recovery | invite
	Email    string `json:"email"`
	Password string `json:"password"`
}

// adminGenerateLink mints a verification code and returns the action link
// directly (for custom email pipelines), instead of sending mail.
func (s *Server) adminGenerateLink(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req adminGenerateLinkRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	var kind string
	switch req.Type {
	case "signup", "invite":
		kind = metadata.AuthCodeVerifyEmail
	case "magiclink":
		kind = metadata.AuthCodeMagicLink
	case "recovery":
		kind = metadata.AuthCodeRecovery
	default:
		writeError(w, http.StatusBadRequest, "type must be signup, magiclink, recovery or invite")
		return
	}
	u, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email)
	if err != nil {
		if req.Type != "signup" && req.Type != "invite" && req.Type != "magiclink" {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		u = &metadata.ProjectUser{ProjectID: projectID, Email: &email}
		if req.Password != "" {
			hash, herr := auth.HashPassword(req.Password)
			if herr != nil {
				s.writeErr(w, herr)
				return
			}
			u.PasswordHash = hash
		}
		if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
			s.writeErr(w, err)
			return
		}
	}
	plaintext, hash, err := newOpaqueCode(32)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.CreateProjectAuthCode(r.Context(), &metadata.ProjectAuthCode{
		ProjectID: projectID, UserID: u.ID, Kind: kind,
		TokenHash: hash, ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		s.writeErr(w, err)
		return
	}
	st, _ := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	base := ""
	if st != nil && st.SiteURL != "" {
		base = strings.TrimSuffix(st.SiteURL, "/")
	} else {
		base = publicBaseURL(r, s.svc.PublicBaseURL)
	}
	link := base + "/verify?token=" + plaintext + "&type=" + kind
	if err := s.adminAudit(r, projectID, "admin.generate_link", u.ID, map[string]any{"kind": kind}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"action_link": link})
}

type adminImpersonateRequest struct {
	UserID string `json:"user_id"`
}

// adminImpersonate issues a short-lived (5 min, no refresh session)
// access token for any user. Always audit-logged; banned users refuse.
func (s *Server) adminImpersonate(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req adminImpersonateRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, req.UserID)
	if err != nil {
		s.writeStoreErr(w, err)
		return
	}
	if u.Banned(time.Now()) {
		writeError(w, http.StatusForbidden, "user is banned")
		return
	}
	mgr := s.projectAuthManager(r)
	access, _, err := mgr.IssueForCustom(r.Context(), projectID, u.ID,
		projectauth.RoleAuthenticated, projectauth.AAL1, 5*time.Minute,
		map[string]any{"impersonated": true})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.adminAudit(r, projectID, "admin.impersonate", u.ID, nil); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": access, "token_type": "bearer",
		"expires_in": int((5 * time.Minute).Seconds()),
		"user":       projectUserToView(u),
	})
}

// ---- operator-facing user management (dashboard) ----

// These mirror the service_role admin API but authenticate operators
// (admin+), so the dashboard never handles a service_role key. Responses
// reuse projectUserView.

func (s *Server) opListProjectUsers(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	page, perPage := parseAdminPaging(r)
	users, err := s.svc.Store.ListProjectUsers(r.Context(), projectID,
		r.URL.Query().Get("search"), perPage, (page-1)*perPage)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]projectUserView, 0, len(users))
	for i := range users {
		out = append(out, projectUserToView(&users[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out, "page": page, "per_page": perPage})
}

type opUpdateProjectUserRequest struct {
	Banned       *bool          `json:"banned"`
	BanDuration  *string        `json:"ban_duration"`
	ConfirmEmail *bool          `json:"confirm_email"`
	ConfirmPhone *bool          `json:"confirm_phone"`
	UserMetadata map[string]any `json:"user_metadata"`
}

func (s *Server) opUpdateProjectUser(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	userID := r.PathValue("userID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	var req opUpdateProjectUserRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeStoreErr(w, err)
		return
	}
	if req.Banned != nil {
		if *req.Banned {
			dur := "720h"
			if req.BanDuration != nil {
				dur = *req.BanDuration
			}
			d, err := time.ParseDuration(dur)
			if err != nil {
				writeError(w, http.StatusBadRequest, "ban_duration must be a Go duration")
				return
			}
			ts := time.Now().Add(d)
			u.BannedUntil = &ts
		} else {
			u.BannedUntil = nil
		}
	}
	if req.ConfirmEmail != nil {
		if *req.ConfirmEmail {
			now := time.Now()
			u.EmailConfirmedAt = &now
		} else {
			u.EmailConfirmedAt = nil
		}
	}
	if req.ConfirmPhone != nil {
		if *req.ConfirmPhone {
			now := time.Now()
			u.PhoneConfirmedAt = &now
		} else {
			u.PhoneConfirmedAt = nil
		}
	}
	if req.UserMetadata != nil {
		u.UserMetadata = req.UserMetadata
	}
	if err := s.svc.Store.UpdateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": projectUserToView(u)})
}

func (s *Server) opDeleteProjectUser(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	userID := r.PathValue("userID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteProjectUser(r.Context(), projectID, userID); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
