package airflow

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("build error")

func TestDockerImageBuild(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	options := airflowTypes.ImageBuildConfig{
		Path:            cwd,
		TargetPlatforms: []string{"linux/amd64"},
		NoCache:         false,
	}

	previousCmdExec := cmdExec

	t.Run("build success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build(options, true)
		assert.NoError(t, err)
	})

	t.Run("build --no-cache", func(t *testing.T) {
		options.NoCache = true
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "--no-cache")
			return nil
		}
		err = handler.Build(options, false)
		assert.NoError(t, err)
	})

	t.Run("build error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err = handler.Build(options, false)
		assert.Contains(t, err.Error(), errMock.Error())
	})

	t.Run("unable to read file error", func(t *testing.T) {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		err = handler.Build(options, false)
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestDockerImagePytest(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	options := airflowTypes.ImageBuildConfig{
		Path:            cwd,
		TargetPlatforms: []string{"linux/amd64"},
		NoCache:         false,
	}

	previousCmdExec := cmdExec

	t.Run("pytest success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.Pytest("", "", "", []string{}, options)
		assert.NoError(t, err)
	})

	t.Run("pytest error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		_, err = handler.Pytest("", "", "", []string{}, options)
		assert.Contains(t, err.Error(), errMock.Error())
	})

	t.Run("unable to read file error", func(t *testing.T) {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.Pytest("", "", "", []string{}, options)
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestDockerImagePush(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	t.Run("docker tag failure", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "tag")
			return errMockDocker
		}

		err := handler.Push("test", "", "test", "test")
		assert.ErrorIs(t, err, errMockDocker)
	})

	t.Run("success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "test-username", "test", "test")
		assert.NoError(t, err)
	})

	t.Run("success with docker cred store", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "", "", "test")
		assert.NoError(t, err)
	})
}

func TestDockerImageGetLabel(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	t.Run("success", func(t *testing.T) {
		mockLabel := "test-label"
		mockResp := "test-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args[2], mockLabel)
			io.WriteString(stdout, mockResp)
			return nil
		}

		resp, err := handler.GetLabel(mockLabel)
		assert.NoError(t, err)
		assert.Equal(t, mockResp, resp)
	})

	t.Run("cmdExec error", func(t *testing.T) {
		mockLabel := "test-label"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args[2], mockLabel)
			return errMockDocker
		}

		_, err := handler.GetLabel(mockLabel)
		assert.ErrorIs(t, err, errMockDocker)
	})

	t.Run("cmdExec failure", func(t *testing.T) {
		mockLabel := "test-label"
		mockErrResp := "test-err-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args[2], mockLabel)
			io.WriteString(stderr, mockErrResp)
			return nil
		}

		_, err := handler.GetLabel(mockLabel)
		assert.ErrorIs(t, err, errGetImageLabel)
	})
}

func TestDockerImageListLabel(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	t.Run("success", func(t *testing.T) {
		mockResp := `{"test-label": "test-val"}`
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "inspect")
			io.WriteString(stdout, mockResp)
			return nil
		}

		resp, err := handler.ListLabels()
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"test-label": "test-val"}, resp)
	})

	t.Run("cmdExec error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "inspect")
			return errMockDocker
		}

		_, err := handler.ListLabels()
		assert.ErrorIs(t, err, errMockDocker)
	})

	t.Run("cmdExec failure", func(t *testing.T) {
		mockErrResp := "test-err-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "inspect")
			io.WriteString(stderr, mockErrResp)
			return nil
		}

		_, err := handler.ListLabels()
		assert.ErrorIs(t, err, errGetImageLabel)
	})
}

func TestDockerTagLocalImage(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec

	t.Run("rename local image success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.TagLocalImage("custom-image")
		assert.NoError(t, err)
	})

	t.Run("rename local image error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.TagLocalImage("custom-image")
		assert.Contains(t, err.Error(), errMock.Error())
	})

	cmdExec = previousCmdExec
}

func TestExecCmd(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		err := cmdExec("test", stdout, stderr, "-f", "docker_image_test.go")
		assert.NoError(t, err)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("invalid cmd", func(t *testing.T) {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		err := cmdExec("invalid-cmd", stdout, stderr)
		assert.Contains(t, err.Error(), "failed to find the invalid-cmd command")
	})
}

func TestUseBash(t *testing.T) {
	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	t.Run("success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, []string{"-c", "push", "rmi"}, args[0])
			return nil
		}
		err := useBash(&types.AuthConfig{Username: "testing", Password: "pass"}, "test")
		assert.NoError(t, err)
	})

	t.Run("exec failure", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args[0], "push")
			return errMockDocker
		}
		err := useBash(&types.AuthConfig{}, "test")
		assert.ErrorIs(t, err, errMockDocker)
	})

	t.Run("login exec failure", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, cmd, "bash")
			return errMockDocker
		}
		err := useBash(&types.AuthConfig{Username: "testing"}, "test")
		assert.ErrorIs(t, err, errMockDocker)
	})
}

func TestDockerImageRunTest(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	previousCmdExec := cmdExec
	previousRunCommandCh := RunCommandCh

	t.Run("run success without container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		RunCommandCh = func(taskLogs bool, cutset, command string, flags ...string) (DagRunInfo, error) {
			return DagRunInfo{
				time: "time",
				failedTasks: 0,
			}, nil
		}

		err = handler.RunTest("", "./testfiles/airflow_settings.yaml", "", "", "", true)
		assert.NoError(t, err)
	})

	t.Run("run success with container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		RunCommandCh = func(taskLogs bool, cutset, command string, flags ...string) (DagRunInfo, error) {
			return DagRunInfo{
				time: "time",
				failedTasks: 0,
			}, nil
		}

		err = handler.RunTest("", "./testfiles/airflow_settings.yaml", "", "", "test-container", true)
		assert.NoError(t, err)
	})

	t.Run("run error without container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errExecMock
		}

		RunCommandCh = func(taskLogs bool, cutset, command string, flags ...string) (DagRunInfo, error) {
			return DagRunInfo{
				time: "",
				failedTasks: 1,
			}, errExecMock
		}

		err = handler.RunTest("", "./testfiles/airflow_settings.yaml", "", "", "", true)
		assert.Contains(t, err.Error(), errExecMock.Error())
	})

	cmdExec = previousCmdExec
	RunCommandCh = previousRunCommandCh
}

func TestRunCommandCh(t *testing.T) {

	t.Run("RunCommandCh success", func(t *testing.T) {
		_, err := RunCommandCh(true, "\n", "echo", []string{"\"\ntesting\nTestRunCommandCh\"",}... )
		assert.NoError(t, err)
	})
	t.Run("RunCommandCh unsuccessful command", func(t *testing.T) {
		_, err := RunCommandCh(true, "\n", "error-cmd", []string{}...)
		assert.Contains(t, err.Error(), "error-cmd")
	})
}

func TestParseOuputLine(t *testing.T) {
	t.Run("case running task", func(t *testing.T) {
		failedTask, _, _, tasks := parseOuputLine("Running task test-task", "", "", 0, 0, true)
		assert.Equal(t, 1, tasks)
		assert.Equal(t, failedTask, "test-task")
	})
	t.Run("case Time:  ", func(t *testing.T) {
		_, time, _, _ := parseOuputLine("Time:  time-test", "", "", 0, 0, true)
		assert.Equal(t, time, "time-test")
	})
	t.Run("case ran successfully!", func(t *testing.T) {
		_, _, successfulRun, _ := parseOuputLine(" ran successfully!", "", "", 0, 0, true)
		assert.Equal(t, successfulRun, 1)
	})
	t.Run("case default", func(t *testing.T) {
		failedTask, time, successfulRun, tasks := parseOuputLine("", "", "", 0, 0, true)
		assert.Equal(t, successfulRun, 0)
		assert.Equal(t, time, "")
		assert.Equal(t, tasks, 0)
		assert.Equal(t, failedTask, "")
	})
}