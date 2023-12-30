package airflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var errMock = errors.New("build error")

func TestDockerImageBuild(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

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
		Output:          true,
	}

	previousCmdExec := cmdExec

	t.Run("build success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build("", "", options)
		assert.NoError(t, err)
	})

	t.Run("build --no-cache", func(t *testing.T) {
		options.NoCache = true
		options.Output = false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "--no-cache")
			return nil
		}
		err = handler.Build("", "", options)
		assert.NoError(t, err)
	})

	t.Run("build error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err = handler.Build("", "", options)
		assert.Contains(t, err.Error(), errMock.Error())
	})
	t.Run("unable to read file error", func(t *testing.T) {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		err = handler.Build("", "", options)
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestDockerImagePytest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		Output:          true,
	}

	previousCmdExec := cmdExec

	t.Run("pytest success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		assert.NoError(t, err)
	})

	t.Run("create error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "create":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		assert.Error(t, err)
	})

	t.Run("start error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "start":
				return errMock
			case args[0] == "inspect":
				stdout.Write([]byte(`exit code 1`)) // making sure exit code is captured properly
				return nil
			default:
				return nil
			}
		}
		out, err := handler.Pytest("", "", "", "", []string{}, true, options)
		assert.Error(t, err)
		assert.Equal(t, out, "exit code 1")
	})

	t.Run("copy error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "cp":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		assert.Error(t, err)
	})

	t.Run("pytest error", func(t *testing.T) {
		options = airflowTypes.ImageBuildConfig{
			Path:            cwd,
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		_, err = handler.Pytest("", "", "", "", []string{}, false, options)
		assert.Contains(t, err.Error(), errMock.Error())
	})
	t.Run("unable to read file error", func(t *testing.T) {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.Pytest("", "", "", "", []string{}, false, options)
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestDockerImageConflictTest(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		Output:          true,
	}

	previousCmdExec := cmdExec

	t.Run("conflict test success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.ConflictTest("", "", options)
		assert.NoError(t, err)
	})

	t.Run("conflict test create error", func(t *testing.T) {
		options = airflowTypes.ImageBuildConfig{
			Path:            cwd,
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		_, err = handler.ConflictTest("", "", options)
		assert.Error(t, err)
	})

	t.Run("conflict test cp error", func(t *testing.T) {
		options = airflowTypes.ImageBuildConfig{
			Path:            cwd,
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "cp":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.ConflictTest("", "", options)
		assert.Error(t, err)
	})

	t.Run("conflict test rm error", func(t *testing.T) {
		options = airflowTypes.ImageBuildConfig{
			Path:            cwd,
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "rm":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.ConflictTest("", "", options)
		assert.Error(t, err)
	})
	t.Run("unable to read file error", func(t *testing.T) {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.ConflictTest("", "", options)
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestParseExitCode(t *testing.T) {
	output := "exit code: 1"
	t.Run("success", func(t *testing.T) {
		_ = parseExitCode(output)
		_ = parseExitCode("")
	})
}

func TestDockerCreatePipFreeze(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	pipFreeze := cwd + "/pip-freeze-test.txt"
	defer afero.NewOsFs().Remove(pipFreeze)

	previousCmdExec := cmdExec

	t.Run("create pip freeze success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.CreatePipFreeze("", pipFreeze)
		assert.NoError(t, err)
	})
	t.Run("create pip freeze error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.CreatePipFreeze("", pipFreeze)
		assert.Error(t, err)
	})
	t.Run("unable to read file error", func(t *testing.T) {
		err := handler.CreatePipFreeze("", "")
		assert.Error(t, err)
	})

	cmdExec = previousCmdExec
}

func TestDockerPull(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	handler := DockerImage{
		imageName: "testing",
	}

	previousCmdExec := cmdExec

	t.Run("pull image without username", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.Pull("", "", "", "")
		assert.NoError(t, err)
	})

	t.Run("pull image with username", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.Pull("", "username", "", "")
		assert.NoError(t, err)
	})
	t.Run("pull error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.Pull("", "", "", "")
		assert.Error(t, err)
	})

	t.Run("login error", func(t *testing.T) {
		err := handler.Pull("", "username", "", "")
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

		resp, err := handler.GetLabel("", mockLabel)
		assert.NoError(t, err)
		assert.Equal(t, mockResp, resp)
	})

	t.Run("cmdExec error", func(t *testing.T) {
		mockLabel := "test-label"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args[2], mockLabel)
			return errMockDocker
		}

		_, err := handler.GetLabel("", mockLabel)
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

		_, err := handler.GetLabel("", mockLabel)
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

func TestDoesImageExist(t *testing.T) {
	handler := DockerImage{
		imageName: "testing",
	}
	testImage := "image"

	previousCmdExec := cmdExec
	defer func() { cmdExec = previousCmdExec }()

	t.Run("success", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "inspect")
			return nil
		}

		err := handler.DoesImageExist(testImage)
		assert.NoError(t, err)
	})

	t.Run("cmdExec error", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			assert.Contains(t, args, "inspect")
			return errMockDocker
		}

		err := handler.DoesImageExist(testImage)
		assert.ErrorIs(t, err, errMockDocker)
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

func TestDockerImageRun(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)

	dockerIgnoreFile := cwd + "/.dockerignore"
	fileutil.WriteStringToFile(dockerIgnoreFile, "")
	defer afero.NewOsFs().Remove(dockerIgnoreFile)

	previousCmdExec := cmdExec

	t.Run("run success without container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			if args[0] == "run" {
				expectedArgs := []string{
					"run_dag",
					"./dags/", "",
					"./", "--verbose",
				}
				for i := 0; i < 5; i++ {
					if expectedArgs[i] != args[i+15] {
						fmt.Println(args[i+15])
						fmt.Println(expectedArgs[i])
						return errMock // Elements from index 0 to 4 in slice1 are not equal to elements from index 5 to 9 in slice2
					}
				}
			}

			return nil
		}

		err = handler.Run("", "./testfiles/airflow_settings.yaml", "", "", "", "", true)
		assert.NoError(t, err)
	})

	t.Run("run success with container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		err = handler.Run("", "./testfiles/airflow_settings_invalid.yaml", "", "test-container", "", "", true)
		assert.NoError(t, err)
	})

	t.Run("run error without container", func(t *testing.T) {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errExecMock
		}

		err = handler.Run("", "./testfiles/airflow_settings.yaml", "", "", "", "", true)
		assert.Contains(t, err.Error(), errExecMock.Error())
	})

	cmdExec = previousCmdExec
}
