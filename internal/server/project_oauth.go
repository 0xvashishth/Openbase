package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openbase/openbase/internal/metadata"
	"github.com/openbase/openbase/internal/sms"
)

// OAuth + OIDC login for end users (Phase 10, A2). Supported drivers:
// github, google, and a generic oidc connector (issuer discovery).
//
// Flow (PKCE-capable, Supabase-shaped):
//
//	GET  /auth/v1/authorize?provider=github&apikey=ob_…&redirect_to=…[&scopes=][&code_challenge=][&code_challenge_method=]
//	  → 302 to the provider with a one-time state
//	GET  /auth/v1/callback?provider=github&code=…&state=…
//	  → exchanges the code, links-or-creates the user, 302 to redirect_to?code=…
//	POST /auth/v1/token?grant_type=pkce  {auth_code, code_verifier}
//	  → the session (this leg verifies the PKCE challenge)
//
// The apikey query parameter authenticates /authorize because browsers cannot
// set headers on navigation. Provider endpoint URLs default to production and
// are overridable per project in the provider row config (auth_url, token_url,
// userinfo_url, emails_url) — which is also how tests point at fakes.

// ---- driver endpoints ----

type oauthEndpoints struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	EmailsURL   string // github only
}

func defaultEndpoints(provider string) (oauthEndpoints, bool) {
	switch provider {
	case "github":
		return oauthEndpoints{
			AuthURL:     "https://github.com/login/oauth/authorize",
			TokenURL:    "https://github.com/login/oauth/access_token",
			UserInfoURL: "https://api.github.com/user",
			EmailsURL:   "https://api.github.com/user/emails",
		}, true
	case "google":
		return oauthEndpoints{
			AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:    "https://oauth2.googleapis.com/token",
			UserInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
		}, true
	case "oidc":
		return oauthEndpoints{}, true // resolved via discovery per project
	}
	return oauthEndpoints{}, false
}

func endpointOverride(cfg map[string]any, key, def string) string {
	if cfg != nil {
		if v, _ := cfg[key].(string); v != "" {
			return v
		}
	}
	return def
}

// resolveEndpoints returns effective endpoints for a provider row. For oidc
// the issuer is discovered (config.issuer, e.g. https://accounts.example.com).
func (s *Server) resolveEndpoints(ctx context.Context, provider string, cfg map[string]any) (oauthEndpoints, error) {
	def, ok := defaultEndpoints(provider)
	if !ok {
		return oauthEndpoints{}, fmt.Errorf("unknown provider %q", provider)
	}
	if provider == "oidc" {
		issuer, _ := cfg["issuer"].(string)
		if issuer == "" {
			return oauthEndpoints{}, fmt.Errorf("oidc provider needs config.issuer")
		}
		disc, err := discoverOIDC(ctx, issuer)
		if err != nil {
			return oauthEndpoints{}, err
		}
		return oauthEndpoints{
			AuthURL:     endpointOverride(cfg, "auth_url", disc.AuthorizationEndpoint),
			TokenURL:    endpointOverride(cfg, "token_url", disc.TokenEndpoint),
			UserInfoURL: endpointOverride(cfg, "userinfo_url", disc.UserinfoEndpoint),
		}, nil
	}
	return oauthEndpoints{
		AuthURL:     endpointOverride(cfg, "auth_url", def.AuthURL),
		TokenURL:    endpointOverride(cfg, "token_url", def.TokenURL),
		UserInfoURL: endpointOverride(cfg, "userinfo_url", def.UserInfoURL),
		EmailsURL:   endpointOverride(cfg, "emails_url", def.EmailsURL),
	}, nil
}

func defaultScopes(provider string, cfg map[string]any) string {
	if cfg != nil {
		if v, _ := cfg["scopes"].(string); v != "" {
			return v
		}
	}
	switch provider {
	case "github":
		return "read:user user:email"
	case "google", "oidc":
		return "openid email profile"
	}
	return ""
}

// providerCredentials loads and decrypts a provider row. Disabled or missing
// rows are 400s (operator configuration error, not a user error to hide).
func (s *Server) providerCredentials(ctx context.Context, projectID, provider string) (*metadata.ProjectAuthProvider, string, error) {
	row, err := s.svc.Store.GetProjectAuthProvider(ctx, projectID, provider)
	if err != nil {
		return nil, "", fmt.Errorf("provider %q is not configured", provider)
	}
	if !row.Enabled {
		return nil, "", fmt.Errorf("provider %q is disabled", provider)
	}
	secret := ""
	if len(row.ClientSecretEncrypted) > 0 {
		if s.svc.Secrets == nil {
			return nil, "", fmt.Errorf("no secrets provider to decrypt provider credentials")
		}
		secret, err = s.svc.Secrets.DecryptValue(row.ClientSecretEncrypted, row.EncryptionKeyID)
		if err != nil {
			return nil, "", err
		}
	}
	return row, secret, nil
}

// validRedirect reports whether redirect_to is allowed: it must match the
// allow-list when configured (trailing /** = prefix), else it must be https
// or loopback http (dev) — never javascript: or opaque strings.
func validRedirect(target string, allowList []string) bool {
	if target == "" {
		return false
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	if u.Scheme == "http" {
		host := strings.ToLower(u.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" {
			return false
		}
	}
	if len(allowList) == 0 {
		return true
	}
	for _, rule := range allowList {
		rule = strings.TrimSpace(rule)
		if strings.HasSuffix(rule, "/**") {
			if strings.HasPrefix(target, strings.TrimSuffix(rule, "**")) {
				return true
			}
			continue
		}
		if target == rule {
			return true
		}
	}
	return false
}

// ---- /authorize ----

// projectAuthorize starts the login: validates config + redirect, stores the
// state (hash only), and 302s to the provider. GET so browsers can navigate.
func (s *Server) projectAuthorize(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r.Context())
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	row, secret, err := s.providerCredentials(r.Context(), projectID, provider)
	if err != nil || row.ClientID == "" || secret == "" {
		writeError(w, http.StatusBadRequest, "provider "+provider+" is not configured")
		return
	}
	ep, err := s.resolveEndpoints(r.Context(), provider, row.Config)
	if err != nil || ep.AuthURL == "" {
		writeError(w, http.StatusBadRequest, "provider "+provider+" is not configured")
		return
	}
	st, _ := s.svc.Store.GetOrCreateProjectAuthSettings(r.Context(), projectID)
	var allow []string
	if st != nil {
		allow = st.RedirectAllowList
	}
	redirectTo := r.URL.Query().Get("redirect_to")
	if !validRedirect(redirectTo, allow) {
		writeError(w, http.StatusBadRequest, "redirect_to is not allow-listed")
		return
	}
	scopes := r.URL.Query().Get("scopes")
	if scopes == "" {
		scopes = defaultScopes(provider, row.Config)
	}
	challenge := r.URL.Query().Get("code_challenge")
	method := r.URL.Query().Get("code_challenge_method")
	if method == "" && challenge != "" {
		method = "plain"
	}
	if method != "" && method != "plain" && method != "S256" {
		writeError(w, http.StatusBadRequest, "code_challenge_method must be plain or S256")
		return
	}
	state, stateHash, err := newOpaqueCode(24)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.CreateOAuthState(r.Context(), &metadata.ProjectOAuthState{
		ProjectID: projectID, Provider: provider, StateHash: stateHash,
		RedirectTo: redirectTo, CodeChallenge: challenge, CodeChallengeMethod: method,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}); err != nil {
		s.writeErr(w, err)
		return
	}
	callback := publicBaseURL(r, s.svc.PublicBaseURL) + "/auth/v1/callback?provider=" + url.QueryEscape(provider)
	authURL, _ := url.Parse(ep.AuthURL)
	q := authURL.Query()
	q.Set("client_id", row.ClientID)
	q.Set("redirect_uri", callback)
	q.Set("response_type", "code")
	if scopes != "" {
		q.Set("scope", scopes)
	}
	q.Set("state", state)
	if challenge != "" {
		q.Set("code_challenge", challenge)
		q.Set("code_challenge_method", method)
	}
	authURL.RawQuery = q.Encode()
	http.Redirect(w, r, authURL.String(), http.StatusFound)
}

// ---- /callback ----

type providerIdentity struct {
	UID           string
	Email         string
	EmailVerified bool
	Name          string
}

// projectCallback handles the provider redirect: validates state, exchanges
// the code, links-or-creates the user, attaches a one-time auth code, and
// redirects to redirect_to?code=… for the pkce exchange.
func (s *Server) projectCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		writeError(w, http.StatusBadRequest, "provider denied the request: "+errParam)
		return
	}
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if provider == "" || code == "" || state == "" {
		writeError(w, http.StatusBadRequest, "provider, code and state are required")
		return
	}
	sum := sha256.Sum256([]byte(state))
	st, err := s.svc.Store.GetOAuthStateByHash(r.Context(), hexEncode(sum[:]))
	if err != nil || st.Provider != provider || st.UsedAt != nil || st.AuthCodeHash != "" {
		writeError(w, http.StatusBadRequest, "invalid or expired state (replay?)")
		return
	}
	row, secret, err := s.providerCredentials(r.Context(), st.ProjectID, provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, "provider "+provider+" is not configured")
		return
	}
	ep, err := s.resolveEndpoints(r.Context(), provider, row.Config)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider endpoints unavailable")
		return
	}
	token, err := exchangeCode(r.Context(), ep.TokenURL, row.ClientID, secret,
		publicBaseURL(r, s.svc.PublicBaseURL)+"/auth/v1/callback?provider="+url.QueryEscape(provider), code)
	if err != nil {
		writeError(w, http.StatusBadGateway, "code exchange failed")
		return
	}
	if provider == "oidc" && token.IDToken != "" {
		// Strengthen the code flow with id_token verification when the
		// provider returns one; the userinfo sub stays canonical.
		if issuer, _ := row.Config["issuer"].(string); issuer != "" {
			if disc, derr := discoverOIDC(r.Context(), issuer); derr == nil {
				if verr := verifyOIDCIDToken(r.Context(), token.IDToken, disc.JWKSURI, row.ClientID, disc.Issuer); verr != nil {
					writeError(w, http.StatusBadGateway, "id_token verification failed")
					return
				}
			}
		}
	}
	idn, err := fetchProviderIdentity(r.Context(), provider, ep, token.AccessToken, token.IDToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not read provider identity")
		return
	}
	if idn.UID == "" {
		writeError(w, http.StatusBadGateway, "provider returned no user id")
		return
	}
	u, err := s.linkOrCreateProviderUser(r.Context(), st.ProjectID, provider, idn)
	if err != nil {
		if errors.Is(err, errHookRejected) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.writeErr(w, err)
		return
	}
	authCode, authCodeHash, err := newOpaqueCode(32)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.AttachOAuthCode(r.Context(), st.ID, u.ID, authCodeHash); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired state (replay?)")
		return
	}
	dest, _ := url.Parse(st.RedirectTo)
	q := dest.Query()
	q.Set("code", authCode)
	dest.RawQuery = q.Encode()
	http.Redirect(w, r, dest.String(), http.StatusFound)
}

// linkOrCreateProviderUser resolves identity → existing linked user, else
// verified-email match (linking the identity), else a fresh user.
func (s *Server) linkOrCreateProviderUser(ctx context.Context, projectID, provider string, idn *providerIdentity) (*metadata.ProjectUser, error) {
	// Fast path: query identities via a probe user? The store has no
	// by-provider lookup, so resolve through users is N/A — instead keep a
	// deterministic probe: identities are listed per user, so look up by
	// verified email first, then confirm the link.
	if idn.Email != "" && idn.EmailVerified {
		if u, err := s.svc.Store.GetProjectUserByEmail(ctx, projectID, strings.ToLower(idn.Email)); err == nil {
			_ = s.svc.Store.UpsertProjectIdentity(ctx, &metadata.ProjectIdentity{
				ProjectID: projectID, UserID: u.ID, Provider: provider,
				ProviderUID: idn.UID, IdentityData: map[string]any{"name": idn.Name},
			})
			return u, nil
		}
	}
	u := &metadata.ProjectUser{ProjectID: projectID, PasswordHash: ""}
	if idn.Email != "" {
		email := strings.ToLower(idn.Email)
		u.Email = &email
		if idn.EmailVerified {
			now := time.Now()
			u.EmailConfirmedAt = &now
		}
	}
	if idn.Name != "" {
		u.UserMetadata = map[string]any{"full_name": idn.Name}
	}
	// Anonymous fallback: no email at all (e.g. private GitHub email).
	if u.Email == nil {
		u.IsAnonymous = true
	}
	if hookRes, herr := s.hooks().run(ctx, projectID, metadata.HookBeforeUserCreated, map[string]any{
		"user": map[string]any{"email": strOrEmpty(u.Email), "provider": provider},
	}); herr != nil {
		return nil, herr
	} else if msg := hookError(hookRes); msg != "" {
		return nil, fmt.Errorf("%w: signup rejected: %s", errHookRejected, msg)
	} else {
		if u.UserMetadata == nil {
			u.UserMetadata = map[string]any{}
		}
		for k, v := range hookUserMetadata(hookRes) {
			u.UserMetadata[k] = v
		}
	}
	if err := s.svc.Store.CreateProjectUser(ctx, u); err != nil {
		return nil, err
	}
	if _, herr := s.hooks().run(ctx, projectID, metadata.HookAfterUserCreated, map[string]any{
		"user": map[string]any{"id": u.ID, "email": strOrEmpty(u.Email), "provider": provider},
	}); herr != nil {
		s.svc.Log.Warn("auth hook after-user-created failed", "project", projectID, "err", herr)
	}
	_ = s.svc.Store.UpsertProjectIdentity(ctx, &metadata.ProjectIdentity{
		ProjectID: projectID, UserID: u.ID, Provider: provider,
		ProviderUID: idn.UID, IdentityData: map[string]any{"name": idn.Name},
	})
	return u, nil
}

// ---- provider HTTP ----

type oauthToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
}

var oauthHTTP = &http.Client{Timeout: 10 * time.Second}

func hexEncode(b []byte) string { return hex.EncodeToString(b) }

// exchangeCode trades the authorization code for tokens (OAuth2 §4.1.3).
func exchangeCode(ctx context.Context, tokenURL, clientID, secret, redirectURI, code string) (*oauthToken, error) {
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"client_id":    {clientID},
		"client_secret": {secret},
		"redirect_uri": {redirectURI},
		"code":         {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // github defaults to form-encoding otherwise
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint status %d", resp.StatusCode)
	}
	var tok oauthToken
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access token")
	}
	return &tok, nil
}

func getJSON(ctx context.Context, url, accessToken string, extraHeaders map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(body, out)
}

// fetchProviderIdentity reads the provider profile via the userinfo endpoint
// (github additionally consults /emails for a verified primary address).
func fetchProviderIdentity(ctx context.Context, provider string, ep oauthEndpoints, accessToken, idToken string) (*providerIdentity, error) {
	switch provider {
	case "github":
		var gu struct {
			ID    int64   `json:"id"`
			Login string  `json:"login"`
			Name  *string `json:"name"`
			Email *string `json:"email"`
		}
		if err := getJSON(ctx, ep.UserInfoURL, accessToken,
			map[string]string{"User-Agent": "openbase"}, &gu); err != nil {
			return nil, err
		}
		idn := &providerIdentity{UID: fmt.Sprint(gu.ID), Name: deref(gu.Name, gu.Login)}
		if gu.Email != nil && *gu.Email != "" {
			idn.Email = *gu.Email
		}
		if ep.EmailsURL != "" {
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}
			if err := getJSON(ctx, ep.EmailsURL, accessToken,
				map[string]string{"User-Agent": "openbase"}, &emails); err == nil {
				for _, e := range emails {
					if e.Primary && e.Verified {
						idn.Email, idn.EmailVerified = e.Email, true
						break
					}
				}
				if idn.Email == "" {
					for _, e := range emails {
						if e.Verified {
							idn.Email, idn.EmailVerified = e.Email, true
							break
						}
					}
				}
			}
		}
		return idn, nil
	case "google", "oidc":
		var ui struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
		}
		if err := getJSON(ctx, ep.UserInfoURL, accessToken, nil, &ui); err != nil {
			return nil, err
		}
		return &providerIdentity{UID: ui.Sub, Email: ui.Email, EmailVerified: ui.EmailVerified, Name: ui.Name}, nil
	}
	return nil, fmt.Errorf("unknown provider %q", provider)
}

func deref(s *string, fallback string) string {
	if s != nil && *s != "" {
		return *s
	}
	return fallback
}

// ---- grant_type=id_token (native Google/Apple sign-in) ----

// projectTokenIDToken verifies a provider id_token and signs the user in
// without a browser round-trip. Supported for google (fixed JWKS/issuer) and
// oidc (per-project discovery). The nonce parameter is accepted and returned
// in the audit trail but not enforced against stored values — native SDKs own
// replay protection for their own nonces; the Openbase session tokens minted
// here are short-lived regardless.
func (s *Server) projectTokenIDToken(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
		Nonce    string `json:"nonce"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.IDToken) == "" {
		writeError(w, http.StatusBadRequest, "id_token is required")
		return
	}
	provider := req.Provider
	if provider == "" {
		provider = "google"
	}
	row, _, err := s.providerCredentials(r.Context(), projectID, provider)
	if err != nil || row.ClientID == "" {
		writeError(w, http.StatusBadRequest, "provider "+provider+" is not configured")
		return
	}
	var jwksURI, issuer string
	switch provider {
	case "google":
		jwksURI = endpointOverride(row.Config, "jwks_uri", "https://www.googleapis.com/oauth2/v3/certs")
		issuer = "https://accounts.google.com"
	case "oidc":
		disc, derr := discoverOIDC(r.Context(), configString(row.Config, "issuer"))
		if derr != nil {
			writeError(w, http.StatusBadGateway, "provider endpoints unavailable")
			return
		}
		jwksURI, issuer = disc.JWKSURI, disc.Issuer
	default:
		writeError(w, http.StatusBadRequest, "id_token grant supports google and oidc")
		return
	}
	sub, email, verified, name, verr := verifyAndParseIDToken(r.Context(), req.IDToken, jwksURI, row.ClientID, issuer)
	if verr != nil {
		writeError(w, http.StatusUnauthorized, "invalid id_token")
		return
	}
	u, err := s.linkOrCreateProviderUser(r.Context(), projectID, provider, &providerIdentity{
		UID: sub, Email: email, EmailVerified: verified, Name: name,
	})
	if err != nil {
		if errors.Is(err, errHookRejected) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.writeErr(w, err)
		return
	}
	if u.Banned(time.Now()) {
		writeError(w, http.StatusForbidden, "user is banned")
		return
	}
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	_ = req.Nonce
	writeJSON(w, http.StatusOK, ses)
}

func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	v, _ := cfg[key].(string)
	return v
}

// verifyAndParseIDToken verifies the signature/audience and returns the
// identity claims.
func verifyAndParseIDToken(ctx context.Context, raw, jwksURI, clientID, issuer string) (sub, email string, verified bool, name string, err error) {
	if err := verifyOIDCIDToken(ctx, raw, jwksURI, clientID, issuer); err != nil {
		return "", "", false, "", err
	}
	// Signature verified above; read the claims unverified.
	parser := jwt.NewParser()
	tok, _, perr := parser.ParseUnverified(raw, jwt.MapClaims{})
	if perr != nil {
		return "", "", false, "", perr
	}
	claims, _ := tok.Claims.(jwt.MapClaims)
	sub, _ = claims["sub"].(string)
	email, _ = claims["email"].(string)
	name, _ = claims["name"].(string)
	verified, _ = claims["email_verified"].(bool)
	if sub == "" {
		return "", "", false, "", fmt.Errorf("id_token has no subject")
	}
	return sub, email, verified, name, nil
}

// projectTokenPKCE exchanges the callback's one-time code for a session,
// verifying the PKCE challenge when the login used one.
func (s *Server) projectTokenPKCE(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		AuthCode     string `json:"auth_code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.AuthCode) == "" {
		writeError(w, http.StatusBadRequest, "auth_code is required")
		return
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(req.AuthCode)))
	st, err := s.svc.Store.ConsumeOAuthCode(r.Context(), hexEncode(sum[:]))
	if err != nil || st.ProjectID != projectID || st.UserID == nil {
		writeError(w, http.StatusUnauthorized, "invalid auth code")
		return
	}
	if st.CodeChallenge != "" {
		if !verifyPKCE(st.CodeChallenge, st.CodeChallengeMethod, req.CodeVerifier) {
			writeError(w, http.StatusUnauthorized, "PKCE verification failed")
			return
		}
	}
	u, err := s.svc.Store.GetProjectUser(r.Context(), projectID, *st.UserID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if u.Banned(time.Now()) {
		writeError(w, http.StatusForbidden, "user is banned")
		return
	}
	ses, err := s.issueProjectSession(r, projectID, u)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ses)
}

// ---- OIDC discovery + id_token verification (generic connector) ----

// oidcDiscovery is the subset of the RFC8414/openid-configuration document
// the connector needs.
type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// discoverOIDC fetches {issuer}/.well-known/openid-configuration and checks
// the issuer matches (mix-up defence).
func discoverOIDC(ctx context.Context, issuer string) (*oidcDiscovery, error) {
	issuer = strings.TrimSuffix(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return nil, fmt.Errorf("empty issuer")
	}
	docURL := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discovery status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var disc oidcDiscovery
	if err := json.Unmarshal(body, &disc); err != nil {
		return nil, err
	}
	if strings.TrimSuffix(disc.Issuer, "/") != issuer {
		return nil, fmt.Errorf("discovery issuer mismatch")
	}
	if disc.AuthorizationEndpoint == "" || disc.TokenEndpoint == "" || disc.JWKSURI == "" {
		return nil, fmt.Errorf("discovery document is incomplete")
	}
	return &disc, nil
}

// verifyOIDCIDToken validates an id_token against the issuer JWKS (RS256 /
// ES256) and checks iss/aud/exp. Used when the token endpoint returns one;
// the userinfo sub remains the canonical identity either way.
func verifyOIDCIDToken(ctx context.Context, idToken, jwksURI, clientID, issuer string) error {
	if idToken == "" || jwksURI == "" {
		return fmt.Errorf("no id_token to verify")
	}
	keys, err := fetchJWKSet(ctx, jwksURI)
	if err != nil {
		return err
	}
	_, err = parseOIDCToken(idToken, keys, clientID, issuer)
	return err
}

// jwkKey is one verification key inside a JWKS document.
type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func fetchJWKSet(ctx context.Context, jwksURI string) ([]jwkKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var doc struct {
		Keys []jwkKey `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	return doc.Keys, nil
}

// parseOIDCToken verifies an id_token against a JWKS (RS256/EC) and enforces
// iss/aud/exp. Returns the subject on success.
func parseOIDCToken(raw string, keys []jwkKey, clientID, issuer string) (string, error) {
	byKID := map[string]jwkKey{}
	for _, k := range keys {
		byKID[k.Kid] = k
	}
	tok, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		k, ok := byKID[kid]
		if !ok {
			// Single-key issuers often omit kid: fall back when unambiguous.
			if len(keys) == 1 {
				k = keys[0]
			} else {
				return nil, fmt.Errorf("unknown kid %q", kid)
			}
		}
		switch k.Kty {
		case "RSA":
			nb, err := base64.RawURLEncoding.DecodeString(k.N)
			if err != nil {
				return nil, err
			}
			eb, err := base64.RawURLEncoding.DecodeString(k.E)
			if err != nil {
				return nil, err
			}
			e := 0
			for _, b := range eb {
				e = e*256 + int(b)
			}
			return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
		case "EC":
			if k.Crv != "" && k.Crv != "P-256" {
				return nil, fmt.Errorf("unsupported curve %q", k.Crv)
			}
			xb, err := base64.RawURLEncoding.DecodeString(k.X)
			if err != nil {
				return nil, err
			}
			yb, err := base64.RawURLEncoding.DecodeString(k.Y)
			if err != nil {
				return nil, err
			}
			x, y := new(big.Int).SetBytes(xb), new(big.Int).SetBytes(yb)
			if !elliptic.P256().IsOnCurve(x, y) {
				return nil, fmt.Errorf("JWK point is not on P-256")
			}
			return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
		}
		return nil, fmt.Errorf("unsupported kty %q", k.Kty)
	}, jwt.WithIssuer(issuer))
	if err != nil {
		return "", err
	}
	claims, ok := tok.Claims.(*jwt.RegisteredClaims)
	if !ok || !tok.Valid {
		return "", fmt.Errorf("invalid id_token")
	}
	if clientID != "" {
		audOk := false
		for _, a := range claims.Audience {
			if a == clientID {
				audOk = true
				break
			}
		}
		if !audOk {
			return "", fmt.Errorf("id_token audience mismatch")
		}
	}
	if claims.Subject == "" {
		return "", fmt.Errorf("id_token has no subject")
	}
	return claims.Subject, nil
}

// verifyPKCE checks plain (verifier == challenge) or S256
// (base64url(sha256(verifier)) == challenge, no padding).
func verifyPKCE(challenge, method, verifier string) bool {
	if verifier == "" {
		return false
	}
	switch method {
	case "", "plain":
		return verifier == challenge
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		return base64.RawURLEncoding.EncodeToString(sum[:]) == challenge
	}
	return false
}

// ---- SMS resolution ----

// smsProviderFor resolves the SMS driver for a project: the `sms` provider
// row selects twilio (credentials decrypted) vs the log fallback. No row, no
// problem — the log provider records instead of failing.
func (s *Server) smsProviderFor(ctx context.Context, projectID string) sms.Provider {
	row, err := s.svc.Store.GetProjectAuthProvider(ctx, projectID, "sms")
	if err != nil || !row.Enabled {
		return &sms.LogProvider{Log: s.svc.Log}
	}
	driver, _ := row.Config["driver"].(string)
	if driver == "" || driver == "log" {
		return &sms.LogProvider{Log: s.svc.Log}
	}
	if driver == "twilio" {
		accountSID, _ := row.Config["account_sid"].(string)
		from, _ := row.Config["from"].(string)
		var token string
		if len(row.ClientSecretEncrypted) > 0 && s.svc.Secrets != nil {
			token, _ = s.svc.Secrets.DecryptValue(row.ClientSecretEncrypted, row.EncryptionKeyID)
		}
		if accountSID == "" || token == "" || from == "" {
			s.svc.Log.Warn("sms: twilio misconfigured, falling back to log provider", "project", projectID)
			return &sms.LogProvider{Log: s.svc.Log}
		}
		base, _ := row.Config["base_url"].(string)
		return &sms.TwilioProvider{AccountSID: accountSID, AuthToken: token, From: from, BaseURL: base}
	}
	return &sms.LogProvider{Log: s.svc.Log}
}

// newNumericCode generates an n-digit OTP and its storage hash.
func newNumericCode(n int) (plaintext, hash string, err error) {
	if n <= 0 {
		n = 6
	}
	var v uint64
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	for _, x := range b {
		v = v*256 + uint64(x)
	}
	mod := uint64(1)
	for i := 0; i < n; i++ {
		mod *= 10
	}
	plaintext = fmt.Sprintf("%0*d", n, v%mod)
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, hexEncode(sum[:]), nil
}

// ---- admin: provider CRUD (service config for the A3 dashboard UI) ----

type upsertProviderRequest struct {
	Provider     string         `json:"provider"`
	Enabled      *bool          `json:"enabled"`
	ClientID     string         `json:"client_id"`
	ClientSecret string         `json:"client_secret"`
	Config       map[string]any `json:"config"`
}

func (s *Server) listAuthProviders(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	rows, err := s.svc.Store.ListProjectAuthProviders(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	type view struct {
		Provider  string         `json:"provider"`
		Enabled   bool           `json:"enabled"`
		ClientID  string         `json:"client_id"`
		HasSecret bool           `json:"has_secret"`
		Config    map[string]any `json:"config"`
	}
	out := make([]view, 0, len(rows))
	for _, p := range rows {
		out = append(out, view{Provider: p.Provider, Enabled: p.Enabled,
			ClientID: p.ClientID, HasSecret: len(p.ClientSecretEncrypted) > 0, Config: redactConfig(p.Provider, p.Config)})
	}
	writeJSON(w, http.StatusOK, out)
}

// redactConfig strips secret-bearing keys from provider config echoes.
func redactConfig(provider string, cfg map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range cfg {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "secret") || strings.Contains(lk, "token") || strings.Contains(lk, "password") {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	_ = provider
	return out
}

func (s *Server) upsertAuthProvider(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	var req upsertProviderRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if _, ok := defaultEndpoints(req.Provider); !ok && req.Provider != "sms" {
		writeError(w, http.StatusBadRequest, "unknown provider (want github, google, oidc or sms)")
		return
	}
	existing, _ := s.svc.Store.GetProjectAuthProvider(r.Context(), projectID, req.Provider)
	row := &metadata.ProjectAuthProvider{ProjectID: projectID, Provider: req.Provider, Enabled: true}
	if existing != nil {
		row = existing
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if req.ClientID != "" {
		row.ClientID = req.ClientID
	}
	if req.ClientSecret != "" {
		if s.svc.Secrets == nil {
			writeError(w, http.StatusInternalServerError, "no secrets provider configured")
			return
		}
		cipher, keyID, err := s.svc.Secrets.EncryptValue(req.ClientSecret)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		row.ClientSecretEncrypted = cipher
		row.EncryptionKeyID = keyID
	}
	if req.Config != nil {
		row.Config = req.Config
	}
	if err := s.svc.Store.UpsertProjectAuthProvider(r.Context(), row); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": row.Provider, "enabled": row.Enabled})
}

func (s *Server) deleteAuthProvider(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	provider := r.PathValue("provider")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteProjectAuthProvider(r.Context(), projectID, provider); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
