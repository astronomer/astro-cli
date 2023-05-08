package cloud

import (
	"time"

	"github.com/astronomer/astro-cli/astro-client"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/stretchr/testify/mock"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var deploymentResponse = []astro.Deployment{
	{
		ID:          "test-deployment-id",
		Label:       "test-deployment-label",
		ReleaseName: "great-release-name",
		Workspace:   astro.Workspace{ID: "test-ws-id"},
		Cluster: astro.Cluster{
			ID: "cluster-id",
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
		RuntimeRelease: astro.RuntimeRelease{Version: "6.0.0", AirflowVersion: "2.4.0"},
		DeploymentSpec: astro.DeploymentSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 3,
			},
			Webserver: astro.Webserver{URL: "some-url"},
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
		UpdatedAt: time.Now(),
		Status:    "HEALTHY",
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

func (s *Suite) TestNewDeploymentInspectCmd() {
	expectedHelp := "Inspect an Astro Deployment."
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockClient := new(astro_mocks.Client)
	astroClient = mockClient
	s.Run("-h prints help", func() {
		cmdArgs := []string{"inspect", "-h"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, expectedHelp)
	})
	s.Run("returns deployment in yaml format when a deployment name was provided", func() {
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		cmdArgs := []string{"inspect", "-n", "test-deployment-label"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, deploymentResponse[0].ReleaseName)
		s.Contains(resp, deploymentResponse[0].Label)
		s.Contains(resp, deploymentResponse[0].RuntimeRelease.Version)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns deployment in yaml format when a deployment id was provided", func() {
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		cmdArgs := []string{"inspect", "test-deployment-id"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, deploymentResponse[0].ReleaseName)
		s.Contains(resp, deploymentResponse[0].Label)
		s.Contains(resp, deploymentResponse[0].RuntimeRelease.Version)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns deployment template in yaml format when a deployment id was provided", func() {
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		cmdArgs := []string{"inspect", "test-deployment-id", "--template"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, deploymentResponse[0].RuntimeRelease.Version)
		s.NotContains(resp, deploymentResponse[0].ReleaseName)
		s.NotContains(resp, deploymentResponse[0].Label)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns a deployment's specific field", func() {
		mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(deploymentResponse, nil).Once()
		cmdArgs := []string{"inspect", "-n", "test-deployment-label", "-k", "metadata.cluster_id"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.NoError(err)
		s.Contains(resp, deploymentResponse[0].Cluster.ID)
		mockClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when getting workspace fails", func() {
		testUtil.InitTestConfig(testUtil.Initial)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"inspect", "-n", "doesnotexist"}
		resp, err := execDeploymentCmd(cmdArgs...)
		s.Error(err)
		s.NotContains(resp, expectedOut)
	})
}
