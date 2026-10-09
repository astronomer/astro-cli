package astro

import (
	"bytes"
	"context"
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

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/instances"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const manifestForRouting = `[project]
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
	manifestDeployment = ""
	manifestWorkspace = ""
	noDagsBaseDir = false
	waitForDeploy = false
	workspaceID = ""
	deploymentName = ""
	deployDescription = ""
	nonDags = false
	deployOutput = string(cliout.FormatText)
}

// manifestWithDefaultLink is a project with two links, one marked default.
// Deploy never resolves from the marker, so the render tests below
// name their target with --deployment; the marker's only job here is
// preselecting the prompt, which TestDeployManifestPromptPreselectsDefault drives.
const manifestWithDefaultLink = `[project]
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
	dag    manifestdeploy.DagResult
	img    manifestdeploy.ImageResult
	dagErr error
	imgErr error
	// refuseBeforeBuild returns imgErr the way the transport refuses a deploy
	// the deployment will not take: before the build starts.
	refuseBeforeBuild bool
	dagInput          *manifestdeploy.DagDeploy
	imgInput          *manifestdeploy.ImageDeploy
}

func (f *fakeCmdDeployer) ConfirmTarget([]manifestdeploy.Choice, manifestdeploy.Preselect) (string, error) {
	return "", errors.New("the transport should not be asked when the target is named")
}

func (f *fakeCmdDeployer) ResolveUnlinked(string) (string, error) { return "", nil }

func (f *fakeCmdDeployer) DeployDags(in *manifestdeploy.DagDeploy) (manifestdeploy.DagResult, error) {
	f.dagInput = in
	return f.dag, f.dagErr
}

func (f *fakeCmdDeployer) DeployImage(in *manifestdeploy.ImageDeploy) (manifestdeploy.ImageResult, error) {
	f.imgInput = in
	if f.refuseBeforeBuild {
		return manifestdeploy.ImageResult{}, f.imgErr
	}
	if in.ImageName == "" && in.OnBuild != nil {
		in.OnBuild()
	}
	return f.img, f.imgErr
}

// setupManifestDeploy points config.WorkingPath at a fresh project (one default
// link) and swaps the transport for d, restoring both on cleanup. Each test
// gets its own mock.
func setupManifestDeploy(t *testing.T, d manifestdeploy.Deployer) {
	t.Helper()
	setupManifestDeployWith(t, d, manifestWithDefaultLink)
}

// setupManifestDeployWith is setupManifestDeploy over a manifest of the test's choosing.
func setupManifestDeployWith(t *testing.T, d manifestdeploy.Deployer, toml string) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(toml), 0o600))

	origPath := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = origPath })

	origDeployer := newManifestDeployer
	newManifestDeployer = func(*deployLogin, io.Reader, io.Writer) manifestdeploy.Deployer { return d }
	t.Cleanup(func() { newManifestDeployer = origDeployer })
}

// execDeployCapture runs the deploy command with stdout and stderr captured, so
// a test can read the rendered output instead of the process's real streams.
func execDeployCapture(args ...string) (string, error) {
	defer func() { workspaceID = "" }() // see execDeployCmd
	testUtil.SetupOSArgsForGinkgo()
	root := deployUnderRoot()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	// Through the reporting the root does, so a json-mode failure is the
	// object a user gets.
	err := cliout.Execute(context.Background(), root, append([]string{"deploy"}, args...), &buf, nil)
	return buf.String(), err
}

// deployUnderRoot mounts deploy under a root, as the CLI does. Execute treats
// a failing root differently from a failing subcommand (it forces the root's
// SilenceUsage for the run), so deploy run as the root would not be the
// command a user runs.
func deployUnderRoot() *cobra.Command {
	root := &cobra.Command{Use: "astro"}
	root.AddCommand(NewDeployCmd())
	return root
}

// execDeployIO runs the deploy command with its three streams under the test's
// control: answers is what stdin hands the prompt, and stdout and stderr come
// back apart, because the ordering this command has to get right is a prompt on
// one and a progress line on the other.
func execDeployIO(answers string, args ...string) (out, errOut string, err error) {
	defer func() { workspaceID = "" }() // see execDeployCmd
	testUtil.SetupOSArgsForGinkgo()
	root := deployUnderRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(answers))
	err = cliout.Execute(context.Background(), root, append([]string{"deploy"}, args...), &outBuf, nil)
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

func TestDeployManifestJSONImageAndDag(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{
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

func TestDeployManifestJSONImageOnly(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{
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

func TestDeployManifestJSONDagsOnly(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{dag: manifestdeploy.DagResult{
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

func TestDeployManifestJSONImageName(t *testing.T) {
	fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{
		WorkspaceID:       "clw-ws",
		RuntimeVersion:    "3.1-2",
		ImageTag:          "deploy-2026-07-23T18-40",
		DagTarballVersion: "3-1690000000",
		URL:               "https://cloud.astronomer.io/deployments/clx-dep",
	}}
	setupManifestDeploy(t, fake)

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

func TestDeployManifestJSONError(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{imgErr: errors.New("build boom")})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.Error(t, err)

	m := decodeOneJSON(t, out)
	assert.Equal(t, "build boom", m["error"])
	assert.Equal(t, float64(1), m["code"])
	// json mode carries the whole failure in the one object: no cobra "Error:".
	assert.NotContains(t, out, "Error:")
}

func TestDeployManifestJSONManifestErrorNoUsage(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	// [tool.astro] with no airflow version fails manifest validation. It still
	// routes to the manifest path, which fails at manifest.Load, so this locks in that
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

func TestDeployManifestTextUnchanged(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{
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
func TestDeployManifestAnnouncesBeforeItBuilds(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

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
func TestDeployManifestAnnouncesAnIDWithNoLinkName(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, errOut, err := execDeployIO("", "--deployment", "clx-not-a-link")
	require.NoError(t, err)
	assert.Equal(t, "→ clx-not-a-link\n", errOut)
	assert.Contains(t, out, "to deployment clx-not-a-link.")
}

// A declined prompt exits non-zero and says nothing: the user was asked and
// answered, and reading their answer back as an error adds nothing.
func TestDeployManifestDeclinedPromptIsQuiet(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &manifestdeploy.ImageResult{})

	// Three answers that are not choices, which is how the prompt gives up.
	out, _, err := execDeployIO("nope\nnope\nnope\n")
	require.ErrorIs(t, err, manifestdeploy.ErrAborted)
	assert.Empty(t, out)
}

// The link name reaches the json object, so a consumer sees what the person
// typed and not only the id they would have to look up.
func TestDeployManifestJSONCarriesTheLinkName(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	assert.Equal(t, "prod", m["link"])
	assert.Equal(t, "clx-dep", m["deployment"])
}

// A target named by id has no link, and the field is omitted rather than empty.
func TestDeployManifestJSONOmitsAnAbsentLinkName(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "clx-not-a-link", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	_, has := m["link"]
	assert.False(t, has)
}

func TestDeployManifestJSONCarriesTheCommit(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{
		ImageTag:          "tag",
		DagTarballVersion: "3-1",
		Git: manifestdeploy.Git{Commit: &manifestdeploy.Commit{
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

func TestDeployManifestJSONOmitsGitWhenNoCommitIsRecorded(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}})

	out, err := execDeployCapture("--deployment", "prod", "--output", "json")
	require.NoError(t, err)
	m := decodeOneJSON(t, out)
	_, has := m["git"]
	assert.False(t, has)
}

func TestToManifestDeployGit(t *testing.T) {
	branch, url := "main", "https://github.com/account/repo/commit/0123abcd"
	got := toManifestDeployGit(astrodeploy.ManifestDeployGit{Commit: &astrov1.CreateDeployGitRequest{
		CommitSha: "0123abcd",
		Branch:    &branch,
		CommitUrl: &url,
	}})
	assert.Equal(t, manifestdeploy.Git{Commit: &manifestdeploy.Commit{SHA: "0123abcd", Branch: branch, URL: url}}, got)

	assert.Equal(t, manifestdeploy.Git{Uncommitted: true}, toManifestDeployGit(astrodeploy.ManifestDeployGit{Uncommitted: true}))
}

func TestDeployManifestNotesUncommittedChanges(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			setupManifestDeploy(t, &fakeCmdDeployer{dag: manifestdeploy.DagResult{
				DagTarballVersion: "3-1",
				Git:               manifestdeploy.Git{Uncommitted: true},
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
func TestDeployManifestPromptNamesWhatMovedTheCursor(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})
	t.Setenv(instances.EnvVar, "dev")

	out, errOut, err := execDeployIO("\n")
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "dev", "astro", "deployment", "clx-dev", "ASTRO_DEPLOYMENT"}, pickerRow(t, errOut, "dev"))
	assert.NotContains(t, errOut, "default = true")
	assert.Contains(t, errOut, "\n> [1] ")
	// And Enter took the highlighted entry, which is the one it named.
	assert.Contains(t, out, "to dev (deployment clx-dev).")
}

// A link may legally be called "2". Typing its name must select it, not the
// second entry in the list.
func TestDeployManifestNumericLinkNameSelectsItself(t *testing.T) {
	interactiveDeploy(t)
	fake := &promptDeployer{fakeCmdDeployer: fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"}}}
	setupManifestDeployWith(t, fake, `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
workspace = "clw-ws"

[tool.astro.deployments."2"]
deployment = "clx-two"

[tool.astro.deployments.prod]
deployment = "clx-prod"
`)
	origDeployer := newManifestDeployer
	newManifestDeployer = func(login *deployLogin, in io.Reader, errOut io.Writer) manifestdeploy.Deployer {
		fake.prompt = manifestDeployer{login: login, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newManifestDeployer = origDeployer })

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
// manifestDeployer.ConfirmTarget, wired to the streams execDeployIO controls.
type promptDeployer struct {
	fakeCmdDeployer
	prompt manifestdeploy.Deployer
}

func (p *promptDeployer) ConfirmTarget(choices []manifestdeploy.Choice, preselect manifestdeploy.Preselect) (string, error) {
	return p.prompt.ConfirmTarget(choices, preselect)
}

// newPromptDeploy wires the real prompt onto the command's own streams, so a
// test drives the question a user would actually see.
func newPromptDeploy(t *testing.T, img *manifestdeploy.ImageResult) {
	t.Helper()
	fake := &promptDeployer{fakeCmdDeployer: fakeCmdDeployer{img: *img}}
	setupManifestDeploy(t, fake)
	orig := newManifestDeployer
	newManifestDeployer = func(login *deployLogin, in io.Reader, errOut io.Writer) manifestdeploy.Deployer {
		fake.prompt = manifestDeployer{login: login, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newManifestDeployer = orig })
}

func TestDeployManifestPromptPreselectsDefaultAndAsksAnyway(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

	// The manifest marks prod default, so the prompt highlights it — and still
	// asks. An empty answer takes the highlighted entry.
	out, errOut, err := execDeployIO("\n")
	require.NoError(t, err)

	assert.Contains(t, errOut, "Deploy to which deployment?")
	assert.Equal(t, []string{"#", "NAME", "WHERE", "PRESELECTED", "BY"}, strings.Fields(strings.Split(errOut, "\n")[1]))
	assert.Equal(t, []string{"2", "prod", "astro", "deployment", "clx-dep", "default", "=", "true"}, pickerRow(t, errOut, "prod"))
	assert.Equal(t, []string{"1", "dev", "astro", "deployment", "clx-dev"}, pickerRow(t, errOut, "dev"))
	assert.Contains(t, errOut, "\n> [2] ")
	assert.Contains(t, out, "to prod (deployment clx-dep).")
}

// pickerRow returns the fields of the deploy picker's row for name, its
// highlight stripped, failing when the table has no such row.
func pickerRow(t *testing.T, errOut, name string) []string {
	t.Helper()
	for _, line := range strings.Split(errOut, "\n") {
		line = strings.NewReplacer("\033[1;32m", "", "\033[0m", "").Replace(line)
		if f := strings.Fields(line); len(f) > 1 && f[1] == name {
			return f
		}
	}
	t.Fatalf("no row for %q in %q", name, errOut)
	return nil
}

// With nothing preselected there is no default on Enter and no column saying
// what preselected it: Enter is asked again, like any answer that is not a
// choice, and three of them end the deploy quietly.
func TestDeployManifestPromptWithNoPreselectNeedsAnAnswer(t *testing.T) {
	var errOut bytes.Buffer
	prompt := manifestDeployer{in: strings.NewReader("\n prod\nstaging\n"), errOut: &errOut}

	_, err := prompt.ConfirmTarget([]manifestdeploy.Choice{
		{Name: "dev", Where: "astro deployment clx-dev"},
		{Name: "prod", Where: "astro deployment clx-dep"},
	}, manifestdeploy.Preselect{})
	require.ErrorIs(t, err, manifestdeploy.ErrAborted)

	assert.NotContains(t, errOut.String(), "PRESELECTED BY")
	assert.NotContains(t, errOut.String(), "[", "no default shown in the prompt")
	assert.NotContains(t, errOut.String(), "\033[", "nothing highlighted")
	assert.Equal(t, 2, strings.Count(errOut.String(), "Not one of the choices.\n> "),
		"an empty answer and a padded name are each asked again: %q", errOut.String())
	assert.Equal(t, 3, strings.Count(errOut.String(), "Not one of the choices."),
		"the third wrong answer is told so too, before the deploy ends quietly: %q", errOut.String())
}

// Input that ends partway through an answer is read like any answer: a
// number still picks, and anything else ends the deploy with the message
// naming how to answer, not quietly.
func TestDeployManifestPromptReadsAnAnswerCutShort(t *testing.T) {
	choices := []manifestdeploy.Choice{
		{Name: "dev", Where: "astro deployment clx-dev"},
		{Name: "prod", Where: "astro deployment clx-dep"},
	}
	name, err := manifestDeployer{in: strings.NewReader("1"), errOut: &bytes.Buffer{}}.ConfirmTarget(choices, manifestdeploy.Preselect{})
	require.NoError(t, err)
	assert.Equal(t, "dev", name)

	_, err = manifestDeployer{in: strings.NewReader("prd"), errOut: &bytes.Buffer{}}.ConfirmTarget(choices, manifestdeploy.Preselect{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, manifestdeploy.ErrAborted)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
}

// A run that may not ask refuses before it prints the table, naming the flag
// that answers instead. resolveTarget refuses first on a non-interactive run;
// this is the picker's own check behind it.
func TestDeployManifestPromptRefusesWithoutPrintingWhenItMayNotAsk(t *testing.T) {
	restore := input.SetGuard(func() string { return "with --output json it cannot" })
	defer restore()
	var errOut bytes.Buffer
	prompt := manifestDeployer{in: strings.NewReader("1\n"), errOut: &errOut}

	_, err := prompt.ConfirmTarget(
		[]manifestdeploy.Choice{{Name: "test", Where: "astro deployment clx-dep"}},
		manifestdeploy.Preselect{Name: "test", From: "default = true"},
	)
	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "--deployment")
	assert.Empty(t, errOut.String())
}

// Ctrl-D at the prompt is no answer, so it does not take the preselected
// entry: it ends the deploy with the message naming how to answer instead.
func TestDeployManifestPromptEndedInputTakesNoDefault(t *testing.T) {
	prompt := manifestDeployer{in: strings.NewReader(""), errOut: &bytes.Buffer{}}

	_, err := prompt.ConfirmTarget(
		[]manifestdeploy.Choice{{Name: "test", Where: "astro deployment clx-dep"}},
		manifestdeploy.Preselect{Name: "test", From: "default = true"},
	)
	require.Error(t, err)
	assert.True(t, input.IsRequired(err), "%v", err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
}

func TestDeployManifestPromptWithOneLinkOffersOneNumber(t *testing.T) {
	var errOut bytes.Buffer
	prompt := manifestDeployer{in: strings.NewReader("\n"), errOut: &errOut}

	name, err := prompt.ConfirmTarget(
		[]manifestdeploy.Choice{{Name: "test", Where: "astro deployment clx-dep"}},
		manifestdeploy.Preselect{Name: "test", From: "default = true"},
	)
	require.NoError(t, err)

	assert.Equal(t, "test", name)
	assert.True(t, strings.HasSuffix(errOut.String(), "\n> [1] "), "%q", errOut.String())
}

func TestDeployManifestPromptTakesTheOtherLink(t *testing.T) {
	interactiveDeploy(t)
	newPromptDeploy(t, &manifestdeploy.ImageResult{ImageTag: "tag", DagTarballVersion: "3-1"})

	out, _, err := execDeployIO("dev\n")
	require.NoError(t, err)
	assert.Contains(t, out, "to dev (deployment clx-dev).")
}

// The known ordering wart this issue names: the build line used to print before
// anything had been decided, so a deploy nobody agreed to still announced that
// it was building an image.
func TestDeployManifestAbortedPromptSaysNothingAboutBuilding(t *testing.T) {
	interactiveDeploy(t)
	fake := &promptDeployer{}
	setupManifestDeploy(t, fake)
	origDeployer := newManifestDeployer
	newManifestDeployer = func(login *deployLogin, in io.Reader, errOut io.Writer) manifestdeploy.Deployer {
		fake.prompt = manifestDeployer{login: login, in: in, errOut: errOut}
		return fake
	}
	t.Cleanup(func() { newManifestDeployer = origDeployer })

	// Closed stdin: nobody answered.
	out, _, err := execDeployIO("")
	require.Error(t, err)
	assert.NotContains(t, out, "Building your project image")
	assert.Nil(t, fake.imgInput)
	assert.Nil(t, fake.dagInput)
}

// A deploy the deployment refuses before the build builds nothing, so it says
// nothing about building.
func TestDeployManifestRefusedBeforeTheBuildSaysNothingAboutBuilding(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{imgErr: errors.New("cannot deploy Astro Runtime 3.2"), refuseBeforeBuild: true})

	out, err := execDeployCapture("--deployment", "prod")
	require.ErrorContains(t, err, "cannot deploy Astro Runtime 3.2")
	assert.NotContains(t, out, "Building your project image")
}

func TestDeployManifestNonInteractiveMustNameTheTarget(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{})

	out, err := execDeployCapture()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
	assert.Contains(t, err.Error(), "ASTRO_DEPLOYMENT")
	assert.Contains(t, err.Error(), "dev, prod")
	assert.NotContains(t, out, "Building your project image")
}

// A pin is ambient state, and ambient state never decides a deploy — not even
// the one the query commands would resolve to.
func TestDeployManifestPinDoesNotDecideANonInteractiveDeploy(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{})
	t.Setenv(instances.EnvVar, "prod")

	_, err := execDeployCapture()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a deploy must name the deployment it ships to")
}

// Under --output json a deploy that names nothing is not asked, even at a
// terminal with an answer waiting: it fails as input_required, in deploy's own
// words, and exits 1 as it always has.
func TestDeployManifestJSONNeverAsksForTheTarget(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{})
	interactiveDeploy(t)

	out, errOut, err := execDeployIO("1\n", "--output", "json")
	require.Error(t, err)
	m := decodeOneJSON(t, out)
	assert.Equal(t, string(cliout.KindInputRequired), m["kind"])
	assert.Equal(t, float64(cliout.ExitFailure), m["code"])
	assert.Contains(t, m["error"], "a deploy must name the deployment it ships to")
	assert.NotContains(t, errOut, "Deploy to which deployment?")
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

func TestDeployRoutesManifestProject(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestForRouting), 0o600))

	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	// The manifest names no deployment and the run is non-interactive (go test
	// has no TTY), so the manifest path stops at selection asking for --deployment —
	// before any build or transport work. That message is unique to the manifest path,
	// so it proves routing did not fall through to the 1.x path.
	err := execDeployCmd()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--deployment")
}

// --force and --prompt are accepted and read by nothing: there is no
// uncommitted-changes gate for --force to open (and astronomer/deploy-action
// passes it on every deploy), and the deploy always asks, which is what
// --prompt requested. The run gets as far as asking for --deployment, as one
// without them does.
func TestDeployAcceptsForceAndPromptOnAManifestProject(t *testing.T) {
	for _, flag := range []string{"--force", "-f", "--prompt", "-p"} {
		t.Run(flag, func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			resetDeployFlagVars()

			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestForRouting), 0o600))
			orig := config.WorkingPath
			config.WorkingPath = dir
			t.Cleanup(func() { config.WorkingPath = orig })

			err := execDeployCmd(flag)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--deployment")
		})
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
//     the resolved slice hard-failed every ordinary manifest deploy on any runner
//     exporting that variable. No flag, no Dockerfile, and an error telling the
//     user to declare one they never wanted.
func TestDeployManifestBuildSecretRefusals(t *testing.T) {
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
			body:    manifestForRouting,
			args:    []string{"--build-secret", "id=pypi"},
			wantErr: "reads only the netrc build secret",
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
			body:     manifestForRouting,
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
func TestDeployManifestPositionalDeploymentID(t *testing.T) {
	linkTo := func(dep string) string {
		return manifestForRouting + "workspace = \"clw-ws\"\n\n[tool.astro.deployments.test]\ndeployment = \"" + dep + "\"\n"
	}
	for _, tc := range []struct {
		name     string
		manifest string
	}{
		{"no links", manifestForRouting},
		{"a link to another deployment", linkTo("clx-other")},
		{"a link to the same deployment", linkTo(actionDeploymentID)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCmdDeployer{}
			setupManifestDeployWith(t, fake, tc.manifest)

			_, err := execDeployCapture(actionDeploymentID, "--dags")
			require.NoError(t, err)
			require.NotNil(t, fake.dagInput)
			assert.Equal(t, actionDeploymentID, fake.dagInput.DeploymentID)
		})
	}
}

func TestDeployManifestPositionalAndFlagMustAgree(t *testing.T) {
	setupManifestDeploy(t, &fakeCmdDeployer{})

	_, err := execDeployCapture("clx-a", "--deployment", "clx-b", "--dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name one")
}

// Each shape astronomer/deploy-action v0.16.0 runs, against a project.
func TestDeployManifestDeployActionInvocations(t *testing.T) {
	withDockerfile := strings.Replace(manifestWithDefaultLink, "[tool.astro]\n", "[tool.astro]\ndockerfile = \"Dockerfile\"\n", 1)
	for _, tc := range []struct {
		name        string
		manifest    string
		args        []string
		wantDags    bool
		wantImage   bool
		includeDags bool
	}{
		{name: "dags", manifest: manifestWithDefaultLink, args: []string{actionDeploymentID, "--wait", "--dags", "--force"}, wantDags: true},
		{name: "image", manifest: manifestWithDefaultLink, args: []string{actionDeploymentID, "--wait", "--image", "--force"}, wantImage: true},
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
			setupManifestDeployWith(t, fake, tc.manifest)
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
				assert.Equal(t, "Dockerfile", fake.imgInput.Build.Dockerfile)
				assert.Equal(t, []string{"id=x,env=Y"}, fake.imgInput.BuildSecrets)
				assert.Equal(t, "d", fake.imgInput.Description)
			}
		})
	}
}

const manifestOnProd = `[project]
name = "demo"
dependencies = ["apache-airflow==3.1.*"]

[tool.astro]
domain = 'astronomer.io'
workspace = "clw-ws"

[tool.astro.deployments]
test = {deployment = 'clx-prod', default = true}
`

// setupDeployOnStage is a project on astronomer.io deployed while the CLI is
// switched to astronomer-stage.io. It returns where the login the deploy picked
// is recorded.
func setupDeployOnStage(t *testing.T, toml string) *deployLogin {
	t.Helper()
	t.Setenv(astrosession.EnvAPIToken, "")
	t.Setenv("ASTRO_DOMAIN", "")
	fake := &fakeCmdDeployer{dag: manifestdeploy.DagResult{DagTarballVersion: "v1"}}
	setupManifestDeployWith(t, fake, toml)
	testUtil.InitTestConfig(testUtil.CloudStagePlatform)

	picked := new(deployLogin)
	newManifestDeployer = func(login *deployLogin, _ io.Reader, _ io.Writer) manifestdeploy.Deployer {
		*picked = *login
		return fake
	}
	return picked
}

func logInTo(t *testing.T, domain, token, org string) {
	t.Helper()
	login := config.Context{Domain: domain}
	require.NoError(t, login.SetContextKey("token", token))
	require.NoError(t, login.SetContextKey("organization", org))
	require.NoError(t, login.SetContextKey("workspace", "clw-prod-ws"))
}

// The project names astronomer.io, so the deploy runs under that login even
// while the CLI is switched to stage: a link and a bare Deployment id alike.
func TestDeployManifestUsesTheManifestDomainsLogin(t *testing.T) {
	for _, args := range [][]string{{"test", "--dags"}, {"--deployment", "clx-bare", "--dags"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			picked := setupDeployOnStage(t, manifestOnProd)
			logInTo(t, "astronomer.io", "Bearer prod-token", "prod-org")

			_, err := execDeployCapture(args...)
			require.NoError(t, err)

			assert.False(t, picked.current)
			assert.Equal(t, "astronomer.io", picked.context.Domain)
			assert.Equal(t, "Bearer prod-token", picked.context.Token)
			assert.Equal(t, "prod-org", picked.context.Organization)
			assert.IsType(t, &astrov1.ClientWithResponses{}, picked.client)
		})
	}
}

// --non-dags from a project deploys under the project's host as the project's
// deploy does, a link and a bare Deployment id alike; outside a project a
// bare id goes under the current context.
func TestDeployNonDagsUsesTheManifestDomainsLogin(t *testing.T) {
	for _, args := range [][]string{{"test"}, {"--deployment", "clx-bare"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			setupDeployOnStage(t, manifestOnProd)
			logInTo(t, "astronomer.io", "Bearer prod-token", "prod-org")
			var captured *astrodeploy.DeployBundleInput
			prev := DeployBundle
			t.Cleanup(func() { DeployBundle = prev })
			DeployBundle = func(in *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
				captured = in
				return astrodeploy.BundleDeploy{}, nil
			}

			_, err := execDeployCapture(append(args, "--non-dags", "--non-dags-mount-path", "/x", "--non-dags-local-path", t.TempDir())...)
			require.NoError(t, err)
			require.NotNil(t, captured.Login, "deployed under the project's host")
			assert.Equal(t, "astronomer.io", captured.Login.Domain)
			assert.Equal(t, "prod-org", captured.Login.Organization)
		})
	}

	t.Run("outside a project", func(t *testing.T) {
		setupDeployOnStage(t, manifestOnProd)
		require.NoError(t, os.Remove(filepath.Join(config.WorkingPath, "pyproject.toml")))
		var captured *astrodeploy.DeployBundleInput
		prev := DeployBundle
		t.Cleanup(func() { DeployBundle = prev })
		DeployBundle = func(in *astrodeploy.DeployBundleInput) (astrodeploy.BundleDeploy, error) {
			captured = in
			return astrodeploy.BundleDeploy{}, nil
		}
		_, err := execDeployCapture("--deployment", "clx-bare", "--non-dags", "--non-dags-mount-path", "/x", "--non-dags-local-path", t.TempDir())
		require.NoError(t, err)
		assert.Nil(t, captured.Login, "the current context")
	})
}

// ASTRO_API_TOKEN outranks the stored login, and goes out with its scheme like
// a stored token, since the deploy's CI/CD check splits it off.
func TestDeployManifestSendsTheAPITokenToTheManifestDomain(t *testing.T) {
	picked := setupDeployOnStage(t, manifestOnProd)
	logInTo(t, "astronomer.io", "Bearer prod-token", "prod-org")
	t.Setenv(astrosession.EnvAPIToken, "ci-token")

	_, err := execDeployCapture("test", "--dags")
	require.NoError(t, err)
	assert.Equal(t, "Bearer ci-token", picked.context.Token)
	assert.Equal(t, "prod-org", picked.context.Organization)
}

// With no login for the project's host the deploy stops before anything is
// built or uploaded, and names the login that fixes it.
func TestDeployManifestWithNoLoginForTheManifestDomain(t *testing.T) {
	setupDeployOnStage(t, manifestOnProd)

	_, err := execDeployCapture("test", "--dags")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not logged in to astronomer.io. Log in with `astro login astronomer.io`")
}

// A project that names the host the CLI is already on deploys under the
// current context, as it did before the manifest's domain was read.
func TestDeployManifestOnTheCurrentDomainUsesTheCurrentContext(t *testing.T) {
	picked := setupDeployOnStage(t, strings.Replace(manifestOnProd, "'astronomer.io'", "'https://cloud.astronomer-stage.io/'", 1))

	_, err := execDeployCapture("test", "--dags")
	require.NoError(t, err)
	assert.True(t, picked.current)
	assert.Equal(t, "astronomer-stage.io", picked.context.Domain)
}

// The workspace-level picker lists Deployments under the current context, so a
// project on another host is told to name one instead.
func TestDeployManifestPickerRefusesAnotherHost(t *testing.T) {
	d := manifestDeployer{login: &deployLogin{context: config.Context{Domain: "astronomer.io"}}}
	_, err := d.ResolveUnlinked("clw-ws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--deployment <id>")
	assert.Contains(t, err.Error(), "astro context switch astronomer.io")
}
