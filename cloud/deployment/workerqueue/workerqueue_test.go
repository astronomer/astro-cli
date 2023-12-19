package workerqueue

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	errGetDeployment           = errors.New("test get deployment error")
	errUpdateDeployment        = errors.New("test deployment update error")
	errDeploymentConfigOptions = errors.New("test get deployment error")
)

func TestCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkerQueue := astro.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
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
				Executor: deployment.CeleryExecutor,
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
					Name:              "default",
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
	updateDeploymentInput := astro.UpdateDeploymentInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
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
	keDeployment := []astro.Deployment{
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
				Executor: deployment.KubeExecutor,
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-wq-id",
					Name:       "default",
					IsDefault:  true,
					PodRAM:     "lots",
					PodCPU:     "huge",
					NodePoolID: "test-pool-id",
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
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   0,
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
	t.Run("common across CE and KE executors", func(t *testing.T) {
		t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			defer testUtil.MockUserInput(t, "test-worker-queue")()
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Times(2)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "test-instance-type-1", -1, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
		})
		t.Run("returns an error when listing deployments fails", func(t *testing.T) {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)

			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errGetDeployment)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
			defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
			err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errInvalidNodePool)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when requested worker queue would update an existing queue with the same name", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-1", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errCannotUpdateExistingQueue)
			assert.ErrorContains(t, err, "worker queue already exists: use worker queue update test-queue-1 instead")
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when update deployment fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(t, "y")()
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errUpdateDeployment)
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("when executor is CE", func(t *testing.T) {
		t.Run("happy path creates a new worker queue for a deployment when no worker queues exist", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(t, "y")()
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespNoQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
		t.Run("happy path creates a new worker queue for a deployment when worker queues exist", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(t, "2")()
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", -1, 0, 0, false, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("when executor is KE", func(t *testing.T) {
		t.Run("prompts user for a name if one was not provided", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(t, "bigQ")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, ErrNotSupported)
			assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when requested input is not valid", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, ErrNotSupported)
			assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(t)
		})
		t.Run("returns a queue already exists error when request is to create a new queue", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "default", createAction, "test-instance-type-1", -1, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errCannotUpdateExistingQueue)
			mockClient.AssertExpectations(t)
		})
	})
}

func TestCreateHostedShared(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkerQueue := astro.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "",
	}
	expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
	out := new(bytes.Buffer)
	mockClient := new(astro_mocks.Client)
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   0,
			Ceiling: 30,
			Default: 0,
		},
		MaxWorkerCount: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 30,
			Default: 10,
		},
		WorkerConcurrency: astro.WorkerQueueOption{
			Floor:   1,
			Ceiling: 64,
			Default: 16,
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
						IsDefault:        true,
						NodeInstanceType: "test-instance-type",
						CreatedAt:        time.Now(),
					},
				},
			},
			Type:           "HOSTED_SHARED",
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
					Name:              "default",
					AstroMachine:      "a5",
					IsDefault:         true,
					MaxWorkerCount:    12,
					MinWorkerCount:    1,
					WorkerConcurrency: 5,
					NodePoolID:        "test-pool-id",
				},
				{
					ID:                "test-wq-id-1",
					Name:              "test-queue-1",
					IsDefault:         false,
					AstroMachine:      "a10",
					MaxWorkerCount:    25,
					MinWorkerCount:    8,
					WorkerConcurrency: 10,
					NodePoolID:        "test-pool-id",
				},
			},
		},
	}
	updateDeploymentInput := astro.UpdateDeploymentInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:                "test-wq-id",
				Name:              "default",
				IsDefault:         true,
				AstroMachine:      "a5",
				MaxWorkerCount:    12,
				MinWorkerCount:    1,
				WorkerConcurrency: 5,
				NodePoolID:        "test-pool-id",
			},
			{
				ID:                "test-wq-id-1",
				Name:              "test-queue-1",
				IsDefault:         false,
				AstroMachine:      "a10",
				MaxWorkerCount:    25,
				MinWorkerCount:    8,
				WorkerConcurrency: 10,
				NodePoolID:        "test-pool-id",
			},
			{
				Name:              "test-worker-queue",
				IsDefault:         false,
				AstroMachine:      "a5",
				MaxWorkerCount:    10,
				MinWorkerCount:    0,
				WorkerConcurrency: 5,
				NodePoolID:        "test-pool-id",
			},
		},
	}
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
		AstroMachines: []astro.Machine{
			{
				Type:               "a5",
				ConcurrentTasks:    5,
				ConcurrentTasksMax: 15,
			},
			{
				Type:               "a10",
				ConcurrentTasks:    10,
				ConcurrentTasksMax: 30,
			},
			{
				Type:               "a20",
				ConcurrentTasks:    20,
				ConcurrentTasksMax: 60,
			},
		},
		Components: astro.Components{
			Scheduler: astro.SchedulerConfig{
				AU: astro.AuConfig{
					Default: 5,
					Limit:   24,
				},
				Replicas: astro.ReplicasConfig{
					Default: 1,
					Minimum: 1,
					Limit:   4,
				},
			},
		},
	}, nil).Times(6)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	t.Run("for hosted shared deployments", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 0, 0, true, mockClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("select machine for hosted shared deployments", func(t *testing.T) {
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

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 0, 0, true, mockClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("failed to select astro machines for hosted shared deployments", func(t *testing.T) {
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

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 0, 0, true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidAstroMachine)
	})
	t.Run("failed to get deployment config options for hosted shared deployments", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "test-worker-queue")()
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{}, errDeploymentConfigOptions)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 0, 0, true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errDeploymentConfigOptions)
	})
}

func TestUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
				Executor: deployment.CeleryExecutor,
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
	keDeployment := []astro.Deployment{
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
				Executor: deployment.KubeExecutor,
				Scheduler: astro.Scheduler{
					AU:       5,
					Replicas: 3,
				},
			},
			WorkerQueues: []astro.WorkerQueue{
				{
					ID:         "test-wq-id",
					Name:       "default",
					IsDefault:  true,
					PodCPU:     "huge",
					PodRAM:     "lots",
					NodePoolID: "test-pool-id",
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
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolID:        "test-pool-id-1",
		},
	}
	updateDeploymentInput := astro.UpdateDeploymentInput{
		ID:    deploymentRespWithQueues[0].ID,
		Label: deploymentRespWithQueues[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespWithQueues[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespWithQueues[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: listToUpdate,
	}
	updateKEDeploymentInput := astro.UpdateDeploymentInput{
		ID:    keDeployment[0].ID,
		Label: keDeployment[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  keDeployment[0].DeploymentSpec.Executor,
			Scheduler: keDeployment[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: []astro.WorkerQueue{
			{
				ID:         "test-wq-id",
				Name:       "default",
				IsDefault:  true,
				NodePoolID: "test-pool-id",
			},
		},
	}
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   0,
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
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	t.Run("common across CE and KE executors", func(t *testing.T) {
		t.Run("prompts user for confirmation if --force was not provided", func(t *testing.T) {
			t.Run("updates the queue if user replies yes", func(t *testing.T) {
				expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
				defer testUtil.MockUserInput(t, "y")()
				out := new(bytes.Buffer)
				mockClient := new(astro_mocks.Client)
				mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
					Components: astro.Components{
						Scheduler: astro.SchedulerConfig{
							AU: astro.AuConfig{
								Default: 5,
								Limit:   24,
							},
							Replicas: astro.ReplicasConfig{
								Default: 1,
								Minimum: 1,
								Limit:   4,
							},
						},
					},
				}, nil).Once()
				mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
				mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
				mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
				err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 0, 0, false, mockClient, mockCoreClient, out)
				assert.NoError(t, err)
				assert.Equal(t, expectedOutMessage, out.String())
				mockClient.AssertExpectations(t)
			})
			t.Run("cancels update if user does not confirm", func(t *testing.T) {
				expectedOutMessage := "Canceling worker queue update\n"
				out := new(bytes.Buffer)
				mockClient := new(astro_mocks.Client)
				mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
				mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
				err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", 0, 0, 0, false, mockClient, mockCoreClient, out)
				assert.NoError(t, err)
				assert.Equal(t, expectedOutMessage, out.String())
				mockClient.AssertExpectations(t)
			})
		})
		t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
			defer testUtil.MockUserInput(t, "2")()
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", -1, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
		})
		t.Run("returns an error when selecting a deployment fails", func(t *testing.T) {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
			defer testUtil.MockUserInput(t, "test-invalid-deployment-id")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
			err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when selecting a node pool fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errInvalidNodePool)
			mockClient.AssertExpectations(t)
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
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errInvalidQueue)
			assert.Contains(t, err.Error(), expectedOutMessage)
		})
		t.Run("returns an error when listing deployments fails", func(t *testing.T) {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)

			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errGetDeployment)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when updating requested queue would create a new queue", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-2", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errCannotCreateNewQueue)
			assert.ErrorContains(t, err, "worker queue does not exist: use worker queue create test-queue-2 instead")
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when update deployment fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(t, "y")()
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errUpdateDeployment)
			mockClient.AssertExpectations(t)
		})
		t.Run("when no deployments exists in the workspace", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return([]astro.Deployment{}, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("when executor is CE", func(t *testing.T) {
		t.Run("happy path update existing worker queue for a deployment", func(t *testing.T) {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when getting worker queue default options fails", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errWorkerQueueDefaultOptions)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when requested worker queue input is not valid", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, true, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("when executor is KE", func(t *testing.T) {
		t.Run("happy path update existing worker queue for a deployment", func(t *testing.T) {
			expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Twice()
			mockClient.On("UpdateDeployment", &updateKEDeploymentInput).Return(keDeployment[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type", -1, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
			mockClient.AssertExpectations(t)
		})
		t.Run("update existing worker queue with a new worker type", func(t *testing.T) {
			expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			origNodePoolID := updateKEDeploymentInput.WorkerQueues[0].NodePoolID
			updateKEDeploymentInput.WorkerQueues[0].NodePoolID = "test-pool-id-1"
			defer func() { updateKEDeploymentInput.WorkerQueues[0].NodePoolID = origNodePoolID }()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Twice()
			mockClient.On("UpdateDeployment", &updateKEDeploymentInput).Return(keDeployment[0], nil).Once()
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", -1, 0, 0, true, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Contains(t, out.String(), expectedOutMessage)
			mockClient.AssertExpectations(t)
		})
		t.Run("returns an error when requested input is not valid", func(t *testing.T) {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", updateAction, "test-instance-type-1", -1, 0, 0, false, mockClient, mockCoreClient, out)
			assert.ErrorIs(t, err, ErrNotSupported)
			assert.ErrorContains(t, err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(t)
		})
	})
}

func TestDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	deploymentRespWithQueues := []astro.Deployment{
		{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
			DeploymentSpec: astro.DeploymentSpec{
				Executor: deployment.CeleryExecutor,
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
					PodRAM:            "lots",
					PodCPU:            "huge",
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
	updateDeploymentInput := astro.UpdateDeploymentInput{
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
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
			Components: astro.Components{
				Scheduler: astro.SchedulerConfig{
					AU: astro.AuConfig{
						Default: 5,
						Limit:   24,
					},
					Replicas: astro.ReplicasConfig{
						Default: 1,
						Minimum: 1,
						Limit:   4,
					},
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("prompts user for queue name if one was not provided", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "2")()
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
			Components: astro.Components{
				Scheduler: astro.SchedulerConfig{
					AU: astro.AuConfig{
						Default: 5,
						Limit:   24,
					},
					Replicas: astro.ReplicasConfig{
						Default: 1,
						Minimum: 1,
						Limit:   4,
					},
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, mockCoreClient, out)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), expectedOutMessage)
	})
	t.Run("prompts user for confirmation if --force was not provided", func(t *testing.T) {
		t.Run("deletes the queue if user replies yes", func(t *testing.T) {
			defer testUtil.MockUserInput(t, "y")()
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
				Components: astro.Components{
					Scheduler: astro.SchedulerConfig{
						AU: astro.AuConfig{
							Default: 5,
							Limit:   24,
						},
						Replicas: astro.ReplicasConfig{
							Default: 1,
							Minimum: 1,
							Limit:   4,
						},
					},
				},
			}, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
			mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
		t.Run("cancels deletion if user does not confirm", func(t *testing.T) {
			expectedOutMessage = "Canceling worker queue deletion\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, mockCoreClient, out)
			assert.NoError(t, err)
			assert.Equal(t, expectedOutMessage, out.String())
			mockClient.AssertExpectations(t)
		})
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue", true, mockClient, mockCoreClient, out)
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
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errInvalidQueue)
		assert.NotContains(t, out.String(), expectedOutMessage)
	})
	t.Run("returns an error if user chooses to delete default queue", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "default", true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errCannotDeleteDefaultQueue)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if trying to delete a queue that does not exist", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-non-existent-queue", true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errQueueDoesNotExist)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if deployment update fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{
			Components: astro.Components{
				Scheduler: astro.SchedulerConfig{
					AU: astro.AuConfig{
						Default: 5,
						Limit:   24,
					},
					Replicas: astro.ReplicasConfig{
						Default: 1,
						Minimum: 1,
						Limit:   4,
					},
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errUpdateDeployment).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, mockCoreClient, out)
		assert.ErrorIs(t, err, errUpdateDeployment)
		mockClient.AssertExpectations(t)
	})
	t.Run("when no deployments exists in the workspace", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return([]astro.Deployment{}, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, mockCoreClient, out)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
}

func TestGetWorkerQueueDefaultOptions(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		actualQueue := SetWorkerQueueValues(0, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MinWorkerCount, actualQueue.MinWorkerCount)
	})
	t.Run("sets user provided max worker count for queue", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.MaxWorkerCount, actualQueue.MaxWorkerCount)
	})
	t.Run("sets user provided worker concurrency for queue", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueue.WorkerConcurrency, actualQueue.WorkerConcurrency)
	})
	t.Run("sets default min worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(-1, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.MinWorkerCount.Default, actualQueue.MinWorkerCount)
	})
	t.Run("sets default max worker count for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 0, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.MaxWorkerCount.Default, actualQueue.MaxWorkerCount)
	})
	t.Run("sets default worker concurrency for queue if user did not provide it", func(t *testing.T) {
		actualQueue := SetWorkerQueueValues(10, 150, 0, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.Equal(t, mockWorkerQueueDefaultOptions.WorkerConcurrency.Default, actualQueue.WorkerConcurrency)
	})
}

func TestIsCeleryWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   0,
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
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.NoError(t, err)
	})
	t.Run("returns an error when min worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: worker concurrency must be between 175 and 275")
	})
	t.Run("returns an error when podCPU is present in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		requestedWorkerQueue.PodCPU = "lots"
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "CeleryExecutor does not support pod_cpu in the request. It can only be used with KubernetesExecutor")
	})
	t.Run("returns an error when podRAM is present in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		requestedWorkerQueue.PodCPU = ""
		requestedWorkerQueue.PodRAM = "huge"
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "CeleryExecutor does not support pod_ram in the request. It can only be used with KubernetesExecutor")
	})
}

func TestIsHostedCeleryWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
		MinWorkerCount: astro.WorkerQueueOption{
			Floor:   0,
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

	mockMachineOptions := &astro.Machine{
		Type:               "a5",
		ConcurrentTasks:    5,
		ConcurrentTasksMax: 15,
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
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 10
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.NoError(t, err)
	})
	t.Run("returns an error when min worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	t.Run("returns an error when max worker count is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	t.Run("returns an error when worker concurrency is not between default floor and ceiling values", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 20
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, errInvalidWorkerQueueOption)
		assert.Contains(t, err.Error(), "worker queue option is invalid: worker concurrency must be between 1 and 15")
	})
	t.Run("returns an error when podCPU is present in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 10
		requestedWorkerQueue.PodCPU = "lots"
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "CeleryExecutor does not support pod_cpu in the request. It can only be used with KubernetesExecutor")
	})
	t.Run("returns an error when podRAM is present in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 10
		requestedWorkerQueue.PodCPU = ""
		requestedWorkerQueue.PodRAM = "huge"
		err := IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "CeleryExecutor does not support pod_ram in the request. It can only be used with KubernetesExecutor")
	})
}

func TestIsKubernetesWorkerQueueInputValid(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	requestedWorkerQueue := &astro.WorkerQueue{
		Name:              "default",
		NodePoolID:        "test-pool-id",
		MinWorkerCount:    -1,
		MaxWorkerCount:    0,
		WorkerConcurrency: 0,
	}

	t.Run("returns nil when queue input is valid", func(t *testing.T) {
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.NoError(t, err)
	})
	t.Run("returns an error when queue name is not default", func(t *testing.T) {
		requestedWorkerQueue.Name = "test-queue"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
	})
	t.Run("returns an error when pod_cpu is in input", func(t *testing.T) {
		requestedWorkerQueue.PodCPU = "1.0"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support pod cpu in the request. It will be calculated based on the requested worker type")
	})
	t.Run("returns an error when pod_ram is in input", func(t *testing.T) {
		requestedWorkerQueue.PodRAM = "1.0"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support pod ram in the request. It will be calculated based on the requested worker type")
	})
	t.Run("returns an error when min_worker_count is in input", func(t *testing.T) {
		requestedWorkerQueue.MinWorkerCount = 8
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support minimum worker count in the request. It can only be used with CeleryExecutor")
	})
	t.Run("returns an error when max_worker_count is in input", func(t *testing.T) {
		requestedWorkerQueue.MaxWorkerCount = 25
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
	})
	t.Run("returns an error when worker_concurrency is in input", func(t *testing.T) {
		requestedWorkerQueue.WorkerConcurrency = 350
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:              "default",
				NodePoolID:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		assert.ErrorIs(t, err, ErrNotSupported)
		assert.Contains(t, err.Error(), "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
	})
}

func TestQueueExists(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
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
		err           error
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
		defer testUtil.MockUserInput(t, "2")()
		queueToDelete, err = selectQueue(queueList, out)
		assert.NoError(t, err)
		assert.Equal(t, "my-queue-2", queueToDelete)
	})
	t.Run("errors if user makes an invalid choice", func(t *testing.T) {
		defer testUtil.MockUserInput(t, "4")()
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
		updatedQueueList := updateQueueList(existingQs, &updatedQ, deployment.CeleryExecutor, 3, 16, 20)
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
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, deployment.CeleryExecutor, 3, 16, 20)
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
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, deployment.CeleryExecutor, 0, 0, 0)
		assert.Equal(t, existingQs, updatedQueueList)
	})
	t.Run("does not change any queues if user did not request min, max, concurrency", func(t *testing.T) {
		updatedQRequest := astro.WorkerQueue{
			ID:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    5,
			WorkerConcurrency: 18,
			NodePoolID:        "test-worker",
		}
		updatedQueueList := updateQueueList(existingQs, &updatedQRequest, deployment.CeleryExecutor, -1, 0, 0)
		assert.Equal(t, existingQs, updatedQueueList)
	})
	t.Run("zeroes out podRam and podCPU for KE queue updates", func(t *testing.T) {
		existingKEQ := []astro.WorkerQueue{
			{
				ID:                "q-1",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				WorkerConcurrency: 16,
				PodRAM:            "lots",
				PodCPU:            "huge",
				NodePoolID:        "test-worker-1",
			},
		}
		updatedQRequest := astro.WorkerQueue{
			ID:         "q-1",
			Name:       "default",
			IsDefault:  true,
			PodRAM:     "lots",
			PodCPU:     "huge",
			NodePoolID: "test-worker-1",
		}
		expectedQList := []astro.WorkerQueue{
			{
				ID:         "q-1",
				Name:       "default",
				IsDefault:  true,
				NodePoolID: "test-worker-1",
			},
		}

		updatedQueueList := updateQueueList(existingKEQ, &updatedQRequest, deployment.KubeExecutor, 0, 0, 0)
		assert.Equal(t, expectedQList, updatedQueueList)
	})
	t.Run("updates nodepoolID for KE queue updates", func(t *testing.T) {
		existingKEQ := []astro.WorkerQueue{
			{
				ID:                "q-1",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				WorkerConcurrency: 16,
				PodRAM:            "lots",
				PodCPU:            "huge",
				NodePoolID:        "test-worker-1",
			},
		}
		updatedQRequest := astro.WorkerQueue{
			ID:         "q-1",
			Name:       "default",
			IsDefault:  true,
			PodRAM:     "lots",
			PodCPU:     "huge",
			NodePoolID: "test-worker-2",
		}
		expectedQList := []astro.WorkerQueue{
			{
				ID:         "q-1",
				Name:       "default",
				IsDefault:  true,
				NodePoolID: "test-worker-2",
			},
		}

		updatedQueueList := updateQueueList(existingKEQ, &updatedQRequest, deployment.KubeExecutor, 0, 0, 0)
		assert.Equal(t, expectedQList, updatedQueueList)
	})
}

func TestGetQueueName(t *testing.T) {
	var (
		out                      *bytes.Buffer
		queueList                []astro.WorkerQueue
		existingDeployment       astro.Deployment
		err                      error
		expectedName, actualName string
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
	existingDeployment.WorkerQueues = queueList
	t.Run("prompts a user for a queue name when creating", func(t *testing.T) {
		expectedName = "queue-name"
		defer testUtil.MockUserInput(t, "queue-name")()
		actualName, err = getQueueName("", createAction, &existingDeployment, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedName, actualName)
	})
	t.Run("user selects a queue name when updating", func(t *testing.T) {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(t, "2")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedName, actualName)
	})
	t.Run("returns an error if user makes invalid queue choice when updating", func(t *testing.T) {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(t, "8")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		assert.Error(t, err)
	})
}

func TestSanitizeExistingQueues(t *testing.T) {
	var existingQs, actualQs []astro.WorkerQueue
	existingQs = []astro.WorkerQueue{
		{
			ID:                "q-1",
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
			PodRAM:            "lots",
			PodCPU:            "huge",
			NodePoolID:        "test-worker",
		},
		{
			ID:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
			PodRAM:            "lots+1",
			PodCPU:            "huge+1",
			NodePoolID:        "test-worker-1",
		},
	}
	t.Run("updates existing queues for CeleryExecutor", func(t *testing.T) {
		expectedQs := []astro.WorkerQueue{
			{
				ID:                "q-1",
				Name:              "default",
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
				NodePoolID:        "test-worker-1",
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.CeleryExecutor)
		assert.Equal(t, expectedQs, actualQs)
	})
	t.Run("updates existing queues for KubernetesExecutor", func(t *testing.T) {
		existingQs = []astro.WorkerQueue{existingQs[0]}
		expectedQs := []astro.WorkerQueue{
			{
				ID:         "q-1",
				Name:       "default",
				IsDefault:  true,
				NodePoolID: "test-worker",
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.KubeExecutor)
		assert.Equal(t, expectedQs, actualQs)
	})
}
