//go:build windows

package keychain

import "fmt"

// New returns a file-backed SecureStore on Windows: credentials go to
// credentials.json (mode 0600), not to Credential Manager.
//
// Plaintext is currently the only option here, so setting no_insecure_fallback
// leaves nothing to fall back to and we fail rather than write secrets the user
// has explicitly refused.
func New(allowInsecureFallback bool) (SecureStore, error) {
	if !allowInsecureFallback {
		return nil, fmt.Errorf("%w: the Windows Credential Manager backend is not implemented yet, "+
			"so credentials can only be stored in plain text on this platform", ErrInsecureFallbackRefused)
	}
	fs, err := newFileStore()
	if err != nil {
		return nil, fmt.Errorf("credential store unavailable: %w", err)
	}
	return newCachedStore(fs), nil
}
