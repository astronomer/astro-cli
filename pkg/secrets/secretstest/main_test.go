package secretstest_test

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// TestMain moves HOME and the state directories into a temporary one for the
// package run (isolateHome), makes the default keyring an in-memory one, and
// fails the run on any refusal of the OS keychain, so no test touches the
// developer's machine.
func TestMain(m *testing.M) {
	finish := isolateHome(keyringHooks{Mock: secrets.MockKeyringForTests, Refusals: secrets.OSKeyringRefusals})
	os.Exit(finish(m.Run()))
}
