package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/mfa"
	"github.com/openbase/openbase/internal/projectauth"
)

// End-user MFA (TOTP) — Phase 10, A3.1. All routes require an end-user JWT
// (not just an anon key): enrolment and verification prove possession of the
// account. Successful verification issues an aal2 session; the aal claim is
// recorded in every token so Phase 11 policies can step-up gate sensitive
// collections.

type mfaEnrollRequest struct {
	FactorType   string `json:"factor_type"`
	FriendlyName string `json:"friendly_name"`
}

// mfaUser resolves the caller: project from either auth path, user from JWT.
func (s *Server) mfaUser(r *http.Request) (projectID, userID string, ok bool) {
	projectID, userID, err := s.endUserFromBearer(r)
	if err != nil || projectID == "" || userID == "" {
		return "", "", false
	}
	return projectID, userID, true
}

// mfaEnroll creates an unverified TOTP factor. The secret is shown exactly
// once here (like an API key plaintext) — later reads never include it.
func (s *Server) mfaEnroll(w http.ResponseWriter, r *http.Request) {
	projectID, userID, ok := s.mfaUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "an end-user session is required")
		return
	}
	var req mfaEnrollRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.FactorType != "" && req.FactorType != "totp" {
		writeError(w, http.StatusBadRequest, "only totp factors are supported")
		return
	}
	if s.svc.Secrets == nil {
		writeError(w, http.StatusInternalServerError, "no secrets provider configured")
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	secret, err := mfa.GenerateSecret()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	cipher, keyID, err := s.svc.Secrets.EncryptValue(secret)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	friendly := req.FriendlyName
	if friendly == "" {
		friendly = "totp"
	}
	f := &metadata.ProjectMFAFactor{
		ProjectID: projectID, UserID: userID, FactorType: "totp",
		FriendlyName: friendly, SecretEncrypted: cipher, EncryptionKeyID: keyID,
	}
	if err := s.svc.Store.CreateMFAFactor(r.Context(), f); err != nil {
		s.writeErr(w, err)
		return
	}
	account := strOrEmpty(u.Email)
	if account == "" {
		account = u.ID
	}
	uri := mfa.URI(secret, account, "Openbase")
	writeJSON(w, http.StatusOK, map[string]any{
		"id":           f.ID,
		"factor_type":  "totp",
		"friendly_name": friendly,
		"totp":         map[string]string{"secret": secret, "uri": uri, "qr_code": uri},
	})
}

// mfaListFactors lists enrolled factors without secrets.
func (s *Server) mfaListFactors(w http.ResponseWriter, r *http.Request) {
	projectID, userID, ok := s.mfaUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "an end-user session is required")
		return
	}
	factors, err := s.svc.Store.ListMFAFactors(r.Context(), projectID, userID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	type view struct {
		ID           string `json:"id"`
		FactorType   string `json:"factor_type"`
		FriendlyName string `json:"friendly_name"`
		Status       string `json:"status"`
	}
	out := make([]view, 0, len(factors))
	for _, f := range factors {
		out = append(out, view{ID: f.ID, FactorType: f.FactorType, FriendlyName: f.FriendlyName, Status: f.Status})
	}
	writeJSON(w, http.StatusOK, map[string]any{"totp": out})
}

// mfaChallenge creates a challenge for a factor owned by the caller.
func (s *Server) mfaChallenge(w http.ResponseWriter, r *http.Request) {
	projectID, userID, ok := s.mfaUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "an end-user session is required")
		return
	}
	factorID := r.PathValue("factorID")
	f, err := s.ownedFactor(r, projectID, userID, factorID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	_, hash, err := newOpaqueCode(16)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ch := &metadata.ProjectMFAChallenge{
		FactorID: f.ID, ChallengeOTPHash: hash, ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := s.svc.Store.CreateMFAChallenge(r.Context(), ch); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": ch.ID})
}

type mfaVerifyRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

// mfaVerify checks the TOTP code, marks factor verified, and issues an aal2
// session — the step-up the aal claim exists for.
func (s *Server) mfaVerify(w http.ResponseWriter, r *http.Request) {
	projectID, userID, ok := s.mfaUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "an end-user session is required")
		return
	}
	factorID := r.PathValue("factorID")
	var req mfaVerifyRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.ChallengeID) == "" || strings.TrimSpace(req.Code) == "" {
		writeError(w, http.StatusBadRequest, "challenge_id and code are required")
		return
	}
	f, err := s.ownedFactor(r, projectID, userID, factorID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ch, err := s.svc.Store.GetMFAChallenge(r.Context(), req.ChallengeID)
	if err != nil || ch.FactorID != f.ID || ch.VerifiedAt != nil || ch.ExpiresAt.Before(time.Now()) {
		// Missing, foreign, consumed or expired: indistinguishable 400.
		writeError(w, http.StatusBadRequest, "invalid or expired challenge")
		return
	}
	if s.svc.Secrets == nil {
		writeError(w, http.StatusInternalServerError, "no secrets provider configured")
		return
	}
	secret, err := s.svc.Secrets.DecryptValue(f.SecretEncrypted, f.EncryptionKeyID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if !mfa.VerifyNow(secret, req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	if err := s.svc.Store.MarkMFAChallengeVerified(r.Context(), ch.ID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired challenge")
		return
	}
	if err := s.svc.Store.VerifyMFAFactor(r.Context(), projectID, f.ID); err != nil {
		s.writeErr(w, err)
		return
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, userID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ses, err := s.issueProjectSessionWithClaims(r, projectID, u, projectauth.AAL2, nil)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

// mfaUnenroll removes a factor owned by the caller.
func (s *Server) mfaUnenroll(w http.ResponseWriter, r *http.Request) {
	projectID, userID, ok := s.mfaUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "an end-user session is required")
		return
	}
	factorID := r.PathValue("factorID")
	if _, err := s.ownedFactor(r, projectID, userID, factorID); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteMFAFactor(r.Context(), projectID, factorID); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ownedFactor loads a factor and proves caller ownership (else NotFound —
// never reveal another user's factor IDs).
func (s *Server) ownedFactor(r *http.Request, projectID, userID, factorID string) (*metadata.ProjectMFAFactor, error) {
	f, err := s.svc.Store.GetMFAFactor(r.Context(), projectID, factorID)
	if err != nil {
		return nil, errNotFound
	}
	if f.UserID != userID {
		return nil, errNotFound
	}
	return f, nil
}
