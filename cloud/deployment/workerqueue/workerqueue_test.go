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
	t.Run("happy path creates a new worker queue for an existing deployment when no worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker-queue " + expectedWorkerQueue.Name + " for test-deployment-id in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("happy path creates a new worker queue for an existing deployment when worker queues exist", func(t *testing.T) {
		expectedOutMessage := "worker-queue " + expectedWorkerQueue.Name + " for test-deployment-id in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespWithQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(deploymentRespNoQueues[0], nil).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, mockClient, out)
		assert.NoError(t, err)
		assert.Equal(t, expectedOutMessage, out.String())
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when listing deployments fails", func(t *testing.T) {
		mockClient := new(astro_mocks.Client)
		out := new(bytes.Buffer)

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, errGetDeployment).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, mockClient, out)
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

		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(nil, deployment.ErrInvalidDeploymentKey).Once()
		err = Create("test-ws-id", "", "", false, mockClient, out)
		assert.ErrorIs(t, err, deployment.ErrInvalidDeploymentKey)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error when update deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockClient := new(astro_mocks.Client)
		mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return(deploymentRespNoQueues, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{}, errGetDeployment).Once()
		err := Create("test-ws-id", "test-deployment-id", "", false, mockClient, out)
		assert.ErrorIs(t, err, errGetDeployment)
		mockClient.AssertExpectations(t)
	})
}
