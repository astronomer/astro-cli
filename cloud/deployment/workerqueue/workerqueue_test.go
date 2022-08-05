package workerqueue

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var errGetDeployment = errors.New("test get deployment error")

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
	expectedWorkerQueue := astro.WorkerQueue{
		Name:              "",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	deploymentRespNoQueues := []astro.Deployment{
		{
			ID:             "test-deployment-id",
			Label:          "test-deployment-label",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{},
			WorkerQueues:   []astro.WorkerQueue{},
		},
		{
			ID:             "test-deployment-id-1",
			Label:          "test-deployment-label-1",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{},
			WorkerQueues:   []astro.WorkerQueue{},
		},
	}
	deploymentRespWithQueues := []astro.Deployment{
		{
			ID:             "test-deployment-id",
			Label:          "test-deployment-label",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{},
			WorkerQueues:   []astro.WorkerQueue{},
		},
		{
			ID:             "test-deployment-id-1",
			Label:          "test-deployment-label-1",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{},
			WorkerQueues:   []astro.WorkerQueue{},
		},
	}
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkerCount: astro.WorkerQueueOption{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astro.WorkerQueueOption{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	t.Run("happy path creates a new worker queue for an existing deployment when no worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker-queue " + expectedWorkerQueue.Name + " for test-deployment-id in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 0, 0, 0, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("happy path creates a new worker queue for an existing deployment when worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker-queue " + expectedWorkerQueue.Name + " for test-deployment-id in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 0, 0, 0, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, ErrWorkerQueueDefaultOptions).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 0, 200, 0, mockClient, out)
		assert.ErrorIs(t, err, ErrWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, errGetDeployment).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 0, 0, 0, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		// mock os.Stdin
		expectedInput := []byte("test-invalid-deployment-id")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
		err = Create("test-ws-id", "", "", false, 0, 0, 0, mockClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue is not valid", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 25, 0, 0, mockClient, out)
		assert.ErrorIs(t, err, ErrInvalidWorkerQueueOption)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errGetDeployment).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, 0, 0, 0, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
}

func TestGetWorkerQueueDefaultOptions(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkerCount: astro.WorkerQueueOption{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astro.WorkerQueueOption{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	t.Run("happy path returns worker-queue default options", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err := GetWorkerQueueDefaultOptions(mockClient)
		assert.NoError(t, err)
		assert.Equal(t, mockWorkerQueueDefaultOptions, actual)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, ErrWorkerQueueDefaultOptions).Once()
		_, err := GetWorkerQueueDefaultOptions(mockClient)
		assert.ErrorIs(t, err, ErrWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(t)
	})
}

func TestIsWorkerQueueOptionValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkerCount: astro.WorkerQueueOption{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astro.WorkerQueueOption{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	requestedWorkerQueue := astro.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}

	t.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.NoError(t, err)
	})
	t.Run("returns an error when min worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, ErrInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "min worker count must be between 1 and 20: worker-queue option is invalid")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, ErrInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "max worker count must be between 20 and 200: worker-queue option is invalid")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, ErrInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker concurrency must be between 175 and 275: worker-queue option is invalid")
	})
}
