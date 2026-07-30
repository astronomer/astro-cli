package astrosession

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestBearerReadsTheCurrentContext(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	token, err := Bearer(context.Background())
	if err != nil {
		t.Fatalf("bearer: %v", err)
	}
	// The token travels as the config stores it; airflowapi.BearerToken is what
	// normalizes the scheme away, so nothing here re-implements that.
	if token != "token" {
		t.Fatalf("token = %q, want the stored value", token)
	}
}

func TestBearerWithNoLoginNamesTheFix(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial) // no context at all
	_, err := Bearer(context.Background())
	if err == nil {
		t.Fatal("a logged-out machine produced a token")
	}
	if !strings.Contains(err.Error(), "not logged in") || !strings.Contains(err.Error(), "astro login") {
		t.Fatalf("err = %v, want the logged-out message and its fix", err)
	}
}

// TestBearerTreatsASchemeWithNoTokenAsLoggedOut covers how a context that has
// been logged out of actually spells itself: the scheme survives the logout,
// the credential does not.
func TestBearerTreatsASchemeWithNoTokenAsLoggedOut(t *testing.T) {
	writeConfig(t, "context: astronomer.io\ncontexts:\n  astronomer_io:\n    domain: astronomer.io\n    token: 'Bearer '\n")
	if _, err := Bearer(context.Background()); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("err = %v, want the logged-out message", err)
	}
}

// writeConfig points config/ at an in-memory home holding the given YAML, the
// same way pkg/testing does for its own fixtures.
func writeConfig(t *testing.T, yaml string) {
	t.Helper()
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, config.HomeConfigFile, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config.InitConfig(fs)
}
