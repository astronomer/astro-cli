package workerqueue

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errGetDeployment    = errors.New("test get deployment error")
	errUpdateDeployment = errors.New("test deployment update error")
)

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
	expectedWorkerQueue := astro.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	deploymentRespNoQueues := []astro.Deployment{
		{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
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
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
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
				Scheduler: astro.Scheduler{
					AU:       9,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{},
		},
	}
	deploymentRespWithQueues := []astro.Deployment{
		{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
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
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id",
					Name:              "test-queue",
					IsDefault:         true,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "test-wq-id-1",
					Name:              "test-queue-1",
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
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id-2",
					Name:              "test-queue-2",
					IsDefault:         false,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-nodepool-id-2",
				},
				{
					ID:                "test-wq-id-3",
					Name:              "test-queue-3",
					IsDefault:         true,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-nodepool-id-3",
				},
			},
		},
	}
	deploymentUpdateInput := astro.DeploymentUpdateInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "test-queue",
				IsDefault:         true,
				MaxWorkerCount:    130,
				MinWorkerCount:    12,
				WorkerConcurrency: 110,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				MaxWorkerCount:    175,
				MinWorkerCount:    8,
				WorkerConcurrency: 150,
				NodePoolID:        "test-pool-id-1",
			},
			{
				Name:              "test-worker-queue",
				IsDefault:         false,
				MaxWorkerCount:    125,
				MinWorkerCount:    5,
				WorkerConcurrency: 180,
				NodePoolID:        "test-pool-id-1",
			},
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
	t.Run("happy path creates a new worker queue for a deployment when no worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		defer testUtil.MockUserInput(t, "y")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("happy path creates a new worker queue for a deployment when worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		defer testUtil.MockUserInput(t, "2")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", 0, 0, 0, false, mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, errGetDeployment).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 200, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue would update an existing queue with the same name", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-1", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errCannotUpdateExistingQueue)
		assert.ErrorContains(t, err, "worker queue already exists: use worker queue update test-queue-1 instead")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		defer testUtil.MockUserInput(t, "y")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, false, mockClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockClient.AssertExpectations(t)
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
	expectedWorkerQueue := astro.WorkerQueue{
		Name:              "test-queue-1",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	deploymentRespWithQueues := []astro.Deployment{
		{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
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
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{
				Executor: "CeleryExecutor",
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
					Name:              "test-queue-1",
					IsDefault:         false,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-pool-id-1",
				},
			},
		},
	}
	listToUpdate := []astro.WorkerQueue{
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
			Name:              "test-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    125,
			MinWorkerCount:    5,
			WorkerConcurrency: 180,
			NodePoolID:        "test-pool-id-1",
		},
	}
	deploymentUpdateInput := astro.DeploymentUpdateInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: listToUpdate,
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
	t.Run("happy path update existing worker queue for a deployment", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
		mockClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		defer testUtil.MockUserInput(t, "2")()
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("prompts user for confirmation if --force was not provided", func(t *testing.T) {
		t.Run("updates the queue if user replies yes", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
			defer testUtil.MockUserInput(t, "y")()
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
		t.Run("cancels update if user does not confirm", func(t *testing.T) {
			expectedOutMessage := "Canceling worker queue update\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("returns an error if user makes incorrect choice when selecting a queue to update", func(t *testing.T) {
		expectedOutMessage := "invalid worker queue: 4 selected"
		// mock os.Stdin
		expectedInput := []byte("4") // there is no queue with this index
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.Contains(t, err.Error(), expectedOutMessage)
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, errGetDeployment).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 200, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when updating requested queue would create a new queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-2", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errCannotCreateNewQueue)
		assert.ErrorContains(t, err, "worker queue does not exist: use worker queue create test-queue-2 instead")
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		defer testUtil.MockUserInput(t, "y")()
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, true, mockClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockClient.AssertExpectations(t)
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	ws := "test-ws-id"
	deploymentRespWithQueues := []astro.Deployment{
		{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
			DeploymentSpec: astro.DeploymentSpec{
				Executor: "CeleryExecutor",
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
				EnvironmentVariablesObjects: nil,
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:                "test-wq-id",
					Name:              "default",
					IsDefault:         true,
					MaxWorkerCount:    130,
					MinWorkerCount:    12,
					WorkerConcurrency: 110,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "test-wq-id-1",
					Name:              "test-worker-queue-1",
					IsDefault:         false,
					MaxWorkerCount:    175,
					MinWorkerCount:    8,
					WorkerConcurrency: 150,
					NodePoolID:        "test-pool-id-1",
				},
			},
		},
	}
	listToDelete := []astro.WorkerQueue{
		{
			ID:                "test-wq-id",
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolID:        "test-pool-id",
		},
	}
	deploymentUpdateInput := astro.DeploymentUpdateInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: listToDelete,
	}
	expectedOutMessage := "worker queue test-worker-queue-1 for test-deployment-label in test-ws-id workspace deleted\n"
	t.Run("happy path worker queue gets deleted", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "2")()
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("prompts user for confirmation if --force was not provided", func(t *testing.T) {
		t.Run("deletes the queue if user replies yes", func(t *testing.T) {
			defer testUtil.MockUserInput(t, "y")()
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
		t.Run("cancels deletion if user does not confirm", func(t *testing.T) {
			expectedOutMessage = "Canceling worker queue deletion\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, errGetDeployment).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue", true, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an errors if queue selection fails", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "3")()
		// mock os.Stdin
		expectedInput := []byte("3") // selecting a queue not in list
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.NotContains(t, out.String(), expectedOutMessage)
	})
	t.Run("returns an error if user chooses to delete default queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "default", true, mockClient, out)
		assert.ErrorIs(t, err, errCannotDeleteDefaultQueue)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if trying to delete a queue that does not exist", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-non-existent-queue", true, mockClient, out)
		assert.ErrorIs(t, err, errQueueDoesNotExist)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment update fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
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
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
		_, err := GetWorkerQueueDefaultOptions(mockClient)
		assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(t)
	})
}

func TestSetWorkerQueueValues(t *testing.T) {
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
	mockWorkerQueue := &astro.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	t.Run("sets user provided min worker count for queue", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MinWorkerCount, actualQueue.MinWorkerCount)
	})
	t.Run("sets user provided max worker count for queue", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MaxWorkerCount, actualQueue.MaxWorkerCount)
	})
	t.Run("sets user provided worker concurrency for queue", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.WorkerConcurrency, actualQueue.WorkerConcurrency)
	})
	t.Run("sets default min worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(0, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.MinWorkerCount.Default, actualQueue.MinWorkerCount)
	})
	t.Run("sets default max worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(10, 0, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.MaxWorkerCount.Default, actualQueue.MaxWorkerCount)
	})
	t.Run("sets default worker concurrency for queue if user did not provide it", func(t *testing.T) {
		actualQueue := setWorkerQueueValues(10, 150, 0, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.WorkerConcurrency.Default, actualQueue.WorkerConcurrency)
	})
}

func TestIsWorkerQueueInputValid(t *testing.T) {
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
	requestedWorkerQueue := &astro.WorkerQueue{
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
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: min worker count must be between 1 and 20")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: worker concurrency must be between 175 and 275")
	})
}

func TestQueueExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	existingQueues := []astro.WorkerQueue{
		{
			ID:                "test-wq-id",
			Name:              "test-default-queue",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolID:        "test-nodepool-id",
		},
		{
			ID:                "test-wq-id-1",
			Name:              "test-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolID:        "test-nodepool-id-1",
		},
	}
	t.Run("returns true if queue with same name exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{Name: "test-default-queue"})
		assert.True(t, actual)
	})
	t.Run("returns true if queue with same id exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{ID: "test-wq-id-1"})
		assert.True(t, actual)
	})
	t.Run("returns false if queue with same name does not exist in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{Name: "test-default-queues"})
		assert.False(t, actual)
	})
	t.Run("returns true if queue with same id exists in list of queues", func(t *testing.T) {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{ID: "test-wq-id-10"})
		assert.False(t, actual)
	})
}

func TestSelectNodePool(t *testing.T) {
	var (
		workerType, nodePoolID string
		poolList               []astro.NodePool
		out                    *bytes.Buffer
	)

	out = new(bytes.Buffer)
	poolList = []astro.NodePool{
		{
			ID:               "test-default-pool",
			IsDefault:        true,
			NodeInstanceType: "test-instance",
		},
		{
			ID:               "test-non-default-pool",
			IsDefault:        false,
			NodeInstanceType: "test-instance-1",
		},
		{
			ID:               "test-non-default-pool-1",
			IsDefault:        false,
			NodeInstanceType: "test-instance-2",
		},
	}
	t.Run("prompts user to pick a node pool if worker type was not requested", func(t *testing.T) {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.NoError(t, err)
		assert.Equal(t, poolList[1].ID, nodePoolID)
	})
	t.Run("returns the pool that matches worker type that the user requested", func(t *testing.T) {
		var err error
		workerType = "test-instance-2"
		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.NoError(t, err)
		assert.Equal(t, poolList[2].ID, nodePoolID)
	})
	t.Run("returns an error when user chooses a value not in the list", func(t *testing.T) {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
	})
	t.Run("returns an error when user chooses a workerType that does not exist in any node pools", func(t *testing.T) {
		workerType = "non-existent"

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		assert.ErrorIs(t, err, errInvalidNodePool)
	})
}

func TestSelectQueue(t *testing.T) {
	var (
		out           *bytes.Buffer
		queueList     []astro.WorkerQueue
		queueToDelete string
	)
	out = new(bytes.Buffer)
	queueList = []astro.WorkerQueue{
		{
			ID:        "queue-1",
			Name:      "default",
			IsDefault: true,
		},
		{
			ID:        "queue-2",
			Name:      "my-queue-2",
			IsDefault: false,
		},
		{
			ID:        "queue-3",
			Name:      "my-queue-3",
			IsDefault: false,
		},
	}

	t.Run("user can select a queue to delete", func(t *testing.T) {
		// mock os.Stdin
		expectedInput := []byte("2") // selecting queue with id: queue-2
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		queueToDelete, err = selectQueue(queueList, out)
		assert.NoError(t, err)
		assert.Equal(t, "my-queue-2", queueToDelete)
	})
	t.Run("errors if user makes an invalid choice", func(t *testing.T) {
		// mock os.Stdin
		expectedInput := []byte("4") // selecting queue with id: queue-4
		r, w, err := os.Pipe()
		assert.NoError(t, err)
		_, err = w.Write(expectedInput)
		assert.NoError(t, err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		queueToDelete, err = selectQueue(queueList, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.Equal(t, "", queueToDelete)
	})
}

func TestUpdateQueueList(t *testing.T) {
	existingQs := []astro.WorkerQueue{
		{
			ID:                "q-1",
			Name:              "test-q",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
			NodePoolID:        "test-worker",
		},
		{
			ID:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
			NodePoolID:        "test-worker",
		},
	}
	t.Run("updates min, max, concurrency and node pool when queue exists", func(t *testing.T) {
		updatedQ := astro.WorkerQueue{
			ID:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
			NodePoolID:        "test-worker-1",
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQ)
		assert.Equal(t, updatedQ, updatedQueueList[1])
	})
	t.Run("does not update id or isDefault when queue exists", func(t *testing.T) {
		updatedQRequest := astro.WorkerQueue{
			ID:                "q-3",
			Name:              "test-q-1",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
			NodePoolID:        "test-worker-1",
		}
		updatedQ := astro.WorkerQueue{
			ID:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
			NodePoolID:        "test-worker-1",
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest)
		assert.Equal(t, updatedQ, updatedQueueList[1])
	})
	t.Run("does not change any queues if queue to update does not exist", func(t *testing.T) {
		updatedQRequest := astro.WorkerQueue{
			ID:                "q-4",
			Name:              "test-q-does-not-exist",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
			NodePoolID:        "test-worker-1",
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest)
		assert.Equal(t, existingQs, updatedQueueList)
	})
}
