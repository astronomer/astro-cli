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

// SaveCurrentContext saves a login for domain, with a token of its own, and
// makes it the saved current context, as a developer's last `astro login`
// leaves it. It returns the domain a read of the current context then gives.
func SaveCurrentContext(t *testing.T, domain string) string {
	t.Helper()
	c := config.Context{Domain: domain, Token: "Bearer saved-" + domain, Organization: "saved-org-of-" + domain, Workspace: "saved-ws-of-" + domain}
	if err := c.SetContext(); err != nil {
		t.Fatalf("saving a login for %s: %v", domain, err)
	}
	if err := c.SwitchContext(); err != nil {
		t.Fatalf("making %s current: %v", domain, err)
	}
	got, err := config.GetCurrentDomain()
	if err != nil {
		t.Fatalf("reading the current domain: %v", err)
	}
	return got
}

// ClearCurrentContext leaves the machine with no saved current context, as a
// CI runner that never ran `astro login` has.
func ClearCurrentContext(t *testing.T) {
	t.Helper()
	if err := config.ResetCurrentContext(); err != nil {
		t.Fatalf("clearing the current context: %v", err)
	}
}
