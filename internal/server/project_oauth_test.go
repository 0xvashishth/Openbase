package server_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openbase/openbase/internal/metadata"
)

// fakeGitHub mimics the three GitHub endpoints the driver calls.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("client_secret") != "github-secret" || r.Form.Get("code") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gh-tok","token_type":"bearer"}`))
	})
	mux.HandleFunc("/api/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gh-tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":42,"login":"octocat","name":"The Octocat"}`))
	})
	mux.HandleFunc("/api/emails", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"email":"octo@example.com","primary":true,"verified":true}]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func configureProvider(t *testing.T, ts *testServer, tok, pID, provider, clientID, secret string, cfg map[string]any) {
	t.Helper()
	cipher, keyID, err := ts.svc.Secrets.EncryptValue(secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.store.UpsertProjectAuthProvider(t.Context(), &metadata.ProjectAuthProvider{
		ProjectID: pID, Provider: provider, Enabled: true, ClientID: clientID,
		ClientSecretEncrypted: cipher, EncryptionKeyID: keyID, Config: cfg,
	}); err != nil {
		t.Fatal(err)
	}
}

var noRedirect = &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}}

func TestGitHubOAuthEndToEnd(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)
	gh := fakeGitHub(t)
	configureProvider(t, ts, tok, pID, "github", "github-id", "github-secret", map[string]any{
		"auth_url": gh.URL + "/login/oauth/authorize", "token_url": gh.URL + "/login/oauth/access_token",
		"userinfo_url": gh.URL + "/api/user", "emails_url": gh.URL + "/api/emails",
	})

	// 1. Authorize → 302 to the (fake) provider with a state.
	areq, _ := http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=github&apiKey="+anonKey+
		"&redirect_to="+url.QueryEscape("https://app.example.com/cb"), nil)
	aresp, err := noRedirect.Do(areq)
	if err != nil {
		t.Fatal(err)
	}
	defer aresp.Body.Close()
	if aresp.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d", aresp.StatusCode)
	}
	loc, _ := url.Parse(aresp.Header.Get("Location"))
	if !strings.HasPrefix(loc.String(), gh.URL+"/login/oauth/authorize") {
		t.Fatalf("redirect must target the provider: %s", loc.String())
	}
	state := loc.Query().Get("state")
	if state == "" || loc.Query().Get("client_id") != "github-id" {
		t.Fatalf("provider URL must carry client_id + state: %s", loc.String())
	}

	// 2. Callback → 302 back to the app with a one-time code.
	creq, _ := http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=github&code=fakecode&state="+url.QueryEscape(state), nil)
	cresp, err := noRedirect.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d", cresp.StatusCode)
	}
	cloc, _ := url.Parse(cresp.Header.Get("Location"))
	if !strings.HasPrefix(cloc.String(), "https://app.example.com/cb") {
		t.Fatalf("callback must return to redirect_to: %s", cloc.String())
	}
	code := cloc.Query().Get("code")
	if code == "" {
		t.Fatal("callback must issue a code")
	}

	// 3. Replay the callback state → 400.
	creq2, _ := http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=github&code=fakecode&state="+url.QueryEscape(state), nil)
	cresp2, err := noRedirect.Do(creq2)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp2.Body.Close()
	if cresp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", cresp2.StatusCode)
	}

	// 4. PKCE exchange (no challenge was sent, so no verifier needed).
	exchange := func(authCode string) (*http.Response, map[string]any) {
		b, _ := json.Marshal(map[string]string{"auth_code": authCode})
		req, _ := http.NewRequest("POST", ts.url+"/auth/v1/token?grant_type=pkce", strings.NewReader(string(b)))
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
	resp, js := exchange(code)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pkce exchange status = %d body=%v", resp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	if user["email"] != "octo@example.com" || user["email_confirmed_at"] == nil {
		t.Fatalf("linked user must carry the verified email: %v", user)
	}
	// Second exchange of the same code → 401.
	resp, _ = exchange(code)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code replay status = %d, want 401", resp.StatusCode)
	}

	// 5. Second login links to the SAME user (identity match).
	areq2, _ := http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=github&apiKey="+anonKey+
		"&redirect_to="+url.QueryEscape("https://app.example.com/cb"), nil)
	aresp2, err := noRedirect.Do(areq2)
	if err != nil {
		t.Fatal(err)
	}
	defer aresp2.Body.Close()
	loc2, _ := url.Parse(aresp2.Header.Get("Location"))
	creq3, _ := http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=github&code=fakecode2&state="+url.QueryEscape(loc2.Query().Get("state")), nil)
	cresp3, err := noRedirect.Do(creq3)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp3.Body.Close()
	cloc3, _ := url.Parse(cresp3.Header.Get("Location"))
	resp, js = exchange(cloc3.Query().Get("code"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-login exchange status = %d", resp.StatusCode)
	}
	user2, _ := js["user"].(map[string]any)
	if user2["id"] != user["id"] {
		t.Fatalf("re-login must link the same user: %v vs %v", user2["id"], user["id"])
	}
}

func TestOAuthGuards(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	// Unconfigured provider.
	req, _ := http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=github&apiKey="+anonKey+
		"&redirect_to="+url.QueryEscape("https://app.example.com/cb"), nil)
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unconfigured provider status = %d, want 400", resp.StatusCode)
	}

	// javascript: redirect is rejected even with a configured provider.
	gh := fakeGitHub(t)
	configureProvider(t, ts, tok, pID, "github", "id", "secret", map[string]any{
		"auth_url": gh.URL + "/x", "token_url": gh.URL + "/login/oauth/access_token",
		"userinfo_url": gh.URL + "/api/user",
	})
	req, _ = http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=github&apiKey="+anonKey+
		"&redirect_to="+url.QueryEscape("javascript:alert(1)"), nil)
	resp2, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad redirect status = %d, want 400", resp2.StatusCode)
	}

	// Bad state on callback.
	req, _ = http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=github&code=x&state=nope", nil)
	resp3, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad state status = %d, want 400", resp3.StatusCode)
	}
}

func TestPKCEChallengeEnforced(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)
	gh := fakeGitHub(t)
	configureProvider(t, ts, tok, pID, "github", "github-id", "github-secret", map[string]any{
		"auth_url": gh.URL + "/login/oauth/authorize", "token_url": gh.URL + "/login/oauth/access_token",
		"userinfo_url": gh.URL + "/api/user", "emails_url": gh.URL + "/api/emails",
	})

	// S256 challenge on authorize. Codes are single-use (a failed exchange
	// burns the code), so mint one login per attempt.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	login := func() string {
		areq, _ := http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=github&apiKey="+anonKey+
			"&redirect_to="+url.QueryEscape("https://app.example.com/cb")+
			"&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256", nil)
		aresp, err := noRedirect.Do(areq)
		if err != nil {
			t.Fatal(err)
		}
		defer aresp.Body.Close()
		loc, _ := url.Parse(aresp.Header.Get("Location"))
		if loc.Query().Get("code_challenge_method") != "S256" {
			t.Fatalf("challenge must pass through: %s", loc.String())
		}
		creq, _ := http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=github&code=c1&state="+url.QueryEscape(loc.Query().Get("state")), nil)
		cresp, err := noRedirect.Do(creq)
		if err != nil {
			t.Fatal(err)
		}
		defer cresp.Body.Close()
		cloc, _ := url.Parse(cresp.Header.Get("Location"))
		return cloc.Query().Get("code")
	}

	exchange := func(code, ver string) int {
		b, _ := json.Marshal(map[string]string{"auth_code": code, "code_verifier": ver})
		req, _ := http.NewRequest("POST", ts.url+"/auth/v1/token?grant_type=pkce", strings.NewReader(string(b)))
		req.Header.Set("Authorization", "Bearer "+anonKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	// Wrong verifier → 401 (and the code burns, so the good attempt needs a
	// fresh login).
	if st := exchange(login(), "wrong-verifier"); st != http.StatusUnauthorized {
		t.Fatalf("wrong verifier status = %d, want 401", st)
	}
	if st := exchange(login(), verifier); st != http.StatusOK {
		t.Fatalf("right verifier status = %d, want 200", st)
	}
}

// fakeOIDC serves discovery + JWKS (RSA) + token (signed id_token) + userinfo.
// The mux closes over srvURL so discovery issuer matches the server URL.
func fakeOIDC(t *testing.T, kid string, priv *rsa.PrivateKey, srvURL func() string) http.Handler {
	t.Helper()
	pub := priv.Public().(*rsa.PublicKey)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		issuer := srvURL()
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"userinfo_endpoint":      issuer + "/me",
			"jwks_uri":               issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		claims := jwt.MapClaims{
			"iss": srvURL(), "aud": "oidc-client", "sub": "oidc-sub-1",
			"email": "oidc@example.com", "exp": time.Now().Add(time.Hour).Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = kid
		signed, err := tok.SignedString(priv)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "oidc-tok", "id_token": signed, "token_type": "bearer"})
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": "oidc-sub-1", "email": "oidc@example.com", "email_verified": true, "name": "Oid C",
		})
	})
	return mux
}

func TestOIDCConnectorEndToEnd(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Self-referential issuer: the handler reads the server URL at request time.
	var oidcURL string
	oidcSrv := httptest.NewServer(fakeOIDC(t, "oidc-k1", priv, func() string { return oidcURL }))
	t.Cleanup(oidcSrv.Close)
	oidcURL = oidcSrv.URL
	issuerURL := oidcSrv.URL

	configureProvider(t, ts, tok, pID, "oidc", "oidc-client", "oidc-secret", map[string]any{"issuer": issuerURL})

	areq, _ := http.NewRequest("GET", ts.url+"/auth/v1/authorize?provider=oidc&apiKey="+anonKey+
		"&redirect_to="+url.QueryEscape("https://app.example.com/cb"), nil)
	aresp, err := noRedirect.Do(areq)
	if err != nil {
		t.Fatal(err)
	}
	defer aresp.Body.Close()
	if aresp.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d", aresp.StatusCode)
	}
	loc, _ := url.Parse(aresp.Header.Get("Location"))
	if !strings.HasPrefix(loc.String(), issuerURL+"/auth") {
		t.Fatalf("must redirect to discovered endpoint: %s", loc.String())
	}
	creq, _ := http.NewRequest("GET", ts.url+"/auth/v1/callback?provider=oidc&code=c&state="+url.QueryEscape(loc.Query().Get("state")), nil)
	cresp, err := noRedirect.Do(creq)
	if err != nil {
		t.Fatal(err)
	}
	defer cresp.Body.Close()
	if cresp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d", cresp.StatusCode)
	}
	cloc, _ := url.Parse(cresp.Header.Get("Location"))
	b, _ := json.Marshal(map[string]string{"auth_code": cloc.Query().Get("code")})
	ereq, _ := http.NewRequest("POST", ts.url+"/auth/v1/token?grant_type=pkce", strings.NewReader(string(b)))
	ereq.Header.Set("Authorization", "Bearer "+anonKey)
	ereq.Header.Set("Content-Type", "application/json")
	eresp, err := http.DefaultClient.Do(ereq)
	if err != nil {
		t.Fatal(err)
	}
	defer eresp.Body.Close()
	var js map[string]any
	_ = json.NewDecoder(eresp.Body).Decode(&js)
	if eresp.StatusCode != http.StatusOK {
		t.Fatalf("exchange status = %d body=%v", eresp.StatusCode, js)
	}
	user, _ := js["user"].(map[string]any)
	if user["email"] != "oidc@example.com" {
		t.Fatalf("oidc user: %v", user)
	}
}

func TestAnonymousUpgradeAndPhoneOTP(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)
	anonKey := newAnonKey(t, ts, tok, pID)

	post := func(path, key string, body any) (*http.Response, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", ts.url+path, strings.NewReader(string(b)))
		req.Header.Set("Authorization", "Bearer "+key)
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

	// Anonymous sign-in returns a session for an anon user.
	resp, js := post("/auth/v1/otp", anonKey, map[string]any{"anonymous": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anon signup status = %d body=%v", resp.StatusCode, js)
	}
	anonAccess, _ := js["access_token"].(string)
	user, _ := js["user"].(map[string]any)
	if anonAccess == "" || user["is_anonymous"] != true {
		t.Fatalf("anon session: %v", js)
	}

	// Upgrade via PUT /user with email+password (linkIdentity).
	b, _ := json.Marshal(map[string]string{"email": "graduated@example.com", "password": "long-enough-password"})
	preq, _ := http.NewRequest("PUT", ts.url+"/auth/v1/user", strings.NewReader(string(b)))
	preq.Header.Set("Authorization", "Bearer "+anonAccess)
	preq.Header.Set("Content-Type", "application/json")
	presp, err := http.DefaultClient.Do(preq)
	if err != nil {
		t.Fatal(err)
	}
	defer presp.Body.Close()
	var pjs map[string]any
	_ = json.NewDecoder(presp.Body).Decode(&pjs)
	if presp.StatusCode != http.StatusOK {
		t.Fatalf("upgrade status = %d body=%v", presp.StatusCode, pjs)
	}
	puser, _ := pjs["user"].(map[string]any)
	if puser["email"] != "graduated@example.com" || puser["is_anonymous"] == true {
		t.Fatalf("upgraded user: %v", puser)
	}
	// The graduated credentials sign in with a password.
	resp, _ = post("/auth/v1/token?grant_type=password", anonKey,
		map[string]string{"email": "graduated@example.com", "password": "long-enough-password"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-upgrade login status = %d", resp.StatusCode)
	}

	// Phone OTP creates the user and records a numeric code (log SMS driver).
	resp, _ = post("/auth/v1/otp", anonKey, map[string]string{"phone": "+155500042"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("phone otp status = %d", resp.StatusCode)
	}
	if _, err := ts.store.GetProjectUserByPhone(t.Context(), pID, "+155500042"); err != nil {
		t.Fatalf("phone user must exist: %v", err)
	}
}
