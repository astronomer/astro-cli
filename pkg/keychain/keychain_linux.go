//go:build linux

package keychain

import (
	"fmt"

	"github.com/99designs/keyring"
)

// New returns a Secret Service-backed SecureStore on Linux.
//
// If no Secret Service daemon is available (e.g. headless CI environments) and
// allowInsecureFallback is true, falls back to a plaintext JSON file at
// ~/.astro/credentials.json with 0600 permissions. This matches the previous
// plaintext config.yaml posture and is acceptable because CI environments
// typically use ASTRO_API_TOKEN rather than interactive login credentials.
//
// The fallback announces itself on every write (see warnInsecureWrite) and can
// be refused outright with `astro config set -g no_insecure_fallback true`, in
// which case an unavailable Secret Service is an error. A silent, unrefusable
// downgrade is the part of this design the GitHub CLI regrets: see cli/cli#10108.
func New(allowInsecureFallback bool) (SecureStore, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:             serviceName,
		LibSecretCollectionName: serviceName,
		KWalletAppID:            serviceName,
		KWalletFolder:           serviceName,
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
		return newCachedStore(withTimeout(&keyringStore{ring: ring}, keyringTimeout)), nil
	}
	// Secret Service unavailable — fall back to a plaintext file, unless the
	// user has told us not to.
	if !allowInsecureFallback {
		return nil, fmt.Errorf("%w: no Secret Service available (%v)", ErrInsecureFallbackRefused, err)
	}
	fs, err := newFileStore()
	if err != nil {
		return nil, fmt.Errorf("credential store unavailable: %w", err)
	}
	return newCachedStore(fs), nil
}
