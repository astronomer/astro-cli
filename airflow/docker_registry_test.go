package airflow

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"

	"github.com/docker/docker/api/types/registry"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/astronomer/astro-cli/airflow/runtimes"
	"github.com/astronomer/astro-cli/pkg/logger"
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

	s.Run("debug log hides the token", func() {
		const fakeToken = "fake-secret-token-9d3a7b"
		mockClient := new(mocks.DockerRegistryAPI)
		mockClient.On("NegotiateAPIVersion", context.Background()).Return(nil).Once()
		mockClient.On("RegistryLogin", context.Background(), mock.AnythingOfType("registry.AuthConfig")).Return(registry.AuthenticateOKBody{}, errMockDocker).Once()

		prevLevel := logger.GetLevel()
		var out bytes.Buffer
		logger.SetLevel(logrus.DebugLevel)
		logger.SetOutput(&out)
		defer func() {
			logger.SetLevel(prevLevel)
			logger.SetOutput(os.Stderr)
		}()

		handler := DockerRegistry{registry: "test", cli: mockClient}
		err := handler.Login("testuser", fakeToken)
		s.ErrorIs(err, errMockDocker)
		s.Contains(out.String(), "secret set: true")
		s.NotContains(out.String(), fakeToken)
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
	type loginCall struct{ runtime, server, username, password string }

	s.Run("success with credentials", func() {
		var calls []loginCall
		registryLogin = func(containerRuntime, server, username, password string, _, _ io.Writer) error {
			calls = append(calls, loginCall{containerRuntime, server, username, password})
			return nil
		}

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.NoError(err)
		s.Equal([]loginCall{{"docker", "test.registry.com", "testuser", "testtoken"}}, calls)
	})

	for _, tc := range []struct{ name, username, token string }{
		{"no operation with empty credentials", "", ""},
		{"no operation with empty username", "", "testtoken"},
		{"no operation with empty token", "testuser", ""},
	} {
		s.Run(tc.name, func() {
			loginCalled := false
			registryLogin = func(_, _, _, _ string, _, _ io.Writer) error {
				loginCalled = true
				return nil
			}

			err := DockerLogin("test.registry.com", tc.username, tc.token)
			s.NoError(err)
			s.False(loginCalled)
		})
	}

	s.Run("docker login command fails", func() {
		registryLogin = func(_, _, _, _ string, _, _ io.Writer) error { return errMockDocker }

		err := DockerLogin("test.registry.com", "testuser", "testtoken")
		s.ErrorIs(err, errMockDocker)
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
