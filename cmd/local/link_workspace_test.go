package local

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workspaceLinkDeps is linkDeps with the current login on domain, in
// organization org.
func workspaceLinkDeps(t *testing.T, dir, domain, org string) Deps {
	t.Helper()
	d, _, _ := linkDeps(t, dir)
	d.CurrentDomain = func() string { return domain }
	d.CurrentOrganization = func() string { return org }
	return d
}

// Linking a workspace writes the three keys, the domain and organization
// defaulting to the current login's, and keeps the person's comments.
func TestLinkWorkspaceWritesTheThreeKeys(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d := workspaceLinkDeps(t, dir, "https://cloud.astronomer-dev.io/", "org_1")
	require.NoError(t, execute(t, d, "link", "workspace", "ws_A"))
	m := loadManifest(t, path)
	assert.Equal(t, "ws_A", m.Astro.Workspace)
	assert.Equal(t, "astronomer-dev.io", m.Astro.Domain, "the domain is stored normalized, as astro login stores it")
	assert.Equal(t, "org_1", m.Astro.Organization)
	assertCommentsKept(t, path)

	// Flags win over the login.
	var res workspaceLinkResult
	linkJSON(t, dir, &res, "workspace", "ws_A", "--domain", "astronomer.io", "--organization", "org_2")
	assert.Equal(t, workspaceLinkResult{Workspace: "ws_A", Domain: "astronomer.io", Organization: "org_2", Status: linkStatusLinked, Manifest: path}, res)
}

// With no login, the domain is the one the project already reads its
// workspace on, and no organization is written.
func TestLinkWorkspaceWithNoLoginKeepsTheProjectsDomain(t *testing.T) {
	const bare = "[project]\nname = 'x'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\n"
	dir, path := linkTestProject(t, bare)
	var res workspaceLinkResult
	linkJSON(t, dir, &res, "workspace", "ws_B")
	assert.Equal(t, workspaceLinkResult{Workspace: "ws_B", Domain: "astronomer.io", Status: linkStatusLinked, Manifest: path}, res)
}

// Switching pins every Deployment link that inherited the old workspace, so
// none moves, and says so.
func TestLinkWorkspaceSwitchPinsInheritingLinks(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, out, _ := linkDeps(t, dir)
	require.NoError(t, execute(t, d, "link", "workspace", "ws_B"))
	m := loadManifest(t, path)
	assert.Equal(t, "ws_B", m.Astro.Workspace)
	assert.Equal(t, "ws_A", m.Astro.Deployments["dev"].Workspace)
	assert.Equal(t, "ws_A", m.Astro.Deployments["stage"].Workspace)
	assert.Contains(t, out.String(), "kept dev, stage in workspace ws_A")
}

// Linking what is already linked leaves the file as it was.
func TestLinkWorkspaceUnchanged(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	before := readManifest(t, dir)
	var res workspaceLinkResult
	linkJSON(t, dir, &res, "workspace", "ws_A", "--domain", "astronomer.io")
	assert.Equal(t, linkStatusUnchanged, res.Status)
	assert.Empty(t, res.Pinned)
	assert.Equal(t, before, readManifest(t, dir))
	assert.Equal(t, path, res.Manifest)
}

// --unset removes all three keys, reports the workspace it unlinked, and pins
// the links that inherited it.
func TestLinkWorkspaceUnset(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	var res workspaceLinkResult
	linkJSON(t, dir, &res, "workspace", "--unset")
	assert.Equal(t, workspaceLinkResult{Workspace: "ws_A", Status: linkStatusUnlinked, Manifest: path, Pinned: []string{"dev", "stage"}}, res)
	m := loadManifest(t, path)
	assert.Empty(t, m.Astro.Workspace)
	assert.Empty(t, m.Astro.Domain)
	assert.Empty(t, m.Astro.Organization)
	assert.Equal(t, "ws_A", m.Astro.Deployments["dev"].Workspace)

	linkJSON(t, dir, &res, "workspace", "--unset")
	assert.Equal(t, linkStatusUnchanged, res.Status)
}

// --unset takes no workspace and none of the flags that describe one.
func TestLinkWorkspaceUnsetRefusesALinkArgument(t *testing.T) {
	dir, _ := linkTestProject(t, linkManifest)
	before := readManifest(t, dir)
	for _, args := range [][]string{
		{"workspace", "ws_B", "--unset"},
		{"workspace", "--unset", "--domain", "astronomer.io"},
		{"workspace", "--unset", "--organization", "org_1"},
	} {
		_, _, err := runLink(t, dir, args...)
		require.Error(t, err, "%v", args)
	}
	assert.Equal(t, before, readManifest(t, dir))
}

// In a terminal with no WORKSPACE_ID it asks which workspace to link; a run
// that cannot be asked says how to name one.
func TestLinkWorkspacePicks(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := pickDeps(t, dir, "", PickedDeployment{}, "ws_C")
	require.NoError(t, execute(t, d, "link", "workspace"))
	assert.Equal(t, 1, run.workspacePicks)
	assert.Equal(t, "ws_C", loadManifest(t, path).Astro.Workspace)

	before := readManifest(t, dir)
	_, _, err := runLink(t, dir, "workspace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "astro link workspace WORKSPACE_ID")
	assert.Equal(t, before, readManifest(t, dir))
}
