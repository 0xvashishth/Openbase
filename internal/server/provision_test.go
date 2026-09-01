package server_test

import (
	"context"
	"net/http"
	"os/exec"
	"testing"

	"github.com/openbase/openbase/internal/provision"
)

// dockerAvailable returns true when a usable Docker CLI + daemon exist.
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	return exec.Command("docker", "info").Run() == nil
}

// containerGone reports whether a Docker container no longer exists.
func containerGone(id string) error {
	return exec.Command("docker", "inspect", id).Run()
}

// TestProvisionedConnectionEndToEnd exercises the real Docker-backed
// provisioner: create a project, request a "provisioned" connection, the
// platform spins up a dedicated Postgres container (encrypted creds stored),
// then we browse its tables and finally delete the connection, which destroys
// the container.
func TestProvisionedConnectionEndToEnd(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available; skipping provisioning integration test")
	}

	prov := &provision.Compose{}
	sp := &provision.ServerProvisioner{Inner: prov}
	ts := newTestServerWith(t, sp)

	tok, _, pID := ts.newProject(t)

	// Provision.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]any{"mode": "provisioned", "engine": "postgres"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("provision connect status = %d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("provision should succeed: %v", js)
	}

	// The connection is stored mode=provisioned, connected, and has a
	// container id we can use to assert cleanup.
	rc, conn := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
	if rc.StatusCode != http.StatusOK {
		t.Fatalf("get connection status = %d", rc.StatusCode)
	}
	if conn["mode"] != "provisioned" {
		t.Fatalf("conn mode = %v, want provisioned", conn["mode"])
	}
	if conn["status"] != "connected" {
		t.Fatalf("conn status = %v, want connected", conn["status"])
	}
	containerID, _ := conn["container_id"].(string)
	if containerID == "" {
		t.Fatalf("expected container_id after provisioning")
	}
	t.Cleanup(func() { _ = prov.Destroy(context.Background(), containerID) })

	// Browse: a fresh provisioned Postgres exposes no user tables yet, but the
	// connection must work (status 200, the adapter introspects cleanly).
	cc, cols := ts.do(t, "GET", "/v1/projects/"+pID+"/collections", tok, nil)
	if cc.StatusCode != http.StatusOK {
		t.Fatalf("collections status = %d body=%v", cc.StatusCode, cols)
	}

	// Delete the connection => container destroyed, no connection remains.
	dc, _ := ts.do(t, "DELETE", "/v1/projects/"+pID+"/connections", tok, nil)
	if dc.StatusCode != http.StatusOK {
		t.Fatalf("delete connection status = %d", dc.StatusCode)
	}
	gc, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
	if gc.StatusCode != http.StatusNotFound {
		t.Fatalf("get connection after delete status = %d, want 404", gc.StatusCode)
	}

	// The container should be gone now (docker inspect fails for it).
	if err := containerGone(containerID); err == nil {
		t.Fatalf("container %s leaked; expected DELETE to destroy it", containerID)
	}
}

// TestProvisioningDisabled returns 501 when no provisioner is configured.
func TestProvisioningDisabled(t *testing.T) {
	ts := newTestServer(t) // no provisioner
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]any{"mode": "provisioned"})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("provision with no provisioner status = %d body=%v, want 501", resp.StatusCode, js)
	}
}

// TestProvisionedOverwritesDestroyOld covers the leak-prevention path: saving a
// provisioned connection twice must destroy the first container.
func TestProvisionedOverwritesDestroyOld(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available; skipping provisioning integration test")
	}

	prov := &provision.Compose{}
	sp := &provision.ServerProvisioner{Inner: prov}
	ts := newTestServerWith(t, sp)

	tok, _, pID := ts.newProject(t)

	do := func(co bool) string {
		t.Helper()
		resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
			map[string]any{"mode": "provisioned"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("provision status = %d body=%v", resp.StatusCode, js)
		}
		_, conn := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
		cid, _ := conn["container_id"].(string)
		if cid == "" {
			t.Fatalf("no container_id")
		}
		if co {
			t.Cleanup(func() { _ = prov.Destroy(context.Background(), cid) })
		} else {
			// Not the last one: a later save should have destroyed it.
			cid := cid
			t.Cleanup(func() {
				if err := containerGone(cid); err == nil {
					t.Errorf("old container %s should have been destroyed on overwrite", cid)
				}
			})
		}
		return cid
	}

	old := do(false)
	_ = do(true)

	// After overwriting, the old container is gone.
	if err := containerGone(old); err == nil {
		t.Fatalf("old container %s leaked after overwrite", old)
	}
}