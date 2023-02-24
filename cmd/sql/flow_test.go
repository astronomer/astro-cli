package sql

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	softwareDeployment "github.com/astronomer/astro-cli/software/deployment"

	"github.com/astronomer/astro-cli/houston"
	softwareWorkspace "github.com/astronomer/astro-cli/software/workspace"

	"github.com/onsi/ginkgo/extensions/table"

	"github.com/astronomer/astro-cli/astro-client"
	cloudDeploy "github.com/astronomer/astro-cli/cloud/deploy"
	cloudDeployment "github.com/astronomer/astro-cli/cloud/deployment"
	cloudWorkspace "github.com/astronomer/astro-cli/cloud/workspace"
	cloudCmd "github.com/astronomer/astro-cli/cmd/cloud"
	softwareCmd "github.com/astronomer/astro-cli/cmd/software"
	softwareDeploy "github.com/astronomer/astro-cli/software/deploy"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/pkg/input"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	sql "github.com/astronomer/astro-cli/sql"
	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/astronomer/astro-cli/version"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock            = errors.New("mock error")
	imageBuildResponse = types.ImageBuildResponse{
		Body: io.NopCloser(strings.NewReader("Image built")),
	}
	containerCreateCreatedBody            = container.ContainerCreateCreatedBody{ID: "123"}
	sampleLog                             = io.NopCloser(strings.NewReader("Sample log"))
	originalGetCloudWorkspaceSelection    = cloudWorkspace.GetWorkspaceSelection
	originalGetSoftwareWorkspaceSelection = softwareWorkspace.GetWorkspaceSelectionID
	originalGetCloudDeployments           = cloudDeployment.GetDeployments
	originalGetSoftwareDeployments        = softwareDeployment.GetDeployments
	originalSelectCloudDeployment         = cloudDeployment.SelectDeployment
	originalSelectSoftwareDeployment      = softwareDeployment.SelectDeployment
	originalGetHoustonClient              = GetHoustonClient
	mockGetHoustonClient                  = func() houston.ClientInterface {
		return houston.ClientImplementation{}
	}
	mockGetCloudWorkspaceSelection = func(client astro.Client, out io.Writer) (string, error) {
		return "W1", nil
	}
	mockGetSoftwareWorkspaceSelection = func(client houston.ClientInterface, out io.Writer) (string, error) {
		return "W1", nil
	}
	mockGetCloudWorkspaceSelectionErr = func(client astro.Client, out io.Writer) (string, error) {
		return "", errMock
	}
	mockGetSoftwareWorkspaceSelectionErr = func(client houston.ClientInterface, out io.Writer) (string, error) {
		return "", errMock
	}
	mockGetCloudDeployments = func(ws string, client astro.Client) ([]astro.Deployment, error) {
		return nil, nil
	}
	mockGetCloudDeploymentsErr = func(ws string, client astro.Client) ([]astro.Deployment, error) {
		return nil, errMock
	}
	mockGetSoftwareDeployments = func(ws string, client houston.ClientInterface) ([]houston.Deployment, error) {
		return nil, nil
	}
	mockGetSoftwareDeploymentsErr = func(ws string, client houston.ClientInterface) ([]houston.Deployment, error) {
		return nil, errMock
	}

	mockSelectCloudDeployment = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
		return astro.Deployment{ID: "D1", Workspace: astro.Workspace{ID: "W1"}}, nil
	}
	mockSelectCloudDeploymentErr = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
		return astro.Deployment{}, errMock
	}
	mockSelectSoftwareDeployment = func(deployments []houston.Deployment, message string) (houston.Deployment, error) {
		return houston.Deployment{ID: "D1", Workspace: houston.Workspace{ID: "W1"}}, nil
	}
	mockSelectSoftwareDeploymentErr = func(deployments []houston.Deployment, message string) (houston.Deployment, error) {
		return houston.Deployment{}, errMock
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
func mockExecuteCmdInDockerOutputForJSONConfig() func() {
	outputString := "{\"default\": {\"deployment\": {\"astro_deployment_id\": \"foo\", \"astro_workspace_id\": \"bar\"}}}"
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
func patchDeployCmd(platform string) func() {
	testUtil.InitTestConfig(platform)

	cloudCmd.EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	cloudCmd.DeployImage = func(deployInput cloudDeploy.InputDeploy, client astro.Client) error {
		return nil
	}

	softwareCmd.EnsureProjectDir = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	softwareCmd.DeployAirflowImage = func(houstonClient houston.ClientInterface, path, deploymentID, wsID, byoRegistryDomain string, ignoreCacheDeploy, byoRegistryEnabled, prompt bool) error {
		return nil
	}
	return func() {
		cloudCmd.EnsureProjectDir = utils.EnsureProjectDir
		cloudCmd.DeployImage = cloudDeploy.Deploy
		softwareCmd.EnsureProjectDir = utils.EnsureProjectDir
		softwareCmd.DeployAirflowImage = softwareDeploy.Airflow
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
	version.CurrVersion = "1.8.0"
	testUtil.SetupOSArgsForGinkgo()
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()

	return err
}

func TestFlowCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	err := execFlowCmd()
	assert.NoError(t, err)
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
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy", "test.sql")
	assert.NoError(t, err)
}

func TestFlowDeployWithTooManyArgs(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy", "test.sql", "abc")
	assert.ErrorIs(t, err, ErrTooManyArgs)
}

func TestFlowDeployNoWorkflowsCmd(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
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

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy")
	assert.NoError(t, err)

	Os = sql.NewOsBind
}

func TestFlowDeployWorkflowsCmdCloud(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
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

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy")
	assert.NoError(t, err)

	Os = sql.NewOsBind
}

func TestFlowDeployWorkflowsCmdSoftware(t *testing.T) {
	defer patchDeployCmd(testUtil.SoftwarePlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
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

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy")
	assert.NoError(t, err)

	Os = sql.NewOsBind
}

func TestFlowDeployWorkflowsCmdWithDelete(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return nil
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
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

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	RemoveDirectory = func(path string) error { return nil }
	err := execFlowCmd("deploy", "--delete-dags=true")
	assert.NoError(t, err)
	RemoveDirectory = func(path string) error { return errMock }
	err = execFlowCmd("deploy", "--delete-dags=true")
	assert.Error(t, err)
	Os = sql.NewOsBind
	RemoveDirectory = os.RemoveAll
}

func TestFlowDeployCmdInstalledFlowVersionFailure(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	getInstalledFlowVersion = func() (string, error) {
		return "", errMock
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig()
	err := execFlowCmd("deploy")
	assert.ErrorIs(t, err, errMock)
}

func TestFlowDeployCmdEnsurePythonSDKVersionIsMetFailure(t *testing.T) {
	defer patchDeployCmd(testUtil.CloudPlatform)()

	sql.EnsurePythonSdkVersionIsMet = func(input.PromptRunner, string) error {
		return errMock
	}
	getInstalledFlowVersion = func() (string, error) {
		return "", nil
	}

	defer mockExecuteCmdInDockerOutputForJSONConfig()()
	err := execFlowCmd("deploy")
	assert.ErrorIs(t, err, errMock)
}

func WorkspaceErr(isStart bool) {
	if isStart {
		cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelectionErr
		softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelectionErr
	} else {
		cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
	}
}

func FoundWorkspaceCantFindDeploymentErr(isStart bool) {
	if isStart {
		cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = mockGetCloudDeploymentsErr
		softwareDeployment.GetDeployments = mockGetSoftwareDeploymentsErr
	} else {
		cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
	}
}

func FoundWorkspaceFoundDeploymentSelectDeploymentErr(isStart bool) {
	if isStart {
		cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = mockGetCloudDeployments
		softwareDeployment.GetDeployments = mockGetSoftwareDeployments
		cloudDeployment.SelectDeployment = mockSelectCloudDeploymentErr
		softwareDeployment.SelectDeployment = mockSelectSoftwareDeploymentErr
	} else {
		cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
		cloudDeployment.SelectDeployment = originalSelectCloudDeployment
		softwareDeployment.SelectDeployment = originalSelectSoftwareDeployment
	}
}

func UnsetDeploymentSuccess(isStart bool) {
	if isStart {
		cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = mockGetCloudDeployments
		softwareDeployment.GetDeployments = mockGetSoftwareDeployments
		cloudDeployment.SelectDeployment = mockSelectCloudDeployment
		softwareDeployment.SelectDeployment = mockSelectSoftwareDeployment
	} else {
		cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
		softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
		cloudDeployment.SelectDeployment = originalSelectCloudDeployment
		softwareDeployment.SelectDeployment = originalSelectSoftwareDeployment
	}
}

func WorkspaceGivenGetDeploymentsFailure(isStart bool) {
	if isStart {
		cloudDeployment.GetDeployments = mockGetCloudDeploymentsErr
		softwareDeployment.GetDeployments = mockGetSoftwareDeploymentsErr
	} else {
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
	}
}

func WorkspaceGivenSelectDeploymentFailure(isStart bool) {
	if isStart {
		cloudDeployment.GetDeployments = mockGetCloudDeployments
		softwareDeployment.GetDeployments = mockGetSoftwareDeployments
		cloudDeployment.SelectDeployment = mockSelectCloudDeploymentErr
		softwareDeployment.SelectDeployment = mockSelectSoftwareDeploymentErr
	} else {
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
		cloudDeployment.SelectDeployment = originalSelectCloudDeployment
		softwareDeployment.SelectDeployment = originalSelectSoftwareDeployment
	}
}

func WorkspaceGivenGetDeploymentSuccess(isStart bool) {
	if isStart {
		cloudDeployment.GetDeployments = mockGetCloudDeployments
		softwareDeployment.GetDeployments = mockGetSoftwareDeployments
		cloudDeployment.SelectDeployment = mockSelectCloudDeployment
		softwareDeployment.SelectDeployment = mockSelectSoftwareDeployment
	} else {
		cloudDeployment.GetDeployments = originalGetCloudDeployments
		softwareDeployment.GetDeployments = originalGetSoftwareDeployments
		cloudDeployment.SelectDeployment = originalSelectCloudDeployment
		softwareDeployment.SelectDeployment = originalSelectSoftwareDeployment
	}
}

var _ = Describe("Prompt users for deployment and workspace", func() {
	BeforeEach(func() {
		GetHoustonClient = mockGetHoustonClient
	})
	table.DescribeTable("If neither workspace or deployment given",
		func(isCloud bool, setUpAndTearDownMocks func(bool), throwsError bool, expectedError error) {
			setUpAndTearDownMocks(true)
			_, _, err := promptAstroEnvironmentConfig("", "", isCloud)
			if throwsError {
				Expect(err).To(Equal(expectedError))
			} else {
				Expect(err).To(BeNil())
			}
			setUpAndTearDownMocks(false)
		},
		table.Entry("can't find workspace", true, WorkspaceErr, true, errMock),
		table.Entry("can't find workspace", false, WorkspaceErr, true, errMock),
		table.Entry("found workspace and can't find deployments", true, FoundWorkspaceCantFindDeploymentErr, true, errMock),
		table.Entry("found workspace and can't find deployments", false, FoundWorkspaceCantFindDeploymentErr, true, errMock),
		table.Entry("found workspace and deployments but can not select deployment", false, FoundWorkspaceFoundDeploymentSelectDeploymentErr, true, errMock),
		table.Entry("found workspace and deployments but can not select deployment", true, FoundWorkspaceFoundDeploymentSelectDeploymentErr, true, errMock),
		table.Entry("found workspace and successfully finds deployment", true, UnsetDeploymentSuccess, false, nil),
		table.Entry("found workspace and successfully finds deployment", true, UnsetDeploymentSuccess, false, nil),
	)
	table.DescribeTable("If workspace given but deployment not given",
		func(isCloud bool, setUpAndTearDownMocks func(bool), throwsError bool, expectedError error) {
			setUpAndTearDownMocks(true)
			_, _, err := promptAstroEnvironmentConfig("", "W2", isCloud)
			if throwsError {
				Expect(err).To(Equal(expectedError))
			} else {
				Expect(err).To(BeNil())
			}
			setUpAndTearDownMocks(false)
		},
		table.Entry("can't find deployments", true, WorkspaceGivenGetDeploymentsFailure, true, errMock),
		table.Entry("can't find deployments", false, WorkspaceGivenGetDeploymentsFailure, true, errMock),
		table.Entry("found deployments but can't select deployment", true, WorkspaceGivenSelectDeploymentFailure, true, errMock),
		table.Entry("found deployments but can't select deployment", false, WorkspaceGivenSelectDeploymentFailure, true, errMock),
		table.Entry("successfully found the deployment", true, WorkspaceGivenGetDeploymentSuccess, false, errMock),
		table.Entry("successfully found the deployment", false, WorkspaceGivenGetDeploymentSuccess, false, errMock),
	)
	Describe("If the deployment is successfully found", func() {
		It("returns the deployment and workspace", func() {
			UnsetDeploymentSuccess(true)
			deployment, workspace, err := promptAstroEnvironmentConfig("", "W2", true)
			Expect(err).To(BeNil())
			Expect(workspace).To(Equal("W2"))
			Expect(deployment).To(Equal("D1"))
			deployment, workspace, err = promptAstroEnvironmentConfig("", "W2", false)
			Expect(err).To(BeNil())
			Expect(workspace).To(Equal("W2"))
			Expect(deployment).To(Equal("D1"))
			UnsetDeploymentSuccess(false)
		})
	})
	Describe("If the user supplies a deployment and not a workspace", func() {
		It("fails if it can't find a workspace", func() {
			cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelectionErr
			softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelectionErr
			_, _, err := promptAstroEnvironmentConfig("D1", "", true)
			Expect(err).To(Equal(errMock))
			_, _, err = promptAstroEnvironmentConfig("D1", "", false)
			Expect(err).To(Equal(errMock))
			cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
			softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
		})
		It("passes if it can find a workspace", func() {
			cloudWorkspace.GetWorkspaceSelection = mockGetCloudWorkspaceSelection
			softwareWorkspace.GetWorkspaceSelectionID = mockGetSoftwareWorkspaceSelection
			_, _, err := promptAstroEnvironmentConfig("D1", "", true)
			Expect(err).To(BeNil())
			_, _, err = promptAstroEnvironmentConfig("D1", "", false)
			Expect(err).To(BeNil())
			cloudWorkspace.GetWorkspaceSelection = originalGetCloudWorkspaceSelection
			softwareWorkspace.GetWorkspaceSelectionID = originalGetSoftwareWorkspaceSelection
		})
	})
	AfterEach(func() {
		GetHoustonClient = originalGetHoustonClient
	})
})
