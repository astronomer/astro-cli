package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// apcTable is the prod-apc link's table as the file spells it, read from the
// raw TOML rather than the parsed manifest, which folds defaults in.
func apcTable(t *testing.T, path string) map[string]any {
	const name = "prod-apc"
	t.Helper()
	ed, err := tomledit.NewSurgical([]byte(readFile(t, path)))
	require.NoError(t, err)
	v, ok := ed.Get(linkKey(name))
	require.True(t, ok, "no link %q in:\n%s", name, readFile(t, path))
	table, ok := v.(map[string]any)
	require.True(t, ok, "link %q is not a table", name)
	return table
}

func apcLink(name string) Link {
	return Link{
		Name: name, Kind: manifest.KindAPC,
		Domain: "astro.corp.example", Workspace: "ckws-apc", Deployment: "ckdep-apc",
	}
}

// An apc link round-trips with every field it carries, and comes back as an
// apc link proving itself with the APC login, the kind's default.
func TestSaveLinkRoundTripsAnAPCLink(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, apcLink("prod-apc"))
	name, got := onlyLink(t, path)
	assert.Equal(t, "prod-apc", name)
	assert.Equal(t, manifest.Link{
		Target: "apc", Domain: "astro.corp.example", Workspace: "ckws-apc", Deployment: "ckdep-apc",
		Auth: manifest.Auth{Method: manifest.AuthAPC},
	}, got)
	assert.Equal(t, manifest.KindAPC, got.Kind())
	out := readFile(t, path)
	assert.Contains(t, out, userComment)
	assert.NotContains(t, out, "auth", "the kind's default method needs no table")
}

// An apc link writes its own domain and workspace even where [tool.astro]
// sets both: those describe Astro, and the parser never lets them reach an
// apc link. Its target is written too, since the project's is astro.
func TestSaveLinkAnAPCLinkWritesItsOwnDomainAndWorkspace(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	l := apcLink("prod-apc")
	l.Workspace = "ws-default" // the same id as [tool.astro] workspace, still written
	saveLink(t, dir, l)
	assert.Equal(t, map[string]any{
		"target": "apc", "domain": "astro.corp.example", "workspace": "ws-default", "deployment": "ckdep-apc",
	}, apcTable(t, path))
	m := loadLinks(t, path)
	assert.Equal(t, "astronomer.io", m.Astro.Domain, "the project's Astro host is left alone")
}

// The domain is written the way astro login stores an APC login.
func TestSaveLinkNormalizesAnAPCDomain(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	l := apcLink("prod-apc")
	l.Domain = " https://Astro.Corp.Example/ "
	saveLink(t, dir, l)
	assert.Equal(t, "astro.corp.example", apcTable(t, path)["domain"])
}

// Re-saved without a workspace, an apc link keeps the one it has; a new one
// with none is refused, since it would inherit nothing.
func TestSaveLinkAnAPCLinkKeepsItsWorkspace(t *testing.T) {
	dir, path := linkProject(t, linkFixture)
	saveLink(t, dir, apcLink("prod-apc"))
	again := apcLink("prod-apc")
	again.Workspace, again.Deployment = "", "ckdep-other"
	saveLink(t, dir, again)
	_, got := onlyLink(t, path)
	assert.Equal(t, "ckws-apc", got.Workspace)
	assert.Equal(t, "ckdep-other", got.Deployment)

	fresh := apcLink("other")
	fresh.Workspace = ""
	err := SaveLink(dir, nil, fresh)
	require.ErrorIs(t, err, ErrInvalidLink)
	assert.NotContains(t, loadLinks(t, path).Astro.Deployments, "other")
}

func TestSaveLinkRefusesAnIncompleteAPCLink(t *testing.T) {
	for name, edit := range map[string]func(*Link){
		"no domain":            func(l *Link) { l.Domain = "" },
		"a domain of a scheme": func(l *Link) { l.Domain = "https://" },
		"no deployment":        func(l *Link) { l.Deployment = "" },
		"the reserved name":    func(l *Link) { l.Name = manifest.ReservedLinkName },
		"the bundle name":      func(l *Link) { l.Name = BundleLinkName },
	} {
		t.Run(name, func(t *testing.T) {
			dir, path := linkProject(t, linkFixture)
			l := apcLink("prod-apc")
			edit(&l)
			require.ErrorIs(t, SaveLink(dir, nil, l), ErrInvalidLink)
			assert.Equal(t, linkFixture, readFile(t, path), "a refused save writes nothing")
		})
	}
}

// An apc link added beside a project's only link marks that one default, as
// any second link does, and SetDefaultLink moves the mark onto it.
func TestSaveLinkAnAPCLinkBesideAnAstroLink(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	saveLink(t, dir, Link{Name: "dev", Kind: manifest.KindAstro, Deployment: "clx123"})
	saveLink(t, dir, apcLink("prod-apc"))
	m := loadLinks(t, path)
	assert.True(t, m.Astro.Deployments["dev"].Default)
	assert.Equal(t, "ws-default", m.Astro.Deployments["dev"].Workspace)
	assert.Equal(t, manifest.KindAPC, m.Astro.Deployments["prod-apc"].Kind())

	require.NoError(t, SetDefaultLink(dir, nil, "prod-apc"))
	name, _, ok := manifest.DefaultLink(loadLinks(t, path).Astro.Deployments)
	assert.True(t, ok)
	assert.Equal(t, "prod-apc", name)
}

// Switching the project's Astro workspace pins the astro links inheriting it
// and leaves an apc link, which inherits nothing, as it was.
func TestSetWorkspaceLinkLeavesAnAPCLinkAlone(t *testing.T) {
	dir, path := linkProject(t, inheritingFixture)
	saveLink(t, dir, apcLink("prod-apc"))
	before := apcTable(t, path)
	pinned, err := SetWorkspaceLink(dir, nil, "ws-other", "astronomer.io", "")
	require.NoError(t, err)
	assert.Empty(t, pinned)
	assert.Equal(t, before, apcTable(t, path))
}
