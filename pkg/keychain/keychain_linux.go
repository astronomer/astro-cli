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
// 0600 permissions. This matches the current plaintext config.yaml behavior
// and is intentional — encrypted file fallback is not worth the complexity
// given that CI environments use ASTRO_API_TOKEN anyway.
func New() (SecureStore, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:             serviceName,
		LibSecretCollectionName: "astro-cli",
		KWalletAppID:            "astro-cli",
		KWalletFolder:           "astro-cli",
		// Only allow persistent, non-interactive backends. KeyCtl stores
		// credentials in kernel memory that doesn't survive reboot. Pass
		// and File prompt for passphrases, which breaks non-interactive
		// CLI usage. When neither desktop backend is available we fall
		// through to our own fileStore below.
		AllowedBackends: []keyring.BackendType{
			keyring.SecretServiceBackend,
			keyring.KWalletBackend,
		},
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
