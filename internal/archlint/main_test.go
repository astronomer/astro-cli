package archlint

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/testing/testenv"
)

// TestMain isolates the package run from the machine: the environment
// variables that change what the CLI does are unset, HOME and the state
// directories are a temporary one, and the keychain is in memory
// (testenv.Isolate). A test that needs a variable set sets it itself.
func TestMain(m *testing.M) { testenv.Main(m) }
