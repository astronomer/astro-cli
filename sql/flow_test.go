package sql

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	testCommand        = []string{"flow", "test"}
	errMock            = errors.New("mock error")
	imageBuildResponse = types.ImageBuildResponse{
		Body: io.NopCloser(strings.NewReader("Image built")),
	}
	containerCreateCreatedBody = container.ContainerCreateCreatedBody{ID: "123"}
	sampleLog                  = io.NopCloser(strings.NewReader("Sample log"))
)

func getContainerWaitResponse(raiseError bool) (bodyCh <-chan container.ContainerWaitOKBody, errCh <-chan error) {
	containerWaitOkBodyChannel := make(chan container.ContainerWaitOKBody)
	errChannel := make(chan error, 1)
	go func() {
		if raiseError {
			errChannel <- errMock
			return
		}
		res := container.ContainerWaitOKBody{StatusCode: 200, Error: nil}
		containerWaitOkBodyChannel <- res
		errChannel <- nil
	}()
	// converting bidirectional channel to read only channels for ContainerWait to consume
	var readOnlyStatusCh <-chan container.ContainerWaitOKBody
	var readOnlyErrCh <-chan error
	readOnlyStatusCh = containerWaitOkBodyChannel
	readOnlyErrCh = errChannel
	return readOnlyStatusCh, readOnlyErrCh
}

func TestCommonDockerUtilSuccess(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	assert.NoError(t, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestDockerClientInitFailure(t *testing.T) {
	dockerClientInit = func() (DockerBind, error) {
		return nil, errMock
	}
	err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	expectedErr := fmt.Errorf("docker client initialization failed %w", errMock)
	assert.Equal(t, expectedErr, err)
}

func TestGetPypiVersionFailure(t *testing.T) {
	getPypiVersion = func(projectURL string) (string, error) {
		return "", errMock
	}
	err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	assert.ErrorIs(t, err, errMock)
}

func TestImageBuildFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, errMock)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("image building failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestImageBuildResponseReadFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		return mockDockerBinder, nil
	}
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, errMock
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("image build response read failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestContainerCreateFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, errMock)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container creation failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestContainerStartFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container start failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestContainerWaitFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(true))
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container wait failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestContainerLogsFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, errMock)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container logs fetching failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestCommonDockerUtilLogsCopyFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	dockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		return mockDockerBinder, nil
	}
	ioCopyCallCounter := 1
	ioCopy = func(dst io.Writer, src io.Reader) (written int64, err error) {
		if ioCopyCallCounter == 2 {
			return 0, errMock
		}
		ioCopyCallCounter++
		return 0, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker logs forwarding failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}
