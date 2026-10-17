package deploy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// apcRequest is an Astro Private Cloud deploy under the domain
// astro.corp.example, of a project linking links.
func apcRequest(links map[string]manifest.Link) Request {
	return Request{
		Manifest:         manifestWith(links),
		Platform:         manifest.KindAPC,
		Domain:           "astro.corp.example",
		ContextWorkspace: "ws-ctx",
	}
}

// An Astro Private Cloud deploy resolves the project's apc links on its own
// domain exactly as an Astro deploy resolves astro links: named, offered with
// the default preselected, refused when it cannot ask, and saved for a
// Deployment picked from the workspace. Links of any other kind, or on
// another domain, are not its to ship to.
func TestResolveTargetAPC(t *testing.T) {
	links := map[string]manifest.Link{
		"prod":  {Target: "apc", Domain: "astro.corp.example", Workspace: "ws-prod", Deployment: "dep-prod", Default: true},
		"stage": {Target: "apc", Domain: "astro.corp.example", Workspace: "ws-stage", Deployment: "dep-stage"},
		"other": {Target: "apc", Domain: "astro.other.example", Workspace: "ws-other", Deployment: "dep-other"},
		"cloud": {Target: "astro", Workspace: "ws-cloud", Deployment: "dep-cloud"},
	}

	t.Run("a named apc link", func(t *testing.T) {
		d := &fakeDeployer{}
		target, err := resolveTarget(withName(apcRequest(links), "stage"), d)
		require.NoError(t, err)
		assert.Equal(t, Target{DeploymentID: "dep-stage", WorkspaceID: "ws-stage", LinkName: "stage"}, target)
		assert.Zero(t, d.confirms)
	})

	t.Run("an id falls through, in the context's workspace", func(t *testing.T) {
		target, err := resolveTarget(withName(apcRequest(links), "ckdep123"), &fakeDeployer{})
		require.NoError(t, err)
		assert.Equal(t, Target{DeploymentID: "ckdep123", WorkspaceID: "ws-ctx"}, target)
	})

	t.Run("an astro link is not an APC deploy's", func(t *testing.T) {
		_, err := resolveTarget(withName(apcRequest(links), "cloud"), &fakeDeployer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `link "cloud" is astro, and astro deploy ships to Astro Private Cloud Deployments on astro.corp.example`)
		assert.Contains(t, err.Error(), "apc links on astro.corp.example: prod, stage")
	})

	t.Run("an apc link on another domain says how to reach it", func(t *testing.T) {
		req := withName(apcRequest(links), "other")
		_, err := resolveTarget(req, &fakeDeployer{})
		require.EqualError(t, err, `link "other" is on astro.other.example, and this deploy runs under astro.corp.example`)

		reach := errors.New("run astro context switch astro.other.example")
		req.OtherDomain = func(link, domain string) error {
			assert.Equal(t, "other", link)
			assert.Equal(t, "astro.other.example", domain)
			return reach
		}
		_, err = resolveTarget(req, &fakeDeployer{})
		require.ErrorIs(t, err, reach)
	})

	t.Run("the prompt offers this domain's apc links, the default first and preselected", func(t *testing.T) {
		req := apcRequest(links)
		req.Interactive = true
		d := &fakeDeployer{answer: "prod", listed: []Deployment{
			{ID: "dep-prod", Name: "Prod"},
			{ID: "dep-new", Name: "New one"},
		}}
		target, err := resolveTarget(req, d)
		require.NoError(t, err)
		assert.Equal(t, "dep-prod", target.DeploymentID)
		assert.Equal(t, "prod", d.preselected)
		assert.Equal(t, []Choice{
			{Name: "prod", Where: "apc deployment dep-prod"},
			{Name: "stage", Where: "apc deployment dep-stage"},
			{Name: "new-one", Where: "apc deployment dep-new", Unlinked: true},
		}, d.offered, "the linked Deployment is not offered twice, and nothing off this domain is offered")
		assert.Equal(t, "ws-ctx", d.listedWS, "[tool.astro] workspace is an Astro one, so the context's is listed")
	})

	t.Run("a run that cannot ask names the deployable links", func(t *testing.T) {
		_, err := resolveTarget(apcRequest(links), &fakeDeployer{})
		require.Error(t, err)
		var required *input.RequiredError
		require.ErrorAs(t, err, &required)
		assert.Contains(t, err.Error(), "Deployable links: prod, stage")
	})

	t.Run("a project with links, none of them this domain's apc ones, is refused", func(t *testing.T) {
		req := apcRequest(map[string]manifest.Link{
			"cloud": {Target: "astro", Workspace: "ws-cloud", Deployment: "dep-cloud"},
			"other": links["other"],
		})
		req.Interactive = true
		d := &fakeDeployer{}
		_, err := resolveTarget(req, d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "astro deploy ships to Astro Private Cloud Deployments on astro.corp.example, and this project links none (cloud is astro, other is apc on astro.other.example)")
		assert.Contains(t, err.Error(), "astro link add")
		assert.Zero(t, d.confirms+d.resolves, "nothing is offered")
	})

	t.Run("a picked Deployment is saved as an apc link on the domain", func(t *testing.T) {
		req := apcRequest(nil)
		req.Interactive = true
		var saved scaffold.Link
		req.Link = func(l scaffold.Link) error { saved = l; return nil }
		d := &fakeDeployer{unlinkedID: "dep-new"}
		target, err := resolveTarget(req, d)
		require.NoError(t, err)
		require.NotNil(t, target.newLink)
		saveNewLink(req, target, &Result{})
		assert.Equal(t, manifest.KindAPC, saved.Kind)
		assert.Equal(t, "astro.corp.example", saved.Domain)
		assert.Equal(t, "ws-ctx", saved.Workspace, "an apc link names its own workspace")
		assert.Equal(t, "dep-new", saved.Deployment)
	})
}

func withName(req Request, name string) Request {
	req.LinkName = name
	return req
}

// A both deploy whose Deployment took no DAGs from it is reported as the
// image-only deploy it was.
func TestRunReportsAnImageThatShippedNoDagsAsImageOnly(t *testing.T) {
	for _, tc := range []struct {
		notShipped bool
		want       string
	}{
		{false, "image-and-dag"},
		{true, "image-only"},
	} {
		d := &fakeDeployer{img: ImageResult{DagsNotShipped: tc.notShipped}}
		res, err := Run(Request{Deployment: "dep", ContextWorkspace: "ws", ImageName: "img:1"}, d)
		require.NoError(t, err)
		assert.Equal(t, tc.want, res.Type)
	}
}
