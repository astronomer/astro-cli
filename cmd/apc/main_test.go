package apc

import (
	"flag"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/cmd/utils"
)

// TestMain pins os.Args to a single non-test argument for the whole package run.
// Several tests execute cobra commands, which fall back to os.Args when no args
// are set. Left as-is, the go-test flags (or a slice a prior test mangled to
// empty) leak between tests and make cobra execution order-dependent under
// -shuffle. Restoring afterward leaves the real args for anything that runs later.
//
// The test flags are parsed first, while os.Args still holds them: m.Run parses
// only if nobody has, and from os.Args, so pinning before parsing would make it
// read none, and the run would ignore -run and -v.
//
// It also watches what the commands publish through cliout.Renderer.Emit, and
// fails a passing run that published a shape no golden pins (schema_test.go).
//
// Many cases run 1.x command lines to pin what they print, so the rename hint
// a 1.x spelling prints on stderr is hidden here; cmd's renamed_flags_test.go
// is where the hint is tested.
func TestMain(m *testing.M) {
	flag.Parse()
	origArgs := os.Args
	os.Args = []string{"astro"}
	os.Setenv(utils.RenameHintsDisabledEnv, "1") //nolint:usetesting // for the whole package run

	watching := emitWatch().Arm()
	code := m.Run()
	os.Args = origArgs
	os.Exit(watching.Finish(code))
}
