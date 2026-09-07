// Package auth encapsulates credential hashing and token issuance for the
// platform's own users.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("auth: password must not be empty")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword reports whether plaintext matches the bcrypt hash.
func VerifyPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// TokenManager issues and verifies signed access tokens.
type TokenManager struct {
	secret    []byte
	audience  string
	issuer    string
	ttl       time.Duration
	signMethod jwt.SigningMethod
}

// NewTokenManager builds a manager. secret should be at least 32 bytes.
func NewTokenManager(secret, audience, issuer string, ttl time.Duration) *TokenManager {
	return &TokenManager{
		secret:    []byte(secret),
		audience:  audience,
		issuer:    issuer,
		ttl:       ttl,
		signMethod: jwt.SigningMethodHS256,
	}
}

// Claims identifies the token subject (a platform user).
type Claims struct {
	UserID string `json:"uid"`
	jwt.RegisteredClaims
}

// Issue creates a signed token for userID.
func (m *TokenManager) Issue(userID string) (string, time.Time, error) {
	exp := time.Now().Add(m.ttl)
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Audience:  jwt.ClaimStrings{m.audience},
			Issuer:    m.issuer,
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(m.signMethod, claims)
	signed, err := tok.SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// Parse validates a token and returns its Claims.
func (m *TokenManager) Parse(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("auth: unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithAudience(m.audience), jwt.WithIssuer(m.issuer))
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("auth: invalid token")
	}
	return claims, nil
}

// RandomHex returns n cryptographically random bytes as a hex string.
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Refresh token format: obr_<base64url(32 bytes)>. Only the SHA-256 hash is
// ever stored (sessions.refresh_hash) — like API keys, a leaked database
// never yields usable tokens.
const refreshPrefix = "obr_"

// NewRefreshToken generates an opaque refresh token and its storage hash.
func NewRefreshToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = refreshPrefix + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(sum[:]), nil
}

// HashRefreshToken hashes a presented refresh token for lookup. It validates
// the prefix so malformed input fails before touching the database.
func HashRefreshToken(raw string) (string, error) {
	if !strings.HasPrefix(raw, refreshPrefix) {
		return "", errors.New("auth: invalid refresh token format")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:]), nil
}

// ConstantTimeEqual compares two strings in constant time.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
