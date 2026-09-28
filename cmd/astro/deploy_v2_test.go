package astro

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	v2deploy "github.com/astronomer/astro-cli/internal/deploy"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/instances"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const v2ManifestForRouting = `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
`

// actionDeploymentID is a Deployment id as astronomer/deploy-action passes it:
// the positional argument, naming no manifest link.
const actionDeploymentID = "cexampledeployment0000002"

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

// v2ManifestWithDefaultLink is a v2 project with two links, one marked default.
// Deploy never resolves from the marker, so the render tests below
// name their target with --deployment; the marker's only job here is
// preselecting the prompt, which TestDeployV2PromptPreselectsDefault drives.
const v2ManifestWithDefaultLink = `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*", "pandas"]

[tool.astro]
workspace = "clw-ws"
packages = ["libpq-dev"]

[tool.astro.deployments.prod]
deployment = "clx-dep"
default = true

[tool.astro.deployments.dev]
deployment = "clx-dev"
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

func (f *fakeCmdDeployer) ConfirmTarget([]v2deploy.Choice, v2deploy.Preselect) (string, error) {
	return "", errors.New("the transport should not be asked when the target is named")
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
	setupV2DeployWith(t, d, v2ManifestWithDefaultLink)
}

// setupV2DeployWith is setupV2Deploy over a manifest of the test's choosing.
func setupV2DeployWith(t *testing.T, d v2deploy.Deployer, toml string) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(toml), 0o600))

	origPath := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = origPath })

	origDeployer := newV2Deployer
	newV2Deployer = func(astrov1.APIClient, io.Reader, io.Writer) v2deploy.Deployer { return d }
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

// execDeployIO runs the deploy command with its three streams under the test's
// control: answers is what stdin hands the prompt, and stdout and stderr come
// back apart, because the ordering this command has to get right is a prompt on
// one and a progress line on the other.
func execDeployIO(answers string, args ...string) (out, errOut string, err error) {
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader(answers))
	cmd.SetArgs(args)
	_, err = cmd.ExecuteC()
	return outBuf.String(), errBuf.String(), err
}

// interactiveDeploy makes this run one that can be asked a question, the way a
// terminal would.
func interactiveDeploy(t *testing.T) {
	t.Helper()
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = orig })
}

func TestDeployV2JSONImageAndDag(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026-07-23T18-40",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
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

	out, err := execDeployCapture("--deployment", "prod", "--image", "--output", "json")
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

	out, err := execDeployCapture("--deployment", "prod", "--dags", "--output", "json")
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

	out, err := execDeployCapture("--deployment", "prod", "--image-name", "astro-package/demo:3.1-2-abc", "--output", "json")
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

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
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
	// routes to the v2 path, which fails at manifest.Load, so this locks in that
	// json mode still keeps stdout/stderr to the one error object.
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

	out, errOut, err := execDeployIO("", "prod")
	require.NoError(t, err)

	// stdout is this command's own output: the progress line, the summary, the
	// URL — and nothing else.
	want := "Building your project image, this can take a few minutes...\n" +
		"Deployed image (tag deploy-2026-07-23T18-40) and DAGs (version 3-1690000000) to prod (deployment clx-dep).\n" +
		"Deployment: https://cloud.astronomer.io/deployments/clx-dep\n"
	assert.Equal(t, want, out)
	// The target goes on stderr, the way every resolving command announces one.
	assert.Equal(t, "→ prod (astro deployment clx-dep)\n", errOut)
}

// The announce line lands before the build line and after the target is
// settled, so the order a reader sees is: what I am about to act on, then what
// I am doing to it.
func TestDeployV2AnnouncesBeforeItBuilds(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

	out, errOut, err := execDeployIO("\n")
	require.NoError(t, err)
	assert.Contains(t, errOut, "→ prod (astro deployment clx-dep)")
	// The prompt comes first, the announce line after it — nothing is claimed
	// about a target before there is one.
	assert.Less(t, strings.Index(errOut, "Deploy to which deployment?"), strings.Index(errOut, "→ prod"))
	assert.Contains(t, out, "Building your project image")
}

// A deployment named by id has no link name to show, so the line carries the id
// alone rather than an empty parenthetical.
func TestDeployV2AnnouncesAnIDWithNoLinkName(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, errOut, err := execDeployIO("", "--deployment", "clx-not-a-link")
	require.NoError(t, err)
	assert.Equal(t, "→ clx-not-a-link\n", errOut)
	assert.Contains(t, out, "to deployment clx-not-a-link.")
}

// A declined prompt exits non-zero and says nothing: the user was asked and
// answered, and reading their answer back as an error adds nothing.
func TestDeployV2DeclinedPromptIsQuiet(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &v2deploy.ImageResult{})

	// Three answers that are not choices, which is how the prompt gives up.
	out, _, err := execDeployIO("nope\nnope\nnope\n")
	require.ErrorIs(t, err, v2deploy.ErrAborted)
	assert.Empty(t, out)
}

// The link name reaches the json object, so a consumer sees what the person
// typed and not only the id they would have to look up.
func TestDeployV2JSONCarriesTheLinkName(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	assert.Equal(t, "prod", m["link"])
	assert.Equal(t, "clx-dep", m["deployment"])
}

// A target named by id has no link, and the field is omitted rather than empty.
func TestDeployV2JSONOmitsAnAbsentLinkName(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "clx-not-a-link", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	_, has := m["link"]
	assert.False(t, has)
}

func TestDeployV2JSONCarriesTheCommit(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{
		ImageTag:          "tag",
		DagTarballVersion: "3-1",
		Git: v2deploy.Git{Commit: &v2deploy.Commit{
			SHA:    "0123abcd",
			Branch: "main",
			URL:    "https://github.com/account/repo/commit/0123abcd",
		}},
	}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	assert.Equal(t, map[string]any{
		"commit_sha": "0123abcd",
		"branch":     "main",
		"commit_url": "https://github.com/account/repo/commit/0123abcd",
	}, m["git"])
}

func TestDeployV2JSONOmitsGitWhenNoCommitIsRecorded(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{img: v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	_, has := m["git"]
	assert.False(t, has)
}

func TestToV2DeployGit(t *testing.T) {
	branch, url := "main", "https://github.com/account/repo/commit/0123abcd"
	got := toV2DeployGit(astrodeploy.DeployGitV2{Commit: &astrov1.CreateDeployGitRequest{
		CommitSha: "0123abcd",
		Branch:    &branch,
		CommitUrl: &url,
	}})
	assert.Equal(t, v2deploy.Git{Commit: &v2deploy.Commit{SHA: "0123abcd", Branch: branch, URL: url}}, got)

	assert.Equal(t, v2deploy.Git{Uncommitted: true}, toV2DeployGit(astrodeploy.DeployGitV2{Uncommitted: true}))
}

func TestDeployV2NotesUncommittedChanges(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			setupV2Deploy(t, &fakeCmdDeployer{dag: v2deploy.DagResult{
				DagTarballVersion: "3-1",
				Git:               v2deploy.Git{Uncommitted: true},
			}})

			out, errOut, err := execDeployIO("", "prod", "--dags", "--output", format)
			require.NoError(t, err)
			assert.Contains(t, errOut, "note: the project has uncommitted changes, so this deploy records no git commit\n")
			assert.NotContains(t, out, "uncommitted")
		})
	}
}

// The highlight is labeled with what put it there. ASTRO_DEPLOYMENT outranks
// the manifest's marker for the cursor, and saying "default" over an entry an
// exported variable chose tells the reader their committed file says something
// it does not.
func TestDeployV2PromptNamesWhatMovedTheCursor(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})
	t.Setenv(instances.EnvVar, "dev")

	out, errOut, err := execDeployIO("\n")
	require.NoError(t, err)
	assert.Contains(t, errOut, "dev (astro deployment clx-dev)  ← ASTRO_DEPLOYMENT")
	assert.NotContains(t, errOut, "← default")
	// And Enter took the highlighted entry, which is the one it named.
	assert.Contains(t, out, "to dev (deployment clx-dev).")
}

// A link may legally be called "2". Typing its name must select it, not the
// second entry in the list.
func TestDeployV2NumericLinkNameSelectsItself(t *testing.T) {
	interactiveDeploy(t)
	fake := &promptDeployer{fakeCmdDeployer: fakeCmdDeployer{img: v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}}}
	setupV2DeployWith(t, fake, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "clw-ws"

[tool.astro.deployments."2"]
deployment = "clx-two"

[tool.astro.deployments.prod]
deployment = "clx-prod"
`)
	origDeployer := newV2Deployer
	newV2Deployer = func(client astrov1.APIClient, in io.Reader, errOut io.Writer) v2deploy.Deployer {
		fake.prompt = v2Deployer{client: client, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newV2Deployer = origDeployer })

	// Sorted, "2" is offered first and "prod" second. Reading the answer as a
	// number would ship to prod.
	out, _, err := execDeployIO("2\n")
	require.NoError(t, err)
	assert.Contains(t, out, "to 2 (deployment clx-two).")
}

// Nothing to prompt about means nothing to read the pin for, and reading it
// creates the project's state directory on the way past.
func TestDeployPreselectSkipsTheDiskWhenNoQuestionIsComing(t *testing.T) {
	dir := t.TempDir()
	name, from := deployPreselect(dir, false)
	assert.Empty(t, name)
	assert.Empty(t, from)

	// The env var is free to read and outranks the pin, so it still answers.
	t.Setenv(instances.EnvVar, "prod")
	name, from = deployPreselect(dir, false)
	assert.Equal(t, "prod", name)
	assert.Equal(t, instances.EnvVar, from)
}

// promptDeployer answers nothing itself; the prompt under test is the real
// v2Deployer.ConfirmTarget, wired to the streams execDeployIO controls.
type promptDeployer struct {
	fakeCmdDeployer
	prompt v2deploy.Deployer
}

func (p *promptDeployer) ConfirmTarget(choices []v2deploy.Choice, preselect v2deploy.Preselect) (string, error) {
	return p.prompt.ConfirmTarget(choices, preselect)
}

// newPromptDeploy wires the real prompt onto the command's own streams, so a
// test drives the question a user would actually see.
func newPromptDeploy(t *testing.T, img *v2deploy.ImageResult) {
	t.Helper()
	fake := &promptDeployer{fakeCmdDeployer: fakeCmdDeployer{img: *img}}
	setupV2Deploy(t, fake)
	orig := newV2Deployer
	newV2Deployer = func(client astrov1.APIClient, in io.Reader, errOut io.Writer) v2deploy.Deployer {
		fake.prompt = v2Deployer{client: client, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newV2Deployer = orig })
}

func TestDeployV2PromptPreselectsDefaultAndAsksAnyway(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

	// The manifest marks prod default, so the prompt highlights it — and still
	// asks. An empty answer takes the highlighted entry.
	out, errOut, err := execDeployIO("\n")
	require.NoError(t, err)

	assert.Contains(t, errOut, "Deploy to which deployment?")
	assert.Contains(t, errOut, "prod (astro deployment clx-dep)  ← default = true")
	assert.Contains(t, errOut, "dev (astro deployment clx-dev)")
	assert.Contains(t, errOut, "Choose 1-2 [2]: ")
	assert.Contains(t, out, "to prod (deployment clx-dep).")
}

func TestDeployV2PromptWithOneLinkOffersOneNumber(t *testing.T) {
	var errOut bytes.Buffer
	prompt := v2Deployer{in: strings.NewReader("\n"), errOut: &errOut}

	name, err := prompt.ConfirmTarget(
		[]v2deploy.Choice{{Name: "test", Where: "astro deployment clx-dep"}},
		v2deploy.Preselect{Name: "test", From: "default = true"},
	)
	require.NoError(t, err)

	assert.Equal(t, "test", name)
	assert.Contains(t, errOut.String(), "Choose 1 [1]: ")
	assert.NotContains(t, errOut.String(), "1-1")
}

func TestDeployV2PromptTakesTheOtherLink(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &v2deploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

	out, _, err := execDeployIO("dev\n")
	require.NoError(t, err)
	assert.Contains(t, out, "to dev (deployment clx-dev).")
}

// The known ordering wart this issue names: the build line used to print before
// anything had been decided, so a deploy nobody agreed to still announced that
// it was building an image.
func TestDeployV2AbortedPromptSaysNothingAboutBuilding(t *testing.T) {
	interactiveDeploy(t)
	fake := &promptDeployer{}
	setupV2Deploy(t, fake)
	origDeployer := newV2Deployer
	newV2Deployer = func(client astrov1.APIClient, in io.Reader, errOut io.Writer) v2deploy.Deployer {
		fake.prompt = v2Deployer{client: client, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newV2Deployer = origDeployer })

	// Closed stdin: nobody answered.
	out, _, err := execDeployIO("")
	require.Error(t, err)
	assert.NotContains(t, out, "Building your project image")
	assert.Nil(t, fake.imgInput)
	assert.Nil(t, fake.dagInput)
}

func TestDeployV2NonInteractiveMustNameTheTarget(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{})

	out, err := execDeployCapture()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
	assert.Contains(t, err.Error(), "ASTRO_DEPLOYMENT")
	assert.Contains(t, err.Error(), "dev, prod")
	assert.NotContains(t, out, "Building your project image")
}

// A pin is ambient state, and ambient state never decides a deploy — not even
// the one the query commands would resolve to.
func TestDeployV2PinDoesNotDecideANonInteractiveDeploy(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{})
	t.Setenv(instances.EnvVar, "prod")

	_, err := execDeployCapture()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
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
	DeployImage = func(astrodeploy.InputDeploy, astrov1.APIClient, astrov1alpha1.APIClient) error {
		called = true
		return nil
	}

	err := execDeployCmd("test-deployment-id", "-f", "--workspace-id", "test-ws")
	require.NoError(t, err)
	assert.True(t, called, "a v1 project should run the v1 deploy path")
}

// Eight flags reach astro deploy that the v2 path never reads. Accepting them
// silently means a deploy someone believes ran their tests, shipped from a
// DAGs path it never looked at, or saved a target it did not save. Each is
// refused with what to do instead until an earlier fix ports the ones worth porting.
//
// Eight rather than ten: --build-secret and --build-secrets are deliberately NOT
// in this table any more, because the v2 path READS them now. They still need a
// project Dockerfile to be mounted into, and that refusal lives with the other
// build-secret checks in TestDeployV2BuildSecretRefusals — gated on the flag
// being given, which is why it cannot be a row here.
func TestDeployRefusesFlagsTheV2PathIgnores(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--pytest"}, "uv run pytest"},
		{[]string{"--parse"}, "astro local check"},
		{[]string{"--dags-path", "./elsewhere"}, "not supported yet"},
		{[]string{"--dag-bundle-name", "nightly"}, "not supported on a v2 project yet"},
		{[]string{"--test", "tests/test_dags.py"}, "uv run pytest"},
		{[]string{"--env", ".env.ci"}, "runs no tests"},
		{[]string{"--save"}, "always asks"},
		{[]string{"--deployment-name", "prod"}, "--deployment"},
	}

	for _, tc := range cases {
		t.Run(tc.args[0], func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			resetDeployFlagVars()

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(v2ManifestForRouting), 0o600))
			orig := config.WorkingPath
			config.WorkingPath = dir
			t.Cleanup(func() { config.WorkingPath = orig })

			out, err := execDeployCapture(append([]string{actionDeploymentID}, tc.args...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "has no effect when deploying a v2 project")
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, out, "Usage:", "a refusal prints the error, not the help")
		})
	}
}

// --force and --prompt are read by neither path but refused by neither either.
// The v2 path has no uncommitted-changes gate for --force to open, and it always
// asks, which is what --prompt requested — so both get the outcome the flag
// asked for and refusing them would break CI that passes them out of habit.
func TestDeployAcceptsForceAndPromptOnAV2Project(t *testing.T) {
	for _, flag := range []string{"--force", "--prompt"} {
		t.Run(flag, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			resetDeployFlagVars()

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(v2ManifestForRouting), 0o600))
			orig := config.WorkingPath
			config.WorkingPath = dir
			t.Cleanup(func() { config.WorkingPath = orig })

			err := execDeployCmd(flag)
			if err != nil {
				assert.NotContains(t, err.Error(), "has no effect when deploying a v2 project")
			}
		})
	}
}

// The same flags still work on a v1 project, where the v1 deploy path reads
// them. The refusal is on the v2 branch only.
func TestDeployAllowsThoseFlagsOnAV1Project(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir() // no pyproject.toml, so the v1 path
	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	err := execDeployCmd("--pytest")
	if err != nil {
		assert.NotContains(t, err.Error(), "has no effect when deploying a v2 project")
	}
}

// --build-secret is refused where it cannot work, and only when the USER asked.
//
// Three regressions are pinned here, all from validating in internal/deploy
// instead of at the flags:
//
//   - --dags never reaches runImage, so the refusal there let the flag be
//     silently dropped by a dags-only deploy.
//   - --image-name returns before any build, so the same silent drop applied
//     whenever the project declared a Dockerfile.
//   - the env-var row is the important one. ResolveBuildSecrets reads
//     BUILD_SECRET_INPUT whether or not the flag was given, so a refusal keyed on
//     the resolved slice hard-failed every ordinary v2 deploy on any runner
//     exporting that variable. No flag, no Dockerfile, and an error telling the
//     user to declare one they never wanted.
func TestDeployV2BuildSecretRefusals(t *testing.T) {
	const manifestWithDockerfile = "[project]\nname = 'p'\nversion = '0.1.0'\ndependencies = ['apache-airflow==3.1.*']\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n"

	for _, tc := range []struct {
		name     string
		body     string
		args     []string
		env      string
		wantErr  string
		wantPass bool
	}{
		{
			name:    "no dockerfile declared",
			body:    v2ManifestForRouting,
			args:    []string{"--build-secret", "id=pypi"},
			wantErr: "needs a project Dockerfile",
		},
		{
			name:    "with --dags",
			body:    manifestWithDockerfile,
			args:    []string{"--dags", "--build-secret", "id=pypi"},
			wantErr: "no effect with --dags",
		},
		{
			name:    "with --image-name",
			body:    manifestWithDockerfile,
			args:    []string{"--image-name", "my:tag", "--build-secret", "id=pypi"},
			wantErr: "no effect with --image-name",
		},
		{
			// The flag was not given. An ambient variable must not turn an
			// ordinary deploy into an error about a Dockerfile.
			name:     "BUILD_SECRET_INPUT set but no flag",
			body:     v2ManifestForRouting,
			args:     nil,
			env:      "id=pypi,src=/tmp/x",
			wantPass: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			resetDeployFlagVars()
			if tc.env != "" {
				t.Setenv("BUILD_SECRET_INPUT", tc.env)
			}

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(tc.body), 0o600))
			orig := config.WorkingPath
			config.WorkingPath = dir
			t.Cleanup(func() { config.WorkingPath = orig })

			err := execDeployCmd(tc.args...)
			if tc.wantPass {
				// It gets past the flag checks. Whatever it fails on afterwards
				// is a deploy concern, not a build-secret one — which is the
				// whole assertion.
				if err != nil {
					assert.NotContains(t, err.Error(), "build-secret",
						"an ambient BUILD_SECRET_INPUT must not be refused: %v", err)
				}
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// astronomer/deploy-action runs `astro deploy $DEPLOYMENT_ID ...`, so the
// positional takes a Deployment id the way --deployment does, whatever links the
// project declares.
func TestDeployV2PositionalDeploymentID(t *testing.T) {
	linkTo := func(dep string) string {
		return v2ManifestForRouting + "workspace = \"clw-ws\"\n\n[tool.astro.deployments.test]\ndeployment = \"" + dep + "\"\n"
	}
	for _, tc := range []struct {
		name     string
		manifest string
	}{
		{"no links", v2ManifestForRouting},
		{"a link to another deployment", linkTo("clx-other")},
		{"a link to the same deployment", linkTo(actionDeploymentID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCmdDeployer{}
			setupV2DeployWith(t, fake, tc.manifest)

			_, err := execDeployCapture(actionDeploymentID, "--dags")
			require.NoError(t, err)
			require.NotNil(t, fake.dagInput)
			assert.Equal(t, actionDeploymentID, fake.dagInput.DeploymentID)
		})
	}
}

func TestDeployV2PositionalAndFlagMustAgree(t *testing.T) {
	setupV2Deploy(t, &fakeCmdDeployer{})

	_, err := execDeployCapture("clx-a", "--deployment", "clx-b", "--dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name one")
}

// Each shape astronomer/deploy-action v0.16.0 runs, against a v2 project.
func TestDeployV2DeployActionInvocations(t *testing.T) {
	withDockerfile := strings.Replace(v2ManifestWithDefaultLink, "[tool.astro]\n", "[tool.astro]\ndockerfile = \"Dockerfile\"\n", 1)
	for _, tc := range []struct {
		name        string
		manifest    string
		args        []string
		wantDags    bool
		wantImage   bool
		includeDags bool
	}{
		{name: "dags", manifest: v2ManifestWithDefaultLink, args: []string{actionDeploymentID, "--wait", "--dags", "--force"}, wantDags: true},
		{name: "image", manifest: v2ManifestWithDefaultLink, args: []string{actionDeploymentID, "--wait", "--image", "--force"}, wantImage: true},
		{
			name:        "image and dags",
			manifest:    withDockerfile,
			args:        []string{actionDeploymentID, "--wait", "--force", "--build-secret", "id=x,env=Y", "--description", "d"},
			wantImage:   true,
			includeDags: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCmdDeployer{}
			setupV2DeployWith(t, fake, tc.manifest)
			t.Setenv("Y", "secret")
			require.NoError(t, os.WriteFile(filepath.Join(config.WorkingPath, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1-1\n"), 0o600))

			_, err := execDeployCapture(tc.args...)
			require.NoError(t, err)

			if tc.wantDags {
				require.NotNil(t, fake.dagInput)
				assert.Equal(t, actionDeploymentID, fake.dagInput.DeploymentID)
				assert.True(t, fake.dagInput.Wait)
			} else {
				assert.Nil(t, fake.dagInput)
			}
			if !tc.wantImage {
				assert.Nil(t, fake.imgInput)
				return
			}
			require.NotNil(t, fake.imgInput)
			assert.Equal(t, actionDeploymentID, fake.imgInput.DeploymentID)
			assert.True(t, fake.imgInput.Wait)
			assert.Equal(t, tc.includeDags, fake.imgInput.IncludeDags)
			if tc.includeDags {
				assert.Equal(t, "Dockerfile", fake.imgInput.Dockerfile)
				assert.Equal(t, []string{"id=x,env=Y"}, fake.imgInput.BuildSecrets)
				assert.Equal(t, "d", fake.imgInput.Description)
			}
		})
	}
}
