// Package testenv is the root module's front for pkg/testenv, the shared module
// that isolates a test binary from the machine (see its documentation and
// docs/architecture.md, "Tests never touch the machine"). It adds the one thing
// the shared module cannot know: the root's test binaries link pkg/secrets, so
// they get its in-memory keyring and its count of refused OS keychain requests.
// Every root test package's TestMain is testenv.Main(m).
package testenv

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
	shared "github.com/astronomer/astro-cli/pkg/testenv"
)

// options are the keyring hooks every root test binary passes.
func options() shared.Options {
	return shared.Options{MockKeyring: secrets.MockKeyringForTests, KeyringRefusals: secrets.OSKeyringRefusals}
}

// Isolate makes the test binary hermetic and returns the function that ends the
// run: finish := Isolate(); os.Exit(finish(m.Run())). See pkg/testenv.
func Isolate() (finish func(code int) int) { return shared.Isolate(options()) }

// Main is the whole TestMain of a package with nothing to add to it.
func Main(m *testing.M) { shared.Main(m, options()) }

// RequireTemp exits unless every path is in the test home or the temporary
// directory, as a helper process that writes the config checks first.
func RequireTemp(paths ...string) { shared.RequireTemp(paths...) }
