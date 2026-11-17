package astro

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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

// Setup holds the token as the login of the host the manifest names, and that
// host is the run's current one, while the saved current context stays dev on
// disk and no context is saved for the token's host.
func TestEnvironmentDomainLeavesTheSavedContextsAlone(t *testing.T) {
	home := savedLogin(t, false)
	noLoginFlow(t)
	devSaved(t, home)
	projectNamingHost(t, "astronomer-stage.io")
	if got := environmentDomain(); got != "astronomer-stage.io" {
		t.Fatalf("environmentDomain() = %q", got)
	}
	t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-holds"))
	before := snapshot(t, home)
	if err := EnsureLogin(orgsClient(nil)); err != nil {
		t.Fatal(err)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		t.Fatal(err)
	}
	if c.Domain != "astronomer-stage.io" {
		t.Errorf("the run's current context is %q, want the manifest's host", c.Domain)
	}
	if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
		t.Errorf("a token run that switched nothing changed %v", changed)
	}
	saved, err := config.ListContexts()
	if err != nil {
		t.Fatal(err)
	}
	for key := range saved.Contexts {
		if strings.Contains(key, "stage") {
			t.Errorf("a context was saved for the token's host: %q", key)
		}
	}
}

// A token run on a machine with no saved config at all (CI), and on a HOME
// that cannot be written, works and writes no config. Telemetry is off, as
// the harness has it, so what is checked is the token run's own writes.
func TestATokenRunNeedsNoWritableConfig(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		name := "no config file"
		if readOnly {
			name = "read-only home"
		}
		t.Run(name, func(t *testing.T) {
			if readOnly && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("a directory cannot be made unwritable here")
			}
			home := savedLogin(t, false)
			noLoginFlow(t)
			t.Setenv("ASTRO_TELEMETRY_DISABLED", "1")
			astroDir := filepath.Join(home, ".astro")
			if err := os.Remove(filepath.Join(astroDir, "config.yaml")); err != nil {
				t.Fatal(err)
			}
			config.InitConfig(afero.NewOsFs())
			t.Setenv("ASTRO_DOMAIN", "")
			t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-no-config"))
			if readOnly {
				if err := os.Chmod(astroDir, 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(astroDir, 0o700) })
			}
			before := snapshot(t, home)

			if err := EnsureLogin(orgsClient(nil)); err != nil {
				t.Fatal(err)
			}

			c, err := config.GetCurrentContext()
			if err != nil || c.Domain != "astronomer.io" || !strings.HasPrefix(c.Token, "Bearer ") {
				t.Fatalf("the run's login: domain %q, err %v", c.Domain, err)
			}
			if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
				t.Errorf("a token run wrote %v", changed)
			}
		})
	}
}

// Setup runs for every command, so a manifest that does not load names no
// host there: the run is on ASTRO_DOMAIN's, else the default, and a command
// that is no project's (astro organization list) still works.
func TestATokenRunWithABrokenManifestIsOnASTRODOMAINOrTheDefault(t *testing.T) {
	for _, tc := range []struct{ astroDomain, want string }{
		{"", "astronomer.io"},
		{"astronomer-test.io", "astronomer-test.io"},
	} {
		t.Run("ASTRO_DOMAIN="+tc.astroDomain, func(t *testing.T) {
			home := savedLogin(t, false)
			noLoginFlow(t)
			devSaved(t, home)
			// A domain with no workspace is invalid.
			dir := t.TempDir()
			body := "[project]\nname = 'demo'\n\n[tool.astro]\ndomain = 'astronomer-stage.io'\n"
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			prev := config.WorkingPath
			config.WorkingPath = dir
			t.Cleanup(func() { config.WorkingPath = prev })
			t.Setenv("ASTRO_DOMAIN", tc.astroDomain)
			t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-bad-manifest"))

			if err := EnsureLogin(orgsClient(nil)); err != nil {
				t.Fatalf("a command that is no project's failed over the manifest: %v", err)
			}
			c, err := config.GetCurrentContext()
			if err != nil || c.Domain != tc.want {
				t.Errorf("the run's host is %q (%v), want %q", c.Domain, err, tc.want)
			}
		})
	}
}

// A command that follows the project loads the manifest before setup runs,
// and stops on a manifest that does not load, so the tolerance above leaves
// no project-bound command running on a guessed host.
func TestAProjectBoundCommandStopsOnABrokenManifestUnderAToken(t *testing.T) {
	savedLogin(t, false)
	noLoginFlow(t)
	inProject(t, "[project]\nname = 'demo'\n\n[tool.astro]\ndomain = 'astronomer-stage.io'\n")
	t.Setenv("ASTRO_API_TOKEN", apiToken(t, "env-bound"))
	mc := orgsClient(nil)
	astroV1Client = mc

	group := &cobra.Command{Use: "deployment"}
	group.PersistentPreRunE = followProjectPreRun(group)
	ran := false
	group.AddCommand(&cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { ran = true; return nil }})
	group.SetArgs([]string{"list"})
	group.SetErr(new(bytes.Buffer))

	if err := group.Execute(); err == nil || ran {
		t.Fatalf("the command ran on a manifest that does not load (err %v)", err)
	}
	mc.AssertNotCalled(t, "ListOrganizationsWithResponse", mock.Anything, mock.Anything)
}

// followProjectPreRun runs before setup. Under a token it must not resolve the
// saved login (which can reach the keyring or vault, and rewrite the config)
// for the note it may print.
func TestFollowProjectUnderATokenNeverResolvesTheSavedLogin(t *testing.T) {
	home := savedLogin(t, true)
	noLoginFlow(t)
	devSaved(t, home)
	inProject(t, testProjectManifest)
	t.Setenv("ASTRO_API_TOKEN", testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:env-org", "workspaceId:"+projectWS))
	count, restore := config.CountLoginResolutions()
	t.Cleanup(restore)
	before := snapshot(t, home)

	group := &cobra.Command{Use: "deployment"}
	group.PersistentPreRunE = followProjectPreRun(group)
	ran := false
	group.AddCommand(&cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { ran = true; return nil }})
	group.SetArgs([]string{"list"})
	group.SetErr(new(bytes.Buffer))

	if err := group.Execute(); err != nil || !ran {
		t.Fatalf("the command did not run: %v", err)
	}
	if n := count(); n != 0 {
		t.Errorf("the saved login was resolved %d times under a token", n)
	}
	if changed := changedFiles(before, snapshot(t, home)); len(changed) != 0 {
		t.Errorf("the pre-run changed %v", changed)
	}
}

// The note compares the project's workspace with the token's own, not the
// saved login's.
func TestSelectionBeforeSetupUnderATokenIsTheTokensWorkspace(t *testing.T) {
	home := savedLogin(t, false)
	devSaved(t, home)
	t.Setenv("ASTRO_API_TOKEN", testUtil.MintAPIToken(t, time.Now().Add(time.Hour), "organizationId:o", "workspaceId:ws-token"))
	if got := selectionBeforeSetup(); got.Workspace != "ws-token" || got.Domain != "" {
		t.Errorf("selection = %+v, want only the token's workspace", got)
	}
	t.Setenv("ASTRO_API_TOKEN", "")
	if got := selectionBeforeSetup(); got.Workspace != "dev-ws" {
		t.Errorf("without a token the selection is %q, want the saved login's", got.Workspace)
	}
}
