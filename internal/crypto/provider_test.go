package crypto

import (
	"bytes"
	"errors"
	"testing"
)

func TestLocalKeyWrapUnwrapRoundTrip(t *testing.T) {
	k, err := NewLocalKey("k1", "master-secret-with-entropy")
	if err != nil {
		t.Fatal(err)
	}
	if k.ID() != "k1" {
		t.Fatalf("id = %q", k.ID())
	}
	dek := []byte("0123456789abcdef0123456789abcdef")
	w, err := k.Wrap(dek)
	if err != nil {
		t.Fatal(err)
	}
	got, err := k.Unwrap(w)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, dek) {
		t.Fatalf("unwrap mismatch: %q", got)
	}
}

func TestLocalKeyWrapWrongKeyFails(t *testing.T) {
	k1, _ := NewLocalKey("k1", "secret-one-entropy")
	k2, _ := NewLocalKey("k2", "secret-two-entropy")
	w, _ := k1.Wrap([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := k2.Unwrap(w); err == nil {
		t.Fatal("unwrap with wrong key should fail")
	}
}

func TestMasterKeyProviderRotates(t *testing.T) {
	oldKey, _ := NewLocalKey("key-2023", "old-secret-entropy")
	reg := NewMasterKeyProvider(oldKey)

	// Encrypt a DEK under the old key (simulating a row written pre-rotation).
	oldEnc, _ := NewKeyedEncryptor(oldKey)
	ct, err := oldEnc.Encrypt([]byte("conn string value"))
	if err != nil {
		t.Fatal(err)
	}

	// Rotate: a new key becomes current, the old one stays loaded for decrypt.
	newKey, err := NewLocalKey("key-2024", "new-secret-entropy")
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Rotate(newKey); err != nil {
		t.Fatal(err)
	}

	// New writes go under the current key.
	newEnc, _ := NewKeyedEncryptor(reg.Current())
	ct2, _ := newEnc.Encrypt([]byte("another value"))

	// Old rows still decrypt by their recorded id.
	oldKept, err := reg.Key("key-2023")
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := NewKeyedEncryptor(oldKept)
	pt, err := dec.Decrypt(ct)
	if err != nil {
		t.Fatalf("old row should decrypt after rotation: %v", err)
	}
	if string(pt) != "conn string value" {
		t.Fatalf("old row value = %q", pt)
	}

	// New rows decrypt under the current key id.
	cur, err := reg.Key("key-2024")
	if err != nil {
		t.Fatal(err)
	}
	dec2, _ := NewKeyedEncryptor(cur)
	if pt2, err := dec2.Decrypt(ct2); err != nil || string(pt2) != "another value" {
		t.Fatalf("new row decrypt failed: %v %q", err, pt2)
	}
	if reg.Current().ID() != "key-2024" {
		t.Fatalf("current id = %q", reg.Current().ID())
	}
}

func TestMasterKeyProviderUnknownKeyFails(t *testing.T) {
	k, _ := NewLocalKey("k1", "secret-entropy")
	reg := NewMasterKeyProvider(k)
	if _, err := reg.Key("does-not-exist"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
}

func TestMasterKeyProviderDuplicateIDFails(t *testing.T) {
	k, _ := NewLocalKey("k1", "secret-entropy")
	dup, _ := NewLocalKey("k1", "other-secret-entropy")
	reg := NewMasterKeyProvider(k)
	if err := reg.Add(dup); err == nil {
		t.Fatal("duplicate key id should error")
	}
}

func TestNewLocalKeyRejectsEmpty(t *testing.T) {
	if _, err := NewLocalKey("", "secret-entropy"); err == nil {
		t.Fatal("empty id should error")
	}
	if _, err := NewLocalKey("k1", ""); err == nil {
		t.Fatal("empty secret should error")
	}
}
