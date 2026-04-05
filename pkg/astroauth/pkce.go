package astroauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// GeneratePKCE creates a PKCE verifier and challenge pair for the OAuth flow.
func GeneratePKCE() (verifier, challenge string, err error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return "", "", fmt.Errorf("cannot generate PKCE verifier: %w", err)
	}
	verifier = Base64URLEncode(verifierBytes)
	challengeHash := sha256.Sum256([]byte(verifier))
	challenge = Base64URLEncode(challengeHash[:])
	return verifier, challenge, nil
}

// Base64URLEncode encodes bytes using base64url encoding (RFC 4648 §5),
// without padding.
func Base64URLEncode(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	s = strings.TrimRight(s, "=")
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}
