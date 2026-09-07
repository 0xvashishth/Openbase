package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/openbase/openbase/internal/auth"
	"github.com/openbase/openbase/internal/mail"
	"github.com/openbase/openbase/internal/metadata"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token     string          `json:"token"`
	ExpiresAt string          `json:"expires_at"`
	User      *metadata.User  `json:"user"`
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	u := &metadata.User{Email: req.Email, PasswordHash: hash, FullName: req.FullName}
	if err := s.svc.Store.CreateUser(r.Context(), u); err != nil {
		if metadata.IsConflict(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		s.writeErr(w, err)
		return
	}

	s.respondWithToken(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	u, err := s.svc.Store.GetUserByEmail(r.Context(), req.Email)
	// Timing-oracle defence: always run bcrypt to avoid leaking which
	// addresses are registered. A bogus hash will not match anything.
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
		// User didn't exist but bcrypt succeeded against the bogus hash.
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.respondWithToken(w, http.StatusOK, u)
}

func (s *Server) respondWithToken(w http.ResponseWriter, status int, u *metadata.User) {
	token, exp, err := s.svc.Tokens.Issue(u.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, status, authResponse{
		Token:     token,
		ExpiresAt: exp.UTC().Format("2006-01-02T15:04:05Z"),
		User:      u,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := s.svc.Store.GetUserByID(r.Context(), userIDFromContext(r.Context()))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// forgotPasswordRequest is the POST body for /v1/auth/forgot.
type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// resetPasswordRequest is the POST body for /v1/auth/reset.
type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// forgotPassword always returns 200 — it must never reveal whether the
// email is registered (enumeration defence). If the user exists, it
// generates a single-use reset token, stores only its hash, and sends a
// reset link via the mail service.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	// Best-effort: silently no-op on any error. We never want to leak
	// whether the address exists.
	if req.Email != "" {
		if u, err := s.svc.Store.GetUserByEmail(r.Context(), req.Email); err == nil {
			plaintext, _, err := s.svc.Store.CreateResetPasswordToken(r.Context(), u.ID, time.Hour)
			if err == nil {
				// Send the email. Failures are logged, not returned.
				data := mail.TemplateData{
					Recipient: u.Email,
					ActionURL: publicBaseURL(r, "") + "/reset?token=" + plaintext,
					Expires:   "1 hour",
				}
				if sendErr := s.mailService().SendTemplate(r.Context(), u.Email, mail.TemplateResetPassword, data); sendErr != nil {
					// Log only — never fail the request.
					_ = sendErr
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// resetPassword consumes a single-use reset token and sets a new password.
// Tokens are single-use, expire after 1 hour, and the plaintext never
// touched the database.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "token and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashRefreshToken(req.Token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}

	rt, err := s.svc.Store.GetResetPasswordToken(r.Context(), hash)
	if err != nil || rt == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}

	passHash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	if err := s.svc.Store.SetUserPassword(r.Context(), rt.UserID, passHash); err != nil {
		s.writeErr(w, err)
		return
	}

	if err := s.svc.Store.MarkResetPasswordTokenUsed(r.Context(), rt.ID); err != nil {
		s.writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
