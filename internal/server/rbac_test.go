package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

// TestRBACMatrix verifies the role-based authorization matrix:
// member can read; admin can write project-level; owner can do everything.
func TestRBACMatrix(t *testing.T) {
	ts := newTestServer(t)

	// Owner creates org and project
	ownerTok, orgID, projID := ts.newProject(t)

	// Admin: promote a member to admin
	adminTok := ts.register(t, "admin@example.com", "pw123456")
	resp, js := ts.do(t, "GET", "/v1/me", adminTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get admin user id: %d %v", resp.StatusCode, js)
	}
	adminID := js["id"].(string)
	ts.addMember(t, orgID, adminID, metadata.RoleAdmin)

	// Member: another user with member role
	memberTok := ts.register(t, "member@example.com", "pw123456")
	resp, js = ts.do(t, "GET", "/v1/me", memberTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get member user id: %d %v", resp.StatusCode, js)
	}
	memberID := js["id"].(string)
	ts.addMember(t, orgID, memberID, metadata.RoleMember)

	// Outsider: not a member of the org
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")

	// Helper to generate unique slugs per subtest
	slug := func(prefix string) string {
		return fmt.Sprintf("%s-%s", prefix, t.Name())
	}

	tests := []struct {
		name        string
		method      string
		path        string
		token       string
		wantStatus  int
		description string
	}{
		// --- Organization reads: member+ ---
		{"member GET /orgs/{id}", "GET", "/v1/orgs/" + orgID, memberTok, http.StatusOK, "member can read org"},
		{"admin GET /orgs/{id}", "GET", "/v1/orgs/" + orgID, adminTok, http.StatusOK, "admin can read org"},
		{"owner GET /orgs/{id}", "GET", "/v1/orgs/" + orgID, ownerTok, http.StatusOK, "owner can read org"},
		{"outsider GET /orgs/{id}", "GET", "/v1/orgs/" + orgID, outsiderTok, http.StatusForbidden, "outsider cannot read org"},

		{"member GET /orgs/{id}/projects", "GET", "/v1/orgs/"+orgID+"/projects", memberTok, http.StatusOK, "member can list projects"},
		{"admin GET /orgs/{id}/projects", "GET", "/v1/orgs/"+orgID+"/projects", adminTok, http.StatusOK, "admin can list projects"},
		{"owner GET /orgs/{id}/projects", "GET", "/v1/orgs/"+orgID+"/projects", ownerTok, http.StatusOK, "owner can list projects"},
		{"outsider GET /orgs/{id}/projects", "GET", "/v1/orgs/"+orgID+"/projects", outsiderTok, http.StatusForbidden, "outsider cannot list projects"},

		// --- Project reads: member+ ---
		{"member GET /projects/{id}", "GET", "/v1/projects/"+projID, memberTok, http.StatusOK, "member can read project"},
		{"admin GET /projects/{id}", "GET", "/v1/projects/"+projID, adminTok, http.StatusOK, "admin can read project"},
		{"owner GET /projects/{id}", "GET", "/v1/projects/"+projID, ownerTok, http.StatusOK, "owner can read project"},
		{"outsider GET /projects/{id}", "GET", "/v1/projects/"+projID, outsiderTok, http.StatusForbidden, "outsider cannot read project"},

		// --- Project creation: admin+ ---
		{"member POST /orgs/{id}/projects → 403", "POST", "/v1/orgs/"+orgID+"/projects", memberTok, http.StatusForbidden, "member cannot create project"},
		{"admin POST /orgs/{id}/projects → 201", "POST", "/v1/orgs/"+orgID+"/projects", adminTok, http.StatusCreated, "admin can create project"},
		{"owner POST /orgs/{id}/projects → 201", "POST", "/v1/orgs/"+orgID+"/projects", ownerTok, http.StatusCreated, "owner can create project"},

		// --- Connection (DB source): member+ read, admin+ write ---
		{"member GET /projects/{id}/connections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/connections", memberTok, http.StatusNotFound, "member can read connection (404 no conn)"},
		{"admin GET /projects/{id}/connections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/connections", adminTok, http.StatusNotFound, "admin can read connection (404 no conn)"},
		{"owner GET /projects/{id}/connections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/connections", ownerTok, http.StatusNotFound, "owner can read connection (404 no conn)"},

		// --- API keys: admin+ write, member+ read ---
		{"member GET /projects/{id}/api-keys → 200", "GET", "/v1/projects/"+projID+"/api-keys", memberTok, http.StatusOK, "member can list API keys (empty)"},
		{"admin GET /projects/{id}/api-keys → 200", "GET", "/v1/projects/"+projID+"/api-keys", adminTok, http.StatusOK, "admin can list API keys"},
		{"owner GET /projects/{id}/api-keys → 200", "GET", "/v1/projects/"+projID+"/api-keys", ownerTok, http.StatusOK, "owner can list API keys"},

		{"member POST /projects/{id}/api-keys → 403", "POST", "/v1/projects/"+projID+"/api-keys", memberTok, http.StatusForbidden, "member cannot create API key"},
		{"admin POST /projects/{id}/api-keys → 201", "POST", "/v1/projects/"+projID+"/api-keys", adminTok, http.StatusCreated, "admin can create API key"},
		{"owner POST /projects/{id}/api-keys → 201", "POST", "/v1/projects/"+projID+"/api-keys", ownerTok, http.StatusCreated, "owner can create API key"},

		// --- Triggers: member+ read, admin+ write ---
		{"member GET /projects/{id}/triggers → 200", "GET", "/v1/projects/"+projID+"/triggers", memberTok, http.StatusOK, "member can list triggers (empty)"},
		{"admin GET /projects/{id}/triggers → 200", "GET", "/v1/projects/"+projID+"/triggers", adminTok, http.StatusOK, "admin can list triggers"},
		{"owner GET /projects/{id}/triggers → 200", "GET", "/v1/projects/"+projID+"/triggers", ownerTok, http.StatusOK, "owner can list triggers"},

		// --- Functions: member+ read, admin+ write ---
		{"member GET /projects/{id}/functions → 200", "GET", "/v1/projects/"+projID+"/functions", memberTok, http.StatusOK, "member can list functions (empty)"},
		{"admin GET /projects/{id}/functions → 200", "GET", "/v1/projects/"+projID+"/functions", adminTok, http.StatusOK, "admin can list functions"},
		{"owner GET /projects/{id}/functions → 200", "GET", "/v1/projects/"+projID+"/functions", ownerTok, http.StatusOK, "owner can list functions"},

		// --- Data reads (schema, collections, query): member+ ---
		// These need a connected DB; test the auth path at least
		{"member GET /projects/{id}/collections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/collections", memberTok, http.StatusNotFound, "member hits 404 (auth OK)"},
		{"admin GET /projects/{id}/collections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/collections", adminTok, http.StatusNotFound, "admin hits 404 (auth OK)"},
		{"owner GET /projects/{id}/collections → 404 (no conn)", "GET", "/v1/projects/"+projID+"/collections", ownerTok, http.StatusNotFound, "owner hits 404 (auth OK)"},
		{"outsider GET /projects/{id}/collections → 403", "GET", "/v1/projects/"+projID+"/collections", outsiderTok, http.StatusForbidden, "outsider forbidden"},

		// --- SQL exec (DDL/DML): admin+ ---
		{"member POST /projects/{id}/sql → 403", "POST", "/v1/projects/"+projID+"/sql", memberTok, http.StatusForbidden, "member cannot exec SQL"},
		{"admin POST /projects/{id}/sql → 404 (no conn)", "POST", "/v1/projects/"+projID+"/sql", adminTok, http.StatusNotFound, "admin hits 404 (no conn, auth OK)"},
		{"owner POST /projects/{id}/sql → 404 (no conn)", "POST", "/v1/projects/"+projID+"/sql", ownerTok, http.StatusNotFound, "owner hits 404 (no conn, auth OK)"},
		{"outsider POST /projects/{id}/sql → 403", "POST", "/v1/projects/"+projID+"/sql", outsiderTok, http.StatusForbidden, "outsider forbidden"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
body := map[string]string{"query": "SELECT 1"}
		if tc.method == "POST" && tc.path == "/v1/projects/"+projID+"/api-keys" {
			body = map[string]string{"name": slug(tc.name)}
		} else if tc.method == "POST" && tc.path == "/v1/orgs/"+orgID+"/projects" {
			body = map[string]string{"name": slug(tc.name), "slug": slug(tc.name)}
		}
			resp, _ := ts.do(t, tc.method, tc.path, tc.token, body)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("%s: status = %d, want %d (%s)", tc.name, resp.StatusCode, tc.wantStatus, tc.description)
			}
		})
	}
}