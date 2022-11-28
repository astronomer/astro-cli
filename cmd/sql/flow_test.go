package sql

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	sql "github.com/astronomer/astro-cli/sql"
	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errMock            = errors.New("mock error")
	imageBuildResponse = types.ImageBuildResponse{
		Body: io.NopCloser(strings.NewReader("Image built")),
	}
	containerCreateCreatedBody = container.ContainerCreateCreatedBody{ID: "123"}
	sampleLog                  = io.NopCloser(strings.NewReader("Sample log"))
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

func patchDockerClientInit(t *testing.T, statusCode int64, err error) {
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
	cmd := NewFlowCommand()
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return err
}

func TestFlowCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	err := execFlowCmd()
	assert.NoError(t, err)
}

func TestFlowVersionCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	err := execFlowCmd("version")
	assert.NoError(t, err)
}

func TestFlowAboutCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	err := execFlowCmd("about")
	assert.NoError(t, err)
}

func TestFlowInitCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	defer chdir(t, projectDir)()
	err := execFlowCmd("init")
	assert.NoError(t, err)
}

func TestFlowInitCmdWithFlags(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	AirflowHome := t.TempDir()
	AirflowDagsFolder := t.TempDir()
	err := execFlowCmd("init", projectDir, "--airflow-home", AirflowHome, "--airflow-dags-folder", AirflowDagsFolder)
	assert.NoError(t, err)
}

func TestFlowValidateCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("validate", projectDir, "--connection", "sqlite_conn", "--verbose")
	assert.NoError(t, err)
}

func TestFlowGenerateCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("generate", "example_basic_transform", "--project-dir", projectDir, "--no-generate-tasks", "--verbose")
	assert.NoError(t, err)
}

func TestFlowGenerateGenerateTasksCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("generate", "example_basic_transform", "--project-dir", projectDir, "--generate-tasks")
	assert.NoError(t, err)
}

func TestFlowRunGenerateTasksCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "example_basic_transform", "--project-dir", projectDir, "--generate-tasks")
	assert.NoError(t, err)
}

func TestFlowGenerateCmdWorkflowNameNotSet(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("generate", "--project-dir", projectDir)
	assert.EqualError(t, err, "argument not set:workflow_name")
}

func TestFlowRunCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "example_templating", "--env", "dev", "--project-dir", projectDir, "--no-generate-tasks", "--verbose")
	assert.NoError(t, err)
}

func TestDebugFlowRunCmd(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("--debug", "run", "example_templating", "--env", "dev", "--project-dir", projectDir)
	assert.NoError(t, err)
}

func TestFlowRunCmdWorkflowNameNotSet(t *testing.T) {
	patchDockerClientInit(t, 0, nil)
	projectDir := t.TempDir()
	err := execFlowCmd("init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("run", "--project-dir", projectDir)
	assert.EqualError(t, err, "argument not set:workflow_name")
}

func TestFlowCmdOsExit(t *testing.T) {
	var statusCode int64 = 1
	patchDockerClientInit(t, statusCode, nil)
	osExit = func(code int) {
		assert.Equal(t, code, int(statusCode))
	}
	err := execFlowCmd("version")
	assert.NoError(t, err)
	osExit = os.Exit
}

func TestFlowHelpCmdOsExit(t *testing.T) {
	var statusCode int64 = 1
	patchDockerClientInit(t, statusCode, nil)
	osExit = func(code int) {
		assert.Equal(t, code, int(statusCode))
	}
	err := execFlowCmd()
	assert.NoError(t, err)
	osExit = os.Exit
}

func TestFlowCmdError(t *testing.T) {
	patchDockerClientInit(t, 0, errMock)
	err := execFlowCmd("version")
	assert.EqualError(t, err, "error running [flow version]: docker client initialization failed mock error")
}

func TestFlowHelpCmdError(t *testing.T) {
	patchDockerClientInit(t, 0, errMock)
	assert.PanicsWithError(t, "error running [flow --help]: docker client initialization failed mock error", func() { execFlowCmd() })
}
