package cloud

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestNewDeploymentWorkerQueueRootCmd(t *testing.T) {
	expectedHelp := "Manage worker queues for an Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)

	t.Run("worker-queue command runs", func(t *testing.T) {
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
	mockClient := new(astro_mocks.Client)
	astroClient = mockClient
	deploymentRespNoQueues := []astro.Deployment{
		{
			ID:             "test-deployment-id",
			Label:          "test-deployment-label",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Workers: astro.Workers{
					AU: 10,
				},
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{},
		},
		{
			ID:             "test-deployment-id-1",
			Label:          "test-deployment-label-1",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Workers: astro.Workers{
					AU: 10,
				},
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{},
		},
	}
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkerCount: astro.WorkerQueueOption{
			Floor:   21,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astro.WorkerQueueOption{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}

	t.Run("-h prints worker-queue help", func(t *testing.T) {
		cmdArgs := []string{"worker-queue", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Contains(t, resp, expectedHelp)
	})

	t.Run("create worker queue for existing deployments when no deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue  for test-deployment-id in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		mockClient.On("ListDeployments", mock.Anything).Return(deploymentRespNoQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("create worker queue for existing deployments when deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue  for test-deployment-id-1 in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		mockClient.On("ListDeployments", mock.Anything).Return(deploymentRespNoQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-d", "test-deployment-id-1"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})

	// TODO discuss if we want to print both usage and the error?
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "create"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.Contains(t, resp, expectedOut)
	})
	// TODO When updating existing queues, pass in ID of the existing queues
	// TODO When updating existing queues, is changing the name of the existing queue/s allowed?
	// TODO no short-hand for min-count, max-count, concurrency or isDefault
	// TODO what if no worker-queue name was provided --> (prompt user for the name)
	// TODO what if the user wants to create a worker queue on a non-default nodepool? --> (use the worker-type to map to nodepool)
	// TODO what if no deployments exist? --> (manually test)
	// TODO more error cases
}
