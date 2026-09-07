// Package secrets encrypts and decrypts connection credentials at rest using
// envelope encryption (SCHEMA.md §2). It implements server.SecretsProvider.
package secrets

import (
	"errors"

	"github.com/openbase/openbase/internal/crypto"
	"github.com/openbase/openbase/internal/metadata"
)

// Provider encrypts/decrypts the credential fields of a metadata.Connection.
// It routes key management through a crypto.MasterKeyProvider so that rows
// encrypted under a previously-rotated key remain decryptable (the recorded
// encryption_key_id selects the correct key material).
type Provider struct {
	master *crypto.MasterKeyProvider
}

// New returns a Provider backed by a single local master key. keyID names that
// key and is stamped on every row so it can later be rotated following the
// §2.6 design (add a new current key and keep the old one for decryption).
func New(masterSecret, keyID string) (*Provider, error) {
	key, err := crypto.NewLocalKey(keyID, masterSecret)
	if err != nil {
		return nil, err
	}
	return &Provider{master: crypto.NewMasterKeyProvider(key)}, nil
}

// WithMasterKeys returns a Provider over an explicit key registry, letting the
// caller control rotation (current key for writes, loaded keys for reads).
func WithMasterKeys(master *crypto.MasterKeyProvider) *Provider {
	return &Provider{master: master}
}

// NewRotating builds a rotation-aware Provider. currentSecret/currentID name the
// key used for new writes; historical maps previously-used key ids to their
// secrets so rows encrypted under them remain decryptable. Repeated ids in
// historical are an error.
func NewRotating(currentSecret, currentID string, historical map[string]string) (*Provider, error) {
	current, err := crypto.NewLocalKey(currentID, currentSecret)
	if err != nil {
		return nil, err
	}
	reg := crypto.NewMasterKeyProvider(current)
	for id, secret := range historical {
		k, err := crypto.NewLocalKey(id, secret)
		if err != nil {
			return nil, err
		}
		if err := reg.Add(k); err != nil {
			return nil, err
		}
	}
	return &Provider{master: reg}, nil
}

// EncryptConnection encrypts the connection string, username and password into
// the connection's BYTEA fields. It never touches fields on error.
func (p *Provider) EncryptConnection(c *metadata.Connection, secret metadata.ConnectionSecret) error {
	enc, err := crypto.NewKeyedEncryptor(p.master.Current())
	if err != nil {
		return err
	}
	cs, err := enc.Encrypt([]byte(secret.ConnString))
	if err != nil {
		return err
	}
	us, err := enc.Encrypt([]byte(secret.Username))
	if err != nil {
		return err
	}
	ps, err := enc.Encrypt([]byte(secret.Password))
	if err != nil {
		return err
	}
	c.EncryptedConnString = cs
	c.EncryptedUsername = us
	c.EncryptedPassword = ps
	c.EncryptionKeyID = p.master.Current().ID()
	return nil
}

// DecryptConnection returns the plaintext credentials of a connection. It
// selects the key by the row's recorded encryption_key_id, so rotation keeps
// old rows readable; it fails if no key material for that id is loaded.
func (p *Provider) DecryptConnection(c *metadata.Connection) (metadata.ConnectionSecret, error) {
	if len(c.EncryptedConnString) == 0 {
		return metadata.ConnectionSecret{}, errors.New("secrets: no encrypted connection string stored")
	}
	key, err := p.master.Key(c.EncryptionKeyID)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	enc, err := crypto.NewKeyedEncryptor(key)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	cs, err := enc.Decrypt(c.EncryptedConnString)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	us, err := enc.Decrypt(c.EncryptedUsername)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	ps, err := enc.Decrypt(c.EncryptedPassword)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	return metadata.ConnectionSecret{
		ConnString: string(cs),
		Username:   string(us),
		Password:   string(ps),
	}, nil
}
// EncryptValue envelope-encrypts an arbitrary string (e.g. an SMTP password)
// under the current master key, returning the ciphertext and the key id to
// store alongside it for rotation-aware decryption.
func (p *Provider) EncryptValue(plaintext string) (ciphertext []byte, keyID string, err error) {
	enc, err := crypto.NewKeyedEncryptor(p.master.Current())
	if err != nil {
		return nil, "", err
	}
	ct, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return nil, "", err
	}
	return ct, p.master.Current().ID(), nil
}

// DecryptValue reverses EncryptValue, selecting key material by the stored
// key id so rotated rows stay readable.
func (p *Provider) DecryptValue(ciphertext []byte, keyID string) (string, error) {
	key, err := p.master.Key(keyID)
	if err != nil {
		return "", err
	}
	enc, err := crypto.NewKeyedEncryptor(key)
	if err != nil {
		return "", err
	}
	pt, err := enc.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
