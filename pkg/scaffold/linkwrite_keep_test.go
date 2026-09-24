package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// A re-save that names no workspace keeps the one the link sets itself, so a
// pin SetWorkspaceLink wrote survives the next edit of the link, in either
// spelling. ClearWorkspace is how a caller drops the pin on purpose.
func TestSaveLinkKeepsAPinnedWorkspaceWhenNoneIsGiven(t *testing.T) {
	for name, links := range map[string]string{
		"header": "\n[tool.astro.deployments.prod]\ndeployment = 'dep-prod'\n",
		"inline": "\n[tool.astro.deployments]\nprod = { deployment = 'dep-prod' }\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture+"workspace = 'ws_A'\ndomain = 'astronomer.io'\n"+links)
			pinned, err := SetWorkspaceLink(dir, nil, "ws_B", "astronomer.io")
			require.NoError(t, err)
			require.Equal(t, []string{"prod"}, pinned)

			saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "dep-prod2"})
			_, got := onlyLink(t, path)
			assert.Equal(t, "ws_A", got.Workspace, "the edit moved the pinned link to the project's workspace")
			assert.Equal(t, "dep-prod2", got.Deployment)

			saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "dep-prod2", ClearWorkspace: true})
			assert.NotContains(t, ownKeys(t, path, "prod"), "workspace")
			_, got = onlyLink(t, path)
			assert.Equal(t, "ws_B", got.Workspace)
		})
	}
}

// A clear alongside a workspace writes the workspace.
func TestSaveLinkAClearWithAWorkspaceWritesIt(t *testing.T) {
	dir, path := linkProject(t, linkFixture+"workspace = 'ws_A'\n\n[tool.astro.deployments.prod]\ndeployment = 'd'\nworkspace = 'ws_C'\n")
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Deployment: "d", Workspace: "ws_D", ClearWorkspace: true})
	assert.Equal(t, "ws_D", ownKeys(t, path, "prod")["workspace"])
}

// Coordinates are trimmed like the name, and one that is blank after trimming
// is refused where it is required.
func TestSaveLinkTrimsTheCoordinates(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, Link{Name: "prod", Kind: manifest.KindAstro, Workspace: " W ", Deployment: " clx1 "})
	saveLink(t, dir, Link{Name: "aws", Kind: manifest.KindMWAA, Environment: " orders "})
	oss := endpointLink(&manifest.Auth{Method: manifest.AuthNone})
	oss.URL = "  https://airflow.example.com/  "
	saveLink(t, dir, oss)
	m := loadLinks(t, path)
	assert.Equal(t, "W", m.Astro.Deployments["prod"].Workspace)
	assert.Equal(t, "clx1", m.Astro.Deployments["prod"].Deployment)
	assert.Equal(t, "orders", m.Astro.Deployments["aws"].Environment)
	assert.Equal(t, "https://airflow.example.com/", m.Astro.Deployments["oss"].URL)

	before := readFile(t, path)
	for _, l := range []Link{
		{Name: "x", Kind: manifest.KindAstro, Workspace: "W", Deployment: "  "},
		{Name: "x", Kind: manifest.KindMWAA, Environment: "\t"},
		{Name: "x", Kind: manifest.KindEndpoint, URL: " ", Auth: manifest.Auth{Method: manifest.AuthNone}},
	} {
		require.ErrorIs(t, SaveLink(dir, nil, l), ErrInvalidLink)
	}
	assert.Equal(t, before, readFile(t, path))
}

// A field left over from an earlier method is dropped rather than refused, even
// when it would not pass as an env var name: it is never written.
func TestSaveLinkIgnoresAFieldTheMethodDoesNotTake(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, endpointLink(&manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN", UsernameEnv: "not a name"}))
	_, got := onlyLink(t, path)
	assert.Equal(t, manifest.Auth{Method: manifest.AuthToken, TokenEnv: "AF_TOKEN"}, got.Auth)
	assert.NotContains(t, readFile(t, path), "not a name")

	// The method's own fields are still checked.
	require.ErrorIs(t, SaveLink(dir, nil, endpointLink(&manifest.Auth{Method: manifest.AuthToken, TokenEnv: "not a name"})), ErrInvalidLink)
}
