package secretstest_test

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/testenv"
)

// TestMain isolates the package run from the machine (pkg/testenv): the
// environment variables that change what the CLI does are unset, HOME and the
// state directories are a temporary one, and the keychain is in memory.
func TestMain(m *testing.M) {
	testenv.Main(m, testenv.Options{MockKeyring: secrets.MockKeyringForTests, KeyringRefusals: secrets.OSKeyringRefusals})
}
