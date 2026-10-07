package cmd

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout/cliouttest"
)

// TestMain pins os.Args to a single non-test argument for the whole package run.
// Several tests execute cobra commands, which fall back to os.Args when no args
// are set. Left as-is, the go-test flags (or a slice a prior test mangled to
// empty) leak between tests and make cobra execution order-dependent under
// -shuffle. Restoring afterward leaves the real args for anything that runs later.
//
// The test flags are parsed first, while os.Args still holds them: m.Run parses
// only if nobody has, and from os.Args, so pinning before parsing made it read
// none — every run of this package ignored -run and -v and ran everything.
//
// It also watches what the commands publish through cliout.Renderer.Emit, and
// fails a passing run that published a shape no golden pins (schema_test.go).
func TestMain(m *testing.M) {
	flag.Parse()
	origArgs := os.Args
	os.Args = []string{"astro"}
	problems := watchEmit()
	code := m.Run()
	os.Args = origArgs

	// Only for a passing run that is not rewriting the goldens: a failing
	// run emits a partial set, and gaps reported from it would bury the
	// failure that caused them.
	if code == 0 && !cliouttest.Updating() {
		if found := problems(); len(found) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: what commands emit does not match what is pinned")
			for _, p := range found {
				fmt.Fprintln(os.Stderr, "  "+p)
			}
			code = 1
		}
	}
	os.Exit(code)
}
