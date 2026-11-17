package astro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
)

// An ASTRO_API_TOKEN run is on the host its manifest's domain names, else
// ASTRO_DOMAIN, else astronomer.io, and the saved current context is not
// consulted: a machine switched to dev still sends a production token to
// astronomer.io, and the switch is neither read nor undone.

// devSaved makes astronomer-dev.io the saved current context, beside the
// astronomer.io login savedLogin wrote.
func devSaved(t *testing.T, home string) {
	t.Helper()
	dev := config.Context{Domain: "astronomer-dev.io", Token: "Bearer saved-dev", RefreshToken: "dev-refresh", Organization: "dev-org", Workspace: "dev-ws"}
	if err := dev.SetContext(); err != nil {
		t.Fatal(err)
	}
	if err := dev.SwitchContext(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".astro", "config.yaml")), "context: astronomer-dev.io") {
		t.Fatal("the saved current context is not dev")
	}
}

// projectNamingHost makes the working directory a project whose manifest
// names domain.
func projectNamingHost(t *testing.T, domain string) {
	t.Helper()
	dir := t.TempDir()
	body := "[project]\nname = 'demo'\nrequires-python = '>=3.10'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\nworkspace = 'ws_abc123'\ndomain = '" + domain + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
}

func TestTokenRunHost(t *testing.T) {
	tests := []struct {
		name        string
		manifest    string
		astroDomain string
		want        string
	}{
		{"no manifest domain and no ASTRO_DOMAIN is astronomer.io, not the saved dev context", "", "", "astronomer.io"},
		{"ASTRO_DOMAIN", "", "astronomer-test.io", "astronomer-test.io"},
		{"the manifest's domain", "astronomer-stage.io", "", "astronomer-stage.io"},
		{"the manifest's domain over ASTRO_DOMAIN", "astronomer-stage.io", "astronomer-test.io", "astronomer-stage.io"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := savedLogin(t, false)
			noLoginFlow(t)
			devSaved(t, home)
			if tt.manifest != "" {
				projectNamingHost(t, tt.manifest)
			}
			t.Setenv("ASTRO_DOMAIN", tt.astroDomain)
			t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-host"))
			token := os.Getenv("ASTRO_API_TOKEN")

			if err := EnsureLogin(orgsClient(nil)); err != nil {
				t.Fatal(err)
			}

			c, err := config.GetCurrentContext()
			if err != nil {
				t.Fatal(err)
			}
			if c.Domain != tt.want {
				t.Errorf("the run's host is %q, want %q", c.Domain, tt.want)
			}
			if c.Token != "Bearer "+token {
				t.Errorf("the run's login on %s is not the environment's token", c.Domain)
			}
			if c.Organization != "env-org" || c.Workspace != "env-ws" {
				t.Errorf("the run's selection is %q/%q, want the token's", c.Organization, c.Workspace)
			}

			// The saved current context is still dev on disk, and its login is
			// still its own.
			raw := readFile(t, filepath.Join(home, ".astro", "config.yaml"))
			if !strings.Contains(raw, "context: astronomer-dev.io") {
				t.Error("the run changed the saved current context")
			}
			if strings.Contains(raw, token) {
				t.Error("the config file holds the environment's token")
			}
		})
	}
}

// Naming the token run's host makes nothing current by itself.
func TestEnvironmentDomainLeavesTheCurrentContextAlone(t *testing.T) {
	home := savedLogin(t, false)
	noLoginFlow(t)
	devSaved(t, home)
	projectNamingHost(t, "astronomer-stage.io")
	if domain, current, err := environmentDomain(); err != nil || domain != "astronomer-stage.io" || current {
		t.Fatalf("environmentDomain() = %q, %v, %v", domain, current, err)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Domain != "astronomer-dev.io" {
		t.Errorf("the current context is %q, want the saved dev one", c.Domain)
	}
}

// A manifest that does not load stops a token run before anything is sent: its
// domain is unknown, and another host would get the token.
func TestATokenRunWithAnUnreadableManifestSendsNothing(t *testing.T) {
	home := savedLogin(t, false)
	noLoginFlow(t)
	devSaved(t, home)
	dir := t.TempDir()
	body := "[project]\nname = 'demo'\n\n[tool.astro]\ndomain = 'astronomer-stage.io'\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
	t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-bad-manifest"))
	client := orgsClient(nil)
	if err := EnsureLogin(client); err == nil {
		t.Fatal("a token run with an unreadable manifest went on")
	}
	client.AssertNotCalled(t, "ListOrganizationsWithResponse", mock.Anything, mock.Anything)
}
