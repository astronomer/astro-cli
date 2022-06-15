package airflow

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPodmanPushSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanImageMock := &PodmanImage{imageName: "test", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("Push", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := podmanImageMock.Push("test.astro.io", "token", "2")
	assert.NoError(t, err)
}

func TestPodmanPushFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanImageMock := &PodmanImage{imageName: "test", podmanBind: bindMock, conn: context.TODO()}

	bindMock.On("Push", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(errPodman).Once()

	err := podmanImageMock.Push("test.astro.io", "token", "2")
	assert.Contains(t, err.Error(), "error pushing test image to registry.test.astro.io")
}
