package testing

import (
	"testing"

	"github.com/astronomer/astro-cli/config"
)

// UseLocalCoreURL points the Astro API of the host "localhost" at url (the
// server's base, with no /v1) for the current test config, so a test can drive
// the CLI's real control-plane client against a fake server. Call it after
// InitTestConfig, which replaces the config it writes to.
func UseLocalCoreURL(t *testing.T, url string) {
	t.Helper()
	if err := config.CFG.AstroLocalCoreURL.SetHomeString(url); err != nil {
		t.Fatalf("pointing the local core API at %s: %v", url, err)
	}
}
