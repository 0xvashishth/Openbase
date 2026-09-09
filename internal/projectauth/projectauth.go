// Package projectauth issues and verifies end-user (per-project) tokens.
//
// This is the Phase 10 (A0.4) counterpart to internal/auth, which only knows
// platform operators. End-user access tokens are short-lived (15 min) JWTs
// signed per-project with ES256 — never with the shared HS256 operator secret
// — so third parties can verify them against the project's public JWKS
// (GET /v1/projects/{id}/.well-known/jwks.json) without ever seeing a secret.
//
// Key custody: this package holds keys in memory. Persistence and
// envelope-encryption at rest live in the metadata store (project_signing_keys,
// migration 0008); see KeySet.Export/Import for the row encoding.
package projectauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Authenticator assurance levels, mirrored in token claims.
const (
	AAL1 = "aal1"
	AAL2 = "aal2"
)

// Roles carried in end-user tokens.
const (
	RoleAuthenticated = "authenticated"
	RoleAnonymous     = "anon"
)

// AccessTTL is the default end-user access-token lifetime.
const AccessTTL = 15 * time.Minute

// Claims is the end-user access-token payload.
type Claims struct {
	UserID    string         `json:"uid"`
	ProjectID string         `json:"pid"`
	Role      string         `json:"role"`
	AAL       string         `json:"aal"`
	// Custom carries hook-injected claims (before-token-issued). Verified
	// like any other claim; consumers must namespace their keys.
	Custom    map[string]any `json:"custom,omitempty"`
	jwt.RegisteredClaims
}

// IssuerFor returns the expected iss claim for a project.
func IssuerFor(baseURL, projectID string) string {
	if baseURL == "" {
		return "openbase/" + projectID
	}
	return baseURL + "/auth/v1/" + projectID
}

// JWK is the public JSON Web Key encoding of a P-256 key.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// GenerateKey creates a fresh P-256 signing key with a random kid.
func GenerateKey() (*ecdsa.PrivateKey, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	kid, err := randomKID()
	if err != nil {
		return nil, "", err
	}
	return key, kid, nil
}

func randomKID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "kid_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// PublicJWK exports the public half of key in JWK form.
func PublicJWK(key *ecdsa.PrivateKey, kid string) JWK {
	byteLen := (key.Curve.Params().BitSize + 7) / 8
	xb := key.PublicKey.X.FillBytes(make([]byte, byteLen))
	yb := key.PublicKey.Y.FillBytes(make([]byte, byteLen))
	return JWK{Kty: "EC", Crv: "P-256", Kid: kid, X: b64(xb), Y: b64(yb), Alg: "ES256", Use: "sig"}
}

// ParsePublicJWK rebuilds a P-256 public key from its JWK encoding.
func ParsePublicJWK(j JWK) (*ecdsa.PublicKey, error) {
	if j.Kty != "EC" || j.Crv != "P-256" {
		return nil, fmt.Errorf("projectauth: unsupported JWK %q/%q (want EC/P-256)", j.Kty, j.Crv)
	}
	xb, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil {
		return nil, fmt.Errorf("projectauth: bad JWK x: %w", err)
	}
	yb, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil {
		return nil, fmt.Errorf("projectauth: bad JWK y: %w", err)
	}
	x, y := new(big.Int).SetBytes(xb), new(big.Int).SetBytes(yb)
	if !elliptic.P256().IsOnCurve(x, y) {
		return nil, errors.New("projectauth: JWK point is not on P-256")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

// UnverifiedProjectID extracts the pid claim without verifying the
// signature. Callers must treat it as a routing hint only and pass the token
// to (KeySet).Verify or (Manager).VerifyFor before trusting anything.
func UnverifiedProjectID(tokenStr string) (string, error) {
	parser := jwt.NewParser()
	tok, _, err := parser.ParseUnverified(tokenStr, &Claims{})
	if err != nil {
		return "", err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok {
		return "", errors.New("projectauth: invalid claims")
	}
	return claims.ProjectID, nil
}

// KeySet is one project's signing keys: the active private key plus every
// public key still honored for verification (rotation history).
type KeySet struct {
	projectID string
	issuer    string
	activeKID string
	private   map[string]*ecdsa.PrivateKey // kid -> key (usually one entry)
	public    map[string]*ecdsa.PublicKey  // kid -> key (active + retired)
}

// NewKeySet generates a project's first signing key.
func NewKeySet(projectID, baseURL string) (*KeySet, error) {
	key, kid, err := GenerateKey()
	if err != nil {
		return nil, err
	}
	return &KeySet{
		projectID: projectID,
		issuer:    IssuerFor(baseURL, projectID),
		activeKID: kid,
		private:   map[string]*ecdsa.PrivateKey{kid: key},
		public:    map[string]*ecdsa.PublicKey{kid: &key.PublicKey},
	}, nil
}

// ActiveKID reports the kid new tokens are signed with.
func (k *KeySet) ActiveKID() string { return k.activeKID }

// Issuer reports the iss claim this set mints and expects.
func (k *KeySet) Issuer() string { return k.issuer }

// ActiveJWK exports the active public key for the JWKS endpoint.
func (k *KeySet) ActiveJWK() JWK {
	return PublicJWK(k.private[k.activeKID], k.activeKID)
}

// JWKS exports every verification key (active + retired) for the endpoint.
func (k *KeySet) JWKS() []JWK {
	out := make([]JWK, 0, len(k.public))
	for kid := range k.public {
		key := k.private[kid]
		var pub *ecdsa.PublicKey
		if key != nil {
			pub = &key.PublicKey
		} else {
			pub = k.public[kid]
		}
		xb := pub.X.FillBytes(make([]byte, 32))
		yb := pub.Y.FillBytes(make([]byte, 32))
		out = append(out, JWK{Kty: "EC", Crv: "P-256", Kid: kid, X: b64(xb), Y: b64(yb), Alg: "ES256", Use: "sig"})
	}
	return out
}

// Rotate generates a new active key. Tokens signed with retired keys still
// verify until Forget is called for their kid.
func (k *KeySet) Rotate() (JWK, error) {
	key, kid, err := GenerateKey()
	if err != nil {
		return JWK{}, err
	}
	k.private[kid] = key
	k.public[kid] = &key.PublicKey
	k.activeKID = kid
	return PublicJWK(key, kid), nil
}

// Forget drops a retired kid. Forgetting the active kid is refused.
func (k *KeySet) Forget(kid string) error {
	if kid == k.activeKID {
		return errors.New("projectauth: cannot forget the active key")
	}
	delete(k.private, kid)
	delete(k.public, kid)
	return nil
}

// ImportPublic adds a verification-only key (e.g. loaded from
// project_signing_keys rows whose private half decrypts elsewhere).
func (k *KeySet) ImportPublic(j JWK) error {
	pub, err := ParsePublicJWK(j)
	if err != nil {
		return err
	}
	k.public[j.Kid] = pub
	return nil
}

// Issue mints a short-lived end-user access token.
func (k *KeySet) Issue(userID, role, aal string, ttl time.Duration) (string, time.Time, error) {
	return k.IssueCustom(userID, role, aal, ttl, nil)
}

// IssueCustom mints like Issue with additional hook-provided claims.
func (k *KeySet) IssueCustom(userID, role, aal string, ttl time.Duration, custom map[string]any) (string, time.Time, error) {
	if userID == "" {
		return "", time.Time{}, errors.New("projectauth: userID is required")
	}
	if role == "" {
		role = RoleAuthenticated
	}
	if aal == "" {
		aal = AAL1
	}
	if ttl <= 0 {
		ttl = AccessTTL
	}
	priv := k.private[k.activeKID]
	if priv == nil {
		return "", time.Time{}, errors.New("projectauth: no active private key")
	}
	exp := time.Now().Add(ttl)
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, Claims{
		UserID:    userID,
		ProjectID: k.projectID,
		Role:      role,
		AAL:       aal,
		Custom:    custom,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    k.issuer,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	tok.Header["kid"] = k.activeKID
	signed, err := tok.SignedString(priv)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// Verify checks signature (any known kid), expiry, issuer and project binding.
func (k *KeySet) Verify(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, errors.New("projectauth: unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("projectauth: missing kid")
		}
		pub, ok := k.public[kid]
		if !ok {
			return nil, fmt.Errorf("projectauth: unknown kid %q", kid)
		}
		return pub, nil
	}, jwt.WithIssuer(k.issuer))
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("projectauth: invalid token")
	}
	if claims.ProjectID != k.projectID {
		return nil, errors.New("projectauth: token is for a different project")
	}
	return claims, nil
}
