package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// encPrefix marks an encrypted value so it is unambiguous from plaintext
// when inspecting an on-disk file. Versioned so future schemes can land
// alongside.
const encPrefix = "enc:v1:"

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	return gcm, nil
}

// encrypt seals plain and returns "enc:v1:<base64(nonce||ciphertext||tag)>".
// Empty input round-trips as the empty string so callers don't special-case
// missing values.
func encrypt(gcm cipher.AEAD, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Values without the prefix pass through unchanged so a value file stays
// inspectable and hand-editable during development.
func decrypt(gcm cipher.AEAD, value string) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}
