package projectauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/openbase/openbase/internal/metadata"
)

// KeySecrets encrypts private key material at rest. It mirrors the
// EncryptValue/DecryptValue half of the server's SecretsProvider without
// importing the server package (which imports this one).
type KeySecrets interface {
	EncryptValue(plaintext string) (ciphertext []byte, keyID string, err error)
	DecryptValue(ciphertext []byte, keyID string) (string, error)
}

// KeyStore is the metadata subset the Manager needs.
type KeyStore interface {
	ListProjectSigningKeys(ctx context.Context, projectID string) ([]metadata.ProjectSigningKey, error)
	CreateProjectSigningKey(ctx context.Context, k *metadata.ProjectSigningKey) error
	MarkProjectSigningKeyRotated(ctx context.Context, projectID, keyID string) error
}

// Manager loads (or bootstraps) per-project signing keys and mints/verifies
// end-user tokens. Keys are cached in memory per project; rotation and
// deletion invalidate the entry.
type Manager struct {
	store   KeyStore
	secrets KeySecrets
	baseURL string
}

// NewManager builds a Manager. secrets may be nil where only verification is
// needed (issuing requires decryption of the active private key).
func NewManager(store KeyStore, secrets KeySecrets, baseURL string) *Manager {
	return &Manager{store: store, secrets: secrets, baseURL: baseURL}
}

// Map converts a JWK to its storage encoding.
func (j JWK) Map() map[string]any {
	return map[string]any{
		"kty": j.Kty, "crv": j.Crv, "kid": j.Kid,
		"x": j.X, "y": j.Y, "alg": j.Alg, "use": j.Use,
	}
}

// JWKFromMap parses a stored JWK row back.
func JWKFromMap(m map[string]any) (JWK, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return JWK{}, err
	}
	var j JWK
	if err := json.Unmarshal(raw, &j); err != nil {
		return JWK{}, err
	}
	if j.Kid == "" || j.X == "" || j.Y == "" {
		return JWK{}, errors.New("projectauth: stored JWK is incomplete")
	}
	return j, nil
}

func encodePrivate(key *ecdsa.PrivateKey) (string, error) {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(der), nil
}

func decodePrivate(enc string) (*ecdsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(der)
}

// EnsureKeySet returns the project's KeySet, generating and persisting the
// first key when the project has none. The newest row is the active signer.
func (m *Manager) EnsureKeySet(ctx context.Context, projectID string) (*KeySet, error) {
	if projectID == "" {
		return nil, errors.New("projectauth: projectID is required")
	}
	rows, err := m.store.ListProjectSigningKeys(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return m.bootstrap(ctx, projectID)
	}
	ks := &KeySet{
		projectID: projectID,
		issuer:    IssuerFor(m.baseURL, projectID),
		private:   map[string]*ecdsa.PrivateKey{},
		public:    map[string]*ecdsa.PublicKey{},
	}
	// Rows arrive newest-first; the first row is the active signer.
	for i, row := range rows {
		jwk, err := JWKFromMap(row.PublicJWK)
		if err != nil {
			return nil, err
		}
		pub, err := ParsePublicJWK(jwk)
		if err != nil {
			return nil, err
		}
		ks.public[row.KID] = pub
		if i == 0 {
			if m.secrets == nil {
				return nil, errors.New("projectauth: no secrets provider for issuing")
			}
			plain, err := m.secrets.DecryptValue(row.PrivateEncrypted, row.EncryptionKeyID)
			if err != nil {
				return nil, err
			}
			priv, err := decodePrivate(plain)
			if err != nil {
				return nil, err
			}
			ks.private[row.KID] = priv
			ks.activeKID = row.KID
		}
	}
	if ks.activeKID == "" {
		return nil, errors.New("projectauth: no active key")
	}
	return ks, nil
}

func (m *Manager) bootstrap(ctx context.Context, projectID string) (*KeySet, error) {
	if m.secrets == nil {
		return nil, errors.New("projectauth: no secrets provider for issuing")
	}
	ks, err := NewKeySet(projectID, m.baseURL)
	if err != nil {
		return nil, err
	}
	enc, err := encodePrivate(ks.private[ks.activeKID])
	if err != nil {
		return nil, err
	}
	cipher, keyID, err := m.secrets.EncryptValue(enc)
	if err != nil {
		return nil, err
	}
	if err := m.store.CreateProjectSigningKey(ctx, &metadata.ProjectSigningKey{
		ProjectID:        projectID,
		KID:              ks.activeKID,
		Alg:              "ES256",
		PublicJWK:        ks.ActiveJWK().Map(),
		PrivateEncrypted: cipher,
		EncryptionKeyID:  keyID,
	}); err != nil {
		return nil, err
	}
	return ks, nil
}

// IssueFor mints an end-user access token for a project user.
func (m *Manager) IssueFor(ctx context.Context, projectID, userID, role, aal string, ttl time.Duration) (string, time.Time, error) {
	ks, err := m.EnsureKeySet(ctx, projectID)
	if err != nil {
		return "", time.Time{}, err
	}
	return ks.Issue(userID, role, aal, ttl)
}

// VerifyFor verifies a token against a project's stored public keys. It needs
// no secrets provider — verification is public-key only.
func (m *Manager) VerifyFor(ctx context.Context, projectID, token string) (*Claims, error) {
	rows, err := m.store.ListProjectSigningKeys(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("projectauth: project has no signing keys")
	}
	ks := &KeySet{
		projectID: projectID,
		issuer:    IssuerFor(m.baseURL, projectID),
		private:   map[string]*ecdsa.PrivateKey{},
		public:    map[string]*ecdsa.PublicKey{},
	}
	for _, row := range rows {
		jwk, err := JWKFromMap(row.PublicJWK)
		if err != nil {
			return nil, err
		}
		pub, err := ParsePublicJWK(jwk)
		if err != nil {
			return nil, err
		}
		ks.public[row.KID] = pub
	}
	return ks.Verify(token)
}

// PublicJWKS returns the project's verification keys for the JWKS endpoint.
func (m *Manager) PublicJWKS(ctx context.Context, projectID string) ([]JWK, error) {
	rows, err := m.store.ListProjectSigningKeys(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]JWK, 0, len(rows))
	for _, row := range rows {
		jwk, err := JWKFromMap(row.PublicJWK)
		if err != nil {
			return nil, err
		}
		out = append(out, jwk)
	}
	return out, nil
}

// RotateFor generates a new active key, marks the previous rows rotated, and
// returns the new public JWK. Previously issued tokens keep verifying until
// their rows are deleted.
func (m *Manager) RotateFor(ctx context.Context, projectID string) (JWK, error) {
	if m.secrets == nil {
		return JWK{}, errors.New("projectauth: no secrets provider for issuing")
	}
	ks, err := m.EnsureKeySet(ctx, projectID)
	if err != nil {
		return JWK{}, err
	}
	jwk, err := ks.Rotate()
	if err != nil {
		return JWK{}, err
	}
	enc, err := encodePrivate(ks.private[ks.activeKID])
	if err != nil {
		return JWK{}, err
	}
	cipher, keyID, err := m.secrets.EncryptValue(enc)
	if err != nil {
		return JWK{}, err
	}
	if err := m.store.CreateProjectSigningKey(ctx, &metadata.ProjectSigningKey{
		ProjectID:        projectID,
		KID:              ks.activeKID,
		Alg:              "ES256",
		PublicJWK:        jwk.Map(),
		PrivateEncrypted: cipher,
		EncryptionKeyID:  keyID,
	}); err != nil {
		return JWK{}, err
	}
	// Best-effort: stamp the superseded rows rotated (they stay verifiable
	// until deleted; a stamp failure must not fail the rotation itself).
	if rows, err := m.store.ListProjectSigningKeys(ctx, projectID); err == nil {
		for _, row := range rows {
			if row.KID != ks.activeKID {
				_ = m.store.MarkProjectSigningKeyRotated(ctx, projectID, row.ID)
			}
		}
	}
	return jwk, nil
}
