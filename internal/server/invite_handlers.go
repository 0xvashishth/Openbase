package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/mail"
	"github.com/openbase/openbase/internal/metadata"
)

type createInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type inviteView struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	Role           string    `json:"role"`
	InvitedBy      string    `json:"invited_by"`
	ExpiresAt      time.Time `json:"expires_at"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	FullName string `json:"full_name,omitempty"`
	Password string `json:"password,omitempty"`
}

// createInvite handles POST /v1/orgs/{orgID}/invites — admins+ invite a user.
// Sends an email via the mail service (Phase 9.2 mail capability).
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	m, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req createInviteRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	role := metadata.OrgRole(req.Role)
	if role == "" {
		role = metadata.RoleMember
	}
	if role != metadata.RoleOwner && role != metadata.RoleAdmin && role != metadata.RoleMember {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return
	}
	if role == metadata.RoleOwner && m.Role != metadata.RoleOwner {
		writeError(w, http.StatusForbidden, "only owners can grant owner role")
		return
	}
	org, err := s.svc.Store.GetOrganization(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		s.writeErr(w, err)
		return
	}
	plaintext := base64.RawURLEncoding.EncodeToString(b)
	hashSum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(hashSum[:])

	inv := &metadata.Invite{
		ID:        uuid.NewString(),
		OrganizationID: orgID,
		Email:     req.Email,
		Role:      role,
		TokenHash: hash,
		InvitedBy: m.UserID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.svc.Store.CreateInvite(r.Context(), inv); err != nil {
		s.writeErr(w, err)
		return
	}

	// Send invite email (best-effort).
	data := mail.TemplateData{
		Recipient: req.Email,
		OrgName:   org.Name,
		ActionURL: publicBaseURL(r, "") + "/invites/accept?token=" + plaintext,
		Expires:   "7 days",
	}
	_ = s.mailService().SendTemplate(r.Context(), req.Email, mail.TemplateOrgInvite, data)

	writeJSON(w, http.StatusCreated, toInviteView(inv))
}

func toInviteView(i *metadata.Invite) inviteView {
	return inviteView{
		ID:        i.ID,
		Email:     i.Email,
		Role:      string(i.Role),
		InvitedBy: i.InvitedBy,
		ExpiresAt: i.ExpiresAt,
		AcceptedAt: i.AcceptedAt,
		CreatedAt: i.CreatedAt,
	}
}

// listInvites handles GET /v1/orgs/{orgID}/invites — admins+ list pending invites.
func (s *Server) listInvites(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if _, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	invites, err := s.svc.Store.ListInvites(r.Context(), orgID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]inviteView, len(invites))
	for i, inv := range invites {
		out[i] = toInviteView(&inv)
	}
	writeJSON(w, http.StatusOK, out)
}

// revokeInvite handles DELETE /v1/orgs/{orgID}/invites/{inviteID} — admins+ revoke an invite.
func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgID")
	if _, err := s.authorizeOrgRole(r, orgID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	inviteID := r.PathValue("inviteID")
	if err := s.svc.Store.RevokeInvite(r.Context(), inviteID); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// acceptInvite handles POST /v1/invites/accept — accept a pending invite by token.
// If the user does not exist, optionally create one with full_name/password.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := inviteTokenHash(req.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	inv, err := s.svc.Store.GetInviteByToken(r.Context(), hash)
	if err != nil || inv == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	// Look up user by email; create if not exists.
	uid, err := s.svc.Store.GetUserByEmail(r.Context(), inv.Email)
	if err != nil {
		if req.FullName == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "full_name and password required for new users")
			return
		}
		newUser, err := s.createUserForInvite(r, inv.Email, req.FullName, req.Password)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		uid = newUser
	}

	if _, err := s.svc.Store.AcceptInvite(r.Context(), hash, uid.ID); err != nil {
		s.writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted", "org_id": inv.OrganizationID})
}

func (s *Server) createUserForInvite(r *http.Request, email, fullName, password string) (*metadata.User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	u := &metadata.User{Email: email, PasswordHash: hash, FullName: fullName}
	if err := s.svc.Store.CreateUser(r.Context(), u); err != nil {
		return nil, err
	}
	return u, nil
}

func inviteTokenHash(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", http.ErrAbortHandler
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}
