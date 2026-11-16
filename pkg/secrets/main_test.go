package secrets

import (
	"os"
	"testing"
)

// TestMain moves HOME and the state directories into a temporary one for the
// package run (isolateHome), and makes the default keyring an in-memory one, so
// no test touches the developer's machine. Most tests here pass a fake keyringAPI
// of their own; this is for any that reach the default one, which refuseInTests
// would otherwise fail.
func TestMain(m *testing.M) {
	finish := isolateHome()
	MockKeyringForTests()
	os.Exit(finish(m.Run()))
}
