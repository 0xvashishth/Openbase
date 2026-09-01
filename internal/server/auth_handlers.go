package server

import (
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/auth"
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
	if err != nil {
		// Do not reveal whether the email exists.
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, req.Password) {
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
