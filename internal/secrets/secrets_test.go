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

func TestRotatingDecryptsOldRows(t *testing.T) {
	// Encrypt a row under the current key, then rotate and confirm the old row
	// stays decryptable while new rows go under the new key.
	p1, err := New("old-secret-with-entropy", "key-v1")
	if err != nil {
		t.Fatal(err)
	}
	old := &metadata.Connection{}
	if err := p1.EncryptConnection(old, metadata.ConnectionSecret{ConnString: "postgres://old"}); err != nil {
		t.Fatal(err)
	}
	if old.EncryptionKeyID != "key-v1" {
		t.Fatalf("old row key id = %q", old.EncryptionKeyID)
	}

	// Rotate: key-v2 is current; key-v1 supplied as a historical key.
	p2, err := NewRotating("new-secret-with-entropy", "key-v2", map[string]string{"key-v1": "old-secret-with-entropy"})
	if err != nil {
		t.Fatal(err)
	}
	// New writes stamp the current key id.
	fresh := &metadata.Connection{}
	if err := p2.EncryptConnection(fresh, metadata.ConnectionSecret{ConnString: "postgres://new"}); err != nil {
		t.Fatal(err)
	}
	if fresh.EncryptionKeyID != "key-v2" {
		t.Fatalf("fresh row key id = %q", fresh.EncryptionKeyID)
	}

	// The old row still decrypts.
	got, err := p2.DecryptConnection(old)
	if err != nil {
		t.Fatalf("old row should decrypt after rotation: %v", err)
	}
	if got.ConnString != "postgres://old" {
		t.Fatalf("old row conn string = %q", got.ConnString)
	}
	// The new row decrypts too.
	got2, err := p2.DecryptConnection(fresh)
	if err != nil {
		t.Fatalf("fresh row decrypt: %v", err)
	}
	if got2.ConnString != "postgres://new" {
		t.Fatalf("fresh row conn string = %q", got2.ConnString)
	}
}

func TestRotatingUnknownKeyFails(t *testing.T) {
	p, _ := NewRotating("current-secret-entropy", "key-v2", nil)
	// A row whose key we do not hold must fail to decrypt (honest, no fake key).
	row := &metadata.Connection{EncryptionKeyID: "key-old-unavailable"}
	if _, err := p.DecryptConnection(row); err == nil {
		t.Fatal("decrypt with unavailable key should error")
	}
}