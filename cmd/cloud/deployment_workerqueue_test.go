package cloud

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1_mocks "github.com/astronomer/astro-cli/astro-client-v1/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	testPoolID  = "test-pool-id"
	testPoolID1 = "test-pool-id-1"
)

func TestNewDeploymentWorkerQueueRootCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)

	t.Run("worker-queue command runs", func(t *testing.T) {
		wQueueCmd := newDeploymentWorkerQueueRootCmd(os.Stdout)
		wQueueCmd.SetOut(buf)
		_, err := wQueueCmd.ExecuteC()
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "worker-queue")
	})
}

func TestNewDeploymentWorkerQueueCreateCmd(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	t.Run("create worker queue when no deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-n", "test-queue", "-t", "test-worker-1"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-d", "test-id-1", "-t", "test-worker-1", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "--deployment-name", "test", "-t", "test-worker-1", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
	})
	t.Run("create worker queue when no name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "test-queue")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		cmdArgs := []string{"worker-queue", "create", "-d", "test-id-1", "-t", "test-worker-1"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	t.Run("happy path delete worker queue", func(t *testing.T) {
		testID := "test-wq-id"
		astroMachine := "A5"
		workerQueueList := []astrov1.WorkerQueue{
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
		expectedOutMessage := "worker queue test-worker-queue-1 for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace deleted\n"

		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		cmdArgs := []string{"worker-queue", "delete", "-n", "test-worker-queue-1", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOutMessage)

		mockV1Client.AssertExpectations(t)
		mockV1Client.AssertExpectations(t)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	t.Run("throw error if worker concurrency is set as 0", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "update", "--concurrency", "0"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errZeroConcurrency)
	})
	t.Run("happy path update worker queue", func(t *testing.T) {
		// mock os.Stdin
		defer testUtil.MockUserInput(t, "1")()
		deploymentResponse.JSON200.WorkerQueues = &[]astrov1.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		expectedOutMessage := "worker queue default for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace updated\n"
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)

		// updating min count
		cmdArgs := []string{"worker-queue", "update", "-n", "default", "-t", "test-worker-1", "--min-count", "0", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedOutMessage)
		mockV1Client.AssertExpectations(t)
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
