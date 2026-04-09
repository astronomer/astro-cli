//go:build windows

package keychain

import "fmt"

// New returns a file-backed SecureStore on Windows.
//
// This is a temporary fallback until the Windows Credential Manager backend
// lands — credentials are stored in ~/.astro/credentials.json (mode 0600)
// rather than in Credential Manager. See the follow-up PR for the upgrade.
func New() (SecureStore, error) {
	fs, err := newFileStore()
	if err != nil {
		return nil, fmt.Errorf("credential store unavailable: %w", err)
	}
	return newCachedStore(fs), nil
}
