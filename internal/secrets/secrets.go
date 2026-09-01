// Package secrets encrypts and decrypts connection credentials at rest using
// envelope encryption (SCHEMA.md §2). It implements server.SecretsProvider.
package secrets

import (
	"errors"

	"github.com/openbase/openbase/internal/crypto"
	"github.com/openbase/openbase/internal/metadata"
)

// Provider encrypts/decrypts the credential fields of a metadata.Connection.
type Provider struct {
	enc      *crypto.Encryptor
	keyID    string
}

// New returns a Provider bound to the operator-provided master secret. keyID
// records which key encrypted a row so keys can be rotated later (§2).
func New(masterSecret, keyID string) (*Provider, error) {
	enc, err := crypto.NewEncryptor(masterSecret)
	if err != nil {
		return nil, err
	}
	return &Provider{enc: enc, keyID: keyID}, nil
}

// EncryptConnection encrypts the connection string, username and password into
// the connection's BYTEA fields. It never touches fields on error.
func (p *Provider) EncryptConnection(c *metadata.Connection, secret metadata.ConnectionSecret) error {
	cs, err := p.enc.Encrypt([]byte(secret.ConnString))
	if err != nil {
		return err
	}
	us, err := p.enc.Encrypt([]byte(secret.Username))
	if err != nil {
		return err
	}
	ps, err := p.enc.Encrypt([]byte(secret.Password))
	if err != nil {
		return err
	}
	c.EncryptedConnString = cs
	c.EncryptedUsername = us
	c.EncryptedPassword = ps
	c.EncryptionKeyID = p.keyID
	return nil
}

// DecryptConnection returns the plaintext credentials of a connection. It
// fails if the ciphertext is empty or the master key has changed.
func (p *Provider) DecryptConnection(c *metadata.Connection) (metadata.ConnectionSecret, error) {
	if len(c.EncryptedConnString) == 0 {
		return metadata.ConnectionSecret{}, errors.New("secrets: no encrypted connection string stored")
	}
	cs, err := p.enc.Decrypt(c.EncryptedConnString)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	us, err := p.enc.Decrypt(c.EncryptedUsername)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	ps, err := p.enc.Decrypt(c.EncryptedPassword)
	if err != nil {
		return metadata.ConnectionSecret{}, err
	}
	return metadata.ConnectionSecret{
		ConnString: string(cs),
		Username:   string(us),
		Password:   string(ps),
	}, nil
}