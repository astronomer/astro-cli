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

	"github.com/astronomer/astro-cli/version"

	"github.com/astronomer/astro-cli/sql/mocks"
	"github.com/astronomer/astro-cli/sql/testutil"
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
	mockGetPypiVersionErr = func(configUrl string, cliVersion string) (AstroSQLCliVersion, error) {
		return AstroSQLCliVersion{}, errMock
	}
	mockBaseDockerImageURIErr = func(astroSQLCLIConfigURL string) (string, error) {
		return "", errMock
	}
	originalGetAstroDockerfileRuntimeVersion = getAstroDockerfileRuntimeVersion
	mockGetAstroDockerfileRuntimeVersion     = func() (string, error) {
		return "7.1.0", nil
	}
	mockGetPypiVersion = func(configUrl string, cliVersion string) (AstroSQLCliVersion, error) {
		return AstroSQLCliVersion{}, nil
	}
	mockGetPythonSDKComptabilityUnMatch = func(configURL, sqlCliVersion string) (astroRuntimeVersion, astroSDKPythonVersion string, err error) {
		return ">7.1.0", "==1.3.0", nil
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
	version.CurrVersion = "1.8.0"
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
	mockBufIo := mocks.NewBufIOBind(t)
	BufIo = func() BufIOBind {
		mockBufIo.On("NewScanner", mock.Anything).Return(bufio.NewScanner(strings.NewReader("hello")))
		return mockBufIo
	}
	_, err := getAstroDockerfileRuntimeVersion()
	assert.ErrorIs(t, err, ErrNoBaseAstroRuntimeImage)
	Os = NewOsBind
	BufIo = NewBufIOBind
}

func TestGetAstroDockerfileRuntimeVersionSuccess(t *testing.T) {
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("Open", mock.Anything).Return(&os.File{}, nil)
		return mockOs
	}
	mockBufIo := mocks.NewBufIOBind(t)
	BufIo = func() BufIOBind {
		mockBufIo.On("NewScanner", mock.Anything).Return(bufio.NewScanner(strings.NewReader(runtimeImagePrefix + "7.1.0")))
		return mockBufIo
	}
	_, err := getAstroDockerfileRuntimeVersion()
	assert.NoError(t, err)
	Os = NewOsBind
	BufIo = NewBufIOBind
}

func TestEnsurePythonSdkVersionGetRuntimeVersionFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = func() (string, error) {
		return "", errMock
	}
	err := EnsurePythonSdkVersionIsMet(nil)
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
}

func TestEnsurePythonSdkVersionGetPypiVersionFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersionErr
	err := EnsurePythonSdkVersionIsMet(nil)
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
}

func TestEnsurePythonSdkVersionGetCompatibilityFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = func(configURL, sqlCliVersion string) (astroRuntimeVersion, astroSDKPythonVersion string, err error) {
		return "", "", errMock
	}
	err := EnsurePythonSdkVersionIsMet(nil)
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
}

func TestEnsurePythonSdkVersionRequiredVersionNotMetFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = func(configURL, sqlCliVersion string) (astroRuntimeVersion string, astroSDKPythonVersion string, err error) {
		return "invalid constraint", "", nil
	}
	err := EnsurePythonSdkVersionIsMet(nil)
	assert.EqualError(t, err, "Malformed constraint: invalid constraint")
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
}

func TestEnsurePythonSdkVersionSelectNoPrompt(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return([]byte("\nastro-sdk-python"), nil)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectNoMock{})
	assert.ErrorIs(t, err, ErrPythonSDKVersionNotMet)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
}

func TestEnsurePythonSdkVersionSelectErrPrompt(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return([]byte("\nastro-sdk-python"), nil)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectErrMock{})
	assert.EqualError(t, err, errMock.Error())
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
}

func TestEnsurePythonSdkVersionRequirementsReadFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return(nil, errMock)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectYesMock{})
	assert.ErrorIs(t, err, errMock)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
	Os = NewOsBind
}

func TestEnsurePythonSdkVersionRequirementsContainsDependency(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return([]byte("\nastro-sdk-python==1.3.0"), nil)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectYesMock{})
	assert.NoError(t, err)
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
	Os = NewOsBind
}

func TestEnsurePythonSdkVersionRequirementsAddDependencyOpenAppendFileFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return([]byte("\nastro-sdk-python==1.2.0"), nil)
		mockOs.On("OpenFile", mock.Anything, mock.Anything, mock.Anything).Return(&os.File{}, errMock)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectYesMock{})
	assert.EqualError(t, err, errMock.Error())
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
	Os = NewOsBind
}

func TestEnsurePythonSdkVersionRequirementsAddDependencyWriteAppendFileFailure(t *testing.T) {
	getAstroDockerfileRuntimeVersion = mockGetAstroDockerfileRuntimeVersion
	getPypiVersion = mockGetPypiVersion
	getPythonSDKComptability = mockGetPythonSDKComptabilityUnMatch
	tmpFile := "/tmp/tmp.txt"
	mockOpenFile, _ := os.OpenFile(tmpFile, os.O_APPEND, 0o600)
	defer os.Remove(tmpFile)
	mockOs := mocks.NewOsBind(t)
	Os = func() OsBind {
		mockOs.On("ReadFile", mock.Anything).Return([]byte("\nastro-sdk-python==1.2.0"), nil)
		mockOs.On("OpenFile", mock.Anything, mock.Anything, mock.Anything).Return(mockOpenFile, nil)
		return mockOs
	}
	err := EnsurePythonSdkVersionIsMet(testutil.PromptSelectYesMock{})
	assert.EqualError(t, err, "invalid argument")
	getAstroDockerfileRuntimeVersion = originalGetAstroDockerfileRuntimeVersion
	getPypiVersion = GetPypiVersion
	getPythonSDKComptability = GetPythonSDKComptability
	Os = NewOsBind
}
