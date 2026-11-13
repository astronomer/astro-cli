package user

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain unsets the environment variables that change what the CLI does
// (ASTRO_API_TOKEN, ASTRO_HOME, CI, ...) for the package run, so the tests do
// not depend on the shell they run in. A test that needs one sets it itself.
func TestMain(m *testing.M) {
	testenv.Clear()
	m.Run()
}
