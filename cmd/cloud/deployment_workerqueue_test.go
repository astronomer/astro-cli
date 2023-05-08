package cloud

import (
	"bytes"
	"os"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func (s *Suite) TestNewDeploymentWorkerQueueRootCmd() {
	expectedHelp := "Manage worker queues for an Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)

	s.Run("worker-queue command runs", func() {
		testUtil.SetupOSArgsForGinkgo()
		wQueueCmd := newDeploymentWorkerQueueRootCmd(os.Stdout)
		wQueueCmd.SetOut(buf)
		_, err := wQueueCmd.ExecuteC()
		s.NoError(err)
		s.Contains(buf.String(), "worker-queue")
	})

	s.Run("-h prints worker-queue help", func() {
		cmdArgs := []string{"worker-queue", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
}

func (s *Suite) TestNewDeploymentWorkerQueueCreateCmd() {
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
	listToCreate := []astro.WorkerQueue{
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
		{
			Name:              "test-queue",
			IsDefault:         false,
			MaxWorkerCount:    125,
			MinWorkerCount:    5,
			WorkerConcurrency: 180,
			NodePoolID:        "test-pool-id",
		},
	}
	updateDeploymentInput := astro.UpdateDeploymentInput{
		ID:    deploymentRespDefaultQueue[0].ID,
		Label: deploymentRespDefaultQueue[0].Label,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  deploymentRespDefaultQueue[0].DeploymentSpec.Executor,
			Scheduler: deploymentRespDefaultQueue[0].DeploymentSpec.Scheduler,
		},
		WorkerQueues: listToCreate,
	}
	s.Run("-h prints worker-queue help", func() {
		cmdArgs := []string{"worker-queue", "create", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})

	s.Run("create worker queue when no deployment id was provided", func() {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-n", "test-queue", "-t", "test-instance-type"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Equal(expectedoutput, actualOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("create worker queue when deployment id was provided", func() {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-d", "test-deployment-id", "-t", "test-instance-type", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Equal(expectedoutput, actualOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("create worker queue when deployment name was provided", func() {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("1")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "--deployment-name", "test-deployment-label", "-t", "test-instance-type", "-n", "test-queue"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Equal(expectedoutput, actualOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("create worker queue when no name was provided", func() {
		expectedoutput := "worker queue test-queue for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace created\n"
		// mock os.Stdin
		expectedInput := []byte("test-queue")
		r, w, err := os.Pipe()
		s.NoError(err)
		_, err = w.Write(expectedInput)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespDefaultQueue, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespDefaultQueue[0], nil).Once()
		cmdArgs := []string{"worker-queue", "create", "-d", "test-deployment-id", "-t", "test-instance-type"}
		actualOut, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Equal(expectedoutput, actualOut)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when getting workspace fails", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "create"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
		s.NotContains(resp, expectedOut)
	})
}

func (s *Suite) TestNewDeploymentWorkerQueueDeleteCmd() {
	expectedHelp := "Delete a worker queue from an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)
	astroClient = mockClient

	s.Run("-h prints worker-queue help", func() {
		cmdArgs := []string{"worker-queue", "delete", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("happy path delete worker queue", func() {
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
						Name:              "test-worker-queue",
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
				Name:              "test-worker-queue",
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
		expectedOutMessage := "worker queue test-worker-queue-1 for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace deleted\n"
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Once()
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()

		cmdArgs := []string{"worker-queue", "delete", "-n", "test-worker-queue-1", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOutMessage)
	})
	s.Run("returns an error when getting workspace fails", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "delete"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
		s.NotContains(resp, expectedOut)
	})
}

func (s *Suite) TestNewDeploymentWorkerQueueUpdateCmd() {
	expectedHelp := "Update a worker queue for an Astro Deployment"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)
	astroClient = mockClient

	s.Run("-h prints worker-queue help", func() {
		cmdArgs := []string{"worker-queue", "update", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("happy path update worker queue", func() {
		deploymentRespWithQueues := []astro.Deployment{
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
				MaxWorkerCount:    175,
				MinWorkerCount:    0,
				WorkerConcurrency: 150,
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
			WorkerQueues: listToUpdate,
		}
		mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   0,
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

		expectedOutMessage := "worker queue test-queue-1 for test-deployment-label in ck05r3bor07h40d02y2hw4n4v workspace updated\n"
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
			RuntimeReleases: []astro.RuntimeRelease{
				{
					Version: "4.2.5",
				},
			},
		}, nil).Times(2)
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentRespWithQueues, nil).Twice()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", &updateDeploymentInput).Return(deploymentRespWithQueues[0], nil).Once()

		// updating min count
		cmdArgs := []string{"worker-queue", "update", "-n", "test-queue-1", "-t", "test-instance-type", "--min-count", "0", "-f"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedOutMessage)
	})
	s.Run("returns an error when getting workspace fails", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"worker-queue", "update"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
		s.NotContains(resp, expectedOut)
	})
}
