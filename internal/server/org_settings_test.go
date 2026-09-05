package server_test

import (
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

// userID resolves a token to its user id via /v1/me. Membership rows are keyed
// by user id, so tests that seed roles directly need this.
func (ts *testServer) userID(t *testing.T, token string) string {
	t.Helper()
	resp, js := ts.do(t, "GET", "/v1/me", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/me status = %d body=%v", resp.StatusCode, js)
	}
	id, ok := js["id"].(string)
	if !ok || id == "" {
		t.Fatalf("no user id in /v1/me response: %v", js)
	}
	return id
}

// registerMember creates an account and seeds it into orgID with the given role.
// It returns the token and the user id.
func (ts *testServer) registerMember(t *testing.T, orgID, email string, role metadata.OrgRole) (token, userID string) {
	t.Helper()
	token = ts.register(t, email, "pw123456")
	userID = ts.userID(t, token)
	ts.addMember(t, orgID, userID, role)
	return token, userID
}

func TestUpdateOrgRBACAndValidation(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, _ := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")

	t.Run("owner can rename", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID, ownerTok, map[string]string{"name": "Acme Corp"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v", resp.StatusCode, js)
		}
		if js["name"] != "Acme Corp" {
			t.Fatalf("name = %v, want Acme Corp", js["name"])
		}
		// The caller's role rides along so the dashboard can gate UI with no
		// second round-trip.
		if js["role"] != string(metadata.RoleOwner) {
			t.Fatalf("role = %v, want owner", js["role"])
		}
	})

	t.Run("admin can change slug", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID, adminTok, map[string]string{"slug": "acme-corp"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v", resp.StatusCode, js)
		}
		if js["slug"] != "acme-corp" {
			t.Fatalf("slug = %v, want acme-corp", js["slug"])
		}
		if js["role"] != string(metadata.RoleAdmin) {
			t.Fatalf("role = %v, want admin", js["role"])
		}
	})

	t.Run("member forbidden", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID, memberTok, map[string]string{"name": "Hijack"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d body=%v, want 403", resp.StatusCode, js)
		}
		// The body must name the required role; "forbidden" alone gives the
		// dashboard nothing to render.
		if msg, _ := js["error"].(string); msg == "" || msg == "forbidden" {
			t.Fatalf("403 body should name the required role, got %q", msg)
		}
	})

	t.Run("outsider forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID, outsiderTok, map[string]string{"name": "Hijack"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("anonymous unauthorized", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID, "", map[string]string{"name": "Hijack"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID, ownerTok, map[string]string{"name": "   "})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("no fields rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID, ownerTok, map[string]string{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("duplicate slug conflicts", func(t *testing.T) {
		otherTok := ts.register(t, "other@example.com", "pw123456")
		ts.createOrg(t, otherTok, "Taken", "taken-slug")
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID, ownerTok, map[string]string{"slug": "taken-slug"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", resp.StatusCode)
		}
	})
}

func TestDeleteOrg(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, _ := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)

	t.Run("admin forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/orgs/"+orgID, adminTok, map[string]string{"slug": "acme"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("slug confirmation required", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/orgs/"+orgID, ownerTok, map[string]string{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("wrong slug rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/orgs/"+orgID, ownerTok, map[string]string{"slug": "not-acme"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	// Refusing while projects exist is deliberate: cascading would have to
	// destroy N provisioned containers mid-transaction, and a failure partway
	// through would delete the metadata row and orphan the rest.
	t.Run("blocked while projects exist", func(t *testing.T) {
		ts.createProject(t, ownerTok, orgID, "Web", "web")
		resp, js := ts.do(t, "DELETE", "/v1/orgs/"+orgID, ownerTok, map[string]string{"slug": "acme"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d body=%v, want 409", resp.StatusCode, js)
		}
		// The org must still be readable — a refused delete changes nothing.
		if resp2, _ := ts.do(t, "GET", "/v1/orgs/"+orgID, ownerTok, nil); resp2.StatusCode != http.StatusOK {
			t.Fatalf("org should survive a refused delete, GET status = %d", resp2.StatusCode)
		}
	})

	t.Run("owner deletes an empty org", func(t *testing.T) {
		emptyID := ts.createOrg(t, ownerTok, "Empty", "empty-org")
		resp, js := ts.do(t, "DELETE", "/v1/orgs/"+emptyID, ownerTok, map[string]string{"slug": "empty-org"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		// Gone means gone: membership is deleted too, so reads now 403.
		resp2, _ := ts.do(t, "GET", "/v1/orgs/"+emptyID, ownerTok, nil)
		if resp2.StatusCode != http.StatusForbidden && resp2.StatusCode != http.StatusNotFound {
			t.Fatalf("deleted org GET status = %d, want 403 or 404", resp2.StatusCode)
		}
	})
}
