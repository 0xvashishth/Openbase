package server_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/openbase/openbase/internal/metadata"
)

func newAnonKey(t *testing.T, ts *testServer, tok, pID string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok,
		map[string]string{"name": "anon", "role": "anon"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create anon key status = %d body=%v", resp.StatusCode, js)
	}
	plaintext, _ := js["plaintext"].(string)
	if plaintext == "" {
		t.Fatal("expected plaintext anon key")
	}
	return plaintext
}

func TestProjectAuthSignupTokenRefreshLogout(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	authHeader := func(key string) http.Header {
		h := http.Header{}
		h.Set("Authorization", "Bearer "+key)
		h.Set("Content-Type", "application/json")
		return h
	}
	post := func(path, key string, body any) (*http.Response, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", ts.url+path, bytes.NewReader(b))
		req.Header = authHeader(key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var js map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&js)
		return resp, js
	}

	// Signup.
	resp, js := post("/auth/v1/signup", anonKey, map[string]string{"email": "End@Example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signup status = %d body=%v", resp.StatusCode, js)
	}
	access, _ := js["access_token"].(string)
	refresh, _ := js["refresh_token"].(string)
	user, _ := js["user"].(map[string]any)
	if access == "" || refresh == "" || user["id"] == nil {
		t.Fatalf("signup must return tokens + user: %v", js)
	}

	// Duplicate signup → 422.
	resp, _ = post("/auth/v1/signup", anonKey, map[string]string{"email": "end@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate signup status = %d, want 422", resp.StatusCode)
	}

	// Short password → 400.
	resp, _ = post("/auth/v1/signup", anonKey, map[string]string{"email": "x@example.com", "password": "short"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", resp.StatusCode)
	}

	// GET /user with the access token.
	req, _ := http.NewRequest("GET", ts.url+"/auth/v1/user", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	uresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer uresp.Body.Close()
	if uresp.StatusCode != http.StatusOK {
		t.Fatalf("get user status = %d", uresp.StatusCode)
	}

	// Password grant.
	resp, js = post("/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "end@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("password grant status = %d body=%v", resp.StatusCode, js)
	}
	refresh2, _ := js["refresh_token"].(string)

	// Wrong password → 401 (and unknown email → 401, no enumeration).
	resp, _ = post("/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "end@example.com", "password": "wrong-password-ok-length"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", resp.StatusCode)
	}

	// Unknown grant → 501.
	resp, _ = post("/auth/v1/token?grant_type=magiclink", anonKey, map[string]any{})
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("unknown grant status = %d, want 501", resp.StatusCode)
	}

	// Refresh rotation.
	resp, js = post("/auth/v1/token?grant_type=refresh_token", anonKey, map[string]string{"refresh_token": refresh2})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d body=%v", resp.StatusCode, js)
	}
	refresh3, _ := js["refresh_token"].(string)
	if refresh3 == "" || refresh3 == refresh2 {
		t.Fatal("refresh must rotate the token")
	}
	// Reuse of the rotated token → 401 (chain burned).
	resp, _ = post("/auth/v1/token?grant_type=refresh_token", anonKey, map[string]string{"refresh_token": refresh2})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reuse status = %d, want 401", resp.StatusCode)
	}

	// Data path accepts the end-user JWT (no connection → non-401 backend error).
	dreq, _ := http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
	dreq.Header.Set("Authorization", "Bearer "+access)
	dresp, err := http.DefaultClient.Do(dreq)
	if err != nil {
		t.Fatal(err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode == http.StatusUnauthorized {
		t.Fatal("data path must accept end-user JWTs")
	}

	// Logout revokes the chain: the live refresh token dies too.
	lreq, _ := http.NewRequest("POST", ts.url+"/auth/v1/logout", nil)
	lreq.Header.Set("Authorization", "Bearer "+access)
	lresp, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatal(err)
	}
	defer lresp.Body.Close()
	if lresp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d", lresp.StatusCode)
	}
	resp, _ = post("/auth/v1/token?grant_type=refresh_token", anonKey, map[string]string{"refresh_token": refresh3})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout refresh status = %d, want 401", resp.StatusCode)
	}
}

func TestProjectAuthVerifyAndRecover(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	post := func(path string, body any) (*http.Response, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", ts.url+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+anonKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var js map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&js)
		return resp, js
	}

	// Recover is always 200, registered or not.
	for _, email := range []string{"nobody@example.com"} {
		resp, _ := post("/auth/v1/recover", map[string]string{"email": email})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("recover unknown status = %d", resp.StatusCode)
		}
	}

	// Signup then verify via a directly-seeded code row.
	resp, js := post("/auth/v1/signup", map[string]string{"email": "v@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signup status = %d body=%v", resp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	uid, _ := user["id"].(string)

	sum := sha256.Sum256([]byte("test-verify-code-1"))
	if err := ts.store.CreateProjectAuthCode(t.Context(), &metadata.ProjectAuthCode{
		ProjectID: pID, UserID: uid, Kind: metadata.AuthCodeVerifyEmail,
		TokenHash: hex.EncodeToString(sum[:]), ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	resp, js = post("/auth/v1/verify",
		map[string]string{"email": "v@example.com", "token": "test-verify-code-1", "type": "signup"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify status = %d body=%v", resp.StatusCode, js)
	}
	if js["access_token"] == nil {
		t.Fatalf("verify must issue a session: %v", js)
	}
	// Single-use: second attempt fails.
	resp, _ = post("/auth/v1/verify",
		map[string]string{"email": "v@example.com", "token": "test-verify-code-1", "type": "signup"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("re-verify status = %d, want 400", resp.StatusCode)
	}
	// Wrong type for the kind fails.
	sum2 := sha256.Sum256([]byte("test-verify-code-2"))
	if err := ts.store.CreateProjectAuthCode(t.Context(), &metadata.ProjectAuthCode{
		ProjectID: pID, UserID: uid, Kind: metadata.AuthCodeRecovery,
		TokenHash: hex.EncodeToString(sum2[:]), ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	resp, _ = post("/auth/v1/verify",
		map[string]string{"email": "v@example.com", "token": "test-verify-code-2", "type": "signup"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("kind mismatch status = %d, want 400", resp.StatusCode)
	}

	// OTP request is always 200 (creates unconfirmed users).
	resp, _ = post("/auth/v1/otp", map[string]string{"email": "otp@example.com"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("otp status = %d", resp.StatusCode)
	}
	if _, err := ts.store.GetProjectUserByEmail(t.Context(), pID, "otp@example.com"); err != nil {
		t.Fatalf("otp must create the user: %v", err)
	}

	// JWKS serves the project's public keys without auth.
	jreq, _ := http.NewRequest("GET", ts.url+"/v1/projects/"+pID+"/.well-known/jwks.json", nil)
	jresp, err := http.DefaultClient.Do(jreq)
	if err != nil {
		t.Fatal(err)
	}
	defer jresp.Body.Close()
	var jwks map[string]any
	_ = json.NewDecoder(jresp.Body).Decode(&jwks)
	keys, _ := jwks["keys"].([]any)
	if len(keys) == 0 {
		t.Fatalf("JWKS must serve keys: %v", jwks)
	}
	first, _ := keys[0].(map[string]any)
	if first["kid"] == nil || first["kty"] != "EC" {
		t.Fatalf("bad JWK: %v", first)
	}
	// Unknown project → 404 (FK on bootstrap), not a key leak.
	breq, _ := http.NewRequest("GET", ts.url+"/v1/projects/00000000-0000-0000-0000-000000000000/.well-known/jwks.json", nil)
	bresp, err := http.DefaultClient.Do(breq)
	if err != nil {
		t.Fatal(err)
	}
	defer bresp.Body.Close()
	if bresp.StatusCode != http.StatusNotFound {
		t.Fatalf("bogus project JWKS status = %d, want 404", bresp.StatusCode)
	}
}
