package airflow

import (
	"context"
	"errors"
	"io"

	"github.com/docker/docker/api/types/registry"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/airflow/runtimes"
)

func (s *Suite) TestDockerRegistryInit() {
	resp, err := DockerRegistryInit("test")
	s.NoError(err)
	s.Equal(resp.registry, "test")
}

func (s *Suite) TestRegistryLogin() {
	s.Run("success", func() {
		mockClient := new(mocks.DockerRegistryAPI)
		mockClient.On("NegotiateAPIVersion", context.Background()).Return(nil).Once()
		mockClient.On("RegistryLogin", context.Background(), mock.AnythingOfType("registry.AuthConfig")).Return(registry.AuthenticateOKBody{}, nil).Once()

		handler := DockerRegistry{
			registry: "test",
			cli:      mockClient,
		}

		err := handler.Login("testuser", "testtoken")
		s.NoError(err)
		mockClient.AssertExpectations(s.T())
	})

	s.Run("registry error", func() {
		mockClient := new(mocks.DockerRegistryAPI)
		mockClient.On("NegotiateAPIVersion", context.Background()).Return(nil).Once()
		mockClient.On("RegistryLogin", context.Background(), mock.AnythingOfType("registry.AuthConfig")).Return(registry.AuthenticateOKBody{}, errMockDocker).Once()

		handler := DockerRegistry{
			registry: "test",
			cli:      mockClient,
		}

		err := handler.Login("", "")
		s.ErrorIs(err, errMockDocker)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDockerLogin() {
	// Store original cmdExecWithStdin to restore after tests
	originalCmdExecWithStdin := cmdExecWithStdin
	defer func() {
		cmdExecWithStdin = originalCmdExecWithStdin
	}()

	s.Run("success with credentials", func() {
		var capturedCmd string
		var capturedStdin string
		var capturedArgs []string
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			capturedCmd = cmd
			capturedStdin = stdin
			capturedArgs = args
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.NoError(err)
		s.Equal("docker", capturedCmd)
		s.Equal("testtoken", capturedStdin, "password must be piped to stdin, not interpolated into a command string")
		s.Equal([]string{"login", "test.registry.com", "-u", "testuser", "--password-stdin"}, capturedArgs)
	})

	s.Run("success with bearer token", func() {
		var capturedCmd string
		var capturedStdin string
		var capturedArgs []string
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			capturedCmd = cmd
			capturedStdin = stdin
			capturedArgs = args
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "Bearer testtoken123")
		s.NoError(err)
		s.Equal("docker", capturedCmd)
		// Should strip Bearer prefix
		s.Equal("testtoken123", capturedStdin)
		s.Equal([]string{"login", "test.registry.com", "-u", "testuser", "--password-stdin"}, capturedArgs)
	})

	s.Run("password with shell metacharacters reaches login intact", func() {
		const trickyPass = `p$(whoami)"'` + "`id`"
		var capturedStdin string
		var capturedArgs []string
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			capturedStdin = stdin
			capturedArgs = args
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", trickyPass)
		s.NoError(err)
		s.Equal(trickyPass, capturedStdin, "password must reach the login command unmodified, not interpolated into a shell string")
		s.NotContains(capturedArgs, "-c")
	})

	s.Run("no operation with empty credentials", func() {
		cmdExecCalled := false
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "", "")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExecWithStdin should not be called with empty credentials")
	})

	s.Run("no operation with empty username", func() {
		cmdExecCalled := false
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "", "testtoken")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExecWithStdin should not be called with empty username")
	})

	s.Run("no operation with empty token", func() {
		cmdExecCalled := false
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExecWithStdin should not be called with empty token")
	})

	s.Run("docker login command fails", func() {
		expectedErr := errors.New("docker login failed")
		cmdExecWithStdin = func(cmd, stdin string, stdout, stderr io.Writer, args ...string) error {
			return expectedErr
		}

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.Error(err)
		s.Contains(err.Error(), "docker login failed")
	})

	s.Run("container runtime not found", func() {
		// Mock runtimes.GetContainerRuntimeBinary to return error
		originalFunc := runtimes.GetContainerRuntimeBinary
		runtimes.GetContainerRuntimeBinary = func() (string, error) {
			return "", errors.New("container runtime not found")
		}
		defer func() {
			runtimes.GetContainerRuntimeBinary = originalFunc
		}()

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.Error(err)
		s.Contains(err.Error(), "container runtime not found")
	})
}
