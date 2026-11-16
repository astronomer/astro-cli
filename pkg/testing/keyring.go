package testing

import (
	"testing"

	"github.com/zalando/go-keyring"
)

// MockKeyring swaps the OS keychain for an in-memory one for the test, so
// nothing a test does through the vault (a login, a renewal, an encrypted value)
// can reach the machine's own keychain.
func MockKeyring(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
}
