package server

import (
	"net/http"

	"github.com/openbase/openbase/internal/apikey"
	"github.com/openbase/openbase/internal/metadata"
)

type createAPIKeyRequest struct {
	Name string `json:"name"`
	// Role is optional: "anon" or "service_role" (default). Pre-role
	// behavior is preserved — omitting it mints a full-access service_role key.
	Role string `json:"role,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Role      string   `json:"role"`
	KeyHash   string   `json:"key_hash"`
	Plaintext string   `json:"plaintext,omitempty"`
	Scopes    []string `json:"scopes"`
	CreatedAt string   `json:"created_at"`
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}

	keys, err := s.svc.Store.ListAPIKeys(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}

	type apiKeyView struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Role      string     `json:"role"`
		Scopes    []string   `json:"scopes"`
		CreatedAt string     `json:"created_at"`
		RevokedAt *string    `json:"revoked_at,omitempty"`
	}
	out := make([]apiKeyView, 0, len(keys))
	for _, k := range keys {
		var revokedAt *string
		if k.RevokedAt != nil {
			s := k.RevokedAt.UTC().Format("2006-01-02T15:04:05Z")
			revokedAt = &s
		}
		out = append(out, apiKeyView{
			ID:        k.ID,
			Name:      k.Name,
			Role:      k.Role,
			Scopes:    k.Scopes,
			CreatedAt: k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			RevokedAt: revokedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}

	var req createAPIKeyRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	role := req.Role
	if role == "" {
		role = metadata.APIKeyRoleDefault
	}
	if !metadata.ValidAPIKeyRole(role) {
		writeError(w, http.StatusBadRequest, `role must be "anon" or "service_role"`)
		return
	}

	plaintext, hash, err := apikey.New()
	if err != nil {
		s.writeErr(w, err)
		return
	}

	k := &metadata.APIKey{
		ProjectID: projectID,
		Name:      req.Name,
		Role:      role,
		KeyHash:   hash,
		Scopes:    []string{"read", "write"},
	}
	if err := s.svc.Store.CreateAPIKey(r.Context(), k); err != nil {
		s.writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, createAPIKeyResponse{
		ID:        k.ID,
		Name:      k.Name,
		Role:      k.Role,
		KeyHash:   k.KeyHash,
		Plaintext: plaintext,
		Scopes:    k.Scopes,
		CreatedAt: k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	keyID := r.PathValue("keyID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}

	// Confirm the key belongs to this project before revoking, so a caller can't
	// revoke another project's key by guessing an ID.
	keys, err := s.svc.Store.ListAPIKeys(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	found := false
	for _, k := range keys {
		if k.ID == keyID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "API key not found")
		return
	}

	if err := s.svc.Store.RevokeAPIKey(r.Context(), keyID); err != nil {
		s.writeErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
