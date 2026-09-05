package server_test

import (
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

// TestCreateProjectRBAC tests just the create project RBAC
func TestCreateProjectRBAC(t *testing.T) {
	ts := newTestServer(t)

	// Owner creates org and project
	ownerTok, orgID, _ := ts.newProject(t)

	// Admin: promote a member to admin
	adminTok := ts.register(t, "admin@example.com", "pw123456")
	resp, js := ts.do(t, "GET", "/v1/me", adminTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin user id: %d %v", resp.StatusCode, js)
	}
	adminID := js["id"].(string)
	ts.addMember(t, orgID, adminID, metadata.RoleAdmin)

	// Verify membership exists
	m, err := ts.store.GetMembership(t.Context(), orgID, adminID)
	if err != nil {
		t.Fatalf("membership not found: %v", err)
	}
	t.Logf("admin membership: role=%s", m.Role)

	// Owner tries to create another project
	resp, _ = ts.do(t, "POST", "/v1/orgs/"+orgID+"/projects", ownerTok,
		map[string]string{"name": "OwnerProject", "slug": "owner-proj"})
	t.Logf("owner create project: %d %v", resp.StatusCode, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("owner create project: %d, want 201", resp.StatusCode)
	}

	// Admin tries to create project
	resp, _ = ts.do(t, "POST", "/v1/orgs/"+orgID+"/projects", adminTok,
		map[string]string{"name": "AdminProject", "slug": "admin-proj"})
	t.Logf("admin create project: %d %v", resp.StatusCode, resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create project: %d, want 201", resp.StatusCode)
	}
}