package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/config"
	v2deploy "github.com/astronomer/astro-cli/internal/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const v2ManifestForRouting = `[project]
name = "demo"

[tool.astro]
airflow = "3.1"
`

// resetDeployFlagVars zeroes the package-level deploy flag vars so a routing
// test does not inherit flag state a prior test left behind (cobra binds these
// vars once and never clears them between runs).
func resetDeployFlagVars() {
	dags = false
	image = false
	imageName = ""
	v2Deployment = ""
	v2Workspace = ""
	noDagsBaseDir = false
	waitForDeploy = false
	forceDeploy = false
	forcePrompt = false
	workspaceID = ""
	deploymentName = ""
	deployDescription = ""
	nonDags = false
	deployOutput = string(formatText)
}

// v2ManifestWithDefaultLink is a v2 project whose one link is the default, so
// `astro deploy` resolves a deployment with no prompt and no --deployment — the
// setup every json/text render test below shares.
const v2ManifestWithDefaultLink = `[project]
name = "demo"
dependencies = ["pandas"]

[tool.astro]
airflow = "3.1"
workspace = "clw-ws"
packages = ["libpq-dev"]

[tool.astro.deployments.prod]
deployment = "clx-dep"
default = true
`

// fakeCmdDeployer stands in for the real transport so a cmd-level test drives
// flag parsing, selection, and rendering without a daemon, registry, or API.
type fakeCmdDeployer struct {
	dag      v2deploy.DagResult
	img      v2deploy.ImageResult
	dagErr   error
	imgErr   error
	dagInput *v2deploy.DagDeploy
	imgInput *v2deploy.ImageDeploy
}

func (f *fakeCmdDeployer) ResolveUnlinked(string) (string, error) { return "", nil }

func (f *fakeCmdDeployer) DeployDags(in *v2deploy.DagDeploy) (v2deploy.DagResult, error) {
	f.dagInput = in
	return f.dag, f.dagErr
}

func (f *fakeCmdDeployer) DeployImage(in *v2deploy.ImageDeploy) (v2deploy.ImageResult, error) {
	f.imgInput = in
	return f.img, f.imgErr
}

// setupV2Deploy points config.WorkingPath at a fresh v2 project (one default
// link) and swaps the transport for d, restoring both on cleanup. Each test
// gets its own mock.
func setupV2Deploy(t *testing.T, d v2deploy.Deployer) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(v2ManifestWithDefaultLink), 0o600))

	origPath := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = origPath })

	origDeployer := newV2Deployer
	newV2Deployer = func(astrov1.APIClient) v2deploy.Deployer { return d }
	t.Cleanup(func() { newV2Deployer = origDeployer })
}

// execDeployCapture runs the deploy command with stdout and stderr captured, so
// a test can read the rendered output instead of the process's real streams.
func execDeployCapture(args ...string) (string, error) {
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestDeployV2JSONImageAndDag(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026-07-23T18-40",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}})

	out, err := execDeployCapture("--output", "json")
	require.NoError(t, err)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "clx-dep", m["deployment"])
	assert.Equal(t, "clw-ws", m["workspace"])
	assert.Equal(t, "image-and-dag", m["type"])
	assert.Equal(t, "deploy-2026-07-23T18-40", m["image_tag"])
	assert.Equal(t, "3-1690000000", m["dag_bundle_version"])
	assert.Equal(t, "3.1-2", m["runtime_version"])
	assert.Equal(t, "https://cloud.astronomer.io/deployments/clx-dep", m["url"])
}

func TestDeployV2JSONImageOnly(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{
		WorkspaceID:    "clw-ws",
		RuntimeVersion: "3.1-2",
		ImageTag:       "deploy-2026-07-23T18-40",
		URL:            "https://cloud.astronomer.io/deployments/clx-dep",
	}})

	out, err := execDeployCapture("--image", "--output", "json")
	require.NoError(t, err)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "image-only", m["type"])
	assert.Equal(t, "deploy-2026-07-23T18-40", m["image_tag"])
	// An image-only deploy ships no dags, so the field is omitted, not empty.
	_, hasDag := m["dag_bundle_version"]
	assert.False(t, hasDag, "image-only deploy should omit dag_bundle_version")
}

func TestDeployV2JSONDagsOnly(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{dag: v2deploy.DagResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}})

	out, err := execDeployCapture("--dags", "--output", "json")
	require.NoError(t, err)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "dag-only", m["type"])
	assert.Equal(t, "3-1690000000", m["dag_bundle_version"])
	// A dags-only deploy builds no image, so image_tag is omitted.
	_, hasImage := m["image_tag"]
	assert.False(t, hasImage, "dags-only deploy should omit image_tag")
}

func TestDeployV2JSONImageName(t *testing.T) {
	fake := &fakeCmdDeployer{img: v2deploy.ImageResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026-07-23T18-40",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}}
	setupV2Deploy(t, fake)

	out, err := execDeployCapture("--image-name", "astro-package/demo:3.1-2-abc", "--output", "json")
	require.NoError(t, err)

	// The prebuilt ref reaches the transport, and --image-name without --image
	// still ships dags, so the kind is image-and-dag.
	require.NotNil(t, fake.imgInput)
	assert.Equal(t, "astro-package/demo:3.1-2-abc", fake.imgInput.ImageName)
	assert.True(t, fake.imgInput.IncludeDags)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "image-and-dag", m["type"])
	assert.Equal(t, "3-1690000000", m["dag_bundle_version"])
}

func TestDeployV2JSONError(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{imgErr: errors.New("build boom")})

	out, err := execDeployCapture("--output", "json")
	require.Error(t, err)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "build boom", m["error"])
	assert.Equal(t, float64(1), m["code"])
	// json mode carries the whole failure in the one object: no cobra "Error:".
	assert.NotContains(t, out, "Error:")
}

func TestDeployV2JSONManifestErrorNoUsage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	// [tool.astro] with no airflow version fails manifest validation. It still
	// routes to the v2 path, which fails at manifest.Load — before deployV2 sets
	// SilenceUsage — so this locks in that json mode still keeps stdout/stderr to
	// the one error object.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	origPath := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = origPath })

	out, err := execDeployCapture("--output", "json")
	require.Error(t, err)

	m := decodeOneJSON(t, out)
	assert.NotEmpty(t, m["error"])
	assert.Equal(t, float64(1), m["code"])
	assert.NotContains(t, out, "Usage:")
	assert.NotContains(t, out, "Error:")
}

func TestDeployV2TextUnchanged(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026-07-23T18-40",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}})

	out, err := execDeployCapture()
	require.NoError(t, err)

	// The text path is byte-for-byte what it printed before --output landed: the
	// progress line, then the summary, then the URL.
	want := "Building your project image, this can take a few minutes...\n" +
		"Deployed image (tag deploy-2026-07-23T18-40) and DAGs (version 3-1690000000) to deployment clx-dep.\n" +
		"Deployment: https://cloud.astronomer.io/deployments/clx-dep\n"
	assert.Equal(t, want, out)
}

// decodeOneJSON parses out as a single JSON object and fails if it is not
// exactly one — a json-mode command emits one object and nothing else.
func decodeOneJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(out)))
	var m map[string]any
	require.NoError(t, dec.Decode(&m))
	require.False(t, dec.More(), "json mode should emit exactly one object, got extra: %q", out)
	return m
}

func TestDeployRoutesV2Project(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(v2ManifestForRouting), 0o600))

	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	// The manifest names no deployment and the run is non-interactive (go test
	// has no TTY), so the v2 path stops at selection asking for --deployment —
	// before any build or transport work. That message is unique to the v2 path,
	// so it proves routing did not fall through to v1.
	err := execDeployCmd()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--deployment")
}

func TestDeployRoutesV1Project(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir() // no pyproject.toml, so not a v2 project

	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	origEnsure := EnsureProjectDir
	origDeploy := DeployImage
	t.Cleanup(func() {
		EnsureProjectDir = origEnsure
		DeployImage = origDeploy
	})

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	called := false
	DeployImage = func(cloud.InputDeploy, astrov1.APIClient, astrov1alpha1.APIClient) error {
		called = true
		return nil
	}

	err := execDeployCmd("test-deployment-id", "-f", "--workspace-id", "test-ws")
	require.NoError(t, err)
	assert.True(t, called, "a v1 project should run the v1 deploy path")
}
