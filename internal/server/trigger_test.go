package server_test

import (
	"net/http"
	"testing"
)

func TestFunctionCRUD(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// List functions (empty).
	resp, js := ts.do(t, "GET", "/v1/projects/"+pID+"/functions", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list functions status = %d body=%v", resp.StatusCode, js)
	}

	// Create a function.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/functions", tok, map[string]string{
		"name":    "double",
		"runtime": "node",
		"source":  `exports.handler = async (event) => ({ n: event.data.n * 2 });`,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create function status = %d body=%v", resp.StatusCode, js)
	}
	fnID, _ := js["id"].(string)
	if fnID == "" {
		t.Fatalf("expected function id: %v", js)
	}

	// List functions (1).
	resp, js = ts.do(t, "GET", "/v1/projects/"+pID+"/functions", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list functions status = %d", resp.StatusCode)
	}

	// Get the function.
	resp, js = ts.do(t, "GET", "/v1/projects/"+pID+"/functions/"+fnID, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get function status = %d body=%v", resp.StatusCode, js)
	}
	if name, _ := js["name"].(string); name != "double" {
		t.Fatalf("function name = %q, want double", name)
	}

	// Bad runtime rejected.
	resp, _ = ts.do(t, "POST", "/v1/projects/"+pID+"/functions", tok, map[string]string{
		"name": "bad", "runtime": "brainfuck",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad runtime status = %d, want 400", resp.StatusCode)
	}

	// Delete the function.
	resp, js = ts.do(t, "DELETE", "/v1/projects/"+pID+"/functions/"+fnID, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete function status = %d body=%v", resp.StatusCode, js)
	}
}

func TestTriggerCRUD(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// Create a function target first.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/functions", tok, map[string]string{
		"name":    "double",
		"runtime": "node",
		"source":  `exports.handler = async (event) => ({ n: event.data.n * 2 });`,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create function status = %d", resp.StatusCode)
	}
	fnID, _ := js["id"].(string)

	// Create a trigger (function action).
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/triggers", tok, map[string]string{
		"name":          "on-new-user",
		"collection":    "users",
		"event":         "insert",
		"action_type":   "function",
		"action_target": fnID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create trigger status = %d body=%v", resp.StatusCode, js)
	}
	trigID, _ := js["id"].(string)
	if trigID == "" {
		t.Fatalf("expected trigger id: %v", js)
	}
	if enabled, _ := js["enabled"].(bool); !enabled {
		t.Fatalf("new trigger should be enabled: %v", js)
	}

	// List triggers.
	resp, js = ts.do(t, "GET", "/v1/projects/"+pID+"/triggers", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list triggers status = %d", resp.StatusCode)
	}

	// Update: disable.
	resp, js = ts.do(t, "PUT", "/v1/projects/"+pID+"/triggers/"+trigID, tok, map[string]bool{"enabled": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update trigger status = %d body=%v", resp.StatusCode, js)
	}
	if enabled, _ := js["enabled"].(bool); enabled {
		t.Fatalf("trigger should be disabled: %v", js)
	}

	// Invalid action_type rejected.
	resp, _ = ts.do(t, "POST", "/v1/projects/"+pID+"/triggers", tok, map[string]string{
		"name": "bad", "collection": "users", "event": "insert",
		"action_type": "carrier-pigeon", "action_target": fnID,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad action_type status = %d, want 400", resp.StatusCode)
	}

	// Delete the trigger.
	resp, js = ts.do(t, "DELETE", "/v1/projects/"+pID+"/triggers/"+trigID, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete trigger status = %d body=%v", resp.StatusCode, js)
	}
}

func TestTriggerCreateRequiresTarget(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/triggers", tok, map[string]string{
		"name": "x", "collection": "users", "event": "insert",
		"action_type": "webhook",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing target status = %d, want 400 body=%v", resp.StatusCode, js)
	}
}

// TestTriggerCreateRejectsSSRFTarget proves fail-fast SSRF validation: with
// the production default (no private-webhook escape hatch), a webhook trigger
// pointing at cloud metadata / loopback is refused at creation with a 400
// instead of silently never delivering.
func TestTriggerCreateRejectsSSRFTarget(t *testing.T) {
	ts := newTestServer(t)
	ts.svc.AllowPrivateWebhooks = false
	tok, _, pID := ts.newProject(t)
	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080/hook",
		"http://10.0.0.5/hook",
	} {
		resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/triggers", tok, map[string]string{
			"name": "x", "collection": "users", "event": "insert",
			"action_type": "webhook", "action_target": target,
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("SSRF target %q status = %d, want 400 body=%v", target, resp.StatusCode, js)
		}
	}
}
