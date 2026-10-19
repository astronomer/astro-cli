package apc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// The domain runAPC's context is on, and so the one this project's apc links
// are deployable on.
const testAPCDomain = "astronomer_dev.com"

// linkedProject is a pyproject.toml project at the working directory's root,
// pinning Airflow 3.1, with an apc link prod (the default) and stage on the
// context's domain, one on another domain, and an astro link.
func linkedProject(t *testing.T) string {
	t.Helper()
	dir := inWorkingDir(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(`[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]

[tool.astro.deployments.prod]
target = "apc"
domain = "`+testAPCDomain+`"
workspace = "ws-prod"
deployment = "dep-prod"
default = true

[tool.astro.deployments.stage]
target = "apc"
domain = "`+testAPCDomain+`"
workspace = "ws-stage"
deployment = "dep-stage"

[tool.astro.deployments.elsewhere]
target = "apc"
domain = "astro.other.example"
workspace = "ws-other"
deployment = "dep-other"

[tool.astro.deployments.cloud]
workspace = "ws-cloud"
deployment = "dep-cloud"
`), 0o600))
	return dir
}

// manifestDeploys replaces the platform half of a project's image deploy for
// one test, recording what it was asked and answering res.
func manifestDeploys(t *testing.T, res deploy.ManifestImageResult) *[]deploy.ManifestImage {
	t.Helper()
	prev := DeployManifestImage
	t.Cleanup(func() { DeployManifestImage = prev })
	asked := new([]deploy.ManifestImage)
	DeployManifestImage = func(_ houston.ClientInterface, in *deploy.ManifestImage) (deploy.ManifestImageResult, error) {
		*asked = append(*asked, *in)
		if in.Warn != nil && res.Airflow3Upgrade {
			in.Warn("this deploy upgrades Deployment " + in.DeploymentID + " from Airflow 2 to Airflow 3")
		}
		return res, nil
	}
	return asked
}

var builtAndPushed = deploy.ManifestImageResult{
	RuntimeVersion: "3.1-5",
	Image:          "registry.example.com/rel/airflow:deploy-7",
	URL:            "https://airflow",
	DagsInImage:    true,
}

// A pyproject.toml project's deploy builds and ships to the apc link it
// names, in the link's workspace, and publishes what it shipped in Astro's
// spellings.
func TestDeployManifestToANamedLink(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "stage", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)

	var got deployJSON
	decodeOne(t, run.stdout, &got)
	assert.Equal(t, deployJSON{
		Deployment: "dep-stage", Link: "stage", Workspace: "ws-stage", Type: "image-and-dag",
		Image: builtAndPushed.Image, DagsInImage: true, RuntimeVersion: "3.1-5", URL: "https://airflow",
	}, got)
	require.Len(t, *asked, 1)
	in := (*asked)[0]
	assert.Equal(t, "dep-stage", in.DeploymentID)
	assert.Equal(t, "ws-stage", in.WorkspaceID)
	assert.True(t, in.IncludeDags)
	assert.Equal(t, "3.1", in.Build.AirflowVersion)
	assert.Equal(t, "Deployed via <astro deploy>", in.Description)
}

// In text it says what it shipped where, after the deploy's own progress.
func TestDeployManifestText(t *testing.T) {
	linkedProject(t)
	manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "--deployment", "prod")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.Equal(t, "Deployed image registry.example.com/rel/airflow:deploy-7 to prod (Deployment dep-prod), with the Dags inside the image.\nDeployment: https://airflow\n", run.stdout)
	assert.Contains(t, run.stderr, "→ prod (apc deployment dep-prod)")
}

// An id the project does not link deploys there, in --workspace's
// workspace, or its older spelling's, which must agree.
func TestDeployManifestToAnID(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "ckdep1", "--workspace", "ws-flag", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	require.Len(t, *asked, 1)
	assert.Equal(t, "ckdep1", (*asked)[0].DeploymentID)
	assert.Equal(t, "ws-flag", (*asked)[0].WorkspaceID)

	run = runAPC(t, newAPCClient(), "", "deploy", "ckdep1", "--workspace-id", "ws-old", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.Equal(t, "ws-old", (*asked)[1].WorkspaceID)

	run = runAPC(t, newAPCClient(), "", "deploy", "ckdep1", "--workspace", "ws-a", "--workspace-id", "ws-b", "-o", "json")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stdout, `--workspace \"ws-a\" and --workspace-id \"ws-b\" disagree`)

	run = runAPC(t, newAPCClient(), "", "deploy", "prod", "--deployment", "stage", "-o", "json")
	assert.Equal(t, 2, run.code)
	assert.Contains(t, run.stdout, "disagree")
	assert.Len(t, *asked, 2, "neither refused run deployed")
}

// Deploy never picks for you: a run that cannot ask and names no Deployment
// is input_required, naming the links it could have named, and deploys
// nothing.
func TestDeployManifestNeverPicks(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "-o", "json")
	require.Equal(t, 1, run.code)
	var got struct {
		Error string `json:"error"`
		Kind  string `json:"kind"`
	}
	decodeOne(t, run.stdout, &got)
	assert.Equal(t, "input_required", got.Kind)
	assert.Contains(t, got.Error, "Deployable links: prod, stage")
	assert.Empty(t, *asked)
}

// Asked, it offers this domain's apc links, the default first and
// preselected, then the workspace's Deployments no link names; picking one
// of those links it.
func TestDeployManifestAsks(t *testing.T) {
	dir := linkedProject(t)
	prev := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = prev })
	asked := manifestDeploys(t, builtAndPushed)
	api := newAPCClient()
	api.On("ListDeployments", houston.ListDeploymentsRequest{WorkspaceID: "ck05r3bor07h40d02y2hw4n4v"}).Return([]houston.Deployment{
		{ID: "dep-prod", Label: "Prod"},
		{ID: "dep-new", Label: "New One", Workspace: houston.Workspace{ID: "ck05r3bor07h40d02y2hw4n4v"}},
	}, nil)

	run := runAPC(t, api, "\n", "deploy")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	require.Len(t, *asked, 1)
	assert.Equal(t, "dep-prod", (*asked)[0].DeploymentID, "Enter takes the preselected default")
	assert.Contains(t, run.stderr, "apc deployment dep-new")
	assert.NotContains(t, run.stderr, "dep-other", "a link on another domain is not offered")
	assert.NotContains(t, run.stderr, "dep-cloud", "an astro link is not offered")

	run = runAPC(t, api, "new-one\n", "deploy")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.Equal(t, "dep-new", (*asked)[1].DeploymentID)
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	link := m.Astro.Deployments["new-one"]
	assert.Equal(t, manifest.KindAPC, link.Kind())
	assert.Equal(t, testAPCDomain, link.Domain)
	assert.Equal(t, "dep-new", link.Deployment)
	assert.Equal(t, "ck05r3bor07h40d02y2hw4n4v", link.Workspace)
}

// An apc link on another domain is one this context's Houston does not
// reach: the deploy says how to reach it.
func TestDeployManifestLinkOnAnotherDomain(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "elsewhere", "-o", "json")
	require.Equal(t, 1, run.code)
	assert.Contains(t, run.stdout, "run astro login astro.other.example")
	assert.Empty(t, *asked)
}

// A link on a domain this machine has a context for is reached by switching
// to it; on one it has none for, by logging in there. Whether there is one
// is read off the config, not the login.
func TestOtherDomainNamesTheWayThere(t *testing.T) {
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	assert.ErrorContains(t, otherDomain("elsewhere", "astro.other.example"), "run astro login astro.other.example")

	// A second context in an in-memory config, with no login stored for it.
	fs := afero.NewMemMapFs()
	cfg := strings.Replace(string(testUtil.NewTestConfig(testUtil.SoftwarePlatform)), "contexts:\n", "contexts:\n  astro_other_example:\n    domain: astro.other.example\n", 1)
	require.NoError(t, afero.WriteFile(fs, config.HomeConfigFile, []byte(cfg), 0o600))
	config.InitConfig(fs)
	t.Cleanup(func() { testUtil.InitTestConfig(testUtil.SoftwarePlatform) })
	assert.ErrorContains(t, otherDomain("elsewhere", "astro.other.example"), "run astro context switch astro.other.example")
}

// --dags uploads the project's DAGs to the link named, with no build.
func TestDeployManifestDags(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	seen := deployMocks(t, deployPushed, nil)
	run := runAPC(t, newAPCClient(), "", "deploy", "prod", "--dags", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	var got deployJSON
	decodeOne(t, run.stdout, &got)
	assert.Equal(t, deployJSON{Deployment: "dep-prod", Link: "prod", Workspace: "ws-prod", Type: "dag-only"}, got)
	assert.Equal(t, 1, seen.dagUploads)
	assert.NotNil(t, seen.opts.WarnEmptyDags, "an empty dags directory is warned about, not asked about")
	assert.Empty(t, *asked)
}

// An Airflow 2 to 3 upgrade, and anything else the deploy warns of, is in
// the json result as well as on stderr.
func TestDeployManifestWarnings(t *testing.T) {
	linkedProject(t)
	upgraded := builtAndPushed
	upgraded.Airflow3Upgrade = true
	manifestDeploys(t, upgraded)
	run := runAPC(t, newAPCClient(), "", "deploy", "prod", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	var got deployJSON
	decodeOne(t, run.stdout, &got)
	assert.Equal(t, []string{"this deploy upgrades Deployment dep-prod from Airflow 2 to Airflow 3"}, got.Warnings)
	assert.Contains(t, run.stderr, "Warning: this deploy upgrades Deployment dep-prod from Airflow 2 to Airflow 3")
}

// --force and --prompt answered 1.x's questions, which a project's deploy
// does not ask: accepted, so deploy-action keeps working, hidden, and read
// by nothing, as on Astro.
func TestDeployManifestTakesForceAndPrompt(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "prod", "--force", "-p", "-o", "json")
	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	require.Len(t, *asked, 1)
	assert.Equal(t, "dep-prod", (*asked)[0].DeploymentID)

	// Under json, naming none is input_required, --prompt or not.
	run = runAPC(t, newAPCClient(), "", "deploy", "--prompt", "-o", "json")
	require.Equal(t, 1, run.code)
	assert.Contains(t, run.stdout, "input_required")

	cmd := NewDeployCmd(os.Stdout)
	for _, name := range []string{"force", "prompt", "workspace-id"} {
		assert.True(t, cmd.Flags().Lookup(name).Hidden, name)
	}
	assert.Equal(t, "Has no effect; accepted so scripts written for Astro CLI 1.x keep working", cmd.Flags().Lookup("prompt").Usage)
	for _, name := range []string{"save", "no-cache", "wait"} {
		assert.Nil(t, cmd.Flags().Lookup(name), name)
	}
}

// --build-secret is refused where nothing reads it: a dags-only deploy, an
// image already built, and a generated build for a secret other than the
// one its install step mounts.
func TestDeployManifestRefusesBuildSecretsNothingReads(t *testing.T) {
	linkedProject(t)
	asked := manifestDeploys(t, builtAndPushed)
	for _, tc := range []struct {
		args []string
		says string
	}{
		{[]string{"--dags"}, "--build-secret has no effect with --dags"},
		{[]string{"--image-name", "img:1"}, "--build-secret has no effect with --image-name"},
		{nil, "reads only the netrc build secret"},
	} {
		args := append(append([]string{"deploy", "prod", "--build-secret", "id=mysecret,src=secret.txt"}, tc.args...), "-o", "json")
		run := runAPC(t, newAPCClient(), "", args...)
		require.NotEqual(t, 0, run.code, "%v", tc.args)
		assert.Contains(t, run.stdout, tc.says, "%v", tc.args)
	}
	assert.Empty(t, *asked)
}

// A project whose manifest does not load is refused with why, before
// anything is asked.
func TestDeployManifestRefusesAManifestThatDoesNotLoad(t *testing.T) {
	dir := inWorkingDir(t, true)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	asked := manifestDeploys(t, builtAndPushed)
	run := runAPC(t, newAPCClient(), "", "deploy", "dep", "-o", "json")
	require.Equal(t, 1, run.code)
	assert.True(t, strings.Contains(run.stdout, "names no Airflow"), run.stdout)
	assert.Empty(t, *asked)
}

// --dags with --image is the usage mistake it always was.
func TestDeployManifestRefusesDagsWithImage(t *testing.T) {
	linkedProject(t)
	run := runAPC(t, newAPCClient(), "", "deploy", "prod", "--dags", "--image", "-o", "json")
	assert.Equal(t, 2, run.code)
	assert.True(t, cliout.IsUsage(run.err))
}

// asking is a terminal for one test: the deploy may ask.
func asking(t *testing.T) {
	t.Helper()
	prev := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = prev })
}

// promptAPI is a Houston whose workspace lists dep-old and dep-new, which no
// link of linkedProject names.
func promptAPI() *mocks.ClientInterface {
	api := newAPCClient()
	api.On("ListDeployments", houston.ListDeploymentsRequest{WorkspaceID: "ck05r3bor07h40d02y2hw4n4v"}).Return([]houston.Deployment{
		{ID: "dep-old", Label: "Old One", Workspace: houston.Workspace{ID: "ck05r3bor07h40d02y2hw4n4v"}},
		{ID: "dep-new", Label: "New One", Workspace: houston.Workspace{ID: "ck05r3bor07h40d02y2hw4n4v"}},
	}, nil)
	return api
}

// An interactive deploy that names its Deployment is not asked, --prompt or
// not: naming it is the answer. Naming none, it asks, --prompt or not, the
// default link preselected.
func TestDeployNamedIsNotAsked(t *testing.T) {
	for _, prompt := range [][]string{nil, {"--prompt"}} {
		linkedProject(t)
		asking(t)
		asked := manifestDeploys(t, builtAndPushed)
		run := runAPC(t, promptAPI(), "prod\n", append([]string{"deploy", "stage"}, prompt...)...)
		require.Equal(t, 0, run.code, "%v stderr:\n%s", prompt, run.stderr)
		assert.False(t, run.stdinRead, "%v: nothing is asked", prompt)
		require.Len(t, *asked, 1)
		assert.Equal(t, "dep-stage", (*asked)[0].DeploymentID, "%v", prompt)

		run = runAPC(t, promptAPI(), "\n", append([]string{"deploy"}, prompt...)...)
		require.Equal(t, 0, run.code, "%v stderr:\n%s", prompt, run.stderr)
		assert.True(t, run.stdinRead, "%v: asked", prompt)
		require.Len(t, *asked, 2)
		assert.Equal(t, "dep-prod", (*asked)[1].DeploymentID, "%v: Enter takes the default", prompt)
	}
}
