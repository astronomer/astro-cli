package local

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// apcRun is what an APC link case reads back: what the fake Houston lister
// was asked.
type apcRun struct {
	workspaces []string
	linked     map[string]string
}

// apcDeps is linkDeps under an Astro Private Cloud context on
// astro.corp.example, whose current workspace is current, with a fake
// Deployment lister that answers dep and records what it was asked. The Astro
// pickers are nil, as the root leaves them under APC.
func apcDeps(t *testing.T, dir, current string, dep PickedDeployment) (Deps, *apcRun) {
	t.Helper()
	d, _, _ := linkDeps(t, dir)
	run := &apcRun{}
	d.APC = &APCLinking{
		Domain:           "astro.corp.example",
		CurrentWorkspace: func() string { return current },
		PickDeployment: func(ws string, linked map[string]string) (PickedDeployment, error) {
			run.workspaces = append(run.workspaces, ws)
			run.linked = linked
			return dep, nil
		},
	}
	return d, run
}

// Under an APC context, add with no --deployment lists the Houston
// Deployments of the context's workspace, never [tool.astro] workspace (an
// Astro one), and writes an apc link on the context's domain, named after the
// one picked. The json is the shape an Astro link's add publishes.
func TestLinkAddUnderAPCPicksAHoustonDeployment(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := apcDeps(t, dir, "ckws-current", PickedDeployment{ID: "ckdep-1", Name: "Orders Prod", WorkspaceID: "ckws-current"})
	d.Interactive = func() bool { return true }
	out := d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "link", "add"))
	assert.Equal(t, []string{"ckws-current"}, run.workspaces)
	assert.Contains(t, out.String(), "added link orders-prod to "+path)

	m := loadManifest(t, path)
	assert.Equal(t, manifest.Link{
		Target: "apc", Domain: "astro.corp.example", Workspace: "ckws-current", Deployment: "ckdep-1",
		Auth: manifest.Auth{Method: manifest.AuthAPC},
	}, m.Astro.Deployments["orders-prod"])
	assert.Equal(t, "ws_A", m.Astro.Workspace, "the project's Astro workspace is left alone")
	assertCommentsKept(t, path)

	// The json is the shape an Astro link's add publishes, with kind apc.
	d, _ = apcDeps(t, dir, "ckws-current", PickedDeployment{})
	out = d.Stdout.(interface{ String() string })
	require.NoError(t, execute(t, d, "link", "add", "by-id", "--deployment", "ckdep-2", "--output", "json"))
	var res linkResult
	require.NoError(t, json.Unmarshal([]byte(out.String()), &res), out.String())
	assert.Equal(t, linkResult{Name: "by-id", Kind: "apc", Status: linkStatusAdded, Manifest: path}, res)
}

// A Deployment the project already links on this domain is left out of the
// list; an Astro link's Deployment, which is no Houston id, is not counted.
func TestLinkAddUnderAPCLeavesOutLinkedDeployments(t *testing.T) {
	body := linkManifest + "\n[tool.astro.deployments.apc-old]\ntarget = 'apc'\ndomain = 'astro.corp.example'\n" +
		"workspace = 'ckws-current'\ndeployment = 'ckdep-old'\n" +
		"\n[tool.astro.deployments.apc-elsewhere]\ntarget = 'apc'\ndomain = 'other.corp.example'\n" +
		"workspace = 'ckws-x'\ndeployment = 'ckdep-x'\n"
	dir, _ := linkTestProject(t, body)
	d, run := apcDeps(t, dir, "ckws-current", PickedDeployment{ID: "ckdep-2", Name: "new"})
	d.Interactive = func() bool { return true }
	require.NoError(t, execute(t, d, "link", "add"))
	assert.Equal(t, map[string]string{"ckdep-old": "apc-old"}, run.linked)
}

// By --deployment, nothing is listed: the link names --workspace, else the
// context's workspace. With neither, add says what to pass rather than
// borrowing [tool.astro] workspace.
func TestLinkAddUnderAPCByFlag(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, run := apcDeps(t, dir, "ckws-current", PickedDeployment{})
	require.NoError(t, execute(t, d, "link", "add", "prod-apc", "--deployment", "ckdep-9"))
	assert.Empty(t, run.workspaces, "nothing is listed for a Deployment named by flag")
	link := loadManifest(t, path).Astro.Deployments["prod-apc"]
	assert.Equal(t, manifest.KindAPC, link.Kind())
	assert.Equal(t, "ckws-current", link.Workspace)
	assert.Equal(t, "astro.corp.example", link.Domain)

	d, _ = apcDeps(t, dir, "", PickedDeployment{})
	require.NoError(t, execute(t, d, "link", "add", "other-apc", "--deployment", "ckdep-8", "--workspace", "ckws-given"))
	assert.Equal(t, "ckws-given", loadManifest(t, path).Astro.Deployments["other-apc"].Workspace)

	// Replaced without --workspace, the link keeps the workspace it has.
	d, _ = apcDeps(t, dir, "", PickedDeployment{})
	require.NoError(t, execute(t, d, "link", "add", "other-apc", "--deployment", "ckdep-7", "--replace"))
	got := loadManifest(t, path).Astro.Deployments["other-apc"]
	assert.Equal(t, "ckws-given", got.Workspace)
	assert.Equal(t, "ckdep-7", got.Deployment)

	before := readManifest(t, dir)
	d, _ = apcDeps(t, dir, "", PickedDeployment{})
	err := execute(t, d, "link", "add", "third", "--deployment", "ckdep-6")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--workspace")
	assert.Equal(t, before, readManifest(t, dir))
}

// Under APC the other kinds are still linked with their flags, and an astro
// link with --target astro.
func TestLinkAddUnderAPCOtherKinds(t *testing.T) {
	dir, path := linkTestProject(t, linkManifest)
	d, _ := apcDeps(t, dir, "ckws-current", PickedDeployment{})
	require.NoError(t, execute(t, d, "link", "add", "aws", "--target", "mwaa", "--environment", "orders"))
	d, _ = apcDeps(t, dir, "ckws-current", PickedDeployment{})
	require.NoError(t, execute(t, d, "link", "add", "astro-prod", "--target", "astro", "--deployment", "dep-prod"))
	m := loadManifest(t, path)
	assert.Equal(t, manifest.KindMWAA, m.Astro.Deployments["aws"].Kind())
	assert.Equal(t, manifest.KindAstro, m.Astro.Deployments["astro-prod"].Kind())
	assert.Equal(t, "ws_A", m.Astro.Deployments["astro-prod"].Workspace)
}

// Under an Astro context an apc link cannot be added: its domain is the APC
// context's, and there is none. Nothing is written.
func TestLinkAddAnAPCLinkUnderAstroIsRefused(t *testing.T) {
	dir, _ := linkTestProject(t, linkManifest)
	_, _, err := runLink(t, dir, "add", "prod-apc", "--target", "apc", "--deployment", "ckdep-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "astro context switch")
	assert.Equal(t, linkManifest, readManifest(t, dir))

	// --deployment alone is still an Astro link there.
	_, _, err = runLink(t, dir, "add", "prod", "--deployment", "dep-prod")
	require.NoError(t, err)
}

// An apc link reads in the link pickers and in astro use like any other, with
// where it points.
func TestAPCLinksAreListed(t *testing.T) {
	const apc = "\n[tool.astro.deployments.prod-apc]\ntarget = 'apc'\ndomain = 'astro.corp.example'\n" +
		"workspace = 'ckws-1'\ndeployment = 'ckdep-1'\n"
	dir, _ := linkTestProject(t, linkManifest+apc)
	d, run := pickDeps(t, dir, "\n", PickedDeployment{}, "")
	require.Error(t, execute(t, d, "link", "remove"))
	assertPickerTable(t, run.out.String(), "Select a link to remove", `2\s+prod-apc\s+apc\s+ckdep-1 on astro.corp.example`)

	dir = instanceProject(t, apc)
	d, out, _ := instanceDeps(t, dir)
	require.NoError(t, execute(t, d, "use", "--output", "json"))
	var res useListing
	require.NoError(t, json.Unmarshal(out.Bytes(), &res), out.String())
	require.Len(t, res.Deployments, 1)
	row := res.Deployments[0]
	assert.Equal(t, "prod-apc", row.Name)
	assert.Equal(t, "apc", row.Kind)
	assert.Equal(t, "apc", row.AuthMethod)
}

// apcRefusedLocator fails a test that asks it for an address.
type apcRefusedLocator struct{ t *testing.T }

func (l apcRefusedLocator) BaseURL(context.Context, instances.Instance) (string, error) {
	l.t.Fatal("an apc link reached the Locator")
	return "", nil
}

// A query command resolved to an apc link refuses it by name rather than
// asking the Astro control plane for a Houston id.
func TestAQueryRefusesAnAPCLink(t *testing.T) {
	dir := instanceProject(t, "\n[tool.astro.deployments.prod-apc]\ntarget = 'apc'\ndomain = 'astro.corp.example'\n"+
		"workspace = 'ckws-1'\ndeployment = 'ckdep-1'\n")
	d, _, _ := instanceDeps(t, dir)
	d.Locator = func(string) instances.Locator { return apcRefusedLocator{t} }
	err := execute(t, d, "af", "pools", "list", "-d", "prod-apc")
	require.ErrorIs(t, err, instances.ErrAPCUnsupported)
	assert.True(t, strings.Contains(err.Error(), "prod-apc"), err.Error())
}
