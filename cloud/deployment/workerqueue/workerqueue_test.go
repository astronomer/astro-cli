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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var (
	errGetDeployment    = errors.New("test get deployment error")
	errUpdateDeployment = errors.New("test deployment update error")
)

type Suite struct {
	suite.Suite
}

func TestCloudDeploymentWorkerQueueSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
	s.Run("common across CE and KE executors", func() {
		s.Run("prompts user for queue name if one was not provided", func() {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			defer testUtil.MockUserInput(s.T(), "test-worker-queue")()
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
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "test-instance-type-1", -1, 0, 0, true, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
		})
		s.Run("returns an error when listing deployments fails", func() {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)

			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, errGetDeployment)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when selecting a deployment fails", func() {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
			defer testUtil.MockUserInput(s.T(), "test-invalid-deployment-id")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
			err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, deployment.ErrInvalidDeploymentKey)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when selecting a node pool fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, false, mockClient, out)
			s.ErrorIs(err, errInvalidNodePool)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when requested worker queue would update an existing queue with the same name", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-1", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, errCannotUpdateExistingQueue)
			s.ErrorContains(err, "worker queue already exists: use worker queue update test-queue-1 instead")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when update deployment fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(s.T(), "y")()
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
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, errUpdateDeployment)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when executor is CE", func() {
		s.Run("happy path creates a new worker queue for a deployment when no worker queues exist", func() {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(s.T(), "y")()
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
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.NoError(err)
			s.Equal(expectedOutMessage, out.String())
			mockClient.AssertExpectations(s.T())
		})
		s.Run("happy path creates a new worker queue for a deployment when worker queues exist", func() {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(s.T(), "2")()
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
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", -1, 0, 0, false, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when getting worker queue default options fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, false, mockClient, out)
			s.ErrorIs(err, errWorkerQueueDefaultOptions)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when requested worker queue input is not valid", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, false, mockClient, out)
			s.ErrorIs(err, errInvalidWorkerQueueOption)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when executor is KE", func() {
		s.Run("prompts user for a name if one was not provided", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(s.T(), "bigQ")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, ErrNotSupported)
			s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when requested input is not valid", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, ErrNotSupported)
			s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns a queue already exists error when request is to create a new queue", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "default", createAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, errCannotUpdateExistingQueue)
			mockClient.AssertExpectations(s.T())
		})
	})
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
	s.Run("common across CE and KE executors", func() {
		s.Run("prompts user for confirmation if --force was not provided", func() {
			s.Run("updates the queue if user replies yes", func() {
				expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
				defer testUtil.MockUserInput(s.T(), "y")()
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
				err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 0, 0, false, mockClient, out)
				s.NoError(err)
				s.Equal(expectedOutMessage, out.String())
				mockClient.AssertExpectations(s.T())
			})
			s.Run("cancels update if user does not confirm", func() {
				expectedOutMessage := "Canceling worker queue update\n"
				out := new(bytes.Buffer)
				mockClient := new(astro_mocks.Client)
				mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
				mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
				err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
				s.NoError(err)
				s.Equal(expectedOutMessage, out.String())
				mockClient.AssertExpectations(s.T())
			})
		})
		s.Run("prompts user for queue name if one was not provided", func() {
			expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
			defer testUtil.MockUserInput(s.T(), "2")()
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
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", -1, 0, 0, true, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
		})
		s.Run("returns an error when selecting a deployment fails", func() {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)
			defer testUtil.MockUserInput(s.T(), "test-invalid-deployment-id")()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
			err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, true, mockClient, out)
			s.ErrorIs(err, deployment.ErrInvalidDeploymentKey)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when selecting a node pool fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, true, mockClient, out)
			s.ErrorIs(err, errInvalidNodePool)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error if user makes incorrect choice when selecting a queue to update", func() {
			expectedOutMessage := "invalid worker queue: 4 selected"
			// mock os.Stdin
			expectedInput := []byte("4") // there is no queue with this index
			r, w, err := os.Pipe()
			s.NoError(err)
			_, err = w.Write(expectedInput)
			s.NoError(err)
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
			err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
			s.ErrorIs(err, errInvalidQueue)
			s.Contains(err.Error(), expectedOutMessage)
		})
		s.Run("returns an error when listing deployments fails", func() {
			mockClient := new(astro_mocks.Client)
			out := new(bytes.Buffer)

			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, true, mockClient, out)
			s.ErrorIs(err, errGetDeployment)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when updating requested queue would create a new queue", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-2", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
			s.ErrorIs(err, errCannotCreateNewQueue)
			s.ErrorContains(err, "worker queue does not exist: use worker queue create test-queue-2 instead")
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when update deployment fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			defer testUtil.MockUserInput(s.T(), "y")()
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
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 0, 0, true, mockClient, out)
			s.ErrorIs(err, errUpdateDeployment)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when executor is CE", func() {
		s.Run("happy path update existing worker queue for a deployment", func() {
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
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 0, 0, true, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when getting worker queue default options fails", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, true, mockClient, out)
			s.ErrorIs(err, errWorkerQueueDefaultOptions)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when requested worker queue input is not valid", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, true, mockClient, out)
			s.ErrorIs(err, errInvalidWorkerQueueOption)
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("when executor is KE", func() {
		s.Run("happy path update existing worker queue for a deployment", func() {
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
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type", 0, 0, 0, true, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("update existing worker queue with a new worker type", func() {
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
			err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", 0, 0, 0, true, mockClient, out)
			s.NoError(err)
			s.Contains(out.String(), expectedOutMessage)
			mockClient.AssertExpectations(s.T())
		})
		s.Run("returns an error when requested input is not valid", func() {
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(keDeployment, nil).Once()
			err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", updateAction, "test-instance-type-1", 0, 0, 0, false, mockClient, out)
			s.ErrorIs(err, ErrNotSupported)
			s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
			mockClient.AssertExpectations(s.T())
		})
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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
	s.Run("happy path worker queue gets deleted", func() {
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
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
		mockClient.AssertExpectations(s.T())
	})
	s.Run("prompts user for queue name if one was not provided", func() {
		defer testUtil.MockUserInput(s.T(), "2")()
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
		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
	})
	s.Run("prompts user for confirmation if --force was not provided", func() {
		s.Run("deletes the queue if user replies yes", func() {
			defer testUtil.MockUserInput(s.T(), "y")()
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
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, out)
			s.NoError(err)
			s.Equal(expectedOutMessage, out.String())
			mockClient.AssertExpectations(s.T())
		})
		s.Run("cancels deletion if user does not confirm", func() {
			expectedOutMessage = "Canceling worker queue deletion\n"
			out := new(bytes.Buffer)
			mockClient := new(astro_mocks.Client)
			mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockClient, out)
			s.NoError(err)
			s.Equal(expectedOutMessage, out.String())
			mockClient.AssertExpectations(s.T())
		})
	})
	s.Run("returns an error when listing deployments fails", func() {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(nil, errGetDeployment).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue", true, mockClient, out)
		s.ErrorIs(err, errGetDeployment)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an errors if queue selection fails", func() {
		defer testUtil.MockUserInput(s.T(), "3")()
		// mock os.Stdin
		expectedInput := []byte("3") // selecting a queue not in list
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()
		err = Delete("test-ws-id", "test-deployment-id", "", "", true, mockClient, out)
		s.ErrorIs(err, errInvalidQueue)
		s.NotContains(out.String(), expectedOutMessage)
	})
	s.Run("returns an error if user chooses to delete default queue", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "default", true, mockClient, out)
		s.ErrorIs(err, errCannotDeleteDefaultQueue)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if trying to delete a queue that does not exist", func() {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Once()
		err := Delete("test-ws-id", "test-deployment-id", "", "test-non-existent-queue", true, mockClient, out)
		s.ErrorIs(err, errQueueDoesNotExist)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if deployment update fails", func() {
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
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockClient, out)
		s.ErrorIs(err, errUpdateDeployment)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetWorkerQueueDefaultOptions() {
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
	s.Run("happy path returns worker-queue default options", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		actual, err := GetWorkerQueueDefaultOptions(mockClient)
		s.NoError(err)
		s.Equal(mockWorkerQueueDefaultOptions, actual)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when getting worker queue default options fails", func() {
		mockClient := new(astro_mocks.Client)
		mockClient.On("GetWorkerQueueOptions").Return(astro.WorkerQueueDefaultOptions{}, errWorkerQueueDefaultOptions).Once()
		_, err := GetWorkerQueueDefaultOptions(mockClient)
		s.ErrorIs(err, errWorkerQueueDefaultOptions)
		mockClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSetWorkerQueueValues() {
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
	s.Run("sets user provided min worker count for queue", func() {
		actualQueue := SetWorkerQueueValues(0, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueue.MinWorkerCount, actualQueue.MinWorkerCount)
	})
	s.Run("sets user provided max worker count for queue", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueue.MaxWorkerCount, actualQueue.MaxWorkerCount)
	})
	s.Run("sets user provided worker concurrency for queue", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueue.WorkerConcurrency, actualQueue.WorkerConcurrency)
	})
	s.Run("sets default min worker count for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(-1, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueueDefaultOptions.MinWorkerCount.Default, actualQueue.MinWorkerCount)
	})
	s.Run("sets default max worker count for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(10, 0, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueueDefaultOptions.MaxWorkerCount.Default, actualQueue.MaxWorkerCount)
	})
	s.Run("sets default worker concurrency for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 0, mockWorkerQueue, mockWorkerQueueDefaultOptions)
		s.Equal(mockWorkerQueueDefaultOptions.WorkerConcurrency.Default, actualQueue.WorkerConcurrency)
	})
}

func (s *Suite) TestIsCeleryWorkerQueueInputValid() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
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

	s.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func() {
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.NoError(err)
	})
	s.Run("returns an error when min worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	s.Run("returns an error when max worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	s.Run("returns an error when worker concurrency is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: worker concurrency must be between 175 and 275")
	})
	s.Run("returns an error when podCPU is present in input", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		requestedWorkerQueue.PodCPU = "lots"
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "CeleryExecutor does not support pod_cpu in the request. It can only be used with KubernetesExecutor")
	})
	s.Run("returns an error when podRAM is present in input", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		requestedWorkerQueue.PodCPU = ""
		requestedWorkerQueue.PodRAM = "huge"
		err := IsCeleryWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "CeleryExecutor does not support pod_ram in the request. It can only be used with KubernetesExecutor")
	})
}

func (s *Suite) TestIsKubernetesWorkerQueueInputValid() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	requestedWorkerQueue := &astro.WorkerQueue{
		Name:       "default",
		NodePoolID: "test-pool-id",
	}

	s.Run("returns nil when queue input is valid", func() {
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.NoError(err)
	})
	s.Run("returns an error when queue name is not default", func() {
		requestedWorkerQueue.Name = "test-queue"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
	})
	s.Run("returns an error when pod_cpu is in input", func() {
		requestedWorkerQueue.PodCPU = "1.0"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support pod cpu in the request. It will be calculated based on the requested worker type")
	})
	s.Run("returns an error when pod_ram is in input", func() {
		requestedWorkerQueue.PodRAM = "1.0"
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support pod ram in the request. It will be calculated based on the requested worker type")
	})
	s.Run("returns an error when min_worker_count is in input", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support minimum worker count in the request. It can only be used with CeleryExecutor")
	})
	s.Run("returns an error when max_worker_count is in input", func() {
		requestedWorkerQueue.MaxWorkerCount = 25
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
	})
	s.Run("returns an error when worker_concurrency is in input", func() {
		requestedWorkerQueue.WorkerConcurrency = 350
		defer func() {
			requestedWorkerQueue = &astro.WorkerQueue{
				Name:       "default",
				NodePoolID: "test-pool-id",
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
	})
}

func (s *Suite) TestQueueExists() {
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
	s.Run("returns true if queue with same name exists in list of queues", func() {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{Name: "test-default-queue"})
		s.True(actual)
	})
	s.Run("returns true if queue with same id exists in list of queues", func() {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{ID: "test-wq-id-1"})
		s.True(actual)
	})
	s.Run("returns false if queue with same name does not exist in list of queues", func() {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{Name: "test-default-queues"})
		s.False(actual)
	})
	s.Run("returns true if queue with same id exists in list of queues", func() {
		actual := QueueExists(existingQueues, &astro.WorkerQueue{ID: "test-wq-id-10"})
		s.False(actual)
	})
}

func (s *Suite) TestSelectNodePool() {
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
	s.Run("prompts user to pick a node pool if worker type was not requested", func() {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("2")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		s.NoError(err)
		s.Equal(poolList[1].ID, nodePoolID)
	})
	s.Run("returns the pool that matches worker type that the user requested", func() {
		var err error
		workerType = "test-instance-2"
		nodePoolID, err = selectNodePool(workerType, poolList, out)
		s.NoError(err)
		s.Equal(poolList[2].ID, nodePoolID)
	})
	s.Run("returns an error when user chooses a value not in the list", func() {
		workerType = ""

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		s.ErrorIs(err, errInvalidNodePool)
	})
	s.Run("returns an error when user chooses a workerType that does not exist in any node pools", func() {
		workerType = "non-existent"

		// mock os.Stdin
		expectedInput := []byte("4")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		nodePoolID, err = selectNodePool(workerType, poolList, out)
		s.ErrorIs(err, errInvalidNodePool)
	})
}

func (s *Suite) TestSelectQueue() {
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

	s.Run("user can select a queue to delete", func() {
		defer testUtil.MockUserInput(s.T(), "2")()
		queueToDelete, err = selectQueue(queueList, out)
		s.NoError(err)
		s.Equal("my-queue-2", queueToDelete)
	})
	s.Run("errors if user makes an invalid choice", func() {
		defer testUtil.MockUserInput(s.T(), "4")()
		queueToDelete, err = selectQueue(queueList, out)
		s.ErrorIs(err, errInvalidQueue)
		s.Equal("", queueToDelete)
	})
}

func (s *Suite) TestUpdateQueueList() {
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
	s.Run("updates min, max, concurrency and node pool when queue exists", func() {
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
		s.Equal(updatedQ, updatedQueueList[1])
	})
	s.Run("does not update id or isDefault when queue exists", func() {
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
		s.Equal(updatedQ, updatedQueueList[1])
	})
	s.Run("does not change any queues if queue to update does not exist", func() {
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
		s.Equal(existingQs, updatedQueueList)
	})
	s.Run("does not change any queues if user did not request min, max, concurrency", func() {
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
		s.Equal(existingQs, updatedQueueList)
	})
	s.Run("zeroes out podRam and podCPU for KE queue updates", func() {
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
		s.Equal(expectedQList, updatedQueueList)
	})
	s.Run("updates nodepoolID for KE queue updates", func() {
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
		s.Equal(expectedQList, updatedQueueList)
	})
}

func (s *Suite) TestGetQueueName() {
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
	s.Run("prompts a user for a queue name when creating", func() {
		expectedName = "queue-name"
		defer testUtil.MockUserInput(s.T(), "queue-name")()
		actualName, err = getQueueName("", createAction, &existingDeployment, out)
		s.NoError(err)
		s.Equal(expectedName, actualName)
	})
	s.Run("user selects a queue name when updating", func() {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(s.T(), "2")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		s.NoError(err)
		s.Equal(expectedName, actualName)
	})
	s.Run("returns an error if user makes invalid queue choice when updating", func() {
		expectedName = "my-queue-2"
		defer testUtil.MockUserInput(s.T(), "8")()
		actualName, err = getQueueName("", updateAction, &existingDeployment, out)
		s.Error(err)
	})
}

func (s *Suite) TestSanitizeExistingQueues() {
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
	s.Run("updates existing queues for CeleryExecutor", func() {
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
		s.Equal(expectedQs, actualQs)
	})
	s.Run("updates existing queues for KubernetesExecutor", func() {
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
		s.Equal(expectedQs, actualQs)
	})
}
