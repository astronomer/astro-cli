package cmd

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
// only if nobody has, and from os.Args, so pinning before parsing made it read
// none — every run of this package ignored -run and -v and ran everything.
func TestMain(m *testing.M) {
	flag.Parse()
	origArgs := os.Args
	os.Args = []string{"astro"}
	code := m.Run()
	os.Args = origArgs
	os.Exit(code)
}
