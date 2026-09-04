package server_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/server"
)

// stubProvisioner is a Docker-free fake for auto-provision tests: it records
// the requested engine and returns a canned instance without starting anything.
type stubProvisioner struct {
	instances   []server.ProvisionedInstance
	requested   []server.Engine
	errProvision error
	destroyed   []string
	nextID      int
}

func (f *stubProvisioner) Provision(_ context.Context, engine server.Engine) (server.ProvisionedInstance, error) {
	if f.errProvision != nil {
		return server.ProvisionedInstance{}, f.errProvision
	}
	f.requested = append(f.requested, engine)
	f.nextID++
	inst := server.ProvisionedInstance{
		ContainerID: fmt.Sprintf("stub-container-%d", f.nextID),
		Engine:      engine,
		ConnString:  "postgres://stub:stub@localhost:5432/stub?sslmode=disable",
	}
	f.instances = append(f.instances, inst)
	return inst, nil
}

func (f *stubProvisioner) Destroy(_ context.Context, containerID string) error {
	f.destroyed = append(f.destroyed, containerID)
	return nil
}

var _ server.Provisioner = (*stubProvisioner)(nil)

func TestAutoProvisionEmptyJSONProvisionsPostgres(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok, map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auto-provision status = %d body=%v, want 200", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("auto-provision should succeed: %v", js)
	}
	if js["mode"] != "provisioned" {
		t.Fatalf("mode = %v, want provisioned", js["mode"])
	}
	if js["engine"] != "postgres" {
		t.Fatalf("engine = %v, want postgres", js["engine"])
	}
	if len(stub.requested) != 1 || stub.requested[0] != server.Engine("postgres") {
		t.Fatalf("provisioner requested = %v, want [postgres]", stub.requested)
	}

	rc, conn := ts.do(t, "GET", "/v1/projects/"+pID+"/connections", tok, nil)
	if rc.StatusCode != http.StatusOK {
		t.Fatalf("get connection status = %d", rc.StatusCode)
	}
	if conn["mode"] != "provisioned" || conn["engine"] != "postgres" || conn["status"] != "connected" {
		t.Fatalf("unexpected stored connection: %v", conn)
	}
	if cid, _ := conn["container_id"].(string); cid == "" {
		t.Fatalf("expected container_id after auto-provision: %v", conn)
	}
}

func TestAutoProvisionEmptyBodyProvisionsPostgres(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	// nil body => empty request => same auto-provision fallback.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty-body auto-provision status = %d body=%v, want 200", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("empty-body auto-provision should succeed: %v", js)
	}
}

func TestAutoProvisionWhitespaceConnStringProvisions(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]any{"connection_string": "   "})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whitespace-string auto-provision status = %d body=%v, want 200", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("should succeed: %v", js)
	}
}

func TestAutoProvisionRespectsEngineOverride(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]any{"engine": "ferretdb"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("engine-override status = %d body=%v", resp.StatusCode, js)
	}
	if js["engine"] != "ferretdb" {
		t.Fatalf("engine = %v, want ferretdb", js["engine"])
	}
	if len(stub.requested) != 1 || stub.requested[0] != server.Engine("ferretdb") {
		t.Fatalf("requested = %v, want [ferretdb]", stub.requested)
	}
}

func TestExplicitBYODBEmptyStill400(t *testing.T) {
	stub := &stubProvisioner{}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]any{"mode": "byodb", "connection_string": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("explicit byodb empty status = %d body=%v, want 400", resp.StatusCode, js)
	}
	if msg, _ := js["error"].(string); !strings.Contains(msg, "auto-provision") {
		t.Fatalf("error should hint at auto-provision fallback, got: %v", js)
	}
	if len(stub.requested) != 0 {
		t.Fatalf("explicit byodb empty must not provision, requested=%v", stub.requested)
	}
}

func TestAutoProvisionWithoutProvisionerReturns501(t *testing.T) {
	ts := newTestServer(t) // no provisioner
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok, map[string]any{})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("no-provisioner auto-provision status = %d body=%v, want 501", resp.StatusCode, js)
	}
}

func TestAutoProvisionFailureReturns502(t *testing.T) {
	stub := &stubProvisioner{errProvision: errors.New("no capacity")}
	ts := newTestServerWith(t, stub)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok, map[string]any{})
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("provision-failure status = %d body=%v, want 502", resp.StatusCode, js)
	}
}
