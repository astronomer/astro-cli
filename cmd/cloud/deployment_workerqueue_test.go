package cloud

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient         = new(astrocore_mocks.ClientWithResponsesInterface)
	testPoolID             = "test-pool-id"
	testPoolID1            = "test-pool-id-1"
)

func TestNewDeploymentWorkerQueueRootCmd(t *testing.T) {
	expectedHelp := "Manage worker queues for an Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)

	t.Run("worker-queue command runs", func(t *testing.T) {
		// testUtil.SetupOSArgsForGinkgo()
		wQueueCmd := newDeploymentWorkerQueueRootCmd(os.Stdout)
		wQueueCmd.SetOut(buf)
		_, err := wQueueCmd.ExecuteC()
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "worker-queue")
	})

	t.Run("-h prints worker-queue help", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
}

func TestNewDeploymentWorkerQueueCreateCmd(t *testing.T) {
	expectedHelp := "Create a worker queue for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient
	t.Run("-h prints worker-queue help", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("create worker queue when no deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-n", "test-queue", "-t", "test-worker-1"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-d", "test-id-1", "-t", "test-worker-1", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "--deployment-name", "test", "-t", "test-worker-1", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("create worker queue when no name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "test-queue")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-d", "test-id-1", "-t", "test-worker-1"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "create"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.NotContains(t, resp, expectedOut)
	})
}

func TestNewDeploymentWorkerQueueDeleteCmd(t *testing.T) {
	expectedHelp := "Delete a worker queue from an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	t.Run("-h prints worker-queue help", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "delete", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("happy path delete worker queue", func(t *testing.T) {
		testID := "test-wq-id"
		astroMachine := "A5"
		workerQueueList := []astroplatformcore.WorkerQueue{
			{
				Id:                testID,
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolId:        &testPoolID,
				PodCpu:            "huge",
				PodMemory:         "lots",
				AstroMachine:      &astroMachine,
			},
			{
				Id:                "test-wq-id-1",
				Name:              "test-worker-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolId:        &testPoolID1,
				AstroMachine:      &astroMachine,
			},
		}
		deploymentResponse.JSON200.WorkerQueues = &workerQueueList
		expectedOutMessage := "worker queue test-worker-queue-1 for test in ck05r3bor07h40d02y2hw4n4v workspace deleted\n"

		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		cmdArgs := []string{"worker-queue", "delete", "-n", "test-worker-queue-1", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOutMessage)

		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "delete"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.NotContains(t, resp, expectedOut)
	})
}

func TestNewDeploymentWorkerQueueUpdateCmd(t *testing.T) {
	expectedHelp := "Update a worker queue for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	platformCoreClient = mockPlatformCoreClient
	astroCoreClient = mockCoreClient

	t.Run("-h prints worker-queue help", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})
	t.Run("throw error if worker concurrency is set as 0", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "update", "--concurrency", "0"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errZeroConcurrency)
	})
	t.Run("happy path update worker queue", func(t *testing.T) {
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		expectedOutMessage := "worker queue default for test in ck05r3bor07h40d02y2hw4n4v workspace updated\n"
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)

		// updating min count
		cmdArgs := []string{"worker-queue", "update", "-n", "default", "-t", "test-worker-1", "--min-count", "0", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOutMessage)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "update"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.NotContains(t, resp, expectedOut)
	})
}
