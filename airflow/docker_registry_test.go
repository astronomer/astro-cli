package airflow

import (
	"context"

	"github.com/astronomer/astro-cli/airflow/mocks"
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

		err := handler.Login("", "")
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
