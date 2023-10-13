package airflow

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/docker/docker/api/types/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDockerRegistryInit(t *testing.T) {
	resp, err := DockerRegistryInit("test")
	assert.NoError(t, err)
	assert.Equal(t, resp.registry, "test")
}

func TestRegistryLogin(t *testing.T) {
	// t.Run("success", func(t *testing.T) {
	// 	mockClient := new(mocks.DockerRegistryAPI)
	// 	mockClient.On("NegotiateAPIVersion", context.Background()).Return(nil).Once()
	// 	mockClient.On("RegistryLogin", context.Background(), mock.AnythingOfType("types.AuthConfig")).Return(registry.AuthenticateOKBody{}, nil).Once()

	// 	handler := DockerRegistry{
	// 		registry: "test",
	// 		cli:      mockClient,
	// 	}

	// 	err := handler.Login("", "")
	// 	assert.NoError(t, err)
	// 	mockClient.AssertExpectations(t)
	// })

	t.Run("registry error", func(t *testing.T) {
		mockClient := new(mocks.DockerRegistryAPI)
		mockClient.On("NegotiateAPIVersion", context.Background()).Return(nil).Once()
		mockClient.On("RegistryLogin", context.Background(), mock.AnythingOfType("types.AuthConfig")).Return(registry.AuthenticateOKBody{}, errMockDocker).Once()

		handler := DockerRegistry{
			registry: "test",
			cli:      mockClient,
		}

		err := handler.Login("", "")
		assert.ErrorIs(t, err, errMockDocker)
		mockClient.AssertExpectations(t)
	})
}
