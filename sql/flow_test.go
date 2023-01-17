package sql

import (
	"bufio"
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

func TestExecuteCmdInDockerWithReturnValue(t *testing.T) {
	mockDockerBinder := new(mocks.DockerBind)
	Docker = func() (DockerBind, error) {
		mockDockerBinder.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDockerBinder.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDockerBinder.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDockerBinder.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDockerBinder.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		mockDockerBinder.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockDockerBinder, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, output, err := ExecuteCmdInDocker(testCommand, nil, true)
	assert.NoError(t, err)

	outputString, err := ConvertReadCloserToString(output)
	assert.NoError(t, err)
	assert.Equal(t, "Sample log", outputString)

	mockDockerBinder.AssertExpectations(t)
	DisplayMessages = OriginalDisplayMessages
}

func TestExecuteCmdInDockerSuccess(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, &container.HostConfig{Binds: []string{"/foo:/foo"}}, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDocker.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		mockDocker.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockDocker, nil
	}
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("WriteFile", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		return mockOs
	}
	DisplayMessages = mockDisplayMessagesNil
	_, _, err := ExecuteCmdInDocker(testCommand, []string{"/foo"}, false)
	assert.NoError(t, err)
	DisplayMessages = OriginalDisplayMessages
	Os = NewOsBind
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
	Docker = func() (DockerBind, error) {
		return nil, errMock
	}
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker client initialization failed %w", errMock)
	assert.Equal(t, expectedErr, err)
}

func TestGetPypiVersionFailure(t *testing.T) {
	getPypiVersion = mockGetPypiVersionErr
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	assert.ErrorIs(t, err, errMock)
	getPypiVersion = GetPypiVersion
}

func TestGetBaseDockerImageURI(t *testing.T) {
	getBaseDockerImageURI = mockBaseDockerImageURIErr
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	assert.ErrorIs(t, err, errMock)
	getBaseDockerImageURI = GetBaseDockerImageURI
}

func TestOsWriteFileErr(t *testing.T) {
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("WriteFile", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockOs
	}
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	assert.ErrorIs(t, err, errMock)
	Os = NewOsBind
}

func TestImageBuildFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, errMock)
		return mockDocker, nil
	}
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("image building failed %w", errMock)
	assert.Equal(t, expectedErr, err)
}

func TestImageBuildResponseDisplayMessagesFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesErr
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("image build response read failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
}

func TestContainerCreateFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, errMock)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker container creation failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
}

func TestContainerStartFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker container start failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
}

func TestContainerWaitFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(true))
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker container wait failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
}

func TestContainerLogsFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDocker.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, errMock)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker container logs fetching failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
}

func TestExecuteCmdInDockerLogsCopyFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDocker.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	mockIo := mocks.NewIoBind(t)
	Io = func() IoBind {
		mockIo.On("Copy", mock.Anything, mock.Anything).Return(int64(0), errMock)
		return mockIo
	}
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker logs forwarding failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
	Io = NewIoBind
}

func TestContainerRemoveFailure(t *testing.T) {
	mockDocker := mocks.NewDockerBind(t)
	Docker = func() (DockerBind, error) {
		mockDocker.On("ImageBuild", mock.Anything, mock.Anything, mock.Anything).Return(imageBuildResponse, nil)
		mockDocker.On("ContainerCreate", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(containerCreateCreatedBody, nil)
		mockDocker.On("ContainerStart", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockDocker.On("ContainerWait", mock.Anything, mock.Anything, mock.Anything).Return(getContainerWaitResponse(false))
		mockDocker.On("ContainerLogs", mock.Anything, mock.Anything, mock.Anything).Return(sampleLog, nil)
		mockDocker.On("ContainerRemove", mock.Anything, mock.Anything, mock.Anything).Return(errMock)
		return mockDocker, nil
	}
	DisplayMessages = mockDisplayMessagesNil
	mockIo := mocks.NewIoBind(t)
	Io = func() IoBind {
		mockIo.On("Copy", mock.Anything, mock.Anything).Return(int64(0), nil)
		return mockIo
	}
	_, _, err := ExecuteCmdInDocker(testCommand, nil, false)
	expectedErr := fmt.Errorf("docker remove failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	DisplayMessages = OriginalDisplayMessages
	Io = NewIoBind
}

func TestConvertReadCloserToStringFailure(t *testing.T) {
	mockIo := mocks.NewIoBind(t)
	Io = func() IoBind {
		mockIo.On("Copy", mock.Anything, mock.Anything).Return(int64(0), errMock)
		return mockIo
	}
	_, err := ConvertReadCloserToString(io.NopCloser(strings.NewReader("Hello, world!")))
	expectedErr := fmt.Errorf("converting readcloser output to string failed %w", errMock)
	assert.Equal(t, expectedErr, err)
	Io = NewIoBind
}

func TestGetAstroDockerfileRuntimeVersionDockerfileOpenFailure(t *testing.T) {
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("Open", mock.Anything).Return(&os.File{}, errMock)
		return mockOs
	}
	_, err := getAstroDockerfileRuntimeVersion()
	assert.ErrorIs(t, err, errMock)
	Os = NewOsBind
}

func TestGetAstroDockerfileRuntimeVersionNoBaseAstroRuntimeImage(t *testing.T) {
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("Open", mock.Anything).Return(&os.File{}, nil)
		return mockOs
	}
	mockBufIo := mocks.NewBufIoBind(t)
	BufIo = func() BufIoBind {
		mockBufIo.On("NewScanner", mock.Anything).Return(bufio.NewScanner(strings.NewReader("hello")))
		return mockBufIo
	}
	_, err := getAstroDockerfileRuntimeVersion()
	assert.ErrorIs(t, err, ErrNoBaseAstroRuntimeImage)
	Os = NewOsBind
	BufIo = NewBufIoBind
}

func TestGetAstroDockerfileRuntimeVersionSuccess(t *testing.T) {
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("Open", mock.Anything).Return(&os.File{}, nil)
		return mockOs
	}
	mockBufIo := mocks.NewBufIoBind(t)
	BufIo = func() BufIoBind {
		mockBufIo.On("NewScanner", mock.Anything).Return(bufio.NewScanner(strings.NewReader(runtimeImagePrefix + "7.1.0")))
		return mockBufIo
	}
	_, err := getAstroDockerfileRuntimeVersion()
	assert.NoError(t, err)
	Os = NewOsBind
	BufIo = NewBufIoBind
}

func TestEnsurePythonSdkVersionGetRuntimeVersionFailure(t *testing.T) {
	originalGetAstroDockerfileRuntimeVersion := getAstroDockerfileRuntimeVersion
	getAstroDockerfileRuntimeVersion = func() (string, error) {
		return "", errMock
	}
	err := EnsurePythonSdkVersionIsMet()
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
}

func TestEnsurePythonSdkVersionGetPypiVersionFailure(t *testing.T) {
	originalGetAstroDockerfileRuntimeVersion := getAstroDockerfileRuntimeVersion
	getAstroDockerfileRuntimeVersion = func() (string, error) {
		return "", nil
	}
	originalGetPypiVersion := getPypiVersion
	getPypiVersion = func(projectURL string) (string, error) {
		return "", errMock
	}
	err := EnsurePythonSdkVersionIsMet()
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = originalGetPypiVersion
}

func TestEnsurePythonSdkVersionGetCompatibilityFailure(t *testing.T) {
	originalGetAstroDockerfileRuntimeVersion := getAstroDockerfileRuntimeVersion
	getAstroDockerfileRuntimeVersion = func() (string, error) {
		return "", nil
	}
	originalGetPypiVersion := getPypiVersion
	getPypiVersion = func(projectURL string) (string, error) {
		return "", nil
	}
	originalGetPythonSDKComptability := getPythonSDKComptability
	getPythonSDKComptability = func(configURL, sqlCliVersion string) (astroRuntimeVersion string, astroSDKPythonVersion string, err error) {
		return "", "", errMock
	}
	err := EnsurePythonSdkVersionIsMet()
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = originalGetPypiVersion
	getPythonSDKComptability = originalGetPythonSDKComptability
}
