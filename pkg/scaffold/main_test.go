package scaffold

import (
	"os"
	"testing"
)

// TestMain moves HOME and the state directories into a temporary one for the
// package run (isolateHome), so no test touches the developer's machine.
func TestMain(m *testing.M) {
	finish := isolateHome()
	os.Exit(finish(m.Run()))
}
