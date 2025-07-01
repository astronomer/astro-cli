package workerqueue

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var (
	errGetDeployment           = errors.New("test get deployment error")
	errUpdateDeployment        = errors.New("test deployment update error")
	errDeploymentConfigOptions = errors.New("test get deployment error")
)

var (
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient         = new(astrocore_mocks.ClientWithResponsesInterface)
	testPoolID             = "test-pool-id"
	testPoolID1            = "test-pool-id-1"
	standardType           = astroplatformcore.DeploymentTypeSTANDARD
	dedicatedType          = astroplatformcore.DeploymentTypeDEDICATED
	hybridType             = astroplatformcore.DeploymentTypeHYBRID
	schedulerAU            = 0
	clusterID              = "cluster-id"
	executorCelery         = astroplatformcore.DeploymentExecutorCELERY
	executorKubernetes     = astroplatformcore.DeploymentExecutorKUBERNETES
	highAvailability       = true
	isDevelopmentMode      = false
	resourceQuotaCPU       = "1cpu"
	ResourceQuotaMemory    = "1"
	schedulerSize          = astroplatformcore.DeploymentSchedulerSizeSMALL
	deploymentResponse     = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                  "test-deployment-id",
			RuntimeVersion:      "3.0-4",
			Namespace:           "test-deployment-label",
			WorkspaceId:         "workspace-id",
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Name:                "test-deployment-label",
			Status:              "HEALTHY",
			Type:                &hybridType,
			SchedulerAu:         &schedulerAU,
			ClusterId:           &clusterID,
			Executor:            &executorCelery,
			IsHighAvailability:  &highAvailability,
			IsDevelopmentMode:   &isDevelopmentMode,
			ResourceQuotaCpu:    &resourceQuotaCPU,
			ResourceQuotaMemory: &ResourceQuotaMemory,
			SchedulerSize:       &schedulerSize,
			WorkerQueues:        &[]astroplatformcore.WorkerQueue{},
		},
	}
	nodePools = []astroplatformcore.NodePool{
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "test-instance-type",
		},
		{
			Id:               "test-pool-id-1",
			IsDefault:        false,
			NodeInstanceType: "test-instance-type-1",
		},
	}
	cluster = astroplatformcore.Cluster{
		Id:        "test-cluster-id",
		Name:      "test-cluster",
		NodePools: &nodePools,
	}
	mockGetClusterResponse = astroplatformcore.GetClusterResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &cluster,
	}
	mockListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	testRegion                 = "region"
	testProvider               = astroplatformcore.DeploymentCloudProviderAWS
	testCluster                = "cluster"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Id:             "test-deployment-id",
			Name:           "test-deployment-label",
			Status:         "HEALTHY",
			Type:           &standardType,
			Region:         &testRegion,
			CloudProvider:  &testProvider,
			RuntimeVersion: "3.0-4",
		},
		{
			Id:             "test-id-2",
			Name:           "test",
			Status:         "HEALTHY",
			Type:           &hybridType,
			ClusterName:    &testCluster,
			RuntimeVersion: "3.0-4",
		},
	}
	cloudProvider                = astroplatformcore.DeploymentCloudProviderAWS
	mockUpdateDeploymentResponse = astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{
		JSON200: &astrocore.DeploymentOptions{
			DefaultValues: astrocore.DefaultValueOptions{},
			ResourceQuotas: astrocore.ResourceQuotaOptions{
				ResourceQuota: astrocore.ResourceOption{
					Cpu: astrocore.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astrocore.ResourceRange{
						Ceiling: "2GI",
						Default: "1GI",
						Floor:   "0GI",
					},
				},
			},
			Executors: []string{},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsPlatformResponseOK = astroplatformcore.GetDeploymentOptionsResponse{
		JSON200: &astroplatformcore.DeploymentOptions{
			ResourceQuotas: astroplatformcore.ResourceQuotaOptions{},
			Executors:      []string{},
			WorkerQueues: astroplatformcore.WorkerQueueOptions{
				MaxWorkers: astroplatformcore.Range{
					Floor:   20,
					Ceiling: 200,
					Default: 125,
				},
				MinWorkers: astroplatformcore.Range{
					Floor:   0,
					Ceiling: 20,
					Default: 5,
				},
				WorkerConcurrency: astroplatformcore.Range{
					Floor:   175,
					Ceiling: 275,
					Default: 180,
				},
			},
			WorkerMachines: []astroplatformcore.WorkerMachine{
				{
					Name: "a5",
					Concurrency: astroplatformcore.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
				{
					Name: "a20",
					Concurrency: astroplatformcore.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	emptyGetDeploymentOptionsPlatformResponseOK = astroplatformcore.GetDeploymentOptionsResponse{
		JSON200: &astroplatformcore.DeploymentOptions{
			ResourceQuotas: astroplatformcore.ResourceQuotaOptions{},
			Executors:      []string{},
			WorkerQueues:   astroplatformcore.WorkerQueueOptions{},
			WorkerMachines: []astroplatformcore.WorkerMachine{},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	emptyListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{},
		},
	}
)

type Suite struct {
	suite.Suite
}

func TestWorkerQueue(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkerQueue := astroplatformcore.WorkerQueue{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	s.Run("prompts user for queue name if one was not provided", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		defer testUtil.MockUserInput(s.T(), "test-worker-queue")()
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when listing deployments fails", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errGetDeployment)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when selecting a deployment fails", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "test-invalid-deployment-id")()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, deployment.ErrInvalidDeploymentKey)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("exit early when no deployments in the workspace", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "test-invalid-deployment-id")()
		mockDeploymentListResponse := astroplatformcore.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.DeploymentsPaginated{
				Deployments: []astroplatformcore.Deployment{},
			},
		}
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeploymentListResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when selecting a node pool fails", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidNodePool)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when requested worker queue would update an existing queue with the same name", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "test-queue-1", NodePoolId: &testPoolID}}
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-1", createAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errCannotUpdateExistingQueue)
		s.ErrorContains(err, "worker queue already exists: use worker queue update test-queue-1 instead")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when update deployment fails", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "y")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errUpdateDeployment)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	// CE executor
	deploymentResponse.JSON200.Executor = &executorCelery
	s.Run("happy path creates a new worker queue for a deployment when no worker queues exist", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "y")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("happy path creates a new worker queue for a deployment when worker queues exist", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "2")()
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "test-queue-1", NodePoolId: &testPoolID}}
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", -1, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("happy path creates a new worker queue for a hosted deployment when worker queues exist", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace created\n"
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "2")()
		astroMachine := "a5"
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "test-queue-1", NodePoolId: &testPoolID, AstroMachine: &astroMachine}}
		deploymentResponse.JSON200.Type = &standardType
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-worker-queue", createAction, "", -1, 20, 10, false, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
		deploymentResponse.JSON200.Type = &hybridType
	})
	s.Run("returns an error when getting worker queue default options fails", func() {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errWorkerQueueDefaultOptions).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errWorkerQueueDefaultOptions)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when requested worker queue input is not valid", func() {
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	// kube executor
	deploymentResponse.JSON200.Executor = &executorKubernetes
	s.Run("prompts user for a name if one was not provided", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "bigQ")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)

		s.ErrorIs(err, ErrNotSupported)
		s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when requested input is not valid", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", createAction, "test-instance-type-1", 0, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, ErrNotSupported)
		s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns a queue already exists error when request is to create a new queue", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyGetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "default", createAction, "test-instance-type-1", -1, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errCannotUpdateExistingQueue)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCreateHostedShared() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedOutMessage := "worker queue test-worker-queue for test-deployment-label in test-ws-id workspace created\n"
	out := new(bytes.Buffer)
	// hosted deployments
	deploymentResponse.JSON200.Executor = &executorCelery
	deploymentResponse.JSON200.Type = &standardType
	deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
	s.Run("for hosted shared deployments", func() {
		deploymentResponse.JSON200.Type = &standardType
		defer testUtil.MockUserInput(s.T(), "test-worker-queue")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("select machine for hosted shared deployments", func() {
		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "1")()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("failed to select astro machines for hosted shared deployments", func() {
		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "4")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-worker-queue", createAction, "", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidAstroMachine)
		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("failed to get deployment config options for hosted shared deployments", func() {
		defer testUtil.MockUserInput(s.T(), "test-worker-queue")()
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errDeploymentConfigOptions).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", createAction, "a5", -1, 20, 5, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errDeploymentConfigOptions)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkerQueue := astroplatformcore.WorkerQueue{
		Name:              "test-queue-1",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	s.Run("updates the queue if user replies yes", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		deploymentResponse.JSON200.Type = &hybridType

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorContains(err, "use worker queue create test-queue-1 instead")

		deploymentResponse.JSON200.Type = &standardType

		err = CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "a5", -1, 20, 5, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorContains(err, "use worker queue create test-queue-1 instead")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("cancels update if user does not confirm", func() {
		expectedOutMessage := "Canceling worker queue update\n"
		deploymentResponse.JSON200.Type = &hybridType
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", 0, 20, 175, false, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("prompts user for queue name if one was not provided", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		defer testUtil.MockUserInput(s.T(), "1")()
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when selecting a deployment fails", func() {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(s.T(), "test-invalid-deployment-id")()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, deployment.ErrInvalidDeploymentKey).Times(1)
		err := CreateOrUpdate("test-ws-id", "", "", "", "", "", 0, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, deployment.ErrInvalidDeploymentKey)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when selecting a node pool fails", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "non-existent", 0, 200, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidNodePool)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if user makes incorrect choice when selecting a queue to update", func() {
		expectedOutMessage := "invalid worker queue: 4 selected"
		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "4")()
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "", updateAction, "test-instance-type-1", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidQueue)
		s.Contains(err.Error(), expectedOutMessage)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when listing deployments fails", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "", 0, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errGetDeployment)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when updating requested queue would create a new queue", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-queue-2", updateAction, "test-instance-type-1", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errCannotCreateNewQueue)
		s.ErrorContains(err, "worker queue does not exist: use worker queue create test-queue-2 instead")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when update deployment fails", func() {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}
		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 0, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errUpdateDeployment)
		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	deploymentResponse.JSON200.Executor = &executorCelery
	s.Run("happy path update existing worker queue for a deployment Celery", func() {
		expectedOutMessage := "worker queue " + expectedWorkerQueue.Name + " for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: expectedWorkerQueue.Name, NodePoolId: &testPoolID}}

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "test-queue-1", updateAction, "test-instance-type-1", -1, 20, 175, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when getting worker queue default options fails", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, errWorkerQueueDefaultOptions).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type-1", 0, 200, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errWorkerQueueDefaultOptions)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when requested worker queue input is not valid", func() {
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{}
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "test-instance-type", 25, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidWorkerQueueOption)

		deploymentResponse.JSON200.Type = &dedicatedType

		err = CreateOrUpdate("test-ws-id", "test-deployment-id", "", "", "", "a5", 25, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	// kube executor
	deploymentResponse.JSON200.Executor = &executorKubernetes
	s.Run("happy path update existing worker queue for a deployment", func() {
		expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyGetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}

		deploymentResponse.JSON200.Type = &hybridType
		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type", -1, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("update existing worker queue with a new worker type", func() {
		expectedOutMessage := "worker queue default for test-deployment-label in test-ws-id workspace updated\n"
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyGetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)
		deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{{Name: "default", NodePoolId: &testPoolID}}

		err := CreateOrUpdate("test-ws-id", "", "test-deployment-label", "default", updateAction, "test-instance-type-1", -1, 0, 0, true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error when requested input is not valid", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsPlatformResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := CreateOrUpdate("test-ws-id", "test-deployment-id", "", "test-KE-q", updateAction, "test-instance-type-1", -1, 0, 0, false, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, ErrNotSupported)
		s.ErrorContains(err, "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	testID := "test-wq-id"
	astroMachine := "A5"
	workerQueueList := []astroplatformcore.WorkerQueue{
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
	expectedOutMessage := "worker queue test-worker-queue-1 for test-deployment-label in test-ws-id workspace deleted\n"
	s.Run("happy path worker queue gets deleted", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		out := new(bytes.Buffer)
		// standard type
		deploymentResponse.JSON200.Type = &standardType
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())

		out = new(bytes.Buffer)
		// dedicated type
		deploymentResponse.JSON200.Type = &dedicatedType
		err = Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())

		out = new(bytes.Buffer)
		// hybrid type
		deploymentResponse.JSON200.Type = &hybridType
		err = Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Equal(expectedOutMessage, out.String())

		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("prompts user for queue name if one was not provided", func() {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "2")()

		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		s.Contains(out.String(), expectedOutMessage)

		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("prompts user for confirmation if --force was not provided", func() {
		s.Run("deletes the queue if user replies yes", func() {
			defer testUtil.MockUserInput(s.T(), "y")()
			out := new(bytes.Buffer)
			mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
			mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
			mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockPlatformCoreClient, mockCoreClient, out)
			s.NoError(err)
			mockCoreClient.AssertExpectations(s.T())
			mockPlatformCoreClient.AssertExpectations(s.T())
		})
		s.Run("cancels deletion if user does not confirm", func() {
			defer testUtil.MockUserInput(s.T(), "n")()
			expectedOutMessage = "Canceling worker queue deletion\n"
			out := new(bytes.Buffer)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

			err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", false, mockPlatformCoreClient, mockCoreClient, out)
			s.NoError(err)
			s.Equal(expectedOutMessage, out.String())
			mockCoreClient.AssertExpectations(s.T())
			mockPlatformCoreClient.AssertExpectations(s.T())
		})
	})
	s.Run("returns an error when listing deployments fails", func() {
		out := new(bytes.Buffer)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Times(1)
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue", true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errGetDeployment)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an errors if queue selection fails", func() {
		defer testUtil.MockUserInput(s.T(), "3")()

		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "", true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errInvalidQueue)
		s.NotContains(out.String(), expectedOutMessage)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if user chooses to delete default queue", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "default", true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errCannotDeleteDefaultQueue)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if trying to delete a queue that does not exist", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Delete("test-ws-id", "test-deployment-id", "", "test-non-existent-queue", true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errQueueDoesNotExist)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("returns an error if deployment update fails", func() {
		out := new(bytes.Buffer)
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errUpdateDeployment).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		s.ErrorIs(err, errUpdateDeployment)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("when no deployments exists in the workspace", func() {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)
		err := Delete("test-ws-id", "test-deployment-id", "", "test-worker-queue-1", true, mockPlatformCoreClient, mockCoreClient, out)
		s.NoError(err)
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSetWorkerQueueValues() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		MinWorkers: astroplatformcore.Range{
			Floor:   1,
			Ceiling: 20,
			Default: 5,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	mockWorkerQueue := astroplatformcore.WorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}
	mockMachineOptions := &astroplatformcore.WorkerMachine{
		Name: "a5",
		Concurrency: astroplatformcore.Range{
			Default: 180,
			Ceiling: 275,
			Floor:   175,
		},
	}
	s.Run("sets user provided min worker count for queue", func() {
		actualQueue := SetWorkerQueueValues(0, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(mockWorkerQueue.MinWorkerCount, actualQueue.MinWorkerCount)
	})
	s.Run("sets user provided max worker count for queue", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(mockWorkerQueue.MaxWorkerCount, actualQueue.MaxWorkerCount)
	})
	s.Run("sets user provided worker concurrency for queue", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(mockWorkerQueue.WorkerConcurrency, actualQueue.WorkerConcurrency)
	})
	s.Run("sets default min worker count for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(-1, 150, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(int(mockWorkerQueueDefaultOptions.MinWorkers.Default), actualQueue.MinWorkerCount)
	})
	s.Run("sets default max worker count for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(10, 0, 225, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(int(mockWorkerQueueDefaultOptions.MaxWorkers.Default), actualQueue.MaxWorkerCount)
	})
	s.Run("sets default worker concurrency for queue if user did not provide it", func() {
		actualQueue := SetWorkerQueueValues(10, 150, 0, mockWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.Equal(int(mockWorkerQueueDefaultOptions.WorkerConcurrency.Default), actualQueue.WorkerConcurrency)
	})
}

func (s *Suite) TestIsCeleryWorkerQueueInputValid() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		MinWorkers: astroplatformcore.Range{
			Floor:   0,
			Ceiling: 20,
			Default: 5,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}
	requestedWorkerQueue := astroplatformcore.HybridWorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolId:        "",
	}

	s.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func() {
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 275
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.NoError(err)
	})
	s.Run("returns an error when min worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	s.Run("returns an error when max worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	s.Run("returns an error when worker concurrency is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 350
		err := IsWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: worker concurrency must be between 175 and 275")
	})
}

func (s *Suite) TestIsHostedCeleryWorkerQueueInputValid() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockWorkerQueueDefaultOptions := astroplatformcore.WorkerQueueOptions{
		MinWorkers: astroplatformcore.Range{
			Floor:   0,
			Ceiling: 20,
			Default: 5,
		},
		MaxWorkers: astroplatformcore.Range{
			Floor:   20,
			Ceiling: 200,
			Default: 125,
		},
		WorkerConcurrency: astroplatformcore.Range{
			Floor:   175,
			Ceiling: 275,
			Default: 180,
		},
	}

	mockMachineOptions := &astroplatformcore.WorkerMachine{
		Name: "a5",
		Concurrency: astroplatformcore.Range{
			Default: 5,
			Ceiling: 15,
		},
	}
	requestedWorkerQueue := astroplatformcore.WorkerQueueRequest{
		Name:              "test-worker-queue",
		IsDefault:         false,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
	}

	s.Run("happy path when min or max worker count and worker concurrency are within default floor and ceiling", func() {
		requestedWorkerQueue.MinWorkerCount = 0
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 10
		err := IsHostedWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.NoError(err)
	})
	s.Run("returns an error when min worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 35
		err := IsHostedWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: min worker count must be between 0 and 20")
	})
	s.Run("returns an error when max worker count is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 19
		err := IsHostedWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: max worker count must be between 20 and 200")
	})
	s.Run("returns an error when worker concurrency is not between default floor and ceiling values", func() {
		requestedWorkerQueue.MinWorkerCount = 8
		requestedWorkerQueue.MaxWorkerCount = 25
		requestedWorkerQueue.WorkerConcurrency = 20
		err := IsHostedWorkerQueueInputValid(requestedWorkerQueue, mockWorkerQueueDefaultOptions, mockMachineOptions)
		s.ErrorIs(err, errInvalidWorkerQueueOption)
		s.Contains(err.Error(), "worker queue option is invalid: worker concurrency must be between 1 and 15")
	})
}

func (s *Suite) TestIsKubernetesWorkerQueueInputValid() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	requestedWorkerQueue := astroplatformcore.HybridWorkerQueueRequest{
		Name:              "default",
		MinWorkerCount:    -1,
		MaxWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolId:        "test-pool-id",
	}

	s.Run("returns nil when queue input is valid", func() {
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.NoError(err)
	})
	s.Run("returns an error when queue name is not default", func() {
		requestedWorkerQueue.Name = "test-queue"
		defer func() {
			requestedWorkerQueue = astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support a non default worker queue in the request. Rename the queue to default")
	})
	s.Run("returns an error when max_worker_count is in input", func() {
		requestedWorkerQueue.MaxWorkerCount = 25
		defer func() {
			requestedWorkerQueue = astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support maximum worker count in the request. It can only be used with CeleryExecutor")
	})
	s.Run("returns an error when worker_concurrency is in input", func() {
		requestedWorkerQueue.WorkerConcurrency = 350
		defer func() {
			requestedWorkerQueue = astroplatformcore.HybridWorkerQueueRequest{
				Name:              "default",
				NodePoolId:        "test-pool-id",
				MinWorkerCount:    -1,
				MaxWorkerCount:    0,
				WorkerConcurrency: 0,
			}
		}()
		err := IsKubernetesWorkerQueueInputValid(requestedWorkerQueue)
		s.ErrorIs(err, ErrNotSupported)
		s.Contains(err.Error(), "KubernetesExecutor does not support worker concurrency in the request. It can only be used with CeleryExecutor")
	})
}

func (s *Suite) TestQueueExists() {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	testID := "test-nodepool-id"
	testID1 := "test-nodepool-id-1"
	testWQID := "test-wq-id-1"
	testWQID10 := "test-wq-id-10"
	existingQueues := []astroplatformcore.WorkerQueue{
		{
			Id:                "test-wq-id",
			Name:              "test-default-queue",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolId:        &testID,
		},
		{
			Id:                testWQID,
			Name:              "test-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolId:        &testID1,
		},
	}
	s.Run("returns true if queue with same name exists in list of queues", func() {
		actual := QueueExists(existingQueues, astroplatformcore.WorkerQueueRequest{Name: "test-default-queue"}, astroplatformcore.HybridWorkerQueueRequest{Name: "test-default-queue"})
		s.True(actual)
	})
	s.Run("returns true if queue with same id exists in list of queues", func() {
		actual := QueueExists(existingQueues, astroplatformcore.WorkerQueueRequest{Id: &testWQID}, astroplatformcore.HybridWorkerQueueRequest{Id: &testWQID})
		fmt.Println(actual)
		s.True(actual)
	})
	s.Run("returns false if queue with same name does not exist in list of queues", func() {
		actual := QueueExists(existingQueues, astroplatformcore.WorkerQueueRequest{Name: "test-default-queues"}, astroplatformcore.HybridWorkerQueueRequest{Name: "test-default-queues"})
		s.False(actual)
	})
	s.Run("returns false if queue with same id exists in list of queues", func() {
		actual := QueueExists(existingQueues, astroplatformcore.WorkerQueueRequest{Id: &testWQID10}, astroplatformcore.HybridWorkerQueueRequest{Id: &testWQID10})
		s.False(actual)
	})
}

func (s *Suite) TestSelectNodePool() {
	var (
		workerType, nodePoolID string
		poolList               []astroplatformcore.NodePool
		out                    *bytes.Buffer
	)

	out = new(bytes.Buffer)
	poolList = []astroplatformcore.NodePool{
		{
			Id:               "test-default-pool",
			IsDefault:        true,
			NodeInstanceType: "test-instance",
		},
		{
			Id:               "test-non-default-pool",
			IsDefault:        false,
			NodeInstanceType: "test-instance-1",
		},
		{
			Id:               "test-non-default-pool-1",
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
		s.Equal(poolList[1].Id, nodePoolID)
	})
	s.Run("returns the pool that matches worker type that the user requested", func() {
		var err error
		workerType = "test-instance-2"
		nodePoolID, err = selectNodePool(workerType, poolList, out)
		s.NoError(err)
		s.Equal(poolList[2].Id, nodePoolID)
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
		queueList     []astroplatformcore.WorkerQueue
		queueToDelete string
		err           error
	)
	out = new(bytes.Buffer)
	queueList = []astroplatformcore.WorkerQueue{
		{
			Id:        "queue-1",
			Name:      "default",
			IsDefault: true,
		},
		{
			Id:        "queue-2",
			Name:      "my-queue-2",
			IsDefault: false,
		},
		{
			Id:        "queue-3",
			Name:      "my-queue-3",
			IsDefault: false,
		},
	}

	s.Run("user can select a queue to delete", func() {
		defer testUtil.MockUserInput(s.T(), "2")()
		queueToDelete, err = selectQueue(&queueList, out)
		s.NoError(err)
		s.Equal("my-queue-2", queueToDelete)
	})
	s.Run("errors if user makes an invalid choice", func() {
		defer testUtil.MockUserInput(s.T(), "4")()
		queueToDelete, err = selectQueue(&queueList, out)
		s.ErrorIs(err, errInvalidQueue)
		s.Equal("", queueToDelete)
	})
	s.Run("errors if there are no worker queues", func() {
		queueToDelete, err = selectQueue(nil, out)
		s.ErrorIs(err, errNoWorkerQueues)
		s.Equal("", queueToDelete)
	})
}

func (s *Suite) TestUpdateQueueList() {
	id1 := "q-1"
	id2 := "q-2"
	id3 := "q-3"
	id4 := "q-4"
	deploymentCelery := astroplatformcore.DeploymentExecutorCELERY
	existingQs := []astroplatformcore.WorkerQueueRequest{
		{
			Id:                &id1,
			Name:              "test-q",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
		},
		{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
		},
	}
	s.Run("updates min, max, concurrency and node pool when queue exists", func() {
		updatedQ := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, updatedQ, &deploymentCelery, 3, 16, 20)
		s.Equal(updatedQ, updatedQueueList[1])
	})
	s.Run("does not update id or isDefault when queue exists", func() {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id3,
			Name:              "test-q-1",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQ := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, updatedQRequest, &deploymentCelery, 3, 16, 20)
		s.Equal(updatedQ, updatedQueueList[1])
	})
	s.Run("does not change any queues if queue to update does not exist", func() {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id4,
			Name:              "test-q-does-not-exist",
			IsDefault:         true,
			MaxWorkerCount:    16,
			MinWorkerCount:    3,
			WorkerConcurrency: 20,
		}
		updatedQueueList := updateQueueList(existingQs, updatedQRequest, &deploymentCelery, 0, 0, 0)
		s.Equal(existingQs, updatedQueueList)
	})
	s.Run("does not change any queues if user did not request min, max, concurrency", func() {
		updatedQRequest := astroplatformcore.WorkerQueueRequest{
			Id:                &id2,
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    5,
			WorkerConcurrency: 18,
		}
		updatedQueueList := updateQueueList(existingQs, updatedQRequest, &deploymentCelery, -1, 0, 0)
		s.Equal(existingQs, updatedQueueList)
	})
}

func (s *Suite) TestGetQueueName() {
	var (
		out                      *bytes.Buffer
		queueList                []astroplatformcore.WorkerQueue
		existingDeployment       astroplatformcore.Deployment
		err                      error
		expectedName, actualName string
	)
	out = new(bytes.Buffer)
	queueList = []astroplatformcore.WorkerQueue{
		{
			Id:        "queue-1",
			Name:      "default",
			IsDefault: true,
		},
		{
			Id:        "queue-2",
			Name:      "my-queue-2",
			IsDefault: false,
		},
		{
			Id:        "queue-3",
			Name:      "my-queue-3",
			IsDefault: false,
		},
	}
	existingDeployment.WorkerQueues = &queueList
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
	testWorker := "test-worker"
	testWorker1 := "test-worker-1"
	var existingQs, actualQs []astroplatformcore.WorkerQueue
	existingQs = []astroplatformcore.WorkerQueue{
		{
			Id:                "q-1",
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 16,
			NodePoolId:        &testWorker,
		},
		{
			Id:                "q-2",
			Name:              "test-q-1",
			IsDefault:         false,
			MaxWorkerCount:    15,
			MinWorkerCount:    2,
			WorkerConcurrency: 18,
			NodePoolId:        &testWorker1,
		},
	}
	s.Run("updates existing queues for CeleryExecutor", func() {
		testWorker1 := "test-worker-1"
		expectedQs := []astroplatformcore.WorkerQueue{
			{
				Id:                "q-1",
				Name:              "default",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				WorkerConcurrency: 16,
				NodePoolId:        &testWorker,
			},
			{
				Id:                "q-2",
				Name:              "test-q-1",
				IsDefault:         false,
				MaxWorkerCount:    15,
				MinWorkerCount:    2,
				WorkerConcurrency: 18,
				NodePoolId:        &testWorker1,
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.CeleryExecutor)
		s.Equal(expectedQs, actualQs)
	})
	s.Run("updates existing queues for KubernetesExecutor", func() {
		existingQs = []astroplatformcore.WorkerQueue{existingQs[0]}
		expectedQs := []astroplatformcore.WorkerQueue{
			{
				Id:                "q-1",
				Name:              "default",
				IsDefault:         true,
				NodePoolId:        &testWorker,
				WorkerConcurrency: 16,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
			},
		}
		actualQs = sanitizeExistingQueues(existingQs, deployment.KubeExecutor)
		s.Equal(expectedQs, actualQs)
	})
}
