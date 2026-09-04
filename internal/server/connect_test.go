package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/testutil"
)

func trimScheme(url string) string {
	url = strings.TrimPrefix(url, "https://")
	return strings.TrimPrefix(url, "http://")
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// TestConnectInfoWithoutConnection asserts the Connect tab still gets usable
// URLs before a database is attached (has_connection=false, no capabilities).
func TestConnectInfoWithoutConnection(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "GET", "/v1/projects/"+pID+"/connect-info", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect-info status = %d body=%v", resp.StatusCode, js)
	}
	if got, _ := js["project_id"].(string); got != pID {
		t.Fatalf("project_id = %q, want %q", got, pID)
	}
	if has, _ := js["has_connection"].(bool); has {
		t.Fatalf("has_connection should be false without a DB: %v", js)
	}
	if _, ok := js["capabilities"]; ok {
		t.Fatalf("capabilities must be omitted without a DB: %v", js)
	}
	base, _ := js["api_base_url"].(string)
	if base == "" {
		t.Fatalf("api_base_url missing: %v", js)
	}
	if rt, _ := js["realtime_url"].(string); rt != "ws://"+trimScheme(base)+"/v1/realtime" {
		t.Fatalf("realtime_url = %q, want ws counterpart of %q", rt, base)
	}
	if hdr, _ := js["auth_header"].(string); hdr == "" {
		t.Fatalf("auth_header missing: %v", js)
	}
	eps, ok := js["endpoints"].(map[string]any)
	if !ok {
		t.Fatalf("endpoints missing: %v", js)
	}
	if got, _ := eps["list_tables"].(string); got != "GET /v1/api/tables" {
		t.Fatalf("list_tables = %q", got)
	}
	if n, _ := js["active_api_keys"].(float64); n != 0 {
		t.Fatalf("active_api_keys = %v, want 0", n)
	}
}

// TestConnectInfoWithConnection covers the connected case: engine/mode/status,
// honest capabilities from the live adapter, and the active key count.
func TestConnectInfoWithConnection(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}

	// Two keys, one of which we revoke, so the count must report 1.
	_, k1 := ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "keep"})
	_, k2 := ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "revoke-me"})
	revokeID, _ := k2["id"].(string)
	if _, ok := k1["plaintext"].(string); !ok {
		t.Fatalf("expected plaintext on create: %v", k1)
	}
	if resp, js := ts.do(t, "DELETE", "/v1/projects/"+pID+"/api-keys/"+revokeID, tok, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status = %d body=%v", resp.StatusCode, js)
	}

	resp2, info := ts.do(t, "GET", "/v1/projects/"+pID+"/connect-info", tok, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("connect-info status = %d body=%v", resp2.StatusCode, info)
	}
	if has, _ := info["has_connection"].(bool); !has {
		t.Fatalf("has_connection should be true: %v", info)
	}
	if eng, _ := info["engine"].(string); eng != "postgres" {
		t.Fatalf("engine = %q, want postgres", eng)
	}
	if mode, _ := info["mode"].(string); mode != "byodb" {
		t.Fatalf("mode = %q, want byodb", mode)
	}
	if status, _ := info["status"].(string); status != "connected" {
		t.Fatalf("status = %q, want connected", status)
	}
	caps, ok := info["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing for a live postgres connection: %v", info)
	}
	if joins, _ := caps["supports_relational_joins"].(bool); !joins {
		t.Fatalf("postgres should report relational joins: %v", caps)
	}
	if rt, _ := info["supports_realtime"].(string); rt == "" {
		t.Fatalf("supports_realtime should mirror capabilities: %v", info)
	}
	if n, _ := info["active_api_keys"].(float64); n != 1 {
		t.Fatalf("active_api_keys = %v, want 1 (one key revoked)", n)
	}
}

// TestConnectInfoNeverLeaksCredentials is the security guard: the response must
// not carry the (decryptable) connection string or any encrypted blob.
func TestConnectInfoNeverLeaksCredentials(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	if resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN}); resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}

	_, info := ts.do(t, "GET", "/v1/projects/"+pID+"/connect-info", tok, nil)
	for _, forbidden := range []string{
		"connection_string", "encrypted_conn_string", "encrypted_username",
		"encrypted_password", "password", "username",
	} {
		if _, ok := info[forbidden]; ok {
			t.Fatalf("connect-info must not expose %q: %v", forbidden, info)
		}
	}
	if raw := jsonString(t, info); containsAny(raw, userDB.DSN, "postgres://") {
		t.Fatalf("connect-info leaked a database connection string: %s", raw)
	}
}

// TestConnectInfoAuthorization: only org members may read it.
func TestConnectInfoAuthorization(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	if resp, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/connect-info", "", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous connect-info status = %d, want 401", resp.StatusCode)
	}

	other := ts.register(t, "outsider@example.com", "pw123456")
	if resp, _ := ts.do(t, "GET", "/v1/projects/"+pID+"/connect-info", other, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-member connect-info status = %d, want 403", resp.StatusCode)
	}

	if resp, _ := ts.do(t, "GET", "/v1/projects/00000000-0000-0000-0000-000000000000/connect-info", tok, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown project connect-info status = %d, want 404", resp.StatusCode)
	}
}
