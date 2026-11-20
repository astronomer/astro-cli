package local

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// A token run's host is the manifest's domain, else ASTRO_DOMAIN, else
// astronomer.io, for every lookup in the run: the Deployment lookup of
// `astro af -d <name>` and the Environment Manager read of `astro local env`
// both. The saved context's host is not in the rule. These tests keep the
// saved current context on a host that never resolves, with no ASTRO_DOMAIN,
// and the manifest naming localhost, which the fake API answers.

// hostManifest names a host with no top-level `workspace`: a manifest may name
// a domain only for a workspace or an astro-auth link, and here a link has its
// own workspace, so the only Workspace a Deployment name or an Environment
// Manager read has is the token's claim.
const hostManifest = `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
domain = 'localhost'

[tool.astro.deployments.elsewhere]
deployment = 'clm2xk9dq000108l7a2b3c4f5'
workspace = 'ws_elsewhere'

[tool.astro.env]
DATA_WAREHOUSE_URI = { source = 'workspace' }
`

func hostProject(t *testing.T) string {
	t.Helper()
	dir := workspaceEnvProject(t)
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(hostManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// useDevSavedContext makes a saved login on a host that never resolves the
// current context, so a request that followed it would fail loudly rather
// than reach the fake, and leaves ASTRO_DOMAIN unset.
func useDevSavedContext(t *testing.T, f fakeAstro) {
	t.Helper()
	useFakeAstro(t, f)
	t.Setenv("ASTRO_DOMAIN", "")
	if got := testUtil.SaveCurrentContext(t, "astro-dev.invalid"); got != "astro-dev.invalid" {
		t.Fatalf("the saved current context is %q, want the dev one", got)
	}
}

// useNoSavedConfig is a CI machine: no saved context at all.
func useNoSavedConfig(t *testing.T, f fakeAstro) {
	t.Helper()
	useFakeAstro(t, f)
	t.Setenv("ASTRO_DOMAIN", "")
	testUtil.ClearCurrentContext(t)
}

// oneRun is one process's work: a Deployment named with -d, then an
// Environment Manager read, under the same token.
func oneRun(t *testing.T, f fakeAstro, token, dir string) {
	t.Helper()
	d := productionDeps(t, dir)
	if err := execute(t, d, "af", "pools", "list", "-d", "etl"); err != nil {
		t.Fatalf("af -d etl: %v", err)
	}
	stdout := d.Stdout.(interface{ String() string })
	if err := execute(t, d, "local", "env", "list", "--all", "--output", "json"); err != nil {
		t.Fatalf("local env list: %v", err)
	}
	everyRequestUnder(t, f, token)

	// The Deployment lookup and the Environment Manager read both reached the
	// one host the manifest names, and both asked about the token's Workspace.
	if got := listedWorkspaces(f); len(got) != 1 || got[0] != tokenWS {
		t.Errorf("the Deployment lookup listed %v, want the token's %s", got, tokenWS)
	}
	if len(f.requestsToPath("/v1/organizations/"+tokenOrg+"/environment-objects")) == 0 {
		t.Error("the Environment Manager was not read on the manifest's host")
	}
	if !strings.Contains(stdout.String(), "cloud vault") {
		t.Errorf("the Environment Manager read was not of the token's Workspace:\n%s", stdout.String())
	}
	if !f.sawRequest(http.MethodGet, "/api/v2/pools") {
		t.Error("the Deployment's Airflow was never reached")
	}
}

// A token run with a dev saved context and no ASTRO_DOMAIN goes where the
// manifest says, for the Deployment lookup and the Environment Manager read
// alike, and takes the token's Workspace claim on that host.
func TestTokenRunWithADevSavedContextUsesTheManifestsHost(t *testing.T) {
	for _, kind := range []tokenKind{wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useDevSavedContext(t, f)
			token := useToken(t, kind)
			oneRun(t, f, token, hostProject(t))
		})
	}
}

// In CI there is no saved config, and a manifest naming a host that is not the
// default must not make the Workspace token's claim vanish for af while it
// works for local env.
func TestTokenRunWithNoSavedConfigAndAManifestDomainKeepsTheClaim(t *testing.T) {
	for _, kind := range []tokenKind{wsToken, depToken} {
		t.Run(kind.name, func(t *testing.T) {
			f := newFakeAstro(t)
			useNoSavedConfig(t, f)
			token := useToken(t, kind)
			oneRun(t, f, token, hostProject(t))
		})
	}
}
