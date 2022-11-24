package sql

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	mockDisplayMessagesNil     = func(r io.Reader) error {
		return nil
	}
	mockDisplayMessagesErr = func(r io.Reader) error {
		return errMock
	}
	mockIoCopyNil = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, nil
	}
	mockIoCopyErr = func(dst io.Writer, src io.Reader) (written int64, err error) {
		return 0, errMock
	}
	mockGetPypiVersionErr = func(projectURL string) (string, error) {
		return "", errMock
	}
	mockBaseDockerImageURIErr = func(astroSQLCLIConfigURL string) (string, error) {
		return "", errMock
	}
)

func getContainerWaitResponse(raiseError bool) (bodyCh <-chan container.ContainerWaitOKBody, errCh <-chan error) {
	containerWaitOkBodyChannel := make(chan container.ContainerWaitOKBody)
	errChannel := make(chan error, 1)
	go func() {
		if raiseError {
			errChannel <- errMock
			return
		}
		res := container.ContainerWaitOKBody{StatusCode: 0}
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
		mockDockerBinder.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockDockerBinder, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	assert.NoError(t, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
}

func TestDisplayMessages(t *testing.T) {
	orgStdout := os.Stdout
	defer func() { os.Stdout = orgStdout }()
	r, w, _ := os.Pipe()
	os.Stdout = w

	streams := []string{
		"Step 2/4 : ENV ASTRO_CLI Yes",
		"\n",
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
	err := DisplayMessages(reader)
	assert.NoError(t, err)

	w.Close()
	out, _ := io.ReadAll(r)
	expectedOutput := `Installing flow... This might take some time.
Step 2/4 : ENV ASTRO_CLI Yes
`
	assert.Equal(t, expectedOutput, string(out))
}

func TestDisplayMessagesHasError(t *testing.T) {
	jsonMessage := jsonmessage.JSONMessage{Error: &jsonmessage.JSONError{Message: "An error has occurred."}}
	data, err := json.Marshal(jsonMessage)
	assert.NoError(t, err)

	reader := bytes.NewReader(data)
	err = DisplayMessages(reader)
	assert.Error(t, err)
}

func TestDockerClientInitFailure(t *testing.T) {
	DockerClientInit = func() (DockerBind, error) {
		return nil, errMock
	}
	_, err := CommonDockerUtil(testCommand, nil, map[string]string{"flag": "value"}, []string{"mountDirectory"})
	expectedErr := fmt.Errorf("docker client initialization failed %w", errMock)
	assert.Equal(t, expectedErr, err)
}

func TestGetPypiVersionFailure(t *testing.T) {
	getPypiVersion = mockGetPypiVersionErr
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	assert.ErrorIs(t, err, errMock)
	getPypiVersion = GetPypiVersion
}

func TestGetBaseDockerImageURI(t *testing.T) {
	getBaseDockerImageURI = mockBaseDockerImageURIErr
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	assert.ErrorIs(t, err, errMock)
	getBaseDockerImageURI = GetBaseDockerImageURI
}

func TestImageBuildFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, errMock)
		return mockDockerBinder, nil
	}
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("image building failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
}

func TestImageBuildResponseDisplayMessagesFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		return mockDockerBinder, nil
	}
	DisplayMessages = mockDisplayMessagesErr
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("image build response read failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
}

func TestContainerCreateFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, errMock)
		return mockDockerBinder, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container creation failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
}

func TestContainerStartFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockDockerBinder, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container start failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
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
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container wait failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
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
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker container logs fetching failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
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
	DisplayMessages = mockDisplayMessagesNil
	IoCopy = mockIoCopyErr
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker logs forwarding failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
	IoCopy = io.Copy
}

func TestContainerRemoveFailure(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	DockerClientInit = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		mockDockerBinder.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockDockerBinder, nil
	}
	IoCopy = mockIoCopyNil
	DisplayMessages = mockDisplayMessagesNil
	_, err := CommonDockerUtil(testCommand, nil, nil, nil)
	expectedErr := fmt.Errorf("docker remove failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = displayMessages
	IoCopy = io.Copy
}
