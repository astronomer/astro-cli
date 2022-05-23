package airflow

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/stretchr/testify/assert"
)

func TestDockerImageBuild(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	options := airflowTypes.ImageBuildConfig{
		Path:    cwd,
		NoCache: false,
	}

	previousCmdExec := cmdExec

	t.Run("build success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build(options)
		assert.NoError(t, err)
	})

	t.Run("build --no-cache", func(t *testing.T) {
		options.NoCache = true
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "--no-cache")
			return nil
		}
		err = handler.Build(options)
		assert.NoError(t, err)
	})

	t.Run("build error", func(t *testing.T) {
		mockError := errors.New("build error")
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return mockError
		}
		err = handler.Build(options)
		assert.Contains(t, err.Error(), mockError.Error())
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

		err := handler.Push("test", "", "test", "test")
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
			assert.Contains(t, []string{"\"pass\"", "push"}, args[0])
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
			assert.Contains(t, cmd, "echo")
			return errMockDocker
		}
		err := useBash(&types.AuthConfig{Username: "testing"}, "test")
		assert.ErrorIs(t, err, errMockDocker)
	})
}
