package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Under an Astro Private Cloud context the saved deploy target is a Houston
// Deployment, and becomes an apc link on that context's domain, marked
// default, in both arms. The ids alone could not say so: both are cuids.
func TestASavedDeployTargetUnderAPCBecomesAnAPCLink(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: " + workspaceID1x + "\n"
	for name, extra := range map[string]map[string]string{
		"greenfield": nil,
		"adopted":    {manifest.Marker: "[tool.ruff]\nline-length = 120\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := project1xWithConfig(t, config, extra)
			res, err := Run(dir, Options{APCDomain: "https://Astro.Corp.Example/"})
			require.NoError(t, err)
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			link, ok := m.Astro.Deployments[Link1xName]
			require.True(t, ok, "links: %v", m.Astro.Deployments)
			assert.Equal(t, manifest.Link{
				Target: "apc", Domain: "astro.corp.example",
				Workspace: workspaceID1x, Deployment: deploymentID1x,
				Auth: manifest.Auth{Method: manifest.AuthAPC}, Default: true,
			}, link)
			// [tool.astro] stays Astro's: the Houston workspace is the link's own.
			assert.Empty(t, m.Astro.Workspace)
			assert.Empty(t, m.Astro.Domain)
			assert.Contains(t, res.Advisories, ".astro/config.yaml: linked Deployment "+deploymentID1x+
				" as \"default\" in pyproject.toml (Astro Private Cloud astro.corp.example, workspace "+workspaceID1x+")")
		})
	}
}

// The apc link is SaveLink's too: the same bytes `astro link add` writes
// under an APC context.
func TestTheAPCConversionLinkIsWhatSaveLinkWrites(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: " + workspaceID1x + "\n"
	dir := project1xWithConfig(t, config, nil)
	_, err := Run(dir, Options{APCDomain: "astro.corp.example"})
	require.NoError(t, err)
	converted, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)

	plain := project1xWithConfig(t, "project:\n  name: orders\n", nil)
	_, err = Run(plain, Options{APCDomain: "astro.corp.example"})
	require.NoError(t, err)
	require.NoError(t, SaveLink(plain, nil, Link{
		Name: Link1xName, Kind: manifest.KindAPC, Domain: "astro.corp.example",
		Deployment: deploymentID1x, Workspace: workspaceID1x,
	}))
	require.NoError(t, SetDefaultLink(plain, nil, Link1xName))
	linked, err := os.ReadFile(filepath.Join(plain, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, string(linked), string(converted))
}

// A saved deploy target that cannot become a link under APC is a note naming
// the astro link add that links it under that context, not the Astro entry:
// Astro CLI 1.x's APC deploy saved project.deployment and never
// project.workspace, so this is what most APC conversions get. With a
// workspace that is not an id, the command carries it as --workspace.
func TestAnUnlinkableDeployTargetUnderAPCNamesTheAPCLink(t *testing.T) {
	for _, tc := range []struct {
		name, config, want string
	}{
		{
			"no workspace",
			"project:\n  name: orders\n  deployment: " + deploymentID1x + "\n",
			".astro/config.yaml: saved deploy target " + deploymentID1x + " was not linked. If it is a Deployment on " +
				"Astro Private Cloud astro.corp.example, link it with: astro link add prod --deployment " + deploymentID1x +
				" --workspace <workspace-id>, then: astro link default prod",
		},
		{
			"a workspace that is not an id",
			"project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: ws-1\n",
			".astro/config.yaml: saved deploy target " + deploymentID1x + " was not linked. If it is a Deployment on " +
				"Astro Private Cloud astro.corp.example, link it with: astro link add prod --deployment " + deploymentID1x +
				" --workspace ws-1, then: astro link default prod",
		},
		{
			"a value that cannot be printed",
			"project:\n  name: orders\n  deployment: \"a b\"\n",
			".astro/config.yaml: the saved deploy target (project.deployment) was not linked. If it is a Deployment on " +
				"Astro Private Cloud astro.corp.example, link it with: astro link add prod --deployment <id> " +
				"--workspace <workspace-id>, then: astro link default prod",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := project1xWithConfig(t, tc.config, nil)
			res, err := Run(dir, Options{APCDomain: "astro.corp.example"})
			require.NoError(t, err)
			assert.Contains(t, res.Notes, tc.want)
			assert.NotContains(t, strings.Join(res.Notes, "\n"), "Astro Deployment")
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			assert.Empty(t, m.Astro.Deployments)
		})
	}
}

// Setting the deprecated DeploysToAPC beside a domain converts as the domain
// alone does: the field is read by nothing.
func TestDeploysToAPCWithADomainConverts(t *testing.T) {
	config := "project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: " + workspaceID1x + "\n"
	dir := project1xWithConfig(t, config, nil)
	_, err := Run(dir, Options{DeploysToAPC: true, APCDomain: "astro.corp.example"})
	require.NoError(t, err)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, manifest.KindAPC, m.Astro.Deployments[Link1xName].Kind())
}

// A saved Deployment id with no workspace takes the APC context's current
// workspace, where Astro CLI 1.x's deploy looked it up, and becomes the apc
// link, saying where the workspace came from. APCWorkspace is called once,
// and only then: not for a saved workspace, outside APC, or for a saved value
// that is not an id. A workspace it cannot give keeps the note.
func TestASavedDeploymentTakesTheAPCContextsWorkspace(t *testing.T) {
	deploymentOnly := "project:\n  name: orders\n  deployment: " + deploymentID1x + "\n"
	calls := 0
	current := func(ws string) func() string {
		return func() string { calls++; return ws }
	}

	dir := project1xWithConfig(t, deploymentOnly, nil)
	res, err := Run(dir, Options{APCDomain: "astro.corp.example", APCWorkspace: current(workspaceID1x)})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, manifest.Link{
		Target: "apc", Domain: "astro.corp.example",
		Workspace: workspaceID1x, Deployment: deploymentID1x,
		Auth: manifest.Auth{Method: manifest.AuthAPC}, Default: true,
	}, m.Astro.Deployments[Link1xName])
	assert.Contains(t, res.Advisories, ".astro/config.yaml: linked Deployment "+deploymentID1x+
		" as \"default\" in pyproject.toml (Astro Private Cloud astro.corp.example, workspace "+workspaceID1x+
		"). It saved no workspace, so this uses your current one. If the Deployment is in another workspace, "+
		"relink it with: astro link add default --deployment "+deploymentID1x+" --workspace <workspace-id> --replace")
	assert.NotContains(t, strings.Join(res.Notes, "\n"), "saved deploy target")

	for name, tc := range map[string]struct {
		config string
		opts   Options
	}{
		"a saved workspace": {
			"project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: " + workspaceID1x + "\n",
			Options{APCDomain: "astro.corp.example"},
		},
		"under Astro":             {deploymentOnly, Options{}},
		"a saved value not an id": {"project:\n  name: orders\n  deployment: my-release\n", Options{APCDomain: "astro.corp.example"}},
		"no saved target at all":  {"project:\n  name: orders\n", Options{APCDomain: "astro.corp.example"}},
	} {
		t.Run(name, func(t *testing.T) {
			calls = 0
			tc.opts.APCWorkspace = current(workspaceID1x)
			_, err := Plan(project1xWithConfig(t, tc.config, nil), tc.opts)
			require.NoError(t, err)
			assert.Zero(t, calls, "APCWorkspace is read only when it decides something")
		})
	}

	// A rerun on the converted project, whose 1.x config still saves the
	// Deployment alone, is refused before the link is decided, so it asks
	// nothing.
	t.Run("a refused rerun", func(t *testing.T) {
		dir := project1xWithConfig(t, deploymentOnly, nil)
		_, err := Run(dir, Options{APCDomain: "astro.corp.example"})
		require.NoError(t, err)
		calls = 0
		_, err = Run(dir, Options{APCDomain: "astro.corp.example", APCWorkspace: current(workspaceID1x)})
		require.ErrorIs(t, err, ErrAlreadyAstroProject)
		assert.Zero(t, calls, "a refused run read the context's workspace")
	})

	for name, ws := range map[string]string{"none": "", "not an id": "myworkspace"} {
		t.Run("a workspace that is "+name, func(t *testing.T) {
			dir := project1xWithConfig(t, deploymentOnly, nil)
			res, err := Run(dir, Options{APCDomain: "astro.corp.example", APCWorkspace: current(ws)})
			require.NoError(t, err)
			m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
			require.NoError(t, err)
			assert.Empty(t, m.Astro.Deployments)
			assert.Contains(t, strings.Join(res.Notes, "\n"),
				"astro link add prod --deployment "+deploymentID1x+" --workspace <workspace-id>, then",
				"a workspace that is not an id is not taken")
		})
	}
}
