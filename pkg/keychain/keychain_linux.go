//go:build linux

package keychain

import (
	"fmt"

	"github.com/99designs/keyring"
)

// New returns a Secret Service-backed SecureStore on Linux.
//
// If no Secret Service daemon is available (e.g. headless CI environments),
// falls back to a plaintext JSON file at ~/.astro/credentials.json with
// 0600 permissions. This matches the current plaintext config.yaml behaviour
// and is intentional — encrypted file fallback is not worth the complexity
// given that CI environments use ASTRO_API_TOKEN anyway.
//
// NOTE: if 99designs/keyring fails to connect to Secret Service in environments
// that DO have it running, replace with godbus/dbus directly:
// https://github.com/godbus/dbus — the SecureStore interface is the only
// change boundary.
func New() (SecureStore, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:             serviceName,
		LibSecretCollectionName: "astro-cli",
		KWalletAppID:            "astro-cli",
		KWalletFolder:           "astro-cli",
	})
	if err == nil {
		return newCachedStore(&keyringStore{ring: ring}), nil
	}
	// Secret Service unavailable — fall back to plaintext file.
	fs, err := newFileStore()
	if err != nil {
		return nil, fmt.Errorf("credential store unavailable: %w", err)
	}
	return fs, nil
}
