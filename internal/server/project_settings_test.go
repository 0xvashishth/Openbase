package server_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/server"
)

// failingDestroyProvisioner provisions normally but refuses to destroy, which is
// how a real Docker daemon outage looks to the delete path.
type failingDestroyProvisioner struct {
	stubProvisioner
}

func (f *failingDestroyProvisioner) Destroy(_ context.Context, _ string) error {
	return errors.New("docker daemon unreachable")
}

var _ server.Provisioner = (*failingDestroyProvisioner)(nil)

func TestUpdateProject(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, _ := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")

	projID := ts.createProject(t, ownerTok, orgID, "Web", "web")

	t.Run("admin renames", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/projects/"+projID, adminTok,
			map[string]string{"name": "Website"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		if js["name"] != "Website" {
			t.Fatalf("name = %v, want Website", js["name"])
		}
	})

	t.Run("owner changes slug", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/projects/"+projID, ownerTok,
			map[string]string{"slug": "website"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		if js["slug"] != "website" {
			t.Fatalf("slug = %v, want website", js["slug"])
		}
	})

	t.Run("member forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, memberTok,
			map[string]string{"name": "Hijack"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("outsider forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, outsiderTok,
			map[string]string{"name": "Hijack"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("anonymous unauthorized", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, "", map[string]string{"name": "x"})
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	// Slugs are unique per org, so the collision is with a sibling project.
	t.Run("slug conflict inside the org", func(t *testing.T) {
		ts.createProject(t, ownerTok, orgID, "Mobile", "mobile")
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, ownerTok,
			map[string]string{"slug": "mobile"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", resp.StatusCode)
		}
	})

	// Another org may legitimately hold the same project slug.
	t.Run("same slug in a different org is fine", func(t *testing.T) {
		otherOrg := ts.createOrg(t, ownerTok, "Other", "other")
		otherProj := ts.createProject(t, ownerTok, otherOrg, "Site", "site")
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+otherProj, ownerTok,
			map[string]string{"slug": "mobile"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, ownerTok,
			map[string]string{"name": "  "})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("no fields rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/projects/"+projID, ownerTok, map[string]string{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})
}

func TestDeleteProjectRBAC(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, _ := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)

	projID := ts.createProject(t, ownerTok, orgID, "Web", "web")

	// Deleting a project destroys data, so it is owner-only — an admin who can
	// rename it still cannot remove it.
	t.Run("admin forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/projects/"+projID, adminTok, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("member forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/projects/"+projID, memberTok, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("project survives a refused delete", func(t *testing.T) {
		resp, _ := ts.do(t, "GET", "/v1/projects/"+projID, ownerTok, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("owner deletes", func(t *testing.T) {
		resp, js := ts.do(t, "DELETE", "/v1/projects/"+projID, ownerTok, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		resp2, _ := ts.do(t, "GET", "/v1/projects/"+projID, ownerTok, nil)
		if resp2.StatusCode != http.StatusNotFound {
			t.Fatalf("GET after delete = %d, want 404", resp2.StatusCode)
		}
	})
}

// TestDeleteProjectCascade asserts the ordered cascade: the provisioned
// container is destroyed, child rows disappear, and a previously valid API key
// stops working against the public API.
func TestDeleteProjectCascade(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)

	tok, _, projID := ts.newProject(t)

	// Provision a (stubbed) database so there is a container to destroy.
	resp, js := ts.do(t, "POST", "/v1/projects/"+projID+"/connections", tok, map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("provision status = %d body=%v", resp.StatusCode, js)
	}
	conn, err := ts.store.GetConnectionByProject(t.Context(), projID)
	if err != nil {
		t.Fatalf("get connection: %v", err)
	}
	if conn.ContainerID == nil || *conn.ContainerID == "" {
		t.Fatal("expected a container id after provisioning")
	}
	containerID := *conn.ContainerID

	// Seed one of each child row.
	rk, keyJS := ts.do(t, "POST", "/v1/projects/"+projID+"/api-keys", tok,
		map[string]string{"name": "prod"})
	if rk.StatusCode != http.StatusCreated {
		t.Fatalf("create api key status = %d body=%v", rk.StatusCode, keyJS)
	}
	plaintext, _ := keyJS["plaintext"].(string)
	if plaintext == "" {
		t.Fatalf("no plaintext key in response: %v", keyJS)
	}

	if err := ts.store.CreateTrigger(t.Context(), &metadata.Trigger{
		ProjectID: projID, Name: "t1", Collection: "widgets",
		Event: metadata.TriggerInsert, ActionType: metadata.ActionWebhook,
		ActionTarget: "https://example.test/hook", Enabled: true,
	}); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	if err := ts.store.CreateFunction(t.Context(), &metadata.Function{
		ProjectID: projID, Name: "fn1", Runtime: metadata.RuntimeNode, Source: "exports.handler=async()=>{}",
	}); err != nil {
		t.Fatalf("seed function: %v", err)
	}

	// The key works before the delete, so the 401 afterwards is meaningful.
	if rc, _ := ts.do(t, "GET", "/v1/api/tables", plaintext, nil); rc.StatusCode == http.StatusUnauthorized {
		t.Fatal("api key should be valid before the project is deleted")
	}

	rd, jd := ts.do(t, "DELETE", "/v1/projects/"+projID, tok, nil)
	if rd.StatusCode != http.StatusOK {
		t.Fatalf("delete status = %d body=%v, want 200", rd.StatusCode, jd)
	}

	// The container must be destroyed, and with its own id — not the project id.
	if len(stub.destroyed) != 1 || stub.destroyed[0] != containerID {
		t.Fatalf("destroyed = %v, want exactly [%s]", stub.destroyed, containerID)
	}

	if _, err := ts.store.GetProject(t.Context(), projID); err == nil {
		t.Fatal("project row should be gone")
	}
	if _, err := ts.store.GetConnectionByProject(t.Context(), projID); err == nil {
		t.Fatal("connection row should be gone")
	}
	if keys, err := ts.store.ListAPIKeys(t.Context(), projID); err != nil || len(keys) != 0 {
		t.Fatalf("api keys = %v (err %v), want none", keys, err)
	}
	if trigs, err := ts.store.ListTriggers(t.Context(), projID); err != nil || len(trigs) != 0 {
		t.Fatalf("triggers = %v (err %v), want none", trigs, err)
	}
	if fns, err := ts.store.ListFunctions(t.Context(), projID); err != nil || len(fns) != 0 {
		t.Fatalf("functions = %v (err %v), want none", fns, err)
	}

	// A revoked-by-cascade key must not authenticate anything.
	ra, _ := ts.do(t, "GET", "/v1/api/tables", plaintext, nil)
	if ra.StatusCode != http.StatusUnauthorized {
		t.Fatalf("public API with deleted project's key = %d, want 401", ra.StatusCode)
	}
}

// TestDeleteProjectAbortsWhenDestroyFails pins the ordering guarantee: if the
// container cannot be destroyed, the project must survive. Deleting the row
// first would orphan a container whose id is recorded nowhere else.
func TestDeleteProjectAbortsWhenDestroyFails(t *testing.T) {
	stub := &failingDestroyProvisioner{}
	ts := newTestServerWith(t, stub)

	tok, _, projID := ts.newProject(t)
	if resp, js := ts.do(t, "POST", "/v1/projects/"+projID+"/connections", tok, map[string]any{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("provision status = %d body=%v", resp.StatusCode, js)
	}

	resp, _ := ts.do(t, "DELETE", "/v1/projects/"+projID, tok, nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when Destroy fails", resp.StatusCode)
	}
	if _, err := ts.store.GetProject(t.Context(), projID); err != nil {
		t.Fatalf("project must survive a failed Destroy: %v", err)
	}
}
