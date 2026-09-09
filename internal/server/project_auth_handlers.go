package server

import (
	"crypto/rand"
	"errors"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/mail"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/projectauth"
)

// End-user authentication per project (Phase 10, A1). All routes are
// project-scoped via the anon/service_role API key (requireAPIKey) and
// rate-limited like the operator auth routes. Response shapes mirror GoTrue
// so the SDK and Supabase migration guides translate 1:1.
//
// Sessions are returned immediately on signup (documented default): a
// self-hosted instance usually has no SMTP configured, and locking users out
// until they click a link that can never arrive would be worse. Verification
// via POST /auth/v1/verify still stamps the confirmation timestamps that
// future policies (Phase 11) can require.

// ---- request/response shapes (GoTrue-compatible) ----

type projectUserView struct {
	ID               string         `json:"id"`
	Email            *string        `json:"email,omitempty"`
	Phone            *string        `json:"phone,omitempty"`
	UserMetadata     map[string]any `json:"user_metadata,omitempty"`
	AppMetadata      map[string]any `json:"app_metadata,omitempty"`
	EmailConfirmedAt *string        `json:"email_confirmed_at,omitempty"`
	PhoneConfirmedAt *string        `json:"phone_confirmed_at,omitempty"`
	BannedUntil      *string        `json:"banned_until,omitempty"`
	IsAnonymous      bool           `json:"is_anonymous,omitempty"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
}

func fmtTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtTime(*t)
	return &s
}

func projectUserToView(u *metadata.ProjectUser) projectUserView {
	return projectUserView{
		ID:               u.ID,
		Email:            u.Email,
		Phone:            u.Phone,
		UserMetadata:     u.UserMetadata,
		AppMetadata:      u.AppMetadata,
		EmailConfirmedAt: fmtTimePtr(u.EmailConfirmedAt),
		PhoneConfirmedAt: fmtTimePtr(u.PhoneConfirmedAt),
		BannedUntil:      fmtTimePtr(u.BannedUntil),
		IsAnonymous:      u.IsAnonymous,
		CreatedAt:        fmtTime(u.CreatedAt),
		UpdatedAt:        fmtTime(u.UpdatedAt),
	}
}

type projectSessionResponse struct {
	AccessToken  string          `json:"access_token"`
	TokenType    string          `json:"token_type"`
	ExpiresIn    int             `json:"expires_in"`
	RefreshToken string          `json:"refresh_token"`
	User         projectUserView `json:"user"`
}

// ---- helpers ----

// newOpaqueCode generates a URL-safe verification code and its storage hash.
// Only the hash is ever persisted.
func newOpaqueCode(nBytes int) (plaintext, hash string, err error) {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(sum[:]), nil
}

// projectAuthManager returns the end-user auth manager, wiring it on first
// use from the request-scoped base URL when the service default is empty.
func (s *Server) projectAuthManager(r *http.Request) *projectauth.Manager {
	if s.svc.ProjectAuth != nil {
		return s.svc.ProjectAuth
	}
	return projectauth.NewManager(s.svc.Store, s.svc.Secrets, publicBaseURL(r, s.svc.PublicBaseURL))
}

// issueProjectSession creates a refresh session row and mints the access
// token pair returned by signup / token endpoints.
func (s *Server) issueProjectSession(r *http.Request, projectID string, u *metadata.ProjectUser) (*projectSessionResponse, error) {
	plaintext, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	st, err := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	refreshTTL := 30 * 24 * time.Hour
	if err == nil && st.SessionAbsoluteTimeoutS > 0 {
		refreshTTL = time.Duration(st.SessionAbsoluteTimeoutS) * time.Second
	}
	ses := &metadata.ProjectSession{
		ProjectID:   projectID,
		UserID:      u.ID,
		RefreshHash: hash,
		UserAgent:   r.UserAgent(),
		IP:          r.RemoteAddr,
		ExpiresAt:   time.Now().Add(refreshTTL),
	}
	if err := s.svc.Store.CreateProjectSession(r.Context(), ses); err != nil {
		return nil, err
	}
	mgr := s.projectAuthManager(r)
	access, _, err := mgr.IssueFor(r.Context(), projectID, u.ID, projectauth.RoleAuthenticated, projectauth.AAL1, 0)
	if err != nil {
		return nil, err
	}
	return &projectSessionResponse{
		AccessToken:  access,
		TokenType:    "bearer",
		ExpiresIn:    int((15 * time.Minute).Seconds()),
		RefreshToken: plaintext,
		User:         projectUserToView(u),
	}, nil
}

// endUserFromBearer verifies a user JWT from the Authorization header and
// returns its project + user. Used by /user, /logout and the data path.
func (s *Server) endUserFromBearer(r *http.Request) (projectID, userID string, err error) {
	ah := r.Header.Get("Authorization")
	raw := strings.TrimSpace(strings.TrimPrefix(ah, bearerPrefix))
	if raw == "" {
		return "", "", errUnauthorized
	}
	// The pid claim routes verification without trusting it: VerifyFor checks
	// the signature against that project's keys AND the project binding.
	pid, perr := projectauth.UnverifiedProjectID(raw)
	if perr != nil || pid == "" {
		return "", "", errUnauthorized
	}
	mgr := s.projectAuthManager(r)
	claims, verr := mgr.VerifyFor(r.Context(), pid, raw)
	if verr != nil {
		return "", "", errUnauthorized
	}
	return claims.ProjectID, claims.UserID, nil
}

// ---- handlers ----

type projectSignupRequest struct {
	Email    string         `json:"email"`
	Password string         `json:"password"`
	Data     map[string]any `json:"data"`
}

func (s *Server) projectSignup(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req projectSignupRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !strings.Contains(email, "@") || req.Password == "" {
		writeError(w, http.StatusBadRequest, "a valid email and password are required")
		return
	}
	st, err := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	minLen := 8
	if err == nil && st.PasswordMinLength > 0 {
		minLen = st.PasswordMinLength
	}
	if len(req.Password) < minLen {
		writeError(w, http.StatusBadRequest, "password is too short")
		return
	}
	if _, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email); err == nil {
		writeError(w, http.StatusUnprocessableEntity, "user already registered")
		return
	} else if !errors.Is(err, metadata.ErrNotFound) {
		s.writeErr(w, err)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	u := &metadata.ProjectUser{ProjectID: projectID, Email: &email, PasswordHash: hash, UserMetadata: req.Data}
	if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

type projectTokenRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	RefreshToken string `json:"refresh_token"`
}

func (s *Server) projectToken(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	grant := r.URL.Query().Get("grant_type")
	var req projectTokenRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch grant {
	case "password":
		s.projectTokenPassword(w, r, projectID, req)
	case "refresh_token":
		s.projectTokenRefresh(w, r, projectID, req)
	default:
		// Explicit 501 (not 404): the grant is planned (magiclink/otp/pkce/
		// id_token arrive with A2), the server simply doesn't speak it yet.
		writeError(w, http.StatusNotImplemented, "grant_type "+grant+" is not yet supported (use password or refresh_token)")
	}
}

func (s *Server) projectTokenPassword(w http.ResponseWriter, r *http.Request, projectID string, req projectTokenRequest) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	u, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email)
	// Timing-oracle defence (mirrors operator login): always run bcrypt.
	const bogusHash = "$2a$10$abcdefghijklmnopqrstuOOkNhE5xZqV7i7.LxXNxKlqi7B0uGJaK"
	storedHash := bogusHash
	if err == nil {
		storedHash = u.PasswordHash
	}
	if !auth.VerifyPassword(storedHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.Banned(time.Now()) {
		writeError(w, http.StatusForbidden, "user is banned")
		return
	}
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

func (s *Server) projectTokenRefresh(w http.ResponseWriter, r *http.Request, projectID string, req projectTokenRequest) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	hash, err := auth.HashRefreshToken(req.RefreshToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid refresh token")
		return
	}
	cur, err := s.svc.Store.GetProjectSessionByHash(r.Context(), hash)
	if err != nil || cur.ProjectID != projectID {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	plaintext, newHash, err := auth.NewRefreshToken()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	st, _ := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	refreshTTL := 30 * 24 * time.Hour
	if st != nil && st.SessionAbsoluteTimeoutS > 0 {
		refreshTTL = time.Duration(st.SessionAbsoluteTimeoutS) * time.Second
	}
	next := &metadata.ProjectSession{
		RefreshHash: newHash, UserAgent: r.UserAgent(), IP: r.RemoteAddr,
		ExpiresAt: time.Now().Add(refreshTTL),
	}
	if err := s.svc.Store.RotateProjectSession(r.Context(), hash, next); err != nil {
		// Reuse or expiry: the chain is burned or dead — 401 either way so a
		// thief learns nothing about which.
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, cur.UserID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if u.Banned(time.Now()) {
		writeError(w, http.StatusForbidden, "user is banned")
		return
	}
	mgr := s.projectAuthManager(r)
	access, _, err := mgr.IssueFor(r.Context(), projectID, u.ID, projectauth.RoleAuthenticated, projectauth.AAL1, 0)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, &projectSessionResponse{
		AccessToken: access, TokenType: "bearer",
		ExpiresIn:    int((15 * time.Minute).Seconds()),
		RefreshToken: plaintext, User: projectUserToView(u),
	})
}

func (s *Server) projectLogout(w http.ResponseWriter, r *http.Request) {
	projectID, userID, err := s.endUserFromBearer(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.RevokeProjectUserSessions(r.Context(), projectID, userID); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) projectGetUser(w http.ResponseWriter, r *http.Request) {
	projectID, userID, err := s.endUserFromBearer(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projectUserToView(u))
}

type projectUpdateUserRequest struct {
	Email    *string        `json:"email"`
	Password *string        `json:"password"`
	Data     map[string]any `json:"data"`
}

func (s *Server) projectUpdateUser(w http.ResponseWriter, r *http.Request) {
	projectID, userID, err := s.endUserFromBearer(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var req projectUpdateUserRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			writeError(w, http.StatusBadRequest, "password is too short")
			return
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		u.PasswordHash = hash
		// A password change burns recovery codes and other sessions.
		_ = s.svc.Store.RevokeProjectAuthCodesForUser(r.Context(), projectID, userID, "")
		_ = s.svc.Store.RevokeProjectUserSessions(r.Context(), projectID, userID)
	}
	if req.Data != nil {
		if u.UserMetadata == nil {
			u.UserMetadata = map[string]any{}
		}
		for k, v := range req.Data {
			u.UserMetadata[k] = v
		}
	}
	if req.Email != nil {
		email := strings.TrimSpace(strings.ToLower(*req.Email))
		if !strings.Contains(email, "@") {
			writeError(w, http.StatusBadRequest, "invalid email")
			return
		}
		if existing, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email); err == nil && existing.ID != u.ID {
			writeError(w, http.StatusUnprocessableEntity, "email already in use")
			return
		} else if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			s.writeErr(w, err)
			return
		}
		// Immediate switch, marked unconfirmed; the verification code goes out
		// below. (A pending-email column is a follow-up, not a blocker.)
		u.Email = &email
		u.EmailConfirmedAt = nil
	}
	if err := s.svc.Store.UpdateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	if req.Email != nil {
		s.sendProjectCode(r, projectID, u, metadata.AuthCodeEmailChange, mail.TemplateVerifyEmail, "1 hour")
	}
	// The password rotation above revoked this caller's session; mint a fresh
	// pair so update-password doesn't log the user out.
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

// sendProjectCode stores a single-use code and delivers it via the mailer.
// Mail failures are logged, never returned: the code row exists and the flow
// stays debuggable through the mail log.
func (s *Server) sendProjectCode(r *http.Request, projectID string, u *metadata.ProjectUser, kind, template, expires string) {
	if u.Email == nil || *u.Email == "" {
		return
	}
	plaintext, hash, err := newOpaqueCode(32)
	if err != nil {
		return
	}
	ttl := time.Hour
	if err := s.svc.Store.CreateProjectAuthCode(r.Context(), &metadata.ProjectAuthCode{
		ProjectID: projectID, UserID: u.ID, Kind: kind, TokenHash: hash,
		ExpiresAt: time.Now().Add(ttl),
	}); err != nil {
		return
	}
	st, _ := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	base := ""
	if st != nil && st.SiteURL != "" {
		base = strings.TrimSuffix(st.SiteURL, "/")
	} else {
		base = publicBaseURL(r, s.svc.PublicBaseURL)
	}
	data := mail.TemplateData{
		Recipient: *u.Email,
		ActionURL: base + "/verify?token=" + plaintext + "&type=" + kind,
		Expires:   expires,
	}
	_ = s.mailService().SendTemplate(r.Context(), *u.Email, template, data)
}

type projectRecoverRequest struct {
	Email      string `json:"email"`
	RedirectTo string `json:"redirect_to"`
}

func (s *Server) projectRecover(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req projectRecoverRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Always 200 — never reveal whether the email is registered.
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email != "" {
		if u, err := s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email); err == nil {
			s.sendProjectCode(r, projectID, u, metadata.AuthCodeRecovery, mail.TemplateResetPassword, "1 hour")
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type projectOTPRequest struct {
	Email           string         `json:"email"`
	Phone           string         `json:"phone"`
	Data            map[string]any `json:"data"`
	ShouldCreateUser *bool         `json:"should_create_user"`
}

func (s *Server) projectOTP(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req projectOTPRequest
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
	createUser := true
	if req.ShouldCreateUser != nil {
		createUser = *req.ShouldCreateUser
	}
	var u *metadata.ProjectUser
	var err error
	if email != "" {
		u, err = s.svc.Store.GetProjectUserByEmail(r.Context(), projectID, email)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			s.writeErr(w, err)
			return
		}
		if u == nil && createUser {
			u = &metadata.ProjectUser{ProjectID: projectID, Email: &email, UserMetadata: req.Data}
			if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
				s.writeErr(w, err)
				return
			}
		}
		if u == nil {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		kind := metadata.AuthCodeMagicLink
		if u.EmailConfirmedAt == nil {
			kind = metadata.AuthCodeOTPEmail
		}
		s.sendProjectCode(r, projectID, u, kind, mail.TemplateVerifyEmail, "1 hour")
	} else {
		// Phone delivery needs the A2 SMS provider; the code row + 200 keep
		// the contract stable until sending is wired (honest 501 would break
		// the SDK's signInWithOtp shape, so we record and document instead).
		u, err = s.svc.Store.GetProjectUserByPhone(r.Context(), projectID, phone)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			s.writeErr(w, err)
			return
		}
		if u == nil && createUser {
			u = &metadata.ProjectUser{ProjectID: projectID, Phone: &phone, UserMetadata: req.Data}
			if err := s.svc.Store.CreateProjectUser(r.Context(), u); err != nil {
				s.writeErr(w, err)
				return
			}
		}
		if u != nil {
			plaintext, hash, cerr := newOpaqueCode(16)
			if cerr == nil {
				_ = s.svc.Store.CreateProjectAuthCode(r.Context(), &metadata.ProjectAuthCode{
					ProjectID: projectID, UserID: u.ID, Kind: metadata.AuthCodeOTPPhone,
					TokenHash: hash, ExpiresAt: time.Now().Add(10 * time.Minute),
				})
				_ = plaintext // delivered via SMS once the A2 provider lands
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type projectVerifyRequest struct {
	Email string `json:"email"`
	Phone string `json:"phone"`
	Token string `json:"token"`
	Type  string `json:"type"`
}

// verifyTypeToKind maps GoTrue verify types onto stored code kinds.
func verifyTypeToKind(t string) string {
	switch t {
	case "signup", "invite":
		return metadata.AuthCodeVerifyEmail
	case "magiclink":
		return metadata.AuthCodeMagicLink
	case "recovery":
		return metadata.AuthCodeRecovery
	case "email_change":
		return metadata.AuthCodeEmailChange
	case "sms":
		return metadata.AuthCodeOTPPhone
	case "phone_change":
		return metadata.AuthCodePhoneChange
	case "email":
		return metadata.AuthCodeOTPEmail
	default:
		return ""
	}
}

func (s *Server) projectVerify(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	var req projectVerifyRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := verifyTypeToKind(req.Type)
	if kind == "" || strings.TrimSpace(req.Token) == "" {
		writeError(w, http.StatusBadRequest, "token and a valid type are required")
		return
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(req.Token)))
	code, err := s.svc.Store.GetProjectAuthCodeByHash(r.Context(), hex.EncodeToString(sum[:]))
	if err != nil || code.ProjectID != projectID || code.Kind != kind {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, code.UserID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	// Optional identity check: when an email/phone is supplied it must match.
	if req.Email != "" && (u.Email == nil || *u.Email != strings.ToLower(strings.TrimSpace(req.Email))) {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	if req.Phone != "" && (u.Phone == nil || *u.Phone != strings.TrimSpace(req.Phone)) {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	if err := s.svc.Store.MarkProjectAuthCodeUsed(r.Context(), code.ID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	now := time.Now()
	switch kind {
	case metadata.AuthCodeVerifyEmail, metadata.AuthCodeMagicLink, metadata.AuthCodeOTPEmail:
		u.EmailConfirmedAt = &now
	case metadata.AuthCodeOTPPhone, metadata.AuthCodePhoneChange:
		u.PhoneConfirmedAt = &now
	case metadata.AuthCodeEmailChange:
		u.EmailConfirmedAt = &now
	}
	if err := s.svc.Store.UpdateProjectUser(r.Context(), u); err != nil {
		s.writeErr(w, err)
		return
	}
	// Recovery verify issues a session so the client can set a new password.
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

func (s *Server) projectJWKS(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	// Unknown projects 404 here (rather than 500ing on the bootstrap FK).
	if _, err := s.svc.Store.GetProject(r.Context(), projectID); err != nil {
		s.writeErr(w, errNotFound)
		return
	}
	mgr := s.projectAuthManager(r)
	// Bootstrap on first read so a fresh project serves keys without a
	// signup round-trip; idempotent for projects that already have keys.
	if _, err := mgr.EnsureKeySet(r.Context(), projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	keys, err := mgr.PublicJWKS(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, k.Map())
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}
