// Package mfa implements TOTP enrolment and verification (RFC 4226/6238)
// with no external dependencies: HMAC-SHA1, 30 s steps, 6 digits.
//
// Secrets are 20 random bytes (base32, no padding) shared once at enrolment
// via the otpauth:// URI. Verification accepts ±1 step of clock skew.
// Recovery codes are deliberately out of scope here — they belong to the
// credential-lifecycle slice, not the factor itself.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Period is the TOTP time step; Digits the code length; Skew the accepted
// steps on either side of the current one.
const (
	Period = 30
	Digits = 6
	Skew   = 1
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a fresh base32 (no padding) TOTP secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32NoPad.EncodeToString(b), nil
}

// URI builds the otpauth:// URI authenticator apps consume. Account is
// usually the user's email; issuer identifies the project.
func URI(secret, account, issuer string) string {
	label := account
	if issuer != "" {
		label = issuer + ":" + account
	}
	q := url.Values{
		"secret":    {strings.ToUpper(secret)},
		"issuer":    {issuer},
		"algorithm": {"SHA1"},
		"digits":    {"6"},
		"period":    {"30"},
	}
	return "otpauth://totp/" + url.PathEscape(label) + "?" + q.Encode()
}

// code computes the 6-digit code for a secret at a counter value.
func code(secret string, counter uint64) (string, error) {
	key, err := base32NoPad.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("mfa: bad secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", truncated%1000000), nil
}

// CodeAt returns the code valid at time t (test hook + provisioning check).
func CodeAt(secret string, t time.Time) (string, error) {
	return code(secret, uint64(t.Unix()/Period))
}

// Verify reports whether otp is valid for secret at time now (±Skew steps).
// Comparison is exact-match on short numeric strings; timing side-channels
// are immaterial here (6 digits, rate-limited upstream).
func Verify(secret, otp string, now time.Time) bool {
	otp = strings.TrimSpace(otp)
	if len(otp) != Digits {
		return false
	}
	for _, c := range otp {
		if c < '0' || c > '9' {
			return false
		}
	}
	step := now.Unix() / Period
	for d := int64(-Skew); d <= Skew; d++ {
		c, err := code(secret, uint64(int64(step)+d))
		if err != nil {
			return false
		}
		if c == otp {
			return true
		}
	}
	return false
}

// VerifyNow verifies against the current time.
func VerifyNow(secret, otp string) bool { return Verify(secret, otp, time.Now()) }

// Enabled reports whether a factor status counts as enrolled-and-active.
func Enabled(status string) bool { return status == "verified" }

// ValidateSecret rejects malformed secrets at enrol/import time.
func ValidateSecret(secret string) error {
	if secret == "" {
		return errors.New("mfa: secret is required")
	}
	if _, err := base32NoPad.DecodeString(strings.ToUpper(strings.TrimSpace(secret))); err != nil {
		return errors.New("mfa: secret is not valid base32")
	}
	return nil
}
