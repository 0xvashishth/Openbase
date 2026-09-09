package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Phase 10 (A1.1): anon / service_role key roles.
func TestAPIKeyRoles(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// Default (no role): service_role, preserving pre-role behavior.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "default-role"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d body=%v", resp.StatusCode, js)
	}
	if js["role"] != "service_role" {
		t.Fatalf("default role = %v, want service_role", js["role"])
	}
	defaultPlaintext, _ := js["plaintext"].(string)

	// Explicit anon key.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "browser", "role": "anon"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create anon status = %d body=%v", resp.StatusCode, js)
	}
	if js["role"] != "anon" {
		t.Fatalf("role = %v, want anon", js["role"])
	}
	anonPlaintext, _ := js["plaintext"].(string)

	// Invalid role.
	resp, _ = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "bad", "role": "owner"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", resp.StatusCode)
	}

	// List surfaces roles.
	req, _ := http.NewRequest("GET", ts.url+"/v1/projects/"+pID+"/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	listResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", listResp.StatusCode)
	}
	var listed []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	roles := map[string]bool{}
	for _, k := range listed {
		if r, _ := k["role"].(string); r != "" {
			roles[r] = true
		}
	}
	if !roles["service_role"] || !roles["anon"] {
		t.Fatalf("list must surface both roles, got %v", roles)
	}

	// Both keys authenticate on the data path (roles are propagated, not yet
	// enforced — Phase 11 adds policy enforcement; until then anon === full).
	for name, key := range map[string]string{"service_role": defaultPlaintext, "anon": anonPlaintext} {
		req, _ := http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode == http.StatusUnauthorized {
			t.Fatalf("%s key: data path rejected with 401", name)
		}
	}
}
