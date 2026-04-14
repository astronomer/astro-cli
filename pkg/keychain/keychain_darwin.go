//go:build darwin

package keychain

import (
	"fmt"

	"github.com/99designs/keyring"
)

// New returns a macOS Keychain-backed SecureStore.
//
// Items are stored with per-app ACL (KeychainTrustApplication) so that other
// processes — including the `security` CLI tool — must show a user prompt before
// reading them. After each CLI binary update (binary hash changes), macOS
// re-prompts once on first access. This is expected behavior.
//
// The cachedStore wrapper (see keychain_cached.go) ensures only one keychain
// access per domain per process, so the user sees at most one prompt per
// command invocation.
func New() (SecureStore, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:                    serviceName,
		KeychainTrustApplication:       true,
		KeychainAccessibleWhenUnlocked: true,
	})
	if err != nil {
		return nil, fmt.Errorf("system keychain unavailable: %w", err)
	}
	return newCachedStore(&keyringStore{ring: ring}), nil
}
