package airflow

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPodmanLoginSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanRegistryMock := &PodmanRegistry{conn: context.TODO(), podmanBind: bindMock, registry: "test.astro.io"}
	bindMock.On("Login", podmanRegistryMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := podmanRegistryMock.Login("user", "token")
	assert.NoError(t, err)
}

func TestPodmanLoginFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanRegistryMock := &PodmanRegistry{conn: context.TODO(), podmanBind: bindMock, registry: "test.astro.io"}
	bindMock.On("Login", podmanRegistryMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(errPodman)

	err := podmanRegistryMock.Login("user", "token")
	assert.Contains(t, err.Error(), "some podman error")
}
