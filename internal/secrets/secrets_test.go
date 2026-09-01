package secrets

import (
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

func TestEncryptDecryptConnection(t *testing.T) {
	p, err := New("master-secret-with-entropy", "key-v1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	c := &metadata.Connection{}
	secret := metadata.ConnectionSecret{
		ConnString: "postgres://user:pass@host:5432/db?sslmode=require",
	}
	if err := p.EncryptConnection(c, secret); err != nil {
		t.Fatalf("EncryptConnection: %v", err)
	}
	if c.EncryptionKeyID != "key-v1" {
		t.Fatalf("key id = %q", c.EncryptionKeyID)
	}
	if len(c.EncryptedConnString) == 0 {
		t.Fatal("connection string should be encrypted")
	}

	got, err := p.DecryptConnection(c)
	if err != nil {
		t.Fatalf("DecryptConnection: %v", err)
	}
	if got.ConnString != secret.ConnString {
		t.Fatalf("round trip mismatch: %q vs %q", got.ConnString, secret.ConnString)
	}
}

func TestDecryptWithNoCiphertextFails(t *testing.T) {
	p, _ := New("master-secret-with-entropy", "key-v1")
	if _, err := p.DecryptConnection(&metadata.Connection{}); err == nil {
		t.Fatal("decrypt with no ciphertext should fail")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	p1, _ := New("master-secret-one-entropy", "key-v1")
	p2, _ := New("master-secret-two-entropy", "key-v1")
	c := &metadata.Connection{}
	_ = p1.EncryptConnection(c, metadata.ConnectionSecret{ConnString: "postgres://x"})
	if _, err := p2.DecryptConnection(c); err == nil {
		t.Fatal("decrypt with wrong key should fail")
	}
}

func TestNewRejectsEmptyMaster(t *testing.T) {
	if _, err := New("", "key-v1"); err == nil {
		t.Fatal("empty master secret should error")
	}
}