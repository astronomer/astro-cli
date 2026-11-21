package astro

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
)

const manifestWithBundleLink = `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "clw-ws"

[tool.astro.deployments.prod]
deployment = "clx-dep"
dag-bundle = "nightly"

[tool.astro.deployments.dev]
deployment = "clx-dev"
`

func bundleDagResult() manifestdeploy.DagResult {
	return manifestdeploy.DagResult{WorkspaceID: "clw-ws", RuntimeVersion: "3.1-2", DagTarballVersion: "3-1690000000"}
}

// --dag-bundle reaches the transport on a Dags-only deploy, and the result
// names the bundle in both modes.
func TestDeployDagBundleDagsOnly(t *testing.T) {
	fake := &fakeCmdDeployer{dag: bundleDagResult()}
	setupManifestDeploy(t, fake)

	out, err := execDeployCapture("--deployment", "prod", "--dags", "--dag-bundle", "nightly")
	require.NoError(t, err)
	require.NotNil(t, fake.dagInput)
	assert.Equal(t, "nightly", fake.dagInput.DagBundle)
	assert.Contains(t, out, "Deployed Dags (version 3-1690000000) to Dag bundle nightly on prod (Deployment clx-dep).")

	resetDeployFlagVars()
	out, err = execDeployCapture("--deployment", "prod", "--dags", "--dag-bundle", "nightly", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	assert.Equal(t, "nightly", m["dag_bundle"])
	assert.Equal(t, "dag-only", m["type"])
}

// A deploy that names no bundle sends none and its JSON has no dag_bundle.
func TestDeployWithoutDagBundleSendsNone(t *testing.T) {
	fake := &fakeCmdDeployer{dag: bundleDagResult()}
	setupManifestDeploy(t, fake)

	out, err := execDeployCapture("--deployment", "prod", "--dags", "--output", "json")
	require.NoError(t, err)
	assert.Empty(t, fake.dagInput.DagBundle)
	_, has := decodeOneJSON(t, out)["dag_bundle"]
	assert.False(t, has, "a default-bundle deploy should omit dag_bundle")
}

// A full deploy (image and Dags) takes the bundle too.
func TestDeployDagBundleImageAndDags(t *testing.T) {
	fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{WorkspaceID: "clw-ws", RuntimeVersion: "3.1-2", ImageTag: "t1", DagTarballVersion: "3-1"}}
	setupManifestDeploy(t, fake)

	out, err := execDeployCapture("--deployment", "prod", "--dag-bundle", "nightly")
	require.NoError(t, err)
	require.NotNil(t, fake.imgInput)
	assert.Equal(t, "nightly", fake.imgInput.DagBundle)
	assert.True(t, fake.imgInput.IncludeDags)
	assert.Contains(t, out, "Deployed image (tag t1) and Dags (version 3-1) to Dag bundle nightly on prod")

	// A prebuilt --image-name inside a project ships the Dags as well, so it
	// takes the bundle.
	resetDeployFlagVars()
	_, err = execDeployCapture("--deployment", "prod", "--image-name", "img:1", "--dag-bundle", "nightly")
	require.NoError(t, err)
	assert.Equal(t, "nightly", fake.imgInput.DagBundle)
}

// 1.x's --dag-bundle-name is a hidden alias: the command reads --dag-bundle,
// the run says what to type instead, and the rename is recorded.
func TestDeployDagBundleNameAlias(t *testing.T) {
	var renamed, current []string
	orig := utils.RecordRenamedFlags
	utils.RecordRenamedFlags = func(_ *cobra.Command, r, c []string) { renamed, current = r, c }
	t.Cleanup(func() { utils.RecordRenamedFlags = orig })

	f := NewDeployCmd().Flags().Lookup("dag-bundle-name")
	require.NotNil(t, f)
	assert.True(t, f.Hidden)
	assert.Equal(t, "dag-bundle", utils.AliasOf(f))

	fake := &fakeCmdDeployer{dag: bundleDagResult()}
	setupManifestDeploy(t, fake)
	out, err := execDeployCapture("--deployment", "prod", "--dags", "--dag-bundle-name", "nightly")
	require.NoError(t, err)
	assert.Equal(t, "nightly", fake.dagInput.DagBundle)
	assert.Contains(t, out, "--dag-bundle-name")
	assert.Contains(t, out, "use --dag-bundle")
	assert.Equal(t, []string{"--dag-bundle-name"}, renamed)
	assert.Equal(t, []string{"--dag-bundle"}, current)
}

// Both spellings that disagree are a usage error, as for every alias.
func TestDeployDagBundleAliasDisagreeing(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{dag: bundleDagResult()})
	_, err := execDeployCapture("--deployment", "prod", "--dags", "--dag-bundle", "a", "--dag-bundle-name", "b")
	require.Error(t, err)
	assert.True(t, cliout.IsUsage(err), err.Error())
}

// A bundle named for no Dags is refused before anything is resolved.
func TestDeployDagBundleRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"with --image":                      {[]string{"--image", "--dag-bundle", "nightly"}, "cannot use --dag-bundle with --image"},
		"with --image-name outside":         {[]string{"--image-name", "img:1", "--dag-bundle", "nightly"}, "outside a pyproject.toml project"},
		"an empty name":                     {[]string{"--dag-bundle", ""}, "--dag-bundle cannot be empty"},
		"with --image and the 1.x spelling": {[]string{"--image", "--dag-bundle-name", "nightly"}, "cannot use --dag-bundle with --image"},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{}, dag: bundleDagResult()}
			if name == "with --image-name outside" {
				// No pyproject.toml: the walk finds no project.
				setupManifestDeploy(t, fake)
				deployIn(t, t.TempDir())
			} else {
				setupManifestDeploy(t, fake)
			}
			args := append([]string{"--deployment", "prod"}, tc.args...)
			_, err := execDeployCapture(args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.True(t, cliout.IsUsage(err), err.Error())
			assert.Nil(t, fake.dagInput)
			assert.Nil(t, fake.imgInput)
		})
	}
}

// The link's dag-bundle is the default for a deploy to it, and --dag-bundle
// overrides it for one run.
func TestDeployDagBundleFromTheLink(t *testing.T) {
	fake := &fakeCmdDeployer{dag: bundleDagResult()}
	setupManifestDeployWith(t, fake, manifestWithBundleLink)

	out, err := execDeployCapture("prod", "--dags", "--output", "json")
	require.NoError(t, err)
	assert.Equal(t, "nightly", fake.dagInput.DagBundle)
	assert.Equal(t, "prod", fake.dagInput.DagBundleLink, "a refusal names the link that set it")
	assert.Equal(t, "nightly", decodeOneJSON(t, out)["dag_bundle"])

	resetDeployFlagVars()
	_, err = execDeployCapture("prod", "--dags", "--dag-bundle", "weekly")
	require.NoError(t, err)
	assert.Equal(t, "weekly", fake.dagInput.DagBundle)
	assert.Empty(t, fake.dagInput.DagBundleLink, "the flag named it, not the link")

	// A link with none ships to the default bundle.
	resetDeployFlagVars()
	_, err = execDeployCapture("dev", "--dags")
	require.NoError(t, err)
	assert.Empty(t, fake.dagInput.DagBundle)
}

// --image leaves the running Dags alone, so a link's bundle has nothing to
// apply to and the image deploys as it always did.
func TestDeployImageIgnoresTheLinksDagBundle(t *testing.T) {
	fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{WorkspaceID: "clw-ws", ImageTag: "t1"}}
	setupManifestDeployWith(t, fake, manifestWithBundleLink)

	_, err := execDeployCapture("prod", "--image")
	require.NoError(t, err)
	require.NotNil(t, fake.imgInput)
	assert.Empty(t, fake.imgInput.DagBundle)
}

// A deploy that ships no Dags says, once and on stderr, that the link's
// dag-bundle was not used. stdout stays the result alone, in json too.
func TestDeployImageNotesTheLinksSkippedDagBundle(t *testing.T) {
	const note = "dag-bundle = \"nightly\" under [tool.astro.deployments.prod] in pyproject.toml was not used: --image ships no Dags"
	for _, args := range [][]string{{"prod", "--image"}, {"prod", "--image", "--output", "json"}} {
		fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{WorkspaceID: "clw-ws", ImageTag: "t1"}}
		setupManifestDeployWith(t, fake, manifestWithBundleLink)
		out, errOut, err := execDeployIO("", args...)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(errOut, note), "%v: %s", args, errOut)
		assert.NotContains(t, out, "dag-bundle", "%v", args)
	}

	// No note for a link with none, nor for a deploy that ships Dags.
	for _, args := range [][]string{{"dev", "--image"}, {"prod", "--dags"}} {
		fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{WorkspaceID: "clw-ws"}, dag: bundleDagResult()}
		setupManifestDeployWith(t, fake, manifestWithBundleLink)
		_, errOut, err := execDeployIO("", args...)
		require.NoError(t, err)
		assert.NotContains(t, errOut, "was not used", "%v", args)
	}
}
