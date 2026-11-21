package input

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain isolates the package run from the machine: the environment
// variables that change what the CLI does are unset, HOME and the state
// directories are a temporary one, and the keychain is in memory
// (testenv.Isolate). A test that needs a variable set sets it itself.
func TestMain(m *testing.M) {
	finish := testenv.Isolate()
	// Opening a browser is refused over SSH (input.CanOpenBrowser): a run
	// over the developer's own SSH session must not change what these tests see.
	os.Unsetenv("SSH_CONNECTION")
	os.Unsetenv("SSH_TTY")
	os.Exit(finish(m.Run()))
}
