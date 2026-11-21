package auth

import (
	"os"
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain isolates the package run (testenv.Isolate): the environment variables
// that change what the CLI does are unset, and HOME is a temporary directory.
// CI is among the variables: a run with CI set asks
// nothing (input.Refusal), so a case that expects a prompt would fail on a CI
// runner. The cases about CI, or any other variable, set it themselves.
func TestMain(m *testing.M) {
	finish := testenv.Isolate()
	// Opening a browser is refused over SSH (input.CanOpenBrowser): a run
	// over the developer's own SSH session must not change what these tests see.
	os.Unsetenv("SSH_CONNECTION")
	os.Unsetenv("SSH_TTY")
	os.Exit(finish(m.Run()))
}
