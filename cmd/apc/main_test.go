package apc

import (
	"flag"
	"os"
	"testing"
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
func TestMain(m *testing.M) {
	flag.Parse()
	origArgs := os.Args
	os.Args = []string{"astro"}
	// Deploy refuses a checkout with uncommitted changes, and these tests run
	// in a real one: without this, they would pass or fail with the developer's
	// working tree. A test of the refusal sets its own.
	hasUncommittedChanges = func(string) bool { return false }
	watching := emitWatch().Arm()
	code := m.Run()
	os.Args = origArgs
	os.Exit(watching.Finish(code))
}
