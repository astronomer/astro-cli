package sql

import (
	"errors"
	"fmt"
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
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestFlowConfigCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	testCases := []struct {
		initFlag  string
		configKey string
	}{
		{"--airflow-home", "airflow_home"},
		{"--airflow-dags-folder", "airflow_dags_folder"},
		{"--data-dir", "data_dir"},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("with init flag %s and config key %s", tc.initFlag, tc.configKey), func(t *testing.T) {
			projectDir := t.TempDir()
			err := execFlowCmd("init", projectDir, tc.initFlag, t.TempDir())
			assert.NoError(t, err)

			err = execFlowCmd("config", "--project-dir", projectDir, tc.configKey)
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
		{[]string{cmd, t.TempDir(), "--connection", "sqlite_conn"}},
		{[]string{cmd, t.TempDir(), "--env", "dev"}},
		{[]string{cmd, t.TempDir(), "--verbose"}},
		{[]string{cmd, t.TempDir(), "--no-verbose"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[1])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
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
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--generate-tasks"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--no-generate-tasks"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--no-verbose"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--verbose"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--env", "default"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--env", "dev"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[3])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
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
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--generate-tasks"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--no-generate-tasks"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--no-verbose"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--verbose"}},
		{[]string{cmd, "example_basic_transform", "--project-dir", t.TempDir(), "--env", "default"}},
		{[]string{cmd, "example_templating", "--project-dir", t.TempDir(), "--env", "dev"}},
	}
	for _, tc := range testCases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			err := execFlowCmd("init", tc.args[3])
			assert.NoError(t, err)

			err = execFlowCmd(tc.args...)
			assert.NoError(t, err)
		})
	}
}

func TestDebugFlowFlagInitCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	projectDir := t.TempDir()
	err := execFlowCmd("--debug", "init", projectDir)
	assert.NoError(t, err)
}

func TestDebugFlowFlagRunCmd(t *testing.T) {
	defer patchExecuteCmdInDocker(t, 0, nil)()
	projectDir := t.TempDir()
	err := execFlowCmd("--no-debug", "init", projectDir)
	assert.NoError(t, err)

	err = execFlowCmd("--debug", "run", "example_basic_transform", "--project-dir", projectDir)
	assert.NoError(t, err)
}
