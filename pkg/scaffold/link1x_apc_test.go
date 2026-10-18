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
			advisories := strings.Join(res.Advisories, "\n")
			assert.Contains(t, advisories, "[tool.astro.deployments."+Link1xName+"]")
			assert.Contains(t, advisories, "on Astro Private Cloud astro.corp.example")
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
			".astro/config.yaml: " + deploymentID1x + " is this project's saved deploy target. If that is a Deployment " +
				"on Astro Private Cloud astro.corp.example, link it under this context with astro link add prod " +
				"--deployment " + deploymentID1x + ", then astro link default prod to preselect it",
		},
		{
			"a workspace that is not an id",
			"project:\n  name: orders\n  deployment: " + deploymentID1x + "\n  workspace: ws-1\n",
			".astro/config.yaml: " + deploymentID1x + " is this project's saved deploy target. If that is a Deployment " +
				"on Astro Private Cloud astro.corp.example, link it under this context with astro link add prod " +
				"--deployment " + deploymentID1x + " --workspace ws-1, then astro link default prod to preselect it",
		},
		{
			"a value that cannot be printed",
			"project:\n  name: orders\n  deployment: \"a b\"\n",
			".astro/config.yaml: project.deployment holds this project's saved deploy target. If that is a Deployment " +
				"on Astro Private Cloud astro.corp.example, link it under this context with astro link add prod --deployment <id>",
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
