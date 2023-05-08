package airflow

import (
	"bytes"
	"errors"
	"io"
	"os"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/spf13/afero"
)

var errMock = errors.New("build error")

func (s *Suite) TestDockerImageBuild() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	s.NoError(err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	options := airflowTypes.ImageBuildConfig{
		Path:            cwd,
		TargetPlatforms: []string{"linux/amd64"},
		NoCache:         false,
		Output:          true,
	}

	previousCmdExec := cmdExec

	s.Run("build success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build(options)
		s.NoError(err)
	})

	s.Run("build --no-cache", func() {
		options.NoCache = true
		options.Output = false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "--no-cache")
			return nil
		}
		err = handler.Build(options)
		s.NoError(err)
	})

	s.Run("build error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err = handler.Build(options)
		s.Contains(err.Error(), errMock.Error())
	})

	s.Run("unable to read file error", func() {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		err = handler.Build(options)
		s.Error(err)
	})

	cmdExec = previousCmdExec
}

func (s *Suite) TestDockerImagePytest() {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	s.NoError(err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	options := airflowTypes.ImageBuildConfig{
		Path:            cwd,
		TargetPlatforms: []string{"linux/amd64"},
		NoCache:         false,
		Output:          true,
	}

	previousCmdExec := cmdExec

	s.Run("pytest success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.Pytest("", "", "", []string{}, options)
		s.NoError(err)
	})

	s.Run("pytest error", func() {
		options = airflowTypes.ImageBuildConfig{
			Path:            cwd,
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		_, err = handler.Pytest("", "", "", []string{}, options)
		s.Contains(err.Error(), errMock.Error())
	})

	s.Run("unable to read file error", func() {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.Pytest("", "", "", []string{}, options)
		s.Error(err)
	})

	cmdExec = previousCmdExec
}

func (s *Suite) TestDockerImagePush() {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	s.Run("docker tag failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "tag")
			return errMockDocker
		}

		err := handler.Push("test", "", "test", "test")
		s.ErrorIs(err, errMockDocker)
	})

	s.Run("success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "test-username", "test", "test")
		s.NoError(err)
	})

	s.Run("success with docker cred store", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		displayJSONMessagesToStream = func(responseBody io.ReadCloser, auxCallback func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "", "", "test")
		s.NoError(err)
	})
}

func (s *Suite) TestDockerImageGetLabel() {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	s.Run("success", func() {
		mockLabel := "test-label"
		mockResp := "test-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[2], mockLabel)
			io.WriteString(stdout, mockResp)
			return nil
		}

		resp, err := handler.GetLabel(mockLabel)
		s.NoError(err)
		s.Equal(mockResp, resp)
	})

	s.Run("cmdExec error", func() {
		mockLabel := "test-label"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[2], mockLabel)
			return errMockDocker
		}

		_, err := handler.GetLabel(mockLabel)
		s.ErrorIs(err, errMockDocker)
	})

	s.Run("cmdExec failure", func() {
		mockLabel := "test-label"
		mockErrResp := "test-err-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[2], mockLabel)
			io.WriteString(stderr, mockErrResp)
			return nil
		}

		_, err := handler.GetLabel(mockLabel)
		s.ErrorIs(err, errGetImageLabel)
	})
}

func (s *Suite) TestDockerImageListLabel() {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	s.Run("success", func() {
		mockResp := `{"test-label": "test-val"}`
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "inspect")
			io.WriteString(stdout, mockResp)
			return nil
		}

		resp, err := handler.ListLabels()
		s.NoError(err)
		s.Equal(map[string]string{"test-label": "test-val"}, resp)
	})

	s.Run("cmdExec error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "inspect")
			return errMockDocker
		}

		_, err := handler.ListLabels()
		s.ErrorIs(err, errMockDocker)
	})

	s.Run("cmdExec failure", func() {
		mockErrResp := "test-err-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "inspect")
			io.WriteString(stderr, mockErrResp)
			return nil
		}

		_, err := handler.ListLabels()
		s.ErrorIs(err, errGetImageLabel)
	})
}

func (s *Suite) TestDockerTagLocalImage() {
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec

	s.Run("rename local image success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.TagLocalImage("custom-image")
		s.NoError(err)
	})

	s.Run("rename local image error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.TagLocalImage("custom-image")
		s.Contains(err.Error(), errMock.Error())
	})

	cmdExec = previousCmdExec
}

func (s *Suite) TestExecCmd() {
	s.Run("success", func() {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		err := cmdExec("test", stdout, stderr, "-f", "docker_image_test.go")
		s.NoError(err)
		s.Empty(stdout.String())
		s.Empty(stderr.String())
	})

	s.Run("invalid cmd", func() {
		stdout := new(bytes.Buffer)
		stderr := new(bytes.Buffer)
		err := cmdExec("invalid-cmd", stdout, stderr)
		s.Contains(err.Error(), "failed to find the invalid-cmd command")
	})
}

func (s *Suite) TestUseBash() {
	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	s.Run("success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains([]string{"-c", "push", "rmi"}, args[0])
			return nil
		}
		err := useBash(&types.AuthConfig{Username: "testing", Password: "pass"}, "test")
		s.NoError(err)
	})

	s.Run("exec failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[0], "push")
			return errMockDocker
		}
		err := useBash(&types.AuthConfig{}, "test")
		s.ErrorIs(err, errMockDocker)
	})

	s.Run("login exec failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(cmd, "bash")
			return errMockDocker
		}
		err := useBash(&types.AuthConfig{Username: "testing"}, "test")
		s.ErrorIs(err, errMockDocker)
	})
}

func (s *Suite) TestDockerImageRun() {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	s.NoError(err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	previousCmdExec := cmdExec

	s.Run("run success without container", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		err = handler.Run("", "./testfiles/airflow_settings.yaml", "", "", "", true)
		s.NoError(err)
	})

	s.Run("run success with container", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		err = handler.Run("", "./testfiles/airflow_settings_invalid.yaml", "", "test-container", "", true)
		s.NoError(err)
	})

	s.Run("run error without container", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errExecMock
		}

		err = handler.Run("", "./testfiles/airflow_settings.yaml", "", "", "", true)
		s.Contains(err.Error(), errExecMock.Error())
	})

	cmdExec = previousCmdExec
}
