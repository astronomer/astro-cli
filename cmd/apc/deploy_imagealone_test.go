package apc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// From a project's root, a prebuilt image's Deployment resolves as a
// project's build does, through the project's apc links: a link name, as
// the argument or with --deployment, in the link's workspace, and the
// result names the link. Local or already in a registry alike.
func TestDeployImageNameToALink(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"--image-name", []string{"--image-name", "img:1"}},
		{"--image-name --remote", []string{"--image-name", "quay.example.com/img:1", "--remote", "--runtime-version", "3.1-5"}},
	} {
		for _, named := range [][]string{{"stage"}, {"--deployment", "stage"}} {
			t.Run(tc.name, func(t *testing.T) {
				linkedProject(t)
				seen := deployMocks(t, deployPushed, nil)
				args := append(append(append([]string{"deploy"}, named...), tc.args...), "-o", "json")
				run := runAPC(t, newAPCClient(), "", args...)
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, "stage", got.Link)
				assert.Equal(t, "dep-stage", got.Deployment)
				assert.Equal(t, "ws-stage", got.Workspace)
				require.Len(t, seen.images, 1)
				assert.Equal(t, "dep-stage", seen.images[0].DeploymentID)
				assert.Equal(t, "ws-stage", seen.images[0].WorkspaceID)
				assert.Equal(t, tc.args[1], seen.images[0].ImageName)
			})
		}
	}
}

// Asked, a prebuilt image's deploy offers the project's apc links, the
// default preselected, as a build's does.
func TestDeployImageNameAsksWithTheDefaultPreselected(t *testing.T) {
	linkedProject(t)
	prev := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = prev })
	seen := deployMocks(t, deployPushed, nil)
	api := newAPCClient()
	api.On("ListDeployments", houston.ListDeploymentsRequest{WorkspaceID: deployWorkspace}).Return([]houston.Deployment{{ID: "dep-prod", Label: "Prod"}}, nil)
	for _, args := range [][]string{
		{"deploy", "--image-name", "img:1"},
		{"deploy", "--image-name", "quay.example.com/img:1", "--remote", "--runtime-version", "3.1-5"},
	} {
		run := runAPC(t, api, "\n", args...)
		require.Equal(t, 0, run.code, "%v stderr:\n%s", args, run.stderr)
		assert.Contains(t, run.stderr, "→ prod (apc deployment dep-prod)", "%v", args)
	}
	require.Len(t, seen.images, 2)
	for _, in := range seen.images {
		assert.Equal(t, "dep-prod", in.DeploymentID, "Enter takes the preselected default")
	}
}

// An apc link on another domain is refused with the way there, for a
// prebuilt image as for a build.
func TestDeployImageNameToALinkOnAnotherDomain(t *testing.T) {
	linkedProject(t)
	seen := deployMocks(t, deployPushed, nil)
	run := runAPC(t, newAPCClient(), "", "deploy", "elsewhere", "--image-name", "img:1", "--remote", "--runtime-version", "3.1-5", "-o", "json")
	require.Equal(t, 1, run.code)
	assert.Contains(t, run.stdout, "run astro login astro.other.example")
	assert.Empty(t, seen.images)
}

// Below a pyproject.toml project's root every deploy is refused, --image-name
// included, with the project's root to run from.
func TestDeployBelowAProjectRoot(t *testing.T) {
	deployMocks(t, deployPushed, nil)
	dir := inProject(t, true)
	prev := config.WorkingPath
	config.WorkingPath = filepath.Join(dir, "dags")
	t.Cleanup(func() { config.WorkingPath = prev })
	run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "Run the deploy from the project directory, "+dir)
}

// --dags with --image-name is refused in astro deploy's words, before
// anything is asked: a deploy of DAGs alone has no use for an image.
func TestDeployRefusesDagsWithImageName(t *testing.T) {
	inProject(t, true)
	seen := deployMocks(t, deployPushed, nil)
	run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--dags", "--image-name", "img:1", "-o", "json")
	require.Error(t, run.err)
	assert.EqualError(t, run.err, "--dags deploys only your Dags; drop --image-name")
	assert.True(t, cliout.IsUsage(run.err))
	assert.Zero(t, seen.dagUploads)

	// --remote names an image in a registry: the same mistake, said the same
	// way.
	for _, args := range [][]string{
		{"deploy", "dep-ac", "--dags", "--remote", "-o", "json"},
		{"deploy", "dep-ac", "--dags", "--remote", "--image-name", "img:1", "-o", "json"},
	} {
		run = runAPC(t, newAPCClient(), "", args...)
		require.Error(t, run.err, "%v", args)
		assert.True(t, cliout.IsUsage(run.err), "%v", args)
		assert.Contains(t, run.err.Error(), "--dags deploys only your Dags; drop ", "%v", args)
		assert.Contains(t, run.err.Error(), "--remote", "%v", args)
	}
	assert.Zero(t, seen.dagUploads)
	assert.Empty(t, seen.images)
}

// --build-secret has nothing to mount into an image already built, from
// outside any project as from a project's root.
func TestDeployRefusesBuildSecretWithImageNameOutsideAProject(t *testing.T) {
	inWorkingDir(t, true)
	seen := deployMocks(t, deployPushed, nil)
	run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "--build-secret", "id=s,src=s.txt", "-o", "json")
	require.Error(t, run.err)
	assert.EqualError(t, run.err, "--build-secret has no effect with --image-name: the image is already built")
	assert.Empty(t, seen.images)
}
