package airflow

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/airflow/mocks"
	"github.com/pkg/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPodmanPushSuccess(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanImageMock := &PodmanImage{imageName: "test", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("Tag", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	bindMock.On("Push", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	bindMock.On("Untag", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	err := podmanImageMock.Push("test.astro.io", "token", "2")
	assert.NoError(t, err)
}

func TestPodmanPushFailure(t *testing.T) {
	bindMock := new(mocks.PodmanBind)
	podmanImageMock := &PodmanImage{imageName: "test", podmanBind: bindMock, conn: context.TODO()}
	bindMock.On("Tag", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("some tag error")).Once()

	err := podmanImageMock.Push("test.astro.io", "token", "2")
	assert.Contains(t, err.Error(), "command 'podman tag test registry.test.astro.io/test/airflow:2' failed")

	bindMock.On("Tag", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	bindMock.On("Push", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("some image push error")).Once()

	err = podmanImageMock.Push("test.astro.io", "token", "2")
	assert.Contains(t, err.Error(), "Error pushing test image to registry.test.astro.io")

	bindMock.On("Push", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	bindMock.On("Untag", podmanImageMock.conn, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New("some image untag error"))

	err = podmanImageMock.Push("test.astro.io", "token", "2")
	assert.Contains(t, err.Error(), "command 'podman untag registry.test.astro.io/test/airflow:2' failed")
}
