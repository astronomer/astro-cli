package airflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/astronomer/astro-cli/airflow/mocks"
	airflowTypes "github.com/astronomer/astro-cli/airflow/types"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
)

var errMock = errors.New("build error")

func (s *Suite) TestDockerImageBuild() {
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

	s.Run("build success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err = handler.Build("", "secret", options)
		s.NoError(err)
	})

	s.Run("build --no-cache", func() {
		options.NoCache = true
		options.Output = false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "--no-cache")
			return nil
		}
		err = handler.Build("", "", options)
		s.NoError(err)
	})

	s.Run("build with no --pull", func() {
		dockerfileContent := "FROM nginx"
		dockerfile, _ := os.CreateTemp(os.TempDir(), "temp-Dockerfile")
		defer os.Remove(dockerfile.Name())
		dockerfile.WriteString(dockerfileContent)
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.NotContains(args, "--pull")
			return nil
		}
		err = handler.Build(dockerfile.Name(), "", options)
		s.NoError(err)
	})

	s.Run("build with no --pull with multiple images", func() {
		dockerfileContent := `FROM nginx
FROM quay.io/astronomer/astro-runtime:12.0.0`
		dockerfile, _ := os.CreateTemp(os.TempDir(), "temp-Dockerfile")
		defer os.Remove(dockerfile.Name())
		dockerfile.WriteString(dockerfileContent)
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.NotContains(args, "--pull")
			return nil
		}
		err = handler.Build(dockerfile.Name(), "", options)
		s.NoError(err)
	})

	s.Run("build with --pull", func() {
		dockerfileContent := "FROM quay.io/astronomer/astro-runtime:12.0.0"
		dockerfile, _ := os.CreateTemp(os.TempDir(), "temp-Dockerfile")
		defer os.Remove(dockerfile.Name())
		dockerfile.WriteString(dockerfileContent)
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "--pull")
			return nil
		}
		err = handler.Build(dockerfile.Name(), "", options)
		s.NoError(err)
	})

	s.Run("build --label", func() {
		options.Labels = []string{"io.astronomer.skip.revision=true"}
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "--label", "io.astronomer.skip.revision=true")
			return nil
		}
		err = handler.Build("", "", options)
		s.NoError(err)
	})
	s.Run("build error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err = handler.Build("", "", options)
		s.Contains(err.Error(), errMock.Error())
	})
	s.Run("unable to read file error", func() {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
			Output:          false,
		}

		err = handler.Build("", "", options)
		s.Error(err)
	})
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

	s.Run("pytest success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		s.NoError(err)
	})

	s.Run("create error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "create":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		s.Error(err)
	})

	s.Run("start error", func() {
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
		s.Error(err)
		s.Equal(out, "exit code 1")
	})

	s.Run("copy error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			switch {
			case args[0] == "cp":
				return errMock
			default:
				return nil
			}
		}
		_, err = handler.Pytest("", "", "", "", []string{}, true, options)
		s.Error(err)
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
		_, err = handler.Pytest("", "", "", "", []string{}, false, options)
		s.Contains(err.Error(), errMock.Error())
	})
	s.Run("unable to read file error", func() {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.Pytest("", "", "", "", []string{}, false, options)
		s.Error(err)
	})
}

func (s *Suite) TestDockerImageConflictTest() {
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

	s.Run("conflict test success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		_, err = handler.ConflictTest("", "", options)
		s.NoError(err)
	})

	s.Run("conflict test create error", func() {
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
		s.Error(err)
	})

	s.Run("conflict test cp error", func() {
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
		s.Error(err)
	})

	s.Run("conflict test rm error", func() {
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
		s.Error(err)
	})
	s.Run("unable to read file error", func() {
		options := airflowTypes.ImageBuildConfig{
			Path:            "incorrect-path",
			TargetPlatforms: []string{"linux/amd64"},
			NoCache:         false,
		}

		_, err = handler.ConflictTest("", "", options)
		s.Error(err)
	})
}

func (s *Suite) TestParseExitCode() {
	output := "exit code: 1"
	s.Run("success", func() {
		_ = parseExitCode(output)
		_ = parseExitCode("")
	})
}

func (s *Suite) TestDockerCreatePipFreeze() {
	handler := DockerImage{
		imageName: "testing",
	}

	cwd, err := os.Getwd()
	s.NoError(err)

	pipFreeze := cwd + "/pip-freeze-test.txt"
	defer afero.NewOsFs().Remove(pipFreeze)

	s.Run("create pip freeze success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.CreatePipFreeze("", pipFreeze)
		s.NoError(err)
	})
	s.Run("create pip freeze error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.CreatePipFreeze("", pipFreeze)
		s.Error(err)
	})
	s.Run("unable to read file error", func() {
		err := handler.CreatePipFreeze("", "")
		s.Error(err)
	})
}

func (s *Suite) TestDockerPull() {
	handler := DockerImage{
		imageName: "testing",
	}

	s.Run("pull image without username", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.Pull("", "", "")
		s.NoError(err)
	})

	s.Run("pull image with username", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}
		err := handler.Pull("", "username", "")
		s.NoError(err)
	})
	s.Run("pull error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errMock
		}
		err := handler.Pull("", "", "")
		s.Error(err)
	})

	s.Run("login error", func() {
		err := handler.Pull("", "username", "")
		s.Error(err)
	})

	for _, tc := range []struct {
		input         string
		username      string
		platform      string
		expected      string
		expectedLogin string
	}{
		{"images.astronomer.io/foo/bar:123", "username", testUtil.CloudPlatform, "images.astronomer.io/foo/bar:123", "images.astronomer.io"},
		{"images.astronomer.io/foo/bar:123", "username", testUtil.LocalPlatform, "images.astronomer.io/foo/bar:123", "localhost"},
		// Software doesn't pass a username to Push
		{"images.software/foo/bar:123", "", testUtil.SoftwarePlatform, "images.software/foo/bar:123", ""},
	} {
		s.Run(tc.input, func() {
			testUtil.InitTestConfig(tc.platform)
			pullSeen := false
			loginSeen := false
			cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
				switch cmd {
				case "bash":
					// The _current_ way we log in.
					s.Contains(args[1], tc.expectedLogin)
					loginSeen = true
					return nil
				case "docker":
					if args[0] == "pull" {
						s.Contains(args, tc.expected)
						pullSeen = true
						return nil
					}
				}
				return fmt.Errorf("unexpected command %q %q", cmd, args)
			}
			err := handler.Pull(tc.input, tc.username, "")
			s.NoError(err)
			s.True(pullSeen)
			s.Equal(loginSeen, tc.expectedLogin != "", "docker login expected to be seen %v", tc.expectedLogin != "")
		})
	}
}

func (s *Suite) TestDockerImagePush() {
	handler := DockerImage{
		imageName: "testing",
	}

	s.Run("docker tag failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "tag")
			return errMockDocker
		}

		err := handler.Push("test", "", "test")
		s.ErrorIs(err, errMockDocker)
	})

	// Set the default for the rest of the subtests -- run without error
	cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error { return nil }

	s.Run("success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error { return nil }

		displayJSONMessagesToStream = func(_ io.ReadCloser, _ func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "test-username", "test")
		s.NoError(err)
	})

	s.Run("success with docker cred store", func() {
		displayJSONMessagesToStream = func(_ io.ReadCloser, _ func(jsonmessage.JSONMessage)) error {
			return nil
		}

		err := handler.Push("test", "", "")
		s.NoError(err)
	})

	for _, tc := range []struct {
		input         string
		username      string
		platform      string
		expected      string
		expectedLogin string
	}{
		{"images.astronomer.io/foo/bar:123", "username", testUtil.CloudPlatform, "images.astronomer.io/foo/bar:123", "images.astronomer.io"},
		{"images.dataplane/foo/bar:123", "username", testUtil.CloudPlatform, "images.dataplane/foo/bar:123", "images.dataplane"},
		{"images.astronomer.io/foo/bar:123", "username", testUtil.LocalPlatform, "images.astronomer.io/foo/bar:123", "localhost:5555"},
		// Software doesn't pass a username to Push
		{"images.software/foo/bar:123", "", testUtil.SoftwarePlatform, "images.software/foo/bar:123", ""},
	} {
		s.Run(tc.input, func() {
			testUtil.InitTestConfig(tc.platform)

			mockClient := new(mocks.DockerCLIClient)
			mockClient.On("NegotiateAPIVersion", context.Background()).Once()
			mockClient.On("ImagePush", context.Background(), tc.expected, mock.MatchedBy(func(opts image.PushOptions) bool {
				decodedAuth, err := base64.URLEncoding.DecodeString(opts.RegistryAuth)
				if err != nil {
					return false
				}

				authConfig := map[string]string{}
				err = json.Unmarshal(decodedAuth, &authConfig)
				if err != nil {
					return false
				}

				expected := map[string]string{}
				if tc.expectedLogin != "" {
					expected = map[string]string{
						"username":      "username",
						"serveraddress": tc.expectedLogin,
					}
				}

				return s.Equal(expected, authConfig)
			})).Return(io.NopCloser(strings.NewReader("{}")), nil).Once()

			getDockerClient = func() (client.APIClient, error) { return mockClient, nil }

			err := handler.Push(tc.input, tc.username, "")
			s.NoError(err)
			mockClient.AssertExpectations(s.T())
		})
	}

	s.Run("docker library failure", func() {
		// This path is used to support running Colima whichn is "docker-cli compatible" but wasn't 100% library compatible in the past.
		// That was 3 years ago though, so we should re-test and work out if this fallback to using bash is still needed or not
		getDockerClient = func() (client.APIClient, error) { return nil, fmt.Errorf("foreced error") }
		err := handler.Push("repo/test/image", "username", "")
		s.NoError(err)
	})
}

func (s *Suite) TestDockerImageGetLabel() {
	handler := DockerImage{
		imageName: "testing",
	}

	s.Run("success", func() {
		mockLabel := "test-label"
		mockResp := "test-response"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[2], mockLabel)
			io.WriteString(stdout, mockResp)
			return nil
		}

		resp, err := handler.GetLabel("", mockLabel)
		s.NoError(err)
		s.Equal(mockResp, resp)
	})

	s.Run("cmdExec error", func() {
		mockLabel := "test-label"
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[2], mockLabel)
			return errMockDocker
		}

		_, err := handler.GetLabel("", mockLabel)
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

		_, err := handler.GetLabel("", mockLabel)
		s.ErrorIs(err, errGetImageLabel)
	})
}

func (s *Suite) TestDockerImageListLabel() {
	handler := DockerImage{
		imageName: "testing",
	}

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

func (s *Suite) TestDoesImageExist() {
	handler := DockerImage{
		imageName: "testing",
	}
	testImage := "image"

	s.Run("success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "inspect")
			return nil
		}

		err := handler.DoesImageExist(testImage)
		s.NoError(err)
	})

	s.Run("cmdExec error", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args, "inspect")
			return errMockDocker
		}

		err := handler.DoesImageExist(testImage)
		s.ErrorIs(err, errMockDocker)
	})
}

func (s *Suite) TestDockerTagLocalImage() {
	handler := DockerImage{
		imageName: "testing",
	}

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
	s.Run("success", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains([]string{"-c", "push", "rmi"}, args[0])
			return nil
		}
		err := pushWithBash(&types.AuthConfig{Username: "testing", Password: "pass"}, "test")
		s.NoError(err)
	})

	s.Run("exec failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(args[0], "push")
			return errMockDocker
		}
		err := pushWithBash(&types.AuthConfig{}, "test")
		s.ErrorIs(err, errMockDocker)
	})

	s.Run("login exec failure", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			s.Contains(cmd, "bash")
			return errMockDocker
		}
		err := pushWithBash(&types.AuthConfig{Username: "testing"}, "test")
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

	s.Run("run success without container", func() {
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
		s.NoError(err)
	})

	s.Run("run success with container", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return nil
		}

		err = handler.Run("", "./testfiles/airflow_settings_invalid.yaml", "", "test-container", "", "", true)
		s.NoError(err)
	})

	s.Run("run error without container", func() {
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			return errExecMock
		}

		err = handler.Run("", "./testfiles/airflow_settings.yaml", "", "", "", "", true)
		s.Contains(err.Error(), errExecMock.Error())
	})
}
