// Package crypto handles at-rest encryption for connection secrets using
// envelope encryption (SCHEMA.md §2). A master key (data encryption key) is
// provided by the operator; each value is encrypted with a random per-value
// key wrapped by the master key, so key rotation doesn't require re-encrypting
// every row (the wrapped key is stored with the ciphertext).
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

// Encryptor performs envelope encryption with a 32-byte master key.
type Encryptor struct {
	master []byte
}

// NewEncryptor derives a master key from the operator-provided secret. The
// secret should have at least 32 bytes of entropy; it is hashed to a fixed
// AES-256 key.
func NewEncryptor(masterSecret string) (*Encryptor, error) {
	if masterSecret == "" {
		return nil, errors.New("crypto: empty master secret")
	}
	sum := sha256.Sum256([]byte(masterSecret))
	return &Encryptor{master: sum[:]}, nil
}

// Encrypt envelope-encrypts plaintext. Output layout:
//
//	1 byte  : key length + 1 (for the version byte)
//	1 byte  : version (0x01)
//	<len>   : wrapped data key (nonce + encrypted DEK) [aes-gcm]
//	<nonce> : nonce for the ciphertext [aes-gcm]
//	<ct>    : ciphertext
//
// Simpler: wraps the per-value DEK with the master and encrypts the value under
// the per-value DEK.
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		// Represent empty value explicitly so null vs empty roundtrips.
		plaintext = []byte{}
	}

	// Per-value data key.
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}

	// Wrap the DEK with the master key.
	wrapped, err := e.wrap(dek)
	if err != nil {
		return nil, err
	}

	// Encrypt plaintext under the DEK.
	block, err := aes.NewCipher(dek)
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
	ct := aead.Seal(nil, nonce, plaintext, nil)

	// Layout: [len(wrapped)=1][wrapped][nonce][ct]
	out := make([]byte, 0, 1+len(wrapped)+len(nonce)+len(ct))
	out = append(out, byte(len(wrapped)))
	out = append(out, wrapped...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Decrypt reverses Encrypt.
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, errors.New("crypto: invalid ciphertext")
	}
	kw := int(data[0])
	if len(data) < 1+kw {
		return nil, errors.New("crypto: truncated ciphertext")
	}
	wrapped := data[1 : 1+kw]
	rest := data[1+kw:]

	dek, err := e.unwrap(wrapped)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aead.NonceSize()
	if len(rest) < nonceSize {
		return nil, errors.New("crypto: truncated ciphertext")
	}
	nonce := rest[:nonceSize]
	ct := rest[nonceSize:]
	pt, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return pt, nil
}

func (e *Encryptor) wrap(dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.master)
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

func (e *Encryptor) unwrap(wrapped []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.master)
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