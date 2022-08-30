package cloud

import (
	"bytes"
	"os"
	"testing"
	"time"

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
	deploymentRespDefaultQueue := []astro.Deployment{
		{
			ID:             "test-deployment-id",
			Label:          "test-deployment-label",
			RuntimeRelease: astro.RuntimeRelease{Version: "5.0.8"},
			Cluster: astro.Cluster{
				NodePools: []astro.NodePool{
					{
						ID:               "test-pool-id",
						IsDefault:        false,
						NodeInstanceType: "test-instance-type",
						CreatedAt:        time.Now(),
					},
					{
						ID:               "test-pool-id-1",
						IsDefault:        true,
						NodeInstanceType: "test-instance-type-1",
						CreatedAt:        time.Now(),
					},
				},
			},
			DeploymentSpec: astro.DeploymentSpec{
				Workers: astro.Workers{
					AU: 10,
				},
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id",
					Name:              "test-default-queue",
					IsDefault:         true,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "test-wq-id-1",
					Name:              "test-default-queue-1",
					IsDefault:         false,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-pool-id-1",
				},
			},
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

	t.Run("create worker queue when no deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue  for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
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

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-t", "test-instance-type"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment id was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
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

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-d", "test-deployment-id", "-t", "test-instance-type", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("create worker queue when deployment name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
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

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "--deployment-name", "test-deployment-label", "-t", "test-instance-type", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("create worker queue when no name was provided", func(t *testing.T) {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("test-queue")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-d", "test-deployment-id", "-t", "test-instance-type"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		assert.Equal(t, expectedoutput, actualOut)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "create"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
		assert.NotContains(t, resp, expectedOut)
	})
	// TODO When updating existing queues, pass in ID of the existing queues
	// TODO When updating existing queues, is changing the name of the existing queue/s allowed?
	// TODO any more error cases
}
