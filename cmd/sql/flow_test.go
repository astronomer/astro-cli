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
	mockDockerBinder           = new(mocks.DockerBind)
	_                          = patchDockerClientInit()
)

func getContainerWaitResponse(raiseError bool) (bodyCh <-chan container.ContainerWaitOKBody, errCh <-chan error) {
	containerWaitOkBodyChannel := make(chan container.ContainerWaitOKBody)
	errChannel := make(chan error, 1)
	go func() {
		if raiseError {
			errChannel <- errMock
			return
		}
		res := container.ContainerWaitOKBody{StatusCode: 200, Error: nil}
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

func patchDockerClientInit() error {
	sql.DockerClientInit = func() (sql.DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		return mockDockerBinder, nil
	}
	return nil
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
	err := execFlowCmd([]string{}...)
	assert.NoError(t, err)
}

func TestFlowVersionCmd(t *testing.T) {
	err := execFlowCmd([]string{"version"}...)
	assert.NoError(t, err)
}

func TestFlowAboutCmd(t *testing.T) {
	err := execFlowCmd([]string{"about"}...)
	assert.NoError(t, err)
}

func TestFlowInitCmd(t *testing.T) {
	projectDir := t.TempDir()
	defer chdir(t, projectDir)()
	err := execFlowCmd([]string{"init"}...)
	assert.NoError(t, err)
}

func TestFlowInitCmdWithFlags(t *testing.T) {
	projectDir := t.TempDir()
	AirflowHome := t.TempDir()
	AirflowDagsFolder := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir, "--airflow-home", AirflowHome, "--airflow-dags-folder", AirflowDagsFolder}...)
	assert.NoError(t, err)
}

func TestFlowValidateCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"validate", projectDir, "--connection", "sqlite_conn"}...)
	assert.NoError(t, err)
}

func TestFlowGenerateCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"generate", "example_basic_transform", "--project-dir", projectDir}...)
	assert.NoError(t, err)
}

func TestFlowGenerateCmdWorkflowNameNotSet(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"generate", "--project-dir", projectDir}...)
	assert.EqualError(t, err, "argument not set:workflow_name")
}

func TestFlowRunCmd(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run", "example_templating", "--env", "dev", "--project-dir", projectDir, "--verbose"}...)
	assert.NoError(t, err)
}

func TestFlowRunCmdWorkflowNameNotSet(t *testing.T) {
	projectDir := t.TempDir()
	err := execFlowCmd([]string{"init", projectDir}...)
	assert.NoError(t, err)

	err = execFlowCmd([]string{"run", "--project-dir", projectDir}...)
	assert.EqualError(t, err, "argument not set:workflow_name")
}
