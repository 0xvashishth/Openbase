package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// KeyProvider wraps and unwraps a per-value Data Encryption Key (DEK) with a
// master key material. The concrete implementation decides where the master
// key lives: in-process (LocalKey) or a remote KMS/Vault signer (RemoteKey).
//
// This is the extension point for the Phase 6 hardening: a real
// HashiCorp Vault / cloud-KMS integration simply needs to provide Wrap/Unwrap
// and register itself in a MasterKeyProvider (see ARCHITECTURE.md §2.6).
type KeyProvider interface {
	// ID is the stable identifier recorded on every encrypted row
	// (encryption_key_id) so the correct key can be selected at decrypt time.
	ID() string
	// Wrap encrypts the per-value DEK with this key's master material.
	Wrap(dek []byte) ([]byte, error)
	// Unwrap recovers the per-value DEK from a wrapped value.
	Unwrap(wrapped []byte) ([]byte, error)
}

// LocalKey is an in-process AES-256-GCM key derived from an operator-provided
// secret. Used by default; the equivalent of the V1 envelope-encryption key.
type LocalKey struct {
	id     string
	master []byte
}

// NewLocalKey derives an in-process master key from secret. The secret should
// carry at least 32 bytes of entropy; it is hashed to a fixed AES-256 key.
func NewLocalKey(id, secret string) (*LocalKey, error) {
	if id == "" {
		return nil, errors.New("crypto: empty key id")
	}
	if secret == "" {
		return nil, errors.New("crypto: empty master secret")
	}
	sum := sha256.Sum256([]byte(secret))
	return &LocalKey{id: id, master: sum[:]}, nil
}

// ID implements KeyProvider.
func (k *LocalKey) ID() string { return k.id }

// Wrap implements KeyProvider.
func (k *LocalKey) Wrap(dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.master)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, dek, nil), nil
}

// Unwrap implements KeyProvider.
func (k *LocalKey) Unwrap(wrapped []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.master)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(wrapped) < nonceSize {
		return nil, errors.New("crypto: truncated wrapped key")
	}
	nonce := wrapped[:nonceSize]
	ct := wrapped[nonceSize:]
	return aead.Open(nil, nonce, ct, nil)
}

// ErrUnknownKey is returned by MasterKeyProvider when a row was encrypted with
// a key id that is not loaded. Mirrors the honest-capability rule: we never
// pretend to decrypt a row whose key material we do not actually hold.
var ErrUnknownKey = errors.New("crypto: no key material for encryption_key_id")

// MasterKeyProvider is a named registry of KeyProviders supporting key
// rotation. New writes always use the current key; reads select the key by the
// row's recorded encryption_key_id, so old rows stay readable as long as their
// key is still loaded.
type MasterKeyProvider struct {
	current *LocalKey
	keys    map[string]*LocalKey
}

// NewMasterKeyProvider builds a registry with a single current key. Pass
// additional historical keys via Add to enable decryption of rows written
// under a rotated key.
func NewMasterKeyProvider(current *LocalKey) *MasterKeyProvider {
	return &MasterKeyProvider{
		current: current,
		keys:    map[string]*LocalKey{current.id: current},
	}
}

// Add registers a historical key that may be used for decryption. It is an
// error to register two keys with the same id.
func (m *MasterKeyProvider) Add(k *LocalKey) error {
	if _, exists := m.keys[k.id]; exists {
		return fmt.Errorf("crypto: duplicate key id %q", k.id)
	}
	m.keys[k.id] = k
	return nil
}

// Current returns the key used for new writes.
func (m *MasterKeyProvider) Current() KeyProvider { return m.current }

// Rotate adds a new current key for writes while keeping any previously loaded
// keys available for decrypting older rows. Returns an error if newKey's id is
// already registered.
func (m *MasterKeyProvider) Rotate(newKey *LocalKey) error {
	if err := m.Add(newKey); err != nil {
		return err
	}
	m.current = newKey
	return nil
}

// Key returns the provider for a given id, or ErrUnknownKey if not loaded.
func (m *MasterKeyProvider) Key(id string) (KeyProvider, error) {
	k, ok := m.keys[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKey, id)
	}
	return k, nil
}
