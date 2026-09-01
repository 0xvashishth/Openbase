package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e, err := NewEncryptor("test-master-secret-with-enough-entropy")
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	inputs := [][]byte{
		[]byte("postgres://user:pass@host:5432/db"),
		[]byte(""),
		[]byte("short"),
		bytes.Repeat([]byte("x"), 10_000),
		[]byte("unicode: café ☕"),
	}
	for _, in := range inputs {
		ct, err := e.Encrypt(in)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if len(ct) == 0 {
			t.Fatal("ciphertext should not be empty")
		}
		if len(in) > 0 && bytes.Contains(ct, in) {
			t.Fatal("ciphertext must not contain plaintext")
		}
		pt, err := e.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(pt, in) {
			t.Fatalf("round trip mismatch: got %q want %q", pt, in)
		}
	}
}

func TestEncryptProducesUniqueCiphertexts(t *testing.T) {
	e, _ := NewEncryptor("master-key")
	in := []byte("same value")
	c1, _ := e.Encrypt(in)
	c2, _ := e.Encrypt(in)
	if bytes.Equal(c1, c2) {
		t.Fatal("ciphertexts should differ due to random nonce/key per encryption")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	e1, _ := NewEncryptor("master-key-one")
	e2, _ := NewEncryptor("master-key-two")
	ct, err := e1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e2.Decrypt(ct); err == nil {
		t.Fatal("decrypt with wrong master key should fail")
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	e, _ := NewEncryptor("master-key")
	ct, err := e.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the ciphertext region (past header) and expect failure.
	ct[len(ct)-1] ^= 0xff
	if _, err := e.Decrypt(ct); err == nil {
		t.Fatal("tampered ciphertext should fail to decrypt")
	}
}

func TestNewEncryptorRejectsEmptySecret(t *testing.T) {
	if _, err := NewEncryptor(""); err == nil {
		t.Fatal("empty master secret should error")
	}
}

func TestTruncatedCiphertextFails(t *testing.T) {
	e, _ := NewEncryptor("master-key")
	ct, _ := e.Encrypt([]byte("secret"))
	for _, cut := range []int{0, 1, 4} {
		if _, err := e.Decrypt(ct[:cut]); err == nil {
			t.Fatalf("truncated ciphertext (cut=%d) should fail", cut)
		}
	}
}