package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

func TestMailSettingsOwnerGate(t *testing.T) {
	ts := newTestServer(t)
	ownerTok, orgID, _ := ts.newProject(t)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")

	for _, tok := range []string{memberTok, outsiderTok} {
		for _, tc := range []struct{ method, path string }{
			{"GET", "/v1/admin/mail/settings"},
			{"PUT", "/v1/admin/mail/settings"},
			{"POST", "/v1/admin/mail/test"},
			{"GET", "/v1/admin/mail/log"},
		} {
			resp, _ := ts.do(t, tc.method, tc.path, tok, map[string]string{})
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("%s %s member/outsider status = %d, want 403", tc.method, tc.path, resp.StatusCode)
			}
		}
	}

	// Owner passes the gate.
	resp, _ := ts.do(t, "GET", "/v1/admin/mail/settings", ownerTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner GET settings status = %d, want 200", resp.StatusCode)
	}
}

func TestMailSettingsDefaultsAndRedaction(t *testing.T) {
	ts := newTestServer(t)
	ownerTok, _, _ := ts.newProject(t)

	resp, js := ts.do(t, "GET", "/v1/admin/mail/settings", ownerTok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d body=%v", resp.StatusCode, js)
	}
	if js["provider"] != "log" {
		t.Fatalf("default provider = %v, want log", js["provider"])
	}
	if js["password_set"] == true {
		t.Fatalf("fresh install must not report a stored password: %v", js)
	}
	for _, k := range []string{"smtp_password", "encrypted_password", "password"} {
		if _, ok := js[k]; ok {
			t.Fatalf("settings leak password field %q", k)
		}
	}
}

func TestMailSettingsValidation(t *testing.T) {
	ts := newTestServer(t)
	ownerTok, _, _ := ts.newProject(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"bad provider", map[string]any{"provider": "carrier-pigeon"}},
		{"missing host", map[string]any{"provider": "smtp", "smtp_port": 587, "from_address": "a@b.c"}},
		{"bad port", map[string]any{"provider": "smtp", "smtp_host": "smtp.b.c", "smtp_port": 99999, "from_address": "a@b.c"}},
		{"bad from", map[string]any{"provider": "smtp", "smtp_host": "smtp.b.c", "smtp_port": 587, "from_address": "not-an-email"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, js := ts.do(t, "PUT", "/v1/admin/mail/settings", ownerTok, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d body=%v, want 400", resp.StatusCode, js)
			}
		})
	}
}

func TestMailSettingsSaveAndKeepPassword(t *testing.T) {
	ts := newTestServer(t)
	ownerTok, _, _ := ts.newProject(t)

	save := map[string]any{
		"provider": "smtp", "smtp_host": "smtp.example.com", "smtp_port": 587,
		"smtp_username": "op", "smtp_password": "s3cret",
		"from_address": "noreply@example.com", "from_name": "Test",
	}
	resp, js := ts.do(t, "PUT", "/v1/admin/mail/settings", ownerTok, save)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d body=%v", resp.StatusCode, js)
	}
	if js["password_set"] != true || js["smtp_host"] != "smtp.example.com" {
		t.Fatalf("saved view = %v", js)
	}
	if _, ok := js["smtp_password"]; ok {
		t.Fatal("PUT response must not echo the password")
	}

	// Update another field without a password: the stored secret survives.
	save2 := map[string]any{
		"provider": "smtp", "smtp_host": "smtp2.example.com", "smtp_port": 587,
		"smtp_username": "op", "from_address": "noreply@example.com", "from_name": "Test",
	}
	resp, js = ts.do(t, "PUT", "/v1/admin/mail/settings", ownerTok, save2)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT without password status = %d body=%v", resp.StatusCode, js)
	}
	if js["password_set"] != true {
		t.Fatalf("password lost on password-less update: %v", js)
	}

	// Switch back to the log sink (used by the test-send below).
	resp, _ = ts.do(t, "PUT", "/v1/admin/mail/settings", ownerTok, map[string]any{"provider": "log"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT log status = %d", resp.StatusCode)
	}
}

func TestMailTestSendAndLog(t *testing.T) {
	ts := newTestServer(t)
	ownerTok, _, _ := ts.newProject(t)

	resp, js := ts.do(t, "POST", "/v1/admin/mail/test", ownerTok, map[string]string{"to": "me@example.com"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("test send status = %d body=%v", resp.StatusCode, js)
	}
	if js["ok"] != true || js["provider"] != "log" {
		t.Fatalf("test result = %v, want ok via log", js)
	}

	// Bad address is a 400, not a send.
	resp, _ = ts.do(t, "POST", "/v1/admin/mail/test", ownerTok, map[string]string{"to": "nope"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad address status = %d, want 400", resp.StatusCode)
	}

	// The attempt is in the delivery log (bare JSON array — fetch raw since
	// ts.do decodes into objects).
	req, _ := http.NewRequest("GET", ts.url+"/v1/admin/mail/log", nil)
	req.Header.Set("Authorization", "Bearer "+ownerTok)
	rawResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rawResp.Body.Close()
	var arr []map[string]any
	if err := json.NewDecoder(rawResp.Body).Decode(&arr); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	found := false
	for _, e := range arr {
		if e["to_address"] == "me@example.com" && e["ok"] == true {
			found = true
		}
	}
	if !found {
		t.Fatalf("delivery log missing test send: %v", arr)
	}
}
