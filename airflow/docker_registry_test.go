package airflow

import (
	"context"
	"errors"
	"io"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/docker/docker/api/types/registry"
	"github.com/stretchr/testify/mock"
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
	// Store original cmdExec to restore after tests
	originalCmdExec := cmdExec
	defer func() {
		cmdExec = originalCmdExec
	}()

	s.Run("success with credentials", func() {
		var capturedCmd string
		var capturedArgs []string
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			capturedCmd = cmd
			capturedArgs = args
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.NoError(err)
		s.Equal("bash", capturedCmd)
		s.Equal([]string{"-c", "echo \"testtoken\" | docker login test.registry.com -u testuser --password-stdin"}, capturedArgs)
	})

	s.Run("success with bearer token", func() {
		var capturedCmd string
		var capturedArgs []string
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			capturedCmd = cmd
			capturedArgs = args
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "Bearer testtoken123")
		s.NoError(err)
		s.Equal("bash", capturedCmd)
		// Should strip Bearer prefix
		s.Equal([]string{"-c", "echo \"testtoken123\" | docker login test.registry.com -u testuser --password-stdin"}, capturedArgs)
	})

	s.Run("no operation with empty credentials", func() {
		cmdExecCalled := false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "", "")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExec should not be called with empty credentials")
	})

	s.Run("no operation with empty username", func() {
		cmdExecCalled := false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "", "testtoken")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExec should not be called with empty username")
	})

	s.Run("no operation with empty token", func() {
		cmdExecCalled := false
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
			cmdExecCalled = true
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "")
		s.NoError(err)
		s.False(cmdExecCalled, "cmdExec should not be called with empty token")
	})

	s.Run("docker login command fails", func() {
		expectedErr := errors.New("docker login failed")
		cmdExec = func(cmd string, stdout, stderr io.Writer, args ...string) error {
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
