package sql

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/astronomer/astro-cli/astro-client"
	cloudDeploy "github.com/astronomer/astro-cli/cloud/deploy"
	astroDeployment "github.com/astronomer/astro-cli/cloud/deployment"
	astroWorkspace "github.com/astronomer/astro-cli/cloud/workspace"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	sql "github.com/astronomer/astro-cli/sql"
	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/astronomer/astro-cli/version"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock            = errors.New("mock error")
	imageBuildResponse = types.ImageBuildResponse{
		Body: io.NopCloser(strings.NewReader("Image built")),
	}
	containerCreateCreatedBody    = container.ContainerCreateCreatedBody{ID: "123"}
	sampleLog                     = io.NopCloser(strings.NewReader("Sample log"))
	originalGetWorkspaceSelection = astroWorkspace.GetWorkspaceSelection
	originalGetDeployments        = astroDeployment.GetDeployments
	originalSelectDeployment      = astroDeployment.SelectDeployment
	mockGetWorkspaceSelection     = func(client astro.Client, out io.Writer) (string, error) {
		return "W1", nil
	}
	mockGetWorkspaceSelectionErr = func(client astro.Client, out io.Writer) (string, error) {
		return "", errMock
	}
	mockGetDeployments = func(ws string, client astro.Client) ([]astro.Deployment, error) {
		return nil, nil
	}
	mockGetDeploymentsErr = func(ws string, client astro.Client) ([]astro.Deployment, error) {
		return nil, errMock
	}
	mockSelectDeployment = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
		return astro.Deployment{ID: "D1", Workspace: astro.Workspace{ID: "W1"}}, nil
	}
	mockSelectDeploymentErr = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
		return astro.Deployment{}, errMock
	}
)

func getContainerWaitResponse(raiseError bool, statusCode int64) (bodyCh <-chan container.ContainerWaitOKBody, errCh <-chan error) {
	containerWaitOkBodyChannel := make(chan container.ContainerWaitOKBody)
	errChannel := make(chan error, 1)
	go func() {
		if raiseError {
			errChannel <- errMock
			return
		}
		res := container.ContainerWaitOKBody{StatusCode: statusCode}
		containerWaitOkBodyChannel <- res
		errChannel <- nil
		close(containerWaitOkBodyChannel)
		close(errChannel)
	}()
	// converting bidirectional channel to read only channels for ContainerWait to consume
	var readOnlyStatusCh <-chan container.ContainerWaitOKBody
	var readOnlyErrCh <-chan error
	readOnlyStatusCh = containerWaitOkBodyChannel
	readOnlyErrCh = errChannel
	return readOnlyStatusCh, readOnlyErrCh
}

// patches ExecuteCmdInDocker and
// returns a function that, when called, restores the original values.
func patchExecuteCmdInDocker(t *testing.T, statusCode int64, err error) func() {
	mockDocker := mocks.NewDockerBind(t)
	sql.Docker = func() (sql.DockerBind, error) {
		if err == nil {
			mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
			mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
			mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false, statusCode))
			mockDocker.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
			mockDocker.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		}
		return mockDocker, err
	}
	sql.DisplayMessages = func(r io.Reader) error {
		return nil
	}
	mockIo := mocks.NewIoBind(t)
	sql.Io = func() sql.IoBind {
		mockIo.On("Copy", mock.Anything, mock.Anything).Return(int64(0), nil)
		return mockIo
	}

	return func() {
		sql.Docker = sql.NewDockerBind
		sql.Io = sql.NewIoBind
		sql.DisplayMessages = sql.OriginalDisplayMessages
	}
}

// Mock ExecuteCmdInDocker to return a specific output for "config get --json" calls
func mockExecuteCmdInDockerOutputForJSONConfig(outputString string) func() {
	originalExecuteCmdInDocker := sql.ExecuteCmdInDocker

	sql.ExecuteCmdInDocker = func(cmd, mountDirs []string, returnOutput bool) (exitCode int64, output io.ReadCloser, err error) {
		if returnOutput && cmd[0] == "config" && cmd[1] == "get" && cmd[2] == "--json" {
			return 0, io.NopCloser(strings.NewReader(outputString)), nil
		}
		return 0, io.NopCloser(strings.NewReader("")), nil
	}

	return func() {
		sql.ExecuteCmdInDocker = originalExecuteCmdInDocker
	}
}

// patches "astro deploy" command i.e. cloud.NewDeployCmd()
func patchDeployCmd() func() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	cloudCmd.EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	cloudCmd.DeployImage = func(deployInput cloudDeploy.InputDeploy, client astro.Client) error {
		return nil
	}

	return func() {
		cloudCmd.EnsureProjectDir = utils.EnsureProjectDir
		cloudCmd.DeployImage = cloudDeploy.Deploy
	}
}

// chdir changes the current working directory to the named directory and
// returns a function that, when called, restores the original working
// directory.
func chdir(t *testing.T, dir string) func() {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	return func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restoring working directory: %v", err)
		}
	}
}

func execFlowCmd(args ...string) error {
	version.CurrVersion = "1.8"
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func execFlowCmdWrongVersion(args ...string) error {
	version.CurrVersion = "foo"
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestFlowCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	err := execFlowCmd()
	assert.NoError(t, err)
}

func TestFlowCmdWrongVersion(t *testing.T) {
	assert.PanicsWithError(t, "error running []: error parsing response for SQL CLI version %!w(<nil>)", func() { execFlowCmdWrongVersion() })
}

func TestFlowCmdError(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, errMock)()
	err := execFlowCmd("version")
	assert.EqualError(t, err, "error running [version]: docker client initialization failed mock error")
}

func TestFlowCmdHelpError(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, errMock)()
	assert.PanicsWithError(t, "error running []: docker client initialization failed mock error", func() { execFlowCmd() })
}

func TestFlowCmdDockerCommandError(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 1, nil)()
	err := execFlowCmd("version")
	assert.EqualError(t, err, "docker command has returned a non-zero exit code:1")
}

func TestFlowCmdDockerCommandHelpError(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 1, nil)()
	assert.PanicsWithError(t, "docker command has returned a non-zero exit code:1", func() { execFlowCmd() })
}

func TestFlowVersionCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	err := execFlowCmd("version")
	assert.NoError(t, err)
}

func TestFlowVersionHelpCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	err := execFlowCmd("version", "--help")
	assert.NoError(t, err)
}

func TestFlowAboutCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	err := execFlowCmd("about")
	assert.NoError(t, err)
}

func TestFlowInitCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	projectDir := t.TempDir()
	defer chdir(t, projectDir)()
	err := execFlowCmd("init")
	assert.NoError(t, err)
}

func TestFlowInitCmdWithArgs(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	cmd := "init"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, t.TempDir()}},
		{[]string{cmd, "--airflow-home", t.TempDir()}},
		{[]string{cmd, "--airflow-dags-folder", t.TempDir()}},
		{[]string{cmd, "--data-dir", t.TempDir()}},
		{[]string{cmd, "--verbose"}},
		{[]string{cmd, "--no-verbose"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestFlowValidateCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	cmd := "validate"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, t.TempDir()}},
		{[]string{cmd, "--connection", "<connection>"}},
		{[]string{cmd, "--env", "<env>"}},
		{[]string{cmd, "--verbose"}},
		{[]string{cmd, "--no-verbose"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestFlowGenerateCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	cmd := "generate"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, "--project-dir", t.TempDir()}},
		{[]string{cmd, "--generate-tasks"}},
		{[]string{cmd, "--no-generate-tasks"}},
		{[]string{cmd, "--no-verbose"}},
		{[]string{cmd, "--verbose"}},
		{[]string{cmd, "--env", "<env>"}},
		{[]string{cmd, "--output-dir", t.TempDir()}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestFlowRunCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	cmd := "run"
	testCases := []struct {
		args []string
	}{
		{[]string{cmd, "--project-dir", t.TempDir()}},
		{[]string{cmd, "--generate-tasks"}},
		{[]string{cmd, "--no-generate-tasks"}},
		{[]string{cmd, "--no-verbose"}},
		{[]string{cmd, "--verbose"}},
		{[]string{cmd, "--env", "<env>"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestDebugFlowFlagCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	testCases := []struct {
		args []string
	}{
		{[]string{"--debug", "version"}},
		{[]string{"--no-debug", "version"}},
		{[]string{"--debug", "about"}},
		{[]string{"--no-debug", "about"}},
		{[]string{"--debug", "init"}},
		{[]string{"--no-debug", "init"}},
		{[]string{"--debug", "validate"}},
		{[]string{"--no-debug", "validate"}},
		{[]string{"--debug", "generate"}},
		{[]string{"--no-debug", "generate"}},
		{[]string{"--debug", "run"}},
		{[]string{"--no-debug", "run"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestFlowDeployWithWorkflowCmd(t *testing.T) {
	defer patchDeployCmd()()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner) error {
		return nil
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig("{\"default\": {\"deployment\": {\"astro_deployment_id\": \"foo\", \"astro_workspace_id\": \"bar\"}}}")()
	err := execFlowCmd("deploy", "--workflow-name", "test.sql")
	assert.NoError(t, err)
}

func TestFlowDeployNoWorkflowsCmd(t *testing.T) {
	defer patchDeployCmd()()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner) error {
		return nil
	}
	mockOs := mocks.NewOsBind(t)
	Os = func() sql.OsBind {
		fs := fstest.MapFS{
			"workflows": {
				Mode: fs.ModeDir,
			},
		}
		mockOs.On("ReadDir", mock.Anything).Return(fs.ReadDir("workflows"))
		return mockOs
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig("{\"default\": {\"deployment\": {\"astro_deployment_id\": \"foo\", \"astro_workspace_id\": \"bar\"}}}")()
	err := execFlowCmd("deploy")
	assert.NoError(t, err)

	Os = sql.NewOsBind
}

func TestFlowDeployWorkflowsCmd(t *testing.T) {
	defer patchDeployCmd()()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner) error {
		return nil
	}
	mockOs := mocks.NewOsBind(t)
	Os = func() sql.OsBind {
		fs := fstest.MapFS{
			"workflows/workflow/.sql": {
				Data: []byte("select 1"),
			},
		}
		mockOs.On("ReadDir", mock.Anything).Return(fs.ReadDir("workflows"))
		return mockOs
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig("{\"default\": {\"deployment\": {\"astro_deployment_id\": \"foo\", \"astro_workspace_id\": \"bar\"}}}")()
	err := execFlowCmd("deploy")
	assert.NoError(t, err)

	Os = sql.NewOsBind
}

func TestPromptAstroCloudConfigDeploymentAndWorkspaceUnsetGetWorkspaceSelectionFailure(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelectionErr
	_, _, err := promptAstroCloudConfig("", "")
	assert.ErrorIs(t, err, errMock)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
}

func TestPromptAstroCloudConfigDeploymentAndWorkspaceUnsetGetDeploymentsFailure(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelection
	astroDeployment.GetDeployments = mockGetDeploymentsErr
	_, _, err := promptAstroCloudConfig("", "")
	assert.ErrorIs(t, err, errMock)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
	astroDeployment.GetDeployments = originalGetDeployments
}

func TestPromptAstroCloudConfigDeploymentAndWorkspaceUnsetSelectDeploymentFailure(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelection
	astroDeployment.GetDeployments = mockGetDeployments
	astroDeployment.SelectDeployment = mockSelectDeploymentErr
	_, _, err := promptAstroCloudConfig("", "")
	assert.ErrorIs(t, err, errMock)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
	astroDeployment.GetDeployments = originalGetDeployments
	astroDeployment.SelectDeployment = originalSelectDeployment
}

func TestPromptAstroCloudConfigDeploymentAndWorkspaceUnsetSuccess(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelection
	astroDeployment.GetDeployments = mockGetDeployments
	astroDeployment.SelectDeployment = mockSelectDeployment
	selectedAstroDeploymentID, selectedAstroWorkspaceID, err := promptAstroCloudConfig("", "")
	assert.NoError(t, err)
	assert.EqualValues(t, "D1", selectedAstroDeploymentID)
	assert.EqualValues(t, "W1", selectedAstroWorkspaceID)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
	astroDeployment.GetDeployments = originalGetDeployments
	astroDeployment.SelectDeployment = originalSelectDeployment
}

func TestPromptAstroCloudConfigDeploymentUnsetGetDeploymentsFailure(t *testing.T) {
	astroDeployment.GetDeployments = mockGetDeploymentsErr
	_, _, err := promptAstroCloudConfig("", "W2")
	assert.ErrorIs(t, err, errMock)
	astroDeployment.GetDeployments = originalGetDeployments
}

func TestPromptAstroCloudConfigDeploymentUnsetSelectDeploymentFailure(t *testing.T) {
	astroDeployment.GetDeployments = mockGetDeployments
	astroDeployment.SelectDeployment = mockSelectDeploymentErr
	_, _, err := promptAstroCloudConfig("", "W2")
	assert.ErrorIs(t, err, errMock)
	astroDeployment.GetDeployments = originalGetDeployments
	astroDeployment.SelectDeployment = originalSelectDeployment
}

func TestPromptAstroCloudConfigDeploymentUnsetSuccess(t *testing.T) {
	astroDeployment.GetDeployments = mockGetDeployments
	astroDeployment.SelectDeployment = mockSelectDeployment
	selectedAstroDeploymentID, selectedAstroWorkspaceID, err := promptAstroCloudConfig("", "W2")
	assert.NoError(t, err)
	assert.EqualValues(t, "D1", selectedAstroDeploymentID)
	assert.EqualValues(t, "W2", selectedAstroWorkspaceID)
	astroDeployment.GetDeployments = originalGetDeployments
	astroDeployment.SelectDeployment = originalSelectDeployment
}

func TestPromptAstroCloudConfigWorkspaceUnsetGetWorkspaceSelectionFailure(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelectionErr
	_, _, err := promptAstroCloudConfig("D2", "")
	assert.ErrorIs(t, err, errMock)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
}

func TestPromptAstroCloudConfigWorkspaceUnsetSuccess(t *testing.T) {
	astroWorkspace.GetWorkspaceSelection = mockGetWorkspaceSelection
	selectedAstroDeploymentID, selectedAstroWorkspaceID, err := promptAstroCloudConfig("D2", "")
	assert.NoError(t, err)
	assert.EqualValues(t, "D2", selectedAstroDeploymentID)
	assert.EqualValues(t, "W1", selectedAstroWorkspaceID)
	astroWorkspace.GetWorkspaceSelection = originalGetWorkspaceSelection
}
