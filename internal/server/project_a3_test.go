package server_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/mfa"
)

// jwtPayload decodes the middle segment of a JWT without verifying (tests
// verify through the API first; this only inspects claims).
func jwtPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed jwt: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func postAuth(t *testing.T, ts *testServer, path, key string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", ts.url+path, strings.NewReader(string(b)))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
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

func serviceKey(t *testing.T, ts *testServer, tok, pID string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "svc"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create service key: %d %v", resp.StatusCode, js)
	}
	plaintext, _ := js["plaintext"].(string)
	if js["role"] != "service_role" {
		t.Fatalf("default key must be service_role: %v", js)
	}
	return plaintext
}

func signupEndUser(t *testing.T, ts *testServer, anonKey, email string) (access, refresh, userID string) {
	t.Helper()
	resp, js := postAuth(t, ts, "/auth/v1/signup", anonKey,
		map[string]string{"email": email, "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signup %s: %d %v", email, resp.StatusCode, js)
	}
	access, _ = js["access_token"].(string)
	refresh, _ = js["refresh_token"].(string)
	user, _ := js["user"].(map[string]any)
	uid, _ := user["id"].(string)
	if access == "" || refresh == "" || uid == "" {
		t.Fatalf("bad session: %v", js)
	}
	return access, refresh, uid
}

func TestMFAEnrollChallengeVerifyFlow(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)
	access, _, _ := signupEndUser(t, ts, anonKey, "mfa@example.com")

	// Enroll without a user JWT (anon key alone) → 401.
	resp, _ := postAuth(t, ts, "/auth/v1/factors", anonKey, map[string]string{"friendly_name": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("key-only enroll status = %d, want 401", resp.StatusCode)
	}

	// Enroll.
	resp, js := postAuth(t, ts, "/auth/v1/factors", access, map[string]string{"friendly_name": "phone"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enroll status = %d body=%v", resp.StatusCode, js)
	}
	factorID, _ := js["id"].(string)
	totp, _ := js["totp"].(map[string]any)
	secret, _ := totp["secret"].(string)
	if factorID == "" || secret == "" || totp["uri"] == nil {
		t.Fatalf("enroll must return id + one-time secret: %v", js)
	}

	// List shows the factor without the secret.
	req, _ := http.NewRequest("GET", ts.url+"/auth/v1/factors", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	lresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer lresp.Body.Close()
	var list map[string]any
	_ = json.NewDecoder(lresp.Body).Decode(&list)
	items, _ := list["totp"].([]any)
	if len(items) != 1 {
		t.Fatalf("list factors: %v", list)
	}
	if _, has := items[0].(map[string]any)["secret"]; has {
		t.Fatal("list must never include secrets")
	}

	// Challenge + wrong code → 400.
	resp, js = postAuth(t, ts, "/auth/v1/factors/"+factorID+"/challenge", access, map[string]any{})
	if resp.StatusCode != http.StatusOK || js["id"] == nil {
		t.Fatalf("challenge status = %d body=%v", resp.StatusCode, js)
	}
	challengeID, _ := js["id"].(string)
	resp, _ = postAuth(t, ts, "/auth/v1/factors/"+factorID+"/verify", access,
		map[string]string{"challenge_id": challengeID, "code": "000000"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong code status = %d, want 400", resp.StatusCode)
	}

	// Right code → aal2 session.
	code, err := mfa.CodeAt(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	resp, js = postAuth(t, ts, "/auth/v1/factors/"+factorID+"/verify", access,
		map[string]string{"challenge_id": challengeID, "code": code})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify status = %d body=%v", resp.StatusCode, js)
	}
	aal2, _ := js["access_token"].(string)
	if jwtPayload(t, aal2)["aal"] != "aal2" {
		t.Fatalf("verified session must be aal2: %v", js)
	}
	// Challenge is single-use.
	resp, _ = postAuth(t, ts, "/auth/v1/factors/"+factorID+"/verify", access,
		map[string]string{"challenge_id": challengeID, "code": code})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("challenge replay status = %d, want 400", resp.StatusCode)
	}

	// Unenroll.
	dreq, _ := http.NewRequest("DELETE", ts.url+"/auth/v1/factors/"+factorID, nil)
	dreq.Header.Set("Authorization", "Bearer "+access)
	dresp, err := http.DefaultClient.Do(dreq)
	if err != nil {
		t.Fatal(err)
	}
	defer dresp.Body.Close()
	if dresp.StatusCode != http.StatusOK {
		t.Fatalf("unenroll status = %d", dresp.StatusCode)
	}
}

func TestAdminUsersAndImpersonation(t *testing.T) {
	ts := newTestServer(t)
	tok, orgID, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)
	svcKey := serviceKey(t, ts, tok, pID)

	// Anon keys are refused everywhere in the admin API.
	resp, _ := postAuth(t, ts, "/auth/v1/admin/users", anonKey, map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("anon admin status = %d, want 403", resp.StatusCode)
	}

	// Create (confirmed) + get + update (ban + metadata) + banned login + delete.
	resp, js := postAuth(t, ts, "/auth/v1/admin/users", svcKey,
		map[string]any{"email": "managed@example.com", "password": "long-enough-password", "email_confirm": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("admin create status = %d body=%v", resp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	uid, _ := user["id"].(string)

	getReq, _ := http.NewRequest("GET", ts.url+"/auth/v1/admin/users/"+uid, nil)
	getReq.Header.Set("Authorization", "Bearer "+svcKey)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("admin get status = %d", getResp.StatusCode)
	}

	// Ban for an hour: password login must 403.
	banBody, _ := json.Marshal(map[string]any{"ban_duration": "1h"})
	banReq, _ := http.NewRequest("PUT", ts.url+"/auth/v1/admin/users/"+uid, strings.NewReader(string(banBody)))
	banReq.Header.Set("Authorization", "Bearer "+svcKey)
	banReq.Header.Set("Content-Type", "application/json")
	banResp, err := http.DefaultClient.Do(banReq)
	if err != nil {
		t.Fatal(err)
	}
	defer banResp.Body.Close()
	if banResp.StatusCode != http.StatusOK {
		t.Fatalf("admin ban status = %d", banResp.StatusCode)
	}
	resp, _ = postAuth(t, ts, "/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "managed@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("banned login status = %d, want 403", resp.StatusCode)
	}

	// Impersonation refuses banned users, works after unban, and is audited.
	resp, _ = postAuth(t, ts, "/auth/v1/admin/impersonate", svcKey, map[string]string{"user_id": uid})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("banned impersonate status = %d, want 403", resp.StatusCode)
	}
	unbanBody, _ := json.Marshal(map[string]any{"ban_duration": "none"})
	unbanReq, _ := http.NewRequest("PUT", ts.url+"/auth/v1/admin/users/"+uid, strings.NewReader(string(unbanBody)))
	unbanReq.Header.Set("Authorization", "Bearer "+svcKey)
	unbanReq.Header.Set("Content-Type", "application/json")
	if uresp, err := http.DefaultClient.Do(unbanReq); err != nil {
		t.Fatal(err)
	} else {
		defer uresp.Body.Close()
	}
	resp, js = postAuth(t, ts, "/auth/v1/admin/impersonate", svcKey, map[string]string{"user_id": uid})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("impersonate status = %d body=%v", resp.StatusCode, js)
	}
	impAccess, _ := js["access_token"].(string)
	claims := jwtPayload(t, impAccess)
	if claims["custom"] == nil {
		t.Fatalf("impersonation must be marked in claims: %v", claims)
	}
	// The impersonated token authenticates as the user.
	ureq, _ := http.NewRequest("GET", ts.url+"/auth/v1/user", nil)
	ureq.Header.Set("Authorization", "Bearer "+impAccess)
	uresp, err := http.DefaultClient.Do(ureq)
	if err != nil {
		t.Fatal(err)
	}
	defer uresp.Body.Close()
	if uresp.StatusCode != http.StatusOK {
		t.Fatalf("impersonated get user status = %d", uresp.StatusCode)
	}

	// Audit trail records the admin actions.
	areq, _ := http.NewRequest("GET", ts.url+"/v1/orgs/"+orgID+"/audit?limit=50", nil)
	areq.Header.Set("Authorization", "Bearer "+tok)
	aresp, err := http.DefaultClient.Do(areq)
	if err != nil {
		t.Fatal(err)
	}
	defer aresp.Body.Close()
	var audit []any
	_ = json.NewDecoder(aresp.Body).Decode(&audit)
	found := false
	for _, e := range audit {
		if m, _ := e.(map[string]any); m["action"] == "admin.impersonate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit must contain admin.impersonate: %v", audit)
	}

	// generate_link returns a usable link; invite creates + mails.
	resp, js = postAuth(t, ts, "/auth/v1/admin/generate_link", svcKey,
		map[string]string{"type": "signup", "email": "linked@example.com"})
	if resp.StatusCode != http.StatusOK || js["action_link"] == nil {
		t.Fatalf("generate_link status = %d body=%v", resp.StatusCode, js)
	}
	resp, js = postAuth(t, ts, "/auth/v1/admin/invite", svcKey, map[string]string{"email": "invited@example.com"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("invite status = %d body=%v", resp.StatusCode, js)
	}

	// Delete → get 404s.
	dreq, _ := http.NewRequest("DELETE", ts.url+"/auth/v1/admin/users/"+uid, nil)
	dreq.Header.Set("Authorization", "Bearer "+svcKey)
	if dresp, err := http.DefaultClient.Do(dreq); err != nil {
		t.Fatal(err)
	} else {
		defer dresp.Body.Close()
		if dresp.StatusCode != http.StatusOK {
			t.Fatalf("admin delete status = %d", dresp.StatusCode)
		}
	}
	getReq2, _ := http.NewRequest("GET", ts.url+"/auth/v1/admin/users/"+uid, nil)
	getReq2.Header.Set("Authorization", "Bearer "+svcKey)
	if getResp2, err := http.DefaultClient.Do(getReq2); err != nil {
		t.Fatal(err)
	} else {
		defer getResp2.Body.Close()
		if getResp2.StatusCode != http.StatusNotFound {
			t.Fatalf("deleted get status = %d, want 404", getResp2.StatusCode)
		}
	}
}


func TestOperatorProjectUserManagement(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)
	signupEndUser(t, ts, anonKey, "managed-op@example.com")

	// List (operator token).
	resp, js := ts.do(t, "GET", "/v1/projects/"+pID+"/users", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("op list status = %d body=%v", resp.StatusCode, js)
	}
	users, _ := js["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("op list users: %v", js)
	}
	uid := users[0].(map[string]any)["id"].(string)

	// Search narrows.
	resp, js = ts.do(t, "GET", "/v1/projects/"+pID+"/users?search=managed-op", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("op search status = %d", resp.StatusCode)
	}

	// Ban via operator endpoint; password login 403s.
	resp, _ = ts.do(t, "PUT", "/v1/projects/"+pID+"/users/"+uid, tok,
		map[string]any{"banned": true, "ban_duration": "1h"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("op ban status = %d", resp.StatusCode)
	}
	resp, _ = postAuth(t, ts, "/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "managed-op@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("banned login status = %d, want 403", resp.StatusCode)
	}

	// Unban + confirm email.
	resp, _ = ts.do(t, "PUT", "/v1/projects/"+pID+"/users/"+uid, tok,
		map[string]any{"banned": false, "confirm_email": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("op unban status = %d", resp.StatusCode)
	}

	// Delete.
	resp, _ = ts.do(t, "DELETE", "/v1/projects/"+pID+"/users/"+uid, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("op delete status = %d", resp.StatusCode)
	}
	resp, _ = ts.do(t, "DELETE", "/v1/projects/"+pID+"/users/"+uid, tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("double delete status = %d, want 404", resp.StatusCode)
	}
}

func createHookFunction(t *testing.T, ts *testServer, tok, pID, name, source string) string {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/functions", tok,
		map[string]string{"name": name, "runtime": "node", "source": source})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create function %s: %d %v", name, resp.StatusCode, js)
	}
	id, _ := js["id"].(string)
	if id == "" {
		t.Fatalf("function without id: %v", js)
	}
	return id
}

func upsertHook(t *testing.T, ts *testServer, tok, pID, event, fnID string, failOpen bool) {
	t.Helper()
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/auth-hooks", tok,
		map[string]any{"event": event, "function_id": fnID, "fail_open": failOpen})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert hook %s: %d %v", event, resp.StatusCode, js)
	}
}

func TestAuthHooksRejectMergeAndClaims(t *testing.T) {
	ts := newTestServer(t)
	ts.svc.Functions = function.New()
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	// Rejecting hook.
	rejectFn := createHookFunction(t, ts, tok, pID, "reject",
		`exports.handler = async (event) => ({ error: "no bots allowed" });`)
	upsertHook(t, ts, tok, pID, "before-user-created", rejectFn, false)
	resp, js := postAuth(t, ts, "/auth/v1/signup", anonKey,
		map[string]string{"email": "bot@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rejected signup status = %d body=%v", resp.StatusCode, js)
	}

	// Metadata-merging hook replaces the rejector.
	mergeFn := createHookFunction(t, ts, tok, pID, "merge",
		`exports.handler = async (event) => ({ user_metadata: { tier: "pro" } });`)
	upsertHook(t, ts, tok, pID, "before-user-created", mergeFn, false)
	resp, js = postAuth(t, ts, "/auth/v1/signup", anonKey,
		map[string]string{"email": "vip@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("merged signup status = %d body=%v", resp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	if meta, _ := user["user_metadata"].(map[string]any); meta["tier"] != "pro" {
		t.Fatalf("hook metadata must merge: %v", user)
	}

	// Custom-claims hook on token issue.
	claimsFn := createHookFunction(t, ts, tok, pID, "claims",
		`exports.handler = async (event) => ({ custom_claims: { tenant: "t1" } });`)
	upsertHook(t, ts, tok, pID, "before-token-issued", claimsFn, false)
	resp, js = postAuth(t, ts, "/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "vip@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d body=%v", resp.StatusCode, js)
	}
	claims := jwtPayload(t, js["access_token"].(string))
	custom, _ := claims["custom"].(map[string]any)
	if custom["tenant"] != "t1" {
		t.Fatalf("hook claims must merge: %v", claims)
	}

	// Failing hook: fail-open lets signup through, fail-closed blocks it.
	boomFn := createHookFunction(t, ts, tok, pID, "boom",
		`exports.handler = async (event) => { throw new Error("kaput"); };`)
	upsertHook(t, ts, tok, pID, "before-user-created", boomFn, true)
	resp, _ = postAuth(t, ts, "/auth/v1/signup", anonKey,
		map[string]string{"email": "open@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fail-open signup status = %d, want 200", resp.StatusCode)
	}
	upsertHook(t, ts, tok, pID, "before-user-created", boomFn, false)
	resp, _ = postAuth(t, ts, "/auth/v1/signup", anonKey,
		map[string]string{"email": "closed@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("fail-closed signup status = %d, want 422", resp.StatusCode)
	}
}

func TestIDTokenGrantWithGoogleJWKS(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	// Fake Google: JWKS + minted id_token.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(*rsa.PublicKey)
	var jwksURL string
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": "g-k1", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	}))
	defer jwksSrv.Close()
	jwksURL = jwksSrv.URL

	configureProvider(t, ts, tok, pID, "google", "google-client-id", "google-secret", map[string]any{
		"jwks_uri": jwksURL,
	})
	mint := func(aud string) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": "https://accounts.google.com", "aud": aud, "sub": "google-sub-9",
			"email": "native@example.com", "email_verified": true, "name": "Native",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		tok.Header["kid"] = "g-k1"
		signed, err := tok.SignedString(priv)
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}

	// Wrong audience → 401.
	resp, _ := postAuth(t, ts, "/auth/v1/token?grant_type=id_token", anonKey,
		map[string]string{"provider": "google", "id_token": mint("someone-else")})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad audience status = %d, want 401", resp.StatusCode)
	}
	// Correct → session for the linked user.
	resp, js := postAuth(t, ts, "/auth/v1/token?grant_type=id_token", anonKey,
		map[string]string{"provider": "google", "id_token": mint("google-client-id")})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("id_token grant status = %d body=%v", resp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	if user["email"] != "native@example.com" {
		t.Fatalf("native user: %v", user)
	}
	_ = jwksURL
}
