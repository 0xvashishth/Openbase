package server_test

import (
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

func TestCreateAPIKeyRBAC(t *testing.T) {
	ts := newTestServer(t)

	ownerTok, orgID, projID := ts.newProject(t)

	adminTok := ts.register(t, "admin@example.com", "pw123456")
	resp, js := ts.do(t, "GET", "/v1/me", adminTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin user id: %d %v", resp.StatusCode, js)
	}
	adminID := js["id"].(string)
	ts.addMember(t, orgID, adminID, metadata.RoleAdmin)

	// Admin creates API key
	resp, _ = ts.do(t, "POST", "/v1/projects/"+projID+"/api-keys", adminTok,
		map[string]string{"name": "admin-key"})
	t.Logf("admin create api key: %d %v", resp.StatusCode, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create api key: %d, want 201", resp.StatusCode)
	}

	// Owner creates API key
	resp, _ = ts.do(t, "POST", "/v1/projects/"+projID+"/api-keys", ownerTok,
		map[string]string{"name": "owner-key"})
	t.Logf("owner create api key: %d %v", resp.StatusCode, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("owner create api key: %d, want 201", resp.StatusCode)
	}
}