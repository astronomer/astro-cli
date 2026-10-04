package astro

import (
	"os"
	"testing"
)

// TestMain pins os.Args to a single non-test argument for the whole package run.
// Several tests execute cobra commands, which fall back to os.Args when no args
// are set. Left as-is, the go-test flags (or a slice a prior test mangled to
// empty) leak between tests and make cobra execution order-dependent under
// -shuffle. Restoring afterward leaves the real args for anything that runs later.
func TestMain(m *testing.M) {
	origArgs := os.Args
	// A helper process (TestForcedRenewalHelper) needs its -test.run.
	if os.Getenv(renewHelperEnv) == "" {
		os.Args = []string{"astro"}
	}
	code := m.Run()
	os.Args = origArgs
	os.Exit(code)
}
