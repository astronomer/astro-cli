package secrets

import (
	"crypto/cipher"
	"strings"
	"testing"
)

func testAEAD(t *testing.T, fill byte) cipher.AEAD {
	t.Helper()
	key := make([]byte, keyBytes)
	for i := range key {
		key[i] = byte(i) ^ fill
	}
	gcm, err := newAEAD(key)
	if err != nil {
		t.Fatalf("newAEAD: %v", err)
	}
	return gcm
}

func TestEncryptRoundTrip(t *testing.T) {
	gcm := testAEAD(t, 0)
	got, err := encrypt(gcm, "hunter2")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(got, encPrefix) {
		t.Fatalf("missing prefix: %q", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("ciphertext leaks plaintext: %q", got)
	}
	plain, err := decrypt(gcm, got)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "hunter2" {
		t.Fatalf("round-trip mismatch: got %q", plain)
	}
}

func TestEncryptEmpty(t *testing.T) {
	got, err := encrypt(testAEAD(t, 0), "")
	if err != nil || got != "" {
		t.Fatalf("empty should pass through: got %q err %v", got, err)
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	got, err := decrypt(testAEAD(t, 0), "not-encrypted")
	if err != nil || got != "not-encrypted" {
		t.Fatalf("plaintext should pass through: got %q err %v", got, err)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	enc, err := encrypt(testAEAD(t, 0), "secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decrypt(testAEAD(t, 0xff), enc); err == nil {
		t.Fatal("decrypt with a different key should fail")
	}
}

func TestDecryptTruncatedCiphertext(t *testing.T) {
	if _, err := decrypt(testAEAD(t, 0), encPrefix+"AAAA"); err == nil {
		t.Fatal("truncated ciphertext should fail")
	}
	if _, err := decrypt(testAEAD(t, 0), encPrefix+"!!!not-base64"); err == nil {
		t.Fatal("invalid base64 should fail")
	}
}
