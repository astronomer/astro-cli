package sql

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/jsonmessage"
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
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		return mockDockerBinder, nil
	}
	PrintBuildingSteps = func(r io.Reader) error {
		return nil
	}
	err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	assert.NoError(t, err)
	mockDockerBinder.AssertExpectations(t)
	PrintBuildingSteps = printBuildingSteps
}

func TestPrintBuildingSteps(t *testing.T) {
	expectedOutput := `Installing flow.. This might take some time.
Step 2/4 : ENV ASTRO_CLI Yes
`
	// Capture output from PrintBuildingSteps Println calls
	actualOutput := ""
	Println = func(a ...any) (n int, err error) {
		for _, arg := range a {
			actualOutput += fmt.Sprintln(arg)
		}
		return 0, nil
	}

	streams := []string{
		"Step 2/4 : ENV ASTRO_CLI Yes",
		" ---> Running in 0afb2e0c5ad7",
	}
	var allData []byte
	for _, stream := range streams {
		jsonMessage := jsonmessage.JSONMessage{Stream: stream}
		data, err := json.Marshal(jsonMessage)
		assert.NoError(t, err)
		allData = append(allData, data...)
	}
	reader := bytes.NewReader(allData)
	err := PrintBuildingSteps(reader)
	assert.NoError(t, err)
	assert.Equal(t, expectedOutput, actualOutput)
}

func TestDockerClientInitFailure(t *testing.T) {
	DockerClientInit = func() (DockerBind, error) {
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
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	assert.ErrorIs(t, err, errMock)
	getPypiVersion = GetPypiVersion
}

func TestImageBuildFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, errMock)
		return mockDockerBinder, nil
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("image building failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestImageBuildResponsePrintBuildingStepsFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		return mockDockerBinder, nil
	}
	PrintBuildingSteps = func(r io.Reader) error {
		return errMock
	}
	err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("retrieving logs failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	PrintBuildingSteps = printBuildingSteps
}

func TestImageBuildResponseReadFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
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
	ioCopy = io.Copy
}

func TestContainerCreateFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
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
	DockerClientInit = func() (DockerBind, error) {
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
	DockerClientInit = func() (DockerBind, error) {
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
	DockerClientInit = func() (DockerBind, error) {
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
	DockerClientInit = func() (DockerBind, error) {
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
