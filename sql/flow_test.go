package sql

import (
	"io"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/mock"
)

func TestCommonDockerUtil(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(types.ImageBuildResponse{
			Body: io.NopCloser(strings.NewReader("Image built")),
		}, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(container.ContainerCreateCreatedBody{
			ID: "123",
		}, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		containerWaitOkBodyChannel := make(chan container.ContainerWaitOKBody)
		errChannel := make(chan error, 1)
		go func() {
			res := container.ContainerWaitOKBody{StatusCode: 200, Error: nil}
			containerWaitOkBodyChannel <- res
			errChannel <- nil
		}()
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(<-containerWaitOkBodyChannel, <-errChannel)

		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(io.NopCloser(strings.NewReader("Sample Log")), nil)

		return mockDockerBinder, nil
	}

	CommonDockerUtil([]string{"flow", "test"}, nil, nil, nil)
	mockDockerBinder.AssertExpectations(t)
}
