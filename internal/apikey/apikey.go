// Package apikey manages per-project API keys (SCHEMA.md `api_keys` table).
// Only hashes are stored; the plaintext key is shown once at creation.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	RawLen   = 32 // random bytes in the raw key
	prefix   = "ob_"
)

// New generates a key in the form ob_<base64url(32 bytes)> and returns the
// plaintext and its SHA-256 hex hash.
func New() (plaintext, hash string, err error) {
	b := make([]byte, RawLen)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plaintext, Hash(plaintext), nil
}

// Hash returns the hex SHA-256 of a key, the form stored in the DB.
func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Normalize trims whitespace and validates the key prefix, returning a
// normalized key for hashing.
func Normalize(key string) (string, error) {
	k := strings.TrimSpace(key)
	if !strings.HasPrefix(k, prefix) {
		return "", errors.New("apikey: invalid key format")
	}
	return k, nil
}