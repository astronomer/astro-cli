package astro

import (
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
// It also watches what the commands publish through cliout.Renderer.Emit, and
// fails a passing run that published a shape no golden pins (schema_test.go).
func TestMain(m *testing.M) {
	origArgs := os.Args
	// A helper process (TestForcedRenewalHelper) needs its -test.run, and so
	// does `make update-schemas`, which runs only the golden tests: the rest
	// of the package has nothing to rewrite.
	if os.Getenv(renewHelperEnv) == "" && !cliouttest.Updating() {
		os.Args = []string{"astro"}
	}
	// Deploy refuses a checkout with uncommitted changes, and these tests run
	// in a real one: without this, they would pass or fail with the developer's
	// working tree. A test of the refusal sets its own.
	hasUncommittedChanges = func(string) bool { return false }
	problems := watchEmit()
	code := m.Run()
	os.Args = origArgs

	// Only for a whole, passing run. A failing run emits a partial set, and
	// gaps reported from it would bury the failure that caused them; a helper
	// process or an update run is filtered, so its tally is partial too.
	if code == 0 && os.Getenv(renewHelperEnv) == "" && !cliouttest.Updating() {
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
