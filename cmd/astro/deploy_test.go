package astro

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/internal/project"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func execDeployCmd(args ...string) error {
	// --workspace-id, and deploy itself when it resolves one, set the
	// package's workspaceID, which coalesceWorkspace reads before the context.
	// Left set, it would choose the Workspace of whatever runs next.
	defer func() { workspaceID = "" }()
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewDeployCmd()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

// deployIn points the deploy at dir for one test.
func deployIn(t *testing.T, dir string) {
	t.Helper()
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
}

// make1xProject lays out a project the way Astro CLI 1.x made one.
func make1xProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM quay.io/astronomer/astro-runtime:12.0.0\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".astro", "config.yaml"), []byte("project:\n  name: demo\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
	return dir
}

// noTransport fails the test if the deploy reaches the transport.
func noTransport(t *testing.T) {
	t.Helper()
	prevDeployer := newManifestDeployer
	t.Cleanup(func() { newManifestDeployer = prevDeployer })
	newManifestDeployer = func(*deployLogin, io.Reader, io.Writer) manifestdeploy.Deployer {
		t.Fatal("a refused deploy reaches no transport")
		return nil
	}
}

// v2 deploys only pyproject.toml projects. A project in the Astro CLI 1.x
// layout is refused for every mode that reads the project, naming the two
// ways forward, and the transport is never reached. Outside any project the
// deploy gives the no-project advice. Both are no_project failures.
func TestDeployRefusesA1xProject(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()
	noTransport(t)

	oneX := make1xProject(t)
	deployIn(t, oneX)
	for _, args := range [][]string{
		{"test-deployment-id"},
		{"test-deployment-id", "--dags"},
		{"test-deployment-id", "--image"},
		{"test-deployment-id", "--image-name", "img:1"},
		{"--deployment", "test-deployment-id", "--image-name", "img:1", "--force"},
		{"test-deployment-id", "--force", "--output", "json"},
	} {
		err := execDeployCmd(args...)
		require.Error(t, err, "%v", args)
		assert.EqualError(t, err, utils.Deploy1xRefused("this project", ""), "%v", args)
		var notFound *project.NotFoundError
		assert.True(t, errors.As(err, &notFound), "%v: reported as no_project", args)
	}

	// From below the project, the refusal names it.
	deployIn(t, filepath.Join(oneX, "dags"))
	err := execDeployCmd("test-deployment-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside a project at "+oneX)

	deployIn(t, t.TempDir())
	for _, args := range [][]string{{"test-deployment-id"}, {"test-deployment-id", "--dags"}, {"test-deployment-id", "--image-name="}} {
		err := execDeployCmd(args...)
		require.Error(t, err, "%v", args)
		assert.Contains(t, err.Error(), "this is not an Astro project directory. Change to an Astro project directory, or run astro init", "%v", args)
		assert.NotContains(t, err.Error(), "1.x", "%v", args)
		var notFound *project.NotFoundError
		assert.True(t, errors.As(err, &notFound), "%v: reported as no_project", args)
	}

	// A bad --output is the usage error, wherever the deploy runs.
	_, err = execDeployCapture("test-deployment-id", "--output", "yaml")
	assert.True(t, cliout.IsUsage(err), "%v", err)
}

// imageDeployer swaps in a fake transport for one test.
func imageDeployer(t *testing.T, fake *fakeCmdDeployer) {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()
	prevDeployer := newManifestDeployer
	t.Cleanup(func() { newManifestDeployer = prevDeployer })
	newManifestDeployer = func(*deployLogin, io.Reader, io.Writer) manifestdeploy.Deployer { return fake }
}

// Outside any project --image-name deploys the image alone, never the working
// directory's dags/, to the Deployment the argument or --deployment names by
// id. A Deployment that takes DAG deploys keeps the DAGs it had, which the
// deploy says on stderr and in the json result's warnings.
func TestDeployImageNameOutsideAProject(t *testing.T) {
	for _, tc := range []struct{ dagDeploy, remote bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		dagDeploy := tc.dagDeploy
		t.Run(fmt.Sprintf("dag deploy %v, remote execution %v", tc.dagDeploy, tc.remote), func(t *testing.T) {
			fake := &fakeCmdDeployer{img: manifestdeploy.ImageResult{ImageTag: "deploy-1", WorkspaceID: "clw-ws", DagDeployEnabled: dagDeploy, RemoteExecutionEnabled: tc.remote}}
			imageDeployer(t, fake)
			deployIn(t, t.TempDir())

			out, err := execDeployCapture("--deployment", actionDeploymentID, "--image-name", "img:1", "--output", "json")
			require.NoError(t, err, out)
			require.NotNil(t, fake.imgInput)
			assert.Equal(t, actionDeploymentID, fake.imgInput.DeploymentID)
			assert.Equal(t, "img:1", fake.imgInput.ImageName)
			assert.False(t, fake.imgInput.IncludeDags, "no DAGs ship from outside a project")
			assert.True(t, fake.imgInput.ImageAlone, "the transport is told the image ships alone")
			assert.Contains(t, out, `"type":"image-only"`)
			// With DAG deploys the Deployment keeps its DAGs; without, a
			// remote-execution one runs its remote agents' DAGs, and any
			// other runs whatever DAGs the image carries.
			warning := fmt.Sprintf(warningDagsInImage, actionDeploymentID, "img:1")
			switch {
			case dagDeploy:
				warning = fmt.Sprintf(warningDagsNotUpdated, actionDeploymentID, actionDeploymentID)
			case tc.remote:
				warning = fmt.Sprintf(warningRemoteDagsNotUpdated, actionDeploymentID)
				assert.NotContains(t, out, "inside the image")
			}
			assert.Contains(t, out, `"warnings":["`+warning+`"]`)
			assert.Contains(t, out, "Warning: "+warning)
		})
	}

	t.Run("--dags with it names only the flag given", func(t *testing.T) {
		fake := &fakeCmdDeployer{}
		imageDeployer(t, fake)
		deployIn(t, t.TempDir())
		err := execDeployCmd(actionDeploymentID, "--image-name", "img:1", "--dags")
		assert.EqualError(t, err, "--dags deploys only your Dags; drop --image-name")
		assert.Nil(t, fake.imgInput)
		assert.Nil(t, fake.dagInput)
	})

	t.Run("no target, and none can be asked for", func(t *testing.T) {
		fake := &fakeCmdDeployer{}
		imageDeployer(t, fake)
		deployIn(t, t.TempDir())
		_, err := execDeployCapture("--image-name", "img:1", "--workspace", "clw-ws", "--output", "json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pass the Deployment id as the argument or with --deployment")
		assert.NotContains(t, err.Error(), "project")
	})

	t.Run("below a pyproject.toml project's root it is refused", func(t *testing.T) {
		imageDeployer(t, &fakeCmdDeployer{})
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestForRouting), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o755))
		deployIn(t, filepath.Join(dir, "dags"))
		err := execDeployCmd(actionDeploymentID, "--image-name", "img:1")
		assert.ErrorContains(t, err, "Run the deploy from the project directory, "+dir)
	})
}

// The flag combinations 1.x refused, where they still apply: --wait-time needs
// --wait, and --no-dags-base-dir needs a deploy that ships DAGs.
func TestDeployRefusesFlagsThatCannotAllApply(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inProject bool
		args      []string
		says      string
	}{
		{"--wait-time without --wait", true, []string{actionDeploymentID, "--wait-time", "1m"}, "cannot use --wait-time with --wait=false"},
		{"--no-dags-base-dir with --image", true, []string{actionDeploymentID, "--image", "--no-dags-base-dir"}, "cannot use --no-dags-base-dir with --image"},
		{"--no-dags-base-dir with --image-name outside a project", false, []string{actionDeploymentID, "--image-name", "img:1", "--no-dags-base-dir"}, "cannot use --no-dags-base-dir with --image-name outside a pyproject.toml project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeCmdDeployer{}
			imageDeployer(t, fake)
			dir := t.TempDir()
			if tc.inProject {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestForRouting), 0o600))
			}
			deployIn(t, dir)
			_, err := execDeployCapture(tc.args...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.says)
			assert.True(t, cliout.IsUsage(err))
			assert.Nil(t, fake.imgInput)
		})
	}

	// In a project --image-name ships the project's DAGs, so their layout
	// is the flag's to set.
	fake := &fakeCmdDeployer{}
	imageDeployer(t, fake)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestForRouting), 0o600))
	deployIn(t, dir)
	_, err := execDeployCapture(actionDeploymentID, "--image-name", "img:1", "--no-dags-base-dir")
	require.NoError(t, err)
	require.NotNil(t, fake.imgInput)
	assert.True(t, fake.imgInput.NoDagsBaseDir)
}

// The flags only the 1.x deploy read are gone from the command: a run that
// passes one fails as an unknown flag here, and the root's removed-flags
// registry names what replaced it (cmd's TestRemovedFlagsSayWhatReplacedThem).
// --force and --prompt stay, hidden and read by nothing: astronomer/deploy-action
// passes --force, and a pyproject.toml project's deploy always took both.
func TestDeployDropsThe1xOnlyFlags(t *testing.T) {
	cmd := NewDeployCmd()
	for _, name := range []string{"save", "pytest", "parse", "test", "env", "dags-path", "dag-bundle-name", "deployment-name", "non-dags", "non-dags-mount-path", "non-dags-local-path", "non-dags-bundle-type"} {
		assert.Nil(t, cmd.Flags().Lookup(name), "--%s", name)
	}
	for _, letter := range []string{"s", "t", "e", "n"} {
		assert.Nil(t, cmd.Flags().ShorthandLookup(letter), "-%s", letter)
	}
	for _, name := range []string{"force", "prompt"} {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, name)
		assert.True(t, f.Hidden, name)
	}
}

func testExecCmd(cmd *cobra.Command, args ...string) error {
	if args == nil {
		args = []string{}
	}
	// deploy sets the package's workspaceID from --workspace-id or the one it
	// resolves, and coalesceWorkspace reads it before the context. Left set,
	// it would choose the Workspace of whatever runs next.
	defer func() { workspaceID = "" }()
	testUtil.SetupOSArgsForGinkgo()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

// An empty --deployment or --workspace, as an unset variable leaves it, is a
// usage error before anything runs, rather than a deploy that picks one. The
// 1.x --workspace-id given as "" is the default, as it was in 1.x.
func TestDeployRefusesAnEmptyDeploymentOrWorkspace(t *testing.T) {
	for _, name := range []string{"deployment", "workspace"} {
		err := execDeployCmd("--"+name, "")
		require.EqualError(t, err, "--"+name+" cannot be empty")
		assert.True(t, cliout.IsUsage(err))
	}
}
