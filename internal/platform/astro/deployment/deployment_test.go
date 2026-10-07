package deployment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
)

type Suite struct {
	suite.Suite
}

func TestDeployment(t *testing.T) {
	suite.Run(t, new(Suite))
}

var (
	hybridQueueList                = []astrov1.HybridWorkerQueueRequest{}
	workerQueueRequest             = []astrov1.WorkerQueueRequest{}
	newEnvironmentVariables        = []astrov1.DeploymentEnvironmentVariableRequest{}
	errMock                        = errors.New("mock error")
	errCreateFailed                = errors.New("failed to create deployment")
	nodePools                      = []astrov1.NodePool{}
	mockListClustersResponse       = astrov1.ListClustersResponse{}
	cluster                        = astrov1.Cluster{}
	mockGetClusterResponse         = astrov1.GetClusterResponse{}
	standardType                   = astrov1.DeploymentTypeSTANDARD
	dedicatedType                  = astrov1.DeploymentTypeDEDICATED
	hybridType                     = astrov1.DeploymentTypeHYBRID
	testRegion                     = "region"
	testProvider                   = astrov1.DeploymentCloudProviderGCP
	testCluster                    = "cluster"
	testWorkloadIdentity           = "test-workload-identity"
	mockCoreDeploymentResponse     = []astrov1.Deployment{}
	mockListDeploymentsResponse    = astrov1.ListDeploymentsResponse{}
	emptyListDeploymentsResponse   = astrov1.ListDeploymentsResponse{}
	schedulerAU                    = 5
	clusterID                      = "cluster-id"
	executorCelery                 = astrov1.DeploymentExecutorCELERY
	executorKubernetes             = astrov1.DeploymentExecutorKUBERNETES
	highAvailability               = true
	isDevelopmentMode              = true
	resourceQuotaCPU               = "1cpu"
	ResourceQuotaMemory            = "1"
	schedulerSize                  = astrov1.DeploymentSchedulerSizeSMALL
	deploymentResponse             = astrov1.GetDeploymentResponse{}
	deploymentResponse2            = astrov1.GetDeploymentResponse{}
	GetDeploymentOptionsResponseOK = astrov1.GetDeploymentOptionsResponse{}
	workspaceTestDescription       = "test workspace"
	workspace1                     = astrov1.Workspace{}
	workspaces                     = []astrov1.Workspace{}
	ListWorkspacesResponseOK       = astrov1.ListWorkspacesResponse{}
)

func MockResponseInit() {
	nodePools = []astrov1.NodePool{
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "test-worker-1",
		},
		{
			Id:               "test-pool-id-2",
			IsDefault:        false,
			NodeInstanceType: "test-worker-2",
		},
	}
	mockListClustersResponse = astrov1.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ClustersPaginated{
			Clusters: []astrov1.Cluster{
				{
					Id:        "test-cluster-id",
					Name:      "test-cluster",
					NodePools: &nodePools,
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
		},
	}
	cluster = astrov1.Cluster{
		Id:        "test-cluster-id",
		Name:      "test-cluster",
		NodePools: &nodePools,
	}
	mockGetClusterResponse = astrov1.GetClusterResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &cluster,
	}
	mockCoreDeploymentResponse = []astrov1.Deployment{
		{
			Id:                "test-id-1",
			Name:              "test-1",
			Status:            "HEALTHY",
			Type:              &standardType,
			Region:            &testRegion,
			CloudProvider:     &testProvider,
			WorkspaceName:     &workspace1.Name,
			IsDevelopmentMode: &isDevelopmentMode,
		},
		{
			Id:            "test-id-2",
			Name:          "test",
			Status:        "HEALTHY",
			Type:          &hybridType,
			ClusterName:   &testCluster,
			WorkspaceName: &workspace1.Name,
		},
	}
	mockListDeploymentsResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	emptyListDeploymentsResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: []astrov1.Deployment{},
		},
	}
	deploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                  "test-id-1",
			RuntimeVersion:      "4.2.5",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Name:                "test",
			Status:              "HEALTHY",
			Type:                &hybridType,
			SchedulerAu:         &schedulerAU,
			SchedulerReplicas:   1,
			ClusterId:           &clusterID,
			Executor:            &executorCelery,
			IsHighAvailability:  &highAvailability,
			IsDevelopmentMode:   &isDevelopmentMode,
			ResourceQuotaCpu:    &resourceQuotaCPU,
			ResourceQuotaMemory: &ResourceQuotaMemory,
			SchedulerSize:       &schedulerSize,
			WorkerQueues:        &[]astrov1.WorkerQueue{},
		},
	}
	deploymentResponse2 = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                        "test-id-2",
			RuntimeVersion:            "4.2.5",
			Namespace:                 "test-name",
			WorkspaceId:               ws,
			WebServerUrl:              "test-url",
			IsDagDeployEnabled:        false,
			Name:                      "test",
			Status:                    "HEALTHY",
			Type:                      &hybridType,
			SchedulerAu:               &schedulerAU,
			SchedulerReplicas:         1,
			ClusterId:                 &clusterID,
			Executor:                  &executorCelery,
			IsHighAvailability:        &highAvailability,
			ResourceQuotaCpu:          &resourceQuotaCPU,
			ResourceQuotaMemory:       &ResourceQuotaMemory,
			SchedulerSize:             &schedulerSize,
			WorkerQueues:              &[]astrov1.WorkerQueue{},
			EffectiveWorkloadIdentity: &testWorkloadIdentity,
		},
	}
	GetDeploymentOptionsResponseOK = astrov1.GetDeploymentOptionsResponse{
		JSON200: &astrov1.DeploymentOptions{
			ResourceQuotas: astrov1.ResourceQuotaOptions{
				ResourceQuota: astrov1.ResourceOption{
					Cpu: astrov1.ResourceRange{
						Ceiling: "2CPU",
						Default: "1CPU",
						Floor:   "0CPU",
					},
					Memory: astrov1.ResourceRange{
						Ceiling: "2GI",
						Default: "1GI",
						Floor:   "0GI",
					},
				},
			},
			Executors: []string{},
			WorkerMachines: []astrov1.WorkerMachine{
				{
					Concurrency: astrov1.Range{
						Ceiling: 1,
						Default: 1,
						Floor:   1,
					},
					Name: "test-machine",
					Spec: astrov1.MachineSpec{},
				},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	workspaceTestDescription = "test workspace"
	workspace1 = astrov1.Workspace{
		Name:           "test-workspace",
		Description:    &workspaceTestDescription,
		Id:             "workspace-id",
		OrganizationId: "test-org-id",
	}
	workspaces = []astrov1.Workspace{
		workspace1,
	}
	ListWorkspacesResponseOK = astrov1.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.WorkspacesPaginated{
			Limit:      1,
			Offset:     0,
			TotalCount: 1,
			Workspaces: workspaces,
		},
	}
}

const (
	org       = "test-org-id"
	ws        = "workspace-id"
	dagDeploy = "disable"
	region    = "us-central1"
	mockOrgID = "test-org-id"
)

var mockV1Client *astrov1_mocks.ClientWithResponsesInterface

func (s *Suite) SetupTest() {
	// init mocks
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)

	// init responses object
	MockResponseInit()
}

func (s *Suite) TearDownSubTest() {
	// assert expectations
	mockV1Client.AssertExpectations(s.T())

	// reset mocks
	mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)

	// reset responses object
	MockResponseInit()
}

func (s *Suite) TestConvertCreateQueuesToUpdatePodEphemeralStorage() {
	ephemeral := "10Gi"
	in := []astrov1.WorkerQueueRequest{
		{
			Name:                "default",
			IsDefault:           true,
			MaxWorkerCount:      10,
			MinWorkerCount:      1,
			WorkerConcurrency:   20,
			AstroMachine:        astrov1.WorkerQueueRequestAstroMachine("A5"),
			PodEphemeralStorage: &ephemeral,
		},
		{
			Name:              "test-q-2",
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			WorkerConcurrency: 20,
			AstroMachine:      astrov1.WorkerQueueRequestAstroMachine("A5"),
		},
	}
	out := convertCreateQueuesToUpdate(in)
	s.Len(out, 2)
	if s.NotNil(out[0].PodEphemeralStorage) {
		s.Equal("10Gi", *out[0].PodEphemeralStorage)
	}
	s.Nil(out[1].PodEphemeralStorage)
}

func (s *Suite) TestListDeploymentsPaginates() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	page1 := astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: []astrov1.Deployment{{Id: "deployment-page1"}},
			TotalCount:  2,
			Limit:       1000,
			Offset:      0,
		},
	}
	page2 := astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: []astrov1.Deployment{{Id: "deployment-page2"}},
			TotalCount:  2,
			Limit:       1000,
			Offset:      1000,
		},
	}
	mockClient := new(astrov1_mocks.ClientWithResponsesInterface)
	// First call returns page 1 (offset 0), second call returns page 2 (offset 1000).
	mockClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&page1, nil).Once()
	mockClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&page2, nil).Once()

	got, err := ListDeployments("", "test-org", mockClient)
	s.NoError(err)
	s.Len(got, 2)
	s.Equal("deployment-page1", got[0].Id)
	s.Equal("deployment-page2", got[1].Id)
	mockClient.AssertExpectations(s.T())
}

// TestListDeploymentsPaginatesOverHTTP drives a real astrov1 client against
// a stub API server, exercising the actual HTTP request building (offset/limit
// query params), response parsing, and the pagination loop end-to-end.
func (s *Suite) TestListDeploymentsPaginatesOverHTTP() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	var seenOffsets, seenLimits []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		seenOffsets = append(seenOffsets, q.Get("offset"))
		seenLimits = append(seenLimits, q.Get("limit"))
		w.Header().Set("Content-Type", "application/json")
		if q.Get("offset") == "0" {
			_, _ = io.WriteString(w, `{"deployments":[{"id":"dep-1"}],"limit":1000,"offset":0,"totalCount":2}`)
			return
		}
		_, _ = io.WriteString(w, `{"deployments":[{"id":"dep-2"}],"limit":1000,"offset":1,"totalCount":2}`)
	}))
	defer ts.Close()

	client, err := astrov1.NewClientWithResponses(ts.URL)
	s.Require().NoError(err)

	got, err := ListDeployments("", "test-org", client)
	s.NoError(err)
	s.Len(got, 2)
	s.Equal("dep-1", got[0].Id)
	s.Equal("dep-2", got[1].Id)
	// The real client serialized limit and an advancing offset into the request query
	// across two calls, proving pagination works over the wire (not just via mocks).
	s.Equal([]string{"0", "1"}, seenOffsets)
	s.Equal([]string{"1000", "1000"}, seenLimits)
}

func (s *Suite) TestListData() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("returns structured deployment data", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		data, err := listData(ws, false, mockV1Client)
		s.NoError(err)
		s.Len(data.Deployments, 2)
		s.Contains([]string{data.Deployments[0].DeploymentID, data.Deployments[1].DeploymentID}, "test-id-1")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns error on failure", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		_, err := listData(ws, false, mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("empty result", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		data, err := listData(ws, false, mockV1Client)
		s.NoError(err)
		s.Empty(data.Deployments)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestListWithFormat() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("json output", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListWithFormat(ws, false, mockV1Client, testUtil.Renderer{JSON: true, Out: buf})
		s.NoError(err)

		var result DeploymentList
		s.NoError(json.Unmarshal(buf.Bytes(), &result))
		s.Len(result.Deployments, 2)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("text output", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := ListWithFormat(ws, false, mockV1Client, testUtil.Renderer{Out: buf})
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeployment() {
	deploymentID := "test-id-wrong"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("invalid deployment ID", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", false, nil, mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)

		mockV1Client.AssertExpectations(s.T())
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id-1"
	s.Run("error after invalid deployment Name", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, false, nil, mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("no deployments in workspace", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, true, nil, mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)

		mockV1Client.AssertExpectations(s.T())
	})
	s.Run("correct deployment ID", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", false, nil, mockV1Client)

		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockV1Client.AssertExpectations(s.T())
	})
	deploymentName = "test"
	s.Run("correct deployment Name", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, false, nil, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Name)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("two deployments with the same name", func() {
		mockCoreDeploymentResponse = []astrov1.Deployment{
			{
				Id:            "test-id-1",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &standardType,
				Region:        &testRegion,
				CloudProvider: &testProvider,
				WorkspaceName: &workspace1.Name,
			},
			{
				Id:            "test-id-2",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &hybridType,
				ClusterName:   &testCluster,
				WorkspaceName: &workspace1.Name,
			},
		}

		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", deploymentName, false, nil, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Name)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("deployment name and deployment id", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("create deployment error", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astrov1.DeploymentType, schedulerAU, schedulerReplicas int, remoteExecutionEnabled bool, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogURLPattern *string,
			astroV1Client astrov1.APIClient, waitForStatus bool, waitTimeForDeployment time.Duration,
		) error {
			return errMock
		}

		// Mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", false, nil, mockV1Client)
		s.ErrorIs(err, errMock)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("get deployments after creation error", func() {
		// Assuming org, ws, and errMock are predefined
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astrov1.DeploymentType, schedulerAU, schedulerReplicas int, remoteExecutionEnabled bool, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogURLPattern *string,
			astroV1Client astrov1.APIClient, waitForStatus bool, waitTimeForDeployment time.Duration,
		) error {
			return nil
		}

		// Second ListDeployments call with an error to simulate an error after deployment creation
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Once()

		// Mock os.Stdin
		input := []byte("y")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = GetDeployment(ws, "", "", false, nil, mockV1Client)
		s.ErrorIs(err, errMock)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("test automatic deployment creation", func() {
		// Assuming org, ws, and deploymentID are predefined
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astrov1.DeploymentType, schedulerAU, schedulerReplicas int, remoteExecutionEnabled bool, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogURLPattern *string,
			astroV1Client astrov1.APIClient, waitForStatus bool, waitTimeForDeployment time.Duration,
		) error {
			return nil
		}

		mockCoreDeploymentResponse = []astrov1.Deployment{
			{
				Id:            "test-id-1",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &standardType,
				Region:        &testRegion,
				CloudProvider: &testProvider,
			},
		}

		mockListOneDeploymentsResponse := astrov1.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.DeploymentsPaginated{
				Deployments: mockCoreDeploymentResponse,
			},
		}

		// Second ListDeployments call after successful deployment creation
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListOneDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		deployment, err := GetDeployment(ws, "", "", false, nil, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentByID() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	s.Run("success", func() {
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeploymentByID(org, deploymentID, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success from context", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization_short_name", org)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeploymentByID("", deploymentID, mockV1Client)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("context error", func() {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)

		_, err := GetDeploymentByID("", deploymentID, mockV1Client)
		s.ErrorContains(err, "no context set, have you authenticated to Astro or APC? Run astro login and try again")
	})

	s.Run("error in api response", func() {
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, errMock).Once()

		_, err := GetDeploymentByID(org, deploymentID, mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsDeploymentDedicated() {
	s.Run("if deployment type is dedicated", func() {
		out := IsDeploymentDedicated(astrov1.DeploymentTypeDEDICATED)
		s.Equal(out, true)
	})

	s.Run("if deployment type is hybrid", func() {
		out := IsDeploymentDedicated(astrov1.DeploymentTypeHYBRID)
		s.Equal(out, false)
	})
}

func (s *Suite) TestSelectRegion() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("list regions failure", func() {
		provider := astrov1.GetClusterOptionsParamsProvider(astrov1.GetClusterOptionsParamsProviderGCP) //nolint:unconvert // explicit conversion kept for test readability
		getSharedClusterOptionsParams := &astrov1.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrov1.GetClusterOptionsParamsType(astrov1.GetClusterOptionsParamsTypeDEDICATED), //nolint:unconvert // explicit conversion kept for test readability
		}

		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, getSharedClusterOptionsParams).Return(nil, errMock).Once()

		_, err := selectRegion("gcp", "", mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("region via selection", func() {
		provider := astrov1.GetClusterOptionsParamsProvider(astrov1.GetClusterOptionsParamsProviderGCP) //nolint:unconvert // explicit conversion kept for test readability
		getSharedClusterOptionsParams := &astrov1.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrov1.GetClusterOptionsParamsType(astrov1.GetClusterOptionsParamsTypeDEDICATED), //nolint:unconvert // explicit conversion kept for test readability
		}

		mockOKRegionResponse := &astrov1.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrov1.ClusterOptions{
				{Regions: []astrov1.ProviderRegion{{Name: region}}},
			},
		}

		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectRegion("gcp", "", mockV1Client)
		s.NoError(err)
		s.Equal(region, resp)
	})

	s.Run("region via selection aws", func() {
		provider := astrov1.GetClusterOptionsParamsProvider(astrov1.GetClusterOptionsParamsProviderAWS) //nolint:unconvert // explicit conversion kept for test readability
		getSharedClusterOptionsParams := &astrov1.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrov1.GetClusterOptionsParamsType(astrov1.GetClusterOptionsParamsTypeDEDICATED), //nolint:unconvert // explicit conversion kept for test readability
		}

		mockOKRegionResponse := &astrov1.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrov1.ClusterOptions{
				{Regions: []astrov1.ProviderRegion{{Name: region}}},
			},
		}

		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectRegion("aws", "", mockV1Client)
		s.NoError(err)
		s.Equal(region, resp)
	})

	s.Run("region invalid selection", func() {
		provider := astrov1.GetClusterOptionsParamsProvider(astrov1.GetClusterOptionsParamsProviderGCP) //nolint:unconvert // explicit conversion kept for test readability
		getSharedClusterOptionsParams := &astrov1.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrov1.GetClusterOptionsParamsType(astrov1.GetClusterOptionsParamsTypeDEDICATED), //nolint:unconvert // explicit conversion kept for test readability
		}

		mockOKRegionResponse := &astrov1.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrov1.ClusterOptions{
				{Regions: []astrov1.ProviderRegion{{Name: region}}},
			},
		}

		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		// mock os.Stdin
		input := []byte("4")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectRegion("gcp", "", mockV1Client)
		s.ErrorIs(err, errInvalidRegionKey)
	})

	s.Run("not able to find region", func() {
		provider := astrov1.GetClusterOptionsParamsProvider(astrov1.GetClusterOptionsParamsProviderGCP) //nolint:unconvert // explicit conversion kept for test readability
		getSharedClusterOptionsParams := &astrov1.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrov1.GetClusterOptionsParamsType(astrov1.GetClusterOptionsParamsTypeDEDICATED), //nolint:unconvert // explicit conversion kept for test readability
		}

		mockOKRegionResponse := &astrov1.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrov1.ClusterOptions{
				{Regions: []astrov1.ProviderRegion{{Name: region}}},
			},
		}

		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		_, err := selectRegion("gcp", "test-invalid-region", mockV1Client)
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Region")
	})
}

func (s *Suite) TestLogs() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	logCount := 2
	mockGetDeploymentLogsResponse := astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   2,
			Results: []astrov1.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockGetDeploymentLogsMultipleComponentsResponse := astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         10,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   5,
			Results: []astrov1.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrov1.DeploymentLogEntrySourceWebserver,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceTriggerer,
				},
				{
					Raw:       "test log line 3",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 4",
					Timestamp: 2,
					Source:    astrov1.DeploymentLogEntrySourceWorker,
				},
				{
					Raw:       "test log line 5",
					Timestamp: 1,
					Source:    astrov1.DeploymentLogEntrySourceApiserver,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("success", func() {
		// Mock ListDeployments
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

		res, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, false, false, logCount, mockV1Client)
		s.NoError(err)
		s.Equal([]LogEntry{
			{Source: "scheduler", Message: "test log line"},
			{Source: "scheduler", Message: "test log line 2"},
		}, res.Entries)
		s.Equal(24, res.Hours)

		mockV1Client.AssertExpectations(s.T())
	})
	s.Run("success without deployment", func() {
		// Mock ListDeployments
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse using the reusable variable
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

		// Mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = Logs("", ws, "", "keyword", true, true, true, true, false, nil, false, false, false, 1, mockV1Client)
		s.NoError(err)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		// Mock ListDeployments
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse to return an error
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, errMock).Once()

		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, false, false, false, logCount, mockV1Client)
		s.ErrorIs(err, errMock)

		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("query for more than one log level error", func() {
		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, true, true, logCount, mockV1Client)
		s.Error(err)
		s.Equal(err.Error(), "cannot query for more than one log level and/or keyword at a time")
	})
	s.Run("multiple components", func() {
		// Mock ListDeployments
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsMultipleComponentsResponse, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, false, false, logCount, mockV1Client)
		s.NoError(err)

		mockV1Client.AssertExpectations(s.T())
	})
	s.Run("pagination fetches multiple pages", func() {
		page1Response := astrov1.GetDeploymentLogsResponse{
			JSON200: &astrov1.DeploymentLog{
				Limit:         2,
				MaxNumResults: 10,
				Offset:        0,
				ResultCount:   2,
				Results: []astrov1.DeploymentLogEntry{
					{Raw: "log line 1", Timestamp: 1, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log line 2", Timestamp: 2, Source: astrov1.DeploymentLogEntrySourceScheduler},
				},
				SearchId: "search-id-1",
			},
			HTTPResponse: &http.Response{StatusCode: 200},
		}
		page2Response := astrov1.GetDeploymentLogsResponse{
			JSON200: &astrov1.DeploymentLog{
				Limit:         2,
				MaxNumResults: 10,
				Offset:        2,
				ResultCount:   1,
				Results: []astrov1.DeploymentLogEntry{
					{Raw: "log line 3", Timestamp: 3, Source: astrov1.DeploymentLogEntrySourceScheduler},
				},
				SearchId: "search-id-1",
			},
			HTTPResponse: &http.Response{StatusCode: 200},
		}
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&page1Response, nil).Once()
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&page2Response, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, false, false, 10, mockV1Client)
		s.NoError(err)

		mockV1Client.AssertExpectations(s.T())
	})
	s.Run("subsequent pages shrink to remaining count", func() {
		// logCount=3, page 1 returns 2 (API capping at Limit=2) → page 2 should be
		// requested with Limit=1, not the original Limit=3, so the API doesn't
		// ship results we'd discard.
		page1Response := astrov1.GetDeploymentLogsResponse{
			JSON200: &astrov1.DeploymentLog{
				Limit: 2, MaxNumResults: 3, Offset: 0, ResultCount: 2,
				Results: []astrov1.DeploymentLogEntry{
					{Raw: "log 1", Timestamp: 1, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log 2", Timestamp: 2, Source: astrov1.DeploymentLogEntrySourceScheduler},
				},
				SearchId: "search-id-shrink",
			},
			HTTPResponse: &http.Response{StatusCode: 200},
		}
		page2Response := astrov1.GetDeploymentLogsResponse{
			JSON200: &astrov1.DeploymentLog{
				Limit: 1, MaxNumResults: 3, Offset: 2, ResultCount: 1,
				Results: []astrov1.DeploymentLogEntry{
					{Raw: "log 3", Timestamp: 3, Source: astrov1.DeploymentLogEntrySourceScheduler},
				},
				SearchId: "search-id-shrink",
			},
			HTTPResponse: &http.Response{StatusCode: 200},
		}
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// First call: Limit pointer points to 3.
		mockV1Client.On(
			"GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.GetDeploymentLogsParams) bool { return p.Limit != nil && *p.Limit == 3 }),
		).Return(&page1Response, nil).Once()
		// Second call: Limit pointer should have been shrunk to 1 (logCount - already-fetched).
		mockV1Client.On(
			"GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.GetDeploymentLogsParams) bool { return p.Limit != nil && *p.Limit == 1 }),
		).Return(&page2Response, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, false, false, 3, mockV1Client)
		s.NoError(err)
		mockV1Client.AssertExpectations(s.T())
	})
	s.Run("over-returned page is trimmed to requested count", func() {
		// API returns more results on a single page than logCount asked for. The
		// trim guard at the end of Logs should keep us from printing the extra.
		// We verify by mocking a single call (trim happens, no second call needed)
		// and asserting only one mock call landed.
		oversizedResponse := astrov1.GetDeploymentLogsResponse{
			JSON200: &astrov1.DeploymentLog{
				Limit: 2, MaxNumResults: 2, Offset: 0, ResultCount: 5,
				Results: []astrov1.DeploymentLogEntry{
					{Raw: "log 1", Timestamp: 1, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log 2", Timestamp: 2, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log 3", Timestamp: 3, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log 4", Timestamp: 4, Source: astrov1.DeploymentLogEntrySourceScheduler},
					{Raw: "log 5", Timestamp: 5, Source: astrov1.DeploymentLogEntrySourceScheduler},
				},
			},
			HTTPResponse: &http.Response{StatusCode: 200},
		}
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&oversizedResponse, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", true, true, true, true, false, nil, true, false, false, 2, mockV1Client)
		s.NoError(err)
		mockV1Client.AssertExpectations(s.T())
	})
	// Add after the "multiple components" subtest
	s.Run("airflow 3 deployment fetches apiserver logs", func() {
		// Mock Airflow 3 deployment
		airflow3Deployment := deploymentResponse
		if airflow3Deployment.JSON200 != nil {
			airflow3Deployment.JSON200.RuntimeVersion = "3.0-1"
		}
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&airflow3Deployment, nil).Once()

		mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&astrov1.GetDeploymentLogsResponse{JSON200: &astrov1.DeploymentLog{Results: []astrov1.DeploymentLogEntry{{Raw: "apiserver log", Timestamp: 1, Source: astrov1.DeploymentLogEntrySourceApiserver}}}, HTTPResponse: &http.Response{StatusCode: 200}}, nil).Once()

		_, err := Logs("test-id-1", ws, "", "", true, false, false, false, false, nil, false, false, false, 1, mockV1Client)
		s.NoError(err)
	})
	s.Run("dag-processor flag requests dag-processor source", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On(
			"GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.GetDeploymentLogsParams) bool {
				return len(p.Sources) == 1 && p.Sources[0] == astrov1.GetDeploymentLogsParamsSourcesDagProcessor
			}),
		).Return(&mockGetDeploymentLogsResponse, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", false, false, false, false, true, nil, false, false, false, logCount, mockV1Client)
		s.NoError(err)
	})
	s.Run("generic component flag passes through arbitrary sources", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On(
			"GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.GetDeploymentLogsParams) bool {
				return len(p.Sources) == 2 &&
					p.Sources[0] == astrov1.GetDeploymentLogsParamsSources("scheduler") &&
					p.Sources[1] == astrov1.GetDeploymentLogsParamsSources("future-component")
			}),
		).Return(&mockGetDeploymentLogsResponse, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", false, false, false, false, false, []string{"scheduler", "future-component"}, false, false, false, logCount, mockV1Client)
		s.NoError(err)
	})
	s.Run("default sources include dag-processor when no flags set", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On(
			"GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything,
			mock.MatchedBy(func(p *astrov1.GetDeploymentLogsParams) bool {
				for _, src := range p.Sources {
					if src == astrov1.GetDeploymentLogsParamsSourcesDagProcessor {
						return true
					}
				}
				return false
			}),
		).Return(&mockGetDeploymentLogsResponse, nil).Once()

		_, err := Logs(deploymentID, ws, "", "", false, false, false, false, false, nil, false, false, false, logCount, mockV1Client)
		s.NoError(err)
	})
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	// SleepTime and TickNum are whole seconds, and the wait-for-status cases
	// below poll a mock that is already healthy: no head start, one tick. They
	// are package variables, so put them back after.
	origSleep, origTick := SleepTime, TickNum
	SleepTime, TickNum = 0, 1
	s.T().Cleanup(func() { SleepTime, TickNum = origSleep, origTick })
	csID := "test-cluster-id"
	var (
		cloudProvider                = astrov1.DeploymentCloudProviderAWS
		mockCreateDeploymentResponse = astrov1.CreateDeploymentResponse{
			JSON200: &astrov1.Deployment{
				Id:            "test-id",
				CloudProvider: &cloudProvider,
				Type:          &hybridType,
				ClusterName:   &cluster.Name,
				Region:        &cluster.Region,
			},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}
		mockCreateHostedDeploymentResponse = astrov1.CreateDeploymentResponse{
			JSON200: &astrov1.Deployment{
				Id:                "test-id",
				CloudProvider:     &cloudProvider,
				Type:              &dedicatedType,
				ClusterName:       &cluster.Name,
				Region:            &cluster.Region,
				IsDevelopmentMode: &isDevelopmentMode,
			},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}
	)
	GetClusterOptionsResponseOK := astrov1.GetClusterOptionsResponse{
		JSON200: &[]astrov1.ClusterOptions{
			{
				DatabaseInstances:       []astrov1.ProviderInstanceType{},
				DefaultDatabaseInstance: astrov1.ProviderInstanceType{},
				DefaultNodeInstance:     astrov1.ProviderInstanceType{},

				DefaultPodSubnetRange:      nil,
				DefaultRegion:              astrov1.ProviderRegion{Name: "us-west-2"},
				DefaultServicePeeringRange: nil,
				DefaultServiceSubnetRange:  nil,
				DefaultVpcSubnetRange:      "10.0.0.0/16",
				NodeCountDefault:           1,
				NodeCountMax:               3,
				NodeCountMin:               1,
				NodeInstances:              []astrov1.ProviderInstanceType{},
				Regions:                    []astrov1.ProviderRegion{{Name: "us-west-1"}, {Name: "us-west-2"}},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("success with Celery Executor", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with Celery Executor and different schedulers", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(4)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(4)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(4)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "small", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "large", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "extra_large", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with enabling ci-cd enforcement", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with ci-cd enforcement enabled
		_, err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "enable", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with enabling development mode", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateHostedDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		// Call the Create function with development mode enabled
		_, err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "enable", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with cloud provider and region", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with workload identity", func() {
		mockWorkloadIdentity := "workload-id-1"
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astrov1.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateHybridDeploymentRequest()
				return *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with a non-empty workload ID
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "", "", "", "", "", "", "", "", mockWorkloadIdentity, astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with hosted deployment with workload identity", func() {
		mockWorkloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astrov1.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateDedicatedDeploymentRequest()
				return *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with a non-empty workload ID
		_, err := Create("test-name", ws, "test-desc", csID, "12.0.0", dagDeploy, CeleryExecutor, "aws", "us-west-2", strings.ToLower(string(astrov1.DeploymentSchedulerSizeSMALL)), "", "", "", "", "", "", "", mockWorkloadIdentity, astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with standard/dedicated type different scheduler sizes", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(11)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(11)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(11)
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(6)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		_, err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "small", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KUBERNETES, gcpCloud,
			"us-west-2", "large", "enable", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KUBERNETES, gcpCloud,
			"us-west-2", "extra_large", "enable", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function with deployment type as DEDICATED, cloud provider, and region set
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "small", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CELERY, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, ASTRO, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", "large", "enable", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", "extra_large", "enable", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with region selection", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetClusterOptionsResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for region selection
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function with region selection
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with Kube Executor", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock user input for deployment name and executor type
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "2")()

		// Call the Create function with Kube Executor
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with Astro Executor on Dedicated", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// Call the Create function with Dedicated Deployment and wait for status
		_, err := Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, true, 300*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success with Remote Execution config on Dedicated", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// Call the Create function with Dedicated Deployment and wait for status
		allowedIPAddressRanges := []string{"1.2.3.4/32"}
		taskLogBucket := "task-log-bucket"
		taskLogURLPattern := "task-log-url-pattern"
		_, err := Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, true, &allowedIPAddressRanges, &taskLogBucket, &taskLogURLPattern, mockV1Client, true, 300*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success and wait for status with Dedicated Deployment", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// Call the Create function with Dedicated Deployment and wait for status
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeDEDICATED, 0, 0, false, nil, nil, nil, mockV1Client, true, 300*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns an error when creating a Hybrid deployment fails", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, errCreateFailed).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with Hybrid Deployment that returns an error during creation
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.Error(err)
		s.Contains(err.Error(), "failed to create deployment")

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("failed to get workspaces", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, errMock).Once()

		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("failed to get default options", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Once()

		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns an error if cluster choice is not valid", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock invalid user input for cluster choice
		defer testUtil.MockUserInput(s.T(), "invalid-cluster-choice")()

		// Call the Create function and expect an error due to invalid cluster choice
		_, err := Create("test-name", ws, "test-desc", "", "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.Error(err)
		s.Contains(err.Error(), "invalid Cluster selected")

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("invalid hybrid resources", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Call the Create function and expect an error due to invalid cluster choice
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 10, 10, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.Error(err)
		s.ErrorIs(err, errInvalidResourceRequest)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("invalid workspace failure", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		_, err := Create("test-name", "wrong-workspace", "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeHYBRID, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.ErrorContains(err, "no Workspace with id")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("success for standard deployment with Celery Executor and different scheduler sizes", func() {
		// Set up mock responses and expectations
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockV1Client.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(4)
		mockV1Client.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(4)

		// Mock user input for deployment name
		// defer testUtil.MockUserInput(s.T(), "test-name")()
		// defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		_, err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "small", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "large", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Call the Create function
		_, err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "extra_large", "", "", "", "", "", "", "", "", astrov1.DeploymentTypeSTANDARD, 0, 0, false, nil, nil, nil, mockV1Client, false, 0*time.Second, nil)
		s.NoError(err)

		// Assert expectations
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSelectCluster() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	csID := "test-cluster-id"
	s.Run("list cluster failure", func() {
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListClustersResponse{}, errMock).Once()

		_, err := selectCluster("", mockOrgID, mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("cluster id via selection", func() {
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		resp, err := selectCluster("", mockOrgID, mockV1Client)
		s.NoError(err)
		s.Equal(csID, resp)
	})

	s.Run("cluster id invalid selection", func() {
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// mock os.Stdin
		input := []byte("4")
		r, w, err := os.Pipe()
		s.Require().NoError(err)
		_, err = w.Write(input)
		s.NoError(err)
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = selectCluster("", mockOrgID, mockV1Client)
		s.ErrorIs(err, errInvalidClusterKey)
	})

	s.Run("not able to find cluster", func() {
		mockV1Client.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		_, err := selectCluster("test-invalid-id", mockOrgID, mockV1Client)
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Cluster")
		s.Contains(err.Error(), `"test-invalid-id"`)
		s.Contains(err.Error(), "astro organization cluster list")
	})
}

func (s *Suite) TestCanCiCdDeploy() {
	s.T().Cleanup(func() { parseToken = util.ParseAPIToken })
	permissions := []string{}
	mockClaims := util.CustomClaims{
		Permissions: permissions,
	}

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return &mockClaims, nil
	}

	canDeploy := CanCiCdDeploy("bearer token")
	s.Equal(canDeploy, false)

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return nil, errMock
	}
	canDeploy = CanCiCdDeploy("bearer token")
	s.Equal(canDeploy, false)

	permissions = []string{
		"workspaceId:workspace-id",
		"organizationId:org-ID",
	}
	mockClaims = util.CustomClaims{
		Permissions: permissions,
	}

	parseToken = func(astroAPIToken string) (*util.CustomClaims, error) {
		return &mockClaims, nil
	}

	canDeploy = CanCiCdDeploy("bearer token")
	s.Equal(canDeploy, true)
}

// TestCanCiCdDeployTokenForms runs the real parser over every form a stored
// token can take. One that does not show it carries permissions is not one
// that may deploy past CI/CD enforcement — and is never a panic.
// SelectDeployment takes the only Deployment without asking when CI supplied
// an ASTRO_API_TOKEN, read by the one rule: one holding only the scheme is no
// token, so it asks (and, with nothing to read, fails).
func (s *Suite) TestSelectDeploymentReadsTheAPITokenByTheOneRule() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.T().Setenv("ASTRONOMER_KEY_ID", "")
	s.T().Setenv("ASTRONOMER_KEY_SECRET", "")
	only := []astrov1.Deployment{{Id: "dep-1", Name: "only"}}
	for env, asks := range map[string]bool{"Bearer ": true, "Bearer Bearer": true, "ci-token": false, "Bearer ci-token": false} {
		s.Run(env, func() {
			s.T().Setenv("ASTRO_API_TOKEN", env)
			defer testUtil.MockUserInput(s.T(), "")()
			got, err := SelectDeployment(only, "Which?")
			if asks {
				s.Error(err)
				return
			}
			s.NoError(err)
			s.Equal("dep-1", got.Id)
		})
	}
}

func (s *Suite) TestCanCiCdDeployTokenForms() {
	orig := parseToken
	s.T().Cleanup(func() { parseToken = orig })
	parseToken = util.ParseAPIToken

	jwtWith := func(claims string) string {
		enc := base64.RawURLEncoding.EncodeToString
		return enc([]byte(`{"alg":"HS256","typ":"JWT"}`)) + "." + enc([]byte(claims)) + ".c2ln"
	}
	apiToken := jwtWith(`{"permissions":["workspaceId:ws","organizationId:org"]}`)
	userToken := jwtWith(`{"sub":"user"}`)

	for _, tc := range []struct {
		name, token string
		want        bool
	}{
		{"API token with Bearer", "Bearer " + apiToken, true},
		{"API token with lowercase scheme", "bearer " + apiToken, true},
		{"API token without Bearer", apiToken, true},
		{"API token with surrounding space", "  Bearer " + apiToken + " \n", true},
		{"user token with Bearer", "Bearer " + userToken, false},
		{"user token without Bearer", userToken, false},
		{"empty", "", false},
		{"logged out", "Bearer ", false},
		{"Bearer alone", "Bearer", false},
		{"not a JWT", "Bearer not-a-jwt", false},
		{"malformed JWT", "Bearer a.b", false},
		{"bad base64", "Bearer !!!.@@@.###", false},
	} {
		s.Run(tc.name, func() {
			s.Equal(tc.want, CanCiCdDeploy(tc.token))
		})
	}

	// A stored token that holds no credential at all is no API token, and
	// not a token to warn about: nothing was there to parse.
	for _, stored := range []string{"", "Bearer ", "Bearer", "Bearer Bearer", "bearer  BEARER "} {
		s.Run("quietly no credential: "+stored, func() {
			// A file, not a pipe: nothing has to drain it while the call
			// runs, however much it writes.
			f, err := os.CreateTemp(s.T().TempDir(), "stderr")
			s.Require().NoError(err)
			stderr := os.Stderr
			os.Stderr = f
			s.T().Cleanup(func() { os.Stderr = stderr; f.Close() })
			got := CanCiCdDeploy(stored)
			os.Stderr = stderr
			warned, err := os.ReadFile(f.Name())
			s.Require().NoError(err)
			s.False(got)
			s.Empty(string(warned))
		})
	}
}

func (s *Suite) TestUpdate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cloudProvider := astrov1.DeploymentCloudProviderAZURE
	astroMachine := "test-machine"
	nodeID := "test-id"
	varValue := "VALUE"
	deploymentResponse.JSON200.WorkerQueues = &[]astrov1.WorkerQueue{
		{
			AstroMachine:      &astroMachine,
			Id:                "queue-id",
			IsDefault:         true,
			MaxWorkerCount:    10,
			MinWorkerCount:    1,
			Name:              "worker-name",
			WorkerConcurrency: 20,
			NodePoolId:        &nodeID,
		},
	}
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{
			IsSecret: false,
			Key:      "KEY",
			Value:    &varValue,
		},
	}
	deploymentResponse.JSON200.Executor = &executorCelery

	mockUpdateDeploymentResponse := astrov1.UpdateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Id:            "test-id",
			CloudProvider: &cloudProvider,
			Type:          &hybridType,
			Region:        &cluster.Region,
			ClusterName:   &cluster.Name,
		},

		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("update success", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(6)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Twice()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with hybrid type in this test nothing is being change just ensuring that dag deploy stays true. Addtionally no deployment id/name is given so user input is needed to select one
		_, err := Update("", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, true, mockV1Client, nil)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// success updating the kubernetes executor on hybrid type. deployment name is given
		_, err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with standard type and deployment name input and dag deploy stays the same
		_, err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success updating to kubernetes executor on standard type
		_, err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType
		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()

		// success with dedicated type no changes made asserts that dag deploy stays the same
		_, err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with dedicated updating to kubernetes executor
		_, err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})
	s.Run("successfully update schedulerSize and highAvailability and CICDEnforement", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(6)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		// mock os.Stdin
		// Mock user input for deployment name

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()
		// success with standard type with name
		_, err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// success with large scheduler size
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "large", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// success with extra large scheduler size
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "extra_large", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// success with hybrid type with id
		deploymentResponse.JSON200.Type = &hybridType
		_, err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with hybrid type with id
		deploymentResponse.JSON200.Executor = &executorKubernetes
		_, err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("successfully update developmentMode", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with standard type with name
		_, err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "disable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to dedicatd and set development mode to false
		deploymentResponse.JSON200.Type = &dedicatedType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "disable", "enable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("successfully update worker queues with standard type", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(i astrov1.UpdateDeploymentRequest) bool {
			input, _ := i.AsUpdateStandardDeploymentRequest()
			return input.WorkerQueues != nil && len(*input.WorkerQueues) == 1 && (*input.WorkerQueues)[0].Name == "worker-name"
		})).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType
		deploymentResponse.JSON200.WorkerQueues = &[]astrov1.WorkerQueue{
			{
				AstroMachine:      &astroMachine,
				Id:                "queue-id",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				Name:              "worker-name",
				WorkerConcurrency: 20,
				NodePoolId:        &nodeID,
			},
		}

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with standard type with name
		_, err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "disable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with standard type
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "enable", "disable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, nil, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("successfully update worker queues", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(func(i astrov1.UpdateDeploymentRequest) bool {
			input, _ := i.AsUpdateDedicatedDeploymentRequest()
			return input.WorkerQueues != nil && len(*input.WorkerQueues) == 1 && (*input.WorkerQueues)[0].Name == "worker-name"
		})).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		// change type to dedicated
		deploymentResponse.JSON200.Type = &dedicatedType
		deploymentResponse.JSON200.WorkerQueues = &[]astrov1.WorkerQueue{
			{
				AstroMachine:      &astroMachine,
				Id:                "queue-id",
				IsDefault:         true,
				MaxWorkerCount:    10,
				MinWorkerCount:    1,
				Name:              "worker-name",
				WorkerConcurrency: 20,
				NodePoolId:        &nodeID,
			},
		}

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type with name
		_, err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "disable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type
		_, err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "disable", "enable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, nil, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("failed to validate resources", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		_, err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorIs(err, errMock)

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		deploymentResponse.JSON200.Type = &hybridType
		_, err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "10Gi", "2CPU", "10Gi", "", 100, 100, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorIs(err, errInvalidResourceRequest)
	})

	s.Run("list deployments failure", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		_, err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorIs(err, errMock)
	})

	s.Run("invalid deployment id", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)
		// list deployment error
		_, err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorIs(err, errMock)

		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)

		// invalid id
		_, err = Update("invalid-id", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorContains(err, "the Deployment specified was not found in this workspace.")
		// invalid name
		_, err = Update("", "", ws, "update", "invalid-name", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorContains(err, "the Deployment specified was not found in this workspace.")

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "0")()

		// invalid selection
		_, err = Update("", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorContains(err, "invalid Deployment selected")
	})

	s.Run("cancel update", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "n")()

		_, err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("update deployment failure", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.ErrorIs(err, errMock)
		s.NotContains(err.Error(), organization.AstronomerConnectionErrMsg)
	})

	s.Run("do not update deployment to enable dag deploy if already enabled", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		_, err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("throw warning to enable dag deploy if ci-cd enforcement is enabled", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		defer testUtil.MockUserInput(s.T(), "n")()
		// The warning is a note, not the result: it goes to the writer for
		// notes, never to stdout.
		var notes bytes.Buffer
		_, err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "enable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, &notes)
		s.NoError(err)
		s.Contains(notes.String(), "Warning: You are trying to update the dag deploy setting with ci-cd enforcement enabled.")
	})

	s.Run("do not update deployment to disable dag deploy if already disabled", func() {
		deploymentResponse.JSON200.IsDagDeployEnabled = false
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		_, err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("update deployment to change executor to KubernetesExecutor", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astrov1.UpdateDeploymentRequest) bool {
				// converting to hybrid deployment request type works for all three tests because the executor and worker queues are only being checked and
				// it's common in all three deployment types, if we have to test more than we should break this into multiple test scenarios
				request, err := input.AsUpdateHybridDeploymentRequest()
				s.NoError(err)
				return request.Executor == KUBERNETES && request.WorkerQueues == nil
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		defer testUtil.MockUserInput(s.T(), "y")()

		_, err := Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		_, err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		_, err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)

		s.NoError(err)
	})

	s.Run("update deployment to change executor to CeleryExecutor", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		deploymentResponse.JSON200.Executor = &executorKubernetes

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		_, err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		defer testUtil.MockUserInput(s.T(), "y")()

		// test update with hybrid type
		deploymentResponse.JSON200.Type = &hybridType
		_, err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("do not update deployment if user says no to the executor change", func() {
		// change type to hybrid
		deploymentResponse.JSON200.Type = &hybridType
		deploymentResponse.JSON200.Executor = &executorKubernetes

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		defer testUtil.MockUserInput(s.T(), "n")()

		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("no node pools on hybrid cluster", func() {
		mockGetClusterResponseWithNoNodePools := astrov1.GetClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.Cluster{
				Id:        "test-cluster-id",
				Name:      "test-cluster",
				NodePools: nil,
			},
		}

		// change type to hybrid
		deploymentResponse.JSON200.Type = &hybridType
		deploymentResponse.JSON200.Executor = &executorKubernetes

		defer testUtil.MockUserInput(s.T(), "y")()

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponseWithNoNodePools, nil).Once()

		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})

	s.Run("update workload identity for a hosted deployment", func() {
		mockWorkloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"

		// change type to dedicated
		deploymentResponse.JSON200.Type = &dedicatedType

		// Set up mock responses and expectations
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astrov1.UpdateDeploymentRequest) bool {
				request, err := input.AsUpdateStandardDeploymentRequest()
				assert.NoError(s.T(), err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Once()
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Call the Update function with a non-empty workload ID
		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "small", "enable", "", "disable", "", "", "", "", mockWorkloadIdentity, 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, true, mockV1Client, nil)
		s.NoError(err)
	})
	s.Run("update deployment to change executor to/from ASTRO executor", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(4)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(4)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(4)

		// Set type to standard for ASTRO executor support
		deploymentResponse.JSON200.Type = &standardType

		// CELERY -> ASTRO
		deploymentResponse.JSON200.Executor = &executorCelery
		defer testUtil.MockUserInput(s.T(), "y")()
		_, err := Update("test-id-1", "", ws, "", "", "", AstroExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// ASTRO -> CELERY
		astroExecutor := astrov1.DeploymentExecutorASTRO
		deploymentResponse.JSON200.Executor = &astroExecutor
		defer testUtil.MockUserInput(s.T(), "y")()
		_, err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// ASTRO -> KUBERNETES
		deploymentResponse.JSON200.Executor = &astroExecutor
		defer testUtil.MockUserInput(s.T(), "y")()
		_, err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)

		// KUBERNETES -> ASTRO
		deploymentResponse.JSON200.Executor = &executorKubernetes
		defer testUtil.MockUserInput(s.T(), "y")()
		_, err = Update("test-id-1", "", ws, "", "", "", AstroExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, nil, nil, nil, false, mockV1Client, nil)
		s.NoError(err)
	})
	s.Run("update deployment to change remote execution config", func() {
		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		oldTaskLogBucket := "old-task-log-bucket"
		oldTaskLogURLPattern := "old-task-log-url-pattern"
		oldAllowedIPAddressRanges := []string{"1.2.3.4/32"}
		deploymentResponse.JSON200.RemoteExecution = &astrov1.DeploymentRemoteExecution{
			Enabled:                true,
			AllowedIpAddressRanges: oldAllowedIPAddressRanges,
			TaskLogBucket:          &oldTaskLogBucket,
			TaskLogUrlPattern:      &oldTaskLogURLPattern,
		}
		newTaskLogBucket := "new-task-log-bucket"
		newTaskLogURLPattern := "new-task-log-url-pattern"
		newAllowedIPAddressRanges := []string{"1.2.3.5/32"}
		defer testUtil.MockUserInput(s.T(), "y")()
		_, err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, &newAllowedIPAddressRanges, &newTaskLogBucket, &newTaskLogURLPattern, false, mockV1Client, nil)
		s.NoError(err)
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockDeleteDeploymentResponse := astrov1.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("success", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "y")()

		removal, err := Delete("test-id-1", ws, "", false, mockV1Client)
		s.NoError(err)
		s.Equal(&Removal{DeploymentID: "test-id-1", Name: deploymentResponse.JSON200.Name, WorkspaceID: deploymentResponse.JSON200.WorkspaceId, Action: actionDeleted}, removal)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("list deployments failure", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		_, err := Delete("test-id-1", ws, "", false, mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})

	// A Deployment named in a Workspace with none is not found, as it is in
	// a Workspace with others; it used to print "no Deployments found" and
	// exit 0, having deleted nothing.
	s.Run("no deployments in a workspace", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)

		removal, err := Delete("test-id-1", ws, "", false, mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)
		s.Nil(removal)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("nothing named, and no deployments in a workspace", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)

		_, err := Delete("", ws, "", false, mockV1Client)
		s.EqualError(err, fmt.Sprintf("%s %s to delete", NoDeploymentInWSMsg, ws))
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("invalid deployment id", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "0")()

		_, err := Delete("", ws, "", false, mockV1Client)
		s.ErrorContains(err, "invalid Deployment selected")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("cancel update", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "n")()

		removal, err := Delete("test-id-1", ws, "", false, mockV1Client)
		s.NoError(err)
		s.Nil(removal, "a declined deletion deleted nothing")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("delete deployment failure", func() {
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockV1Client.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, errMock).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "y")()

		_, err := Delete("test-id-1", ws, "", false, mockV1Client)
		s.ErrorIs(err, errMock)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentURL() {
	deploymentID := "deployment-id"
	workspaceID := "workspace-id"

	s.Run("returns deploymentURL for dev environment", func() {
		testUtil.InitTestConfig(testUtil.CloudDevPlatform)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for stage environment", func() {
		testUtil.InitTestConfig(testUtil.CloudStagePlatform)
		expectedURL := "cloud.astronomer-stage.io/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for perf environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPerfPlatform)
		expectedURL := "cloud.astronomer-perf.io/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for cloud (prod) environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedURL := "cloud.astronomer.io/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for pr preview environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for local environment", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedURL := "localhost:5000/workspace-id/deployments/deployment-id"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns an error if getting current context fails", func() {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		expectedURL := ""
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.ErrorContains(err, "no context set")
		s.Equal(expectedURL, actualURL)
	})
}

// func (s *Suite) TestPrintWarning() {
// 	s.Run("when KubernetesExecutor is requested", func() {
// 		s.Run("returns true > 1 queues exist", func() {
// 			actual := printWarning(KubeExecutor, 3)
// 			s.True(actual)
// 		})
// 		s.Run("returns false if only 1 queue exists", func() {
// 			actual := printWarning(KubeExecutor, 1)
// 			s.False(actual)
// 		})
// 	})
// 	s.Run("returns true when CeleryExecutor is requested", func() {
// 		actual := printWarning(CeleryExecutor, 2)
// 		s.True(actual)
// 	})
// 	s.Run("returns false for any other executor is requested", func() {
// 		actual := printWarning("non-existent", 2)
// 		s.False(actual)
// 	})
// }

func (s *Suite) TestGetPlatformDeploymentOptions() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("function success", func() {
		GetDeploymentOptionsResponse := astrov1.GetDeploymentOptionsResponse{
			JSON200: &astrov1.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, nil).Times(1)

		_, err := GetPlatformDeploymentOptions("", astrov1.GetDeploymentOptionsParams{}, mockV1Client)
		s.NoError(err)
	})

	s.Run("function failure", func() {
		GetDeploymentOptionsResponse := astrov1.GetDeploymentOptionsResponse{
			JSON200: &astrov1.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, errMock).Times(1)

		_, err := GetPlatformDeploymentOptions("", astrov1.GetDeploymentOptionsParams{}, mockV1Client)
		s.ErrorIs(err, errMock)
	})
}

func (s *Suite) TestUpdateDeploymentHibernationOverride() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	tests := []struct {
		IsHibernating bool
		command       string
	}{
		{true, "hibernate"},
		{false, "wake-up"},
	}

	for _, tt := range tests {
		s.Run(tt.command, func() {
			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					IsActive:      &isActive,
				},
			}

			mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(s.T(), "y")()

			res, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", tt.IsHibernating, nil, false, mockV1Client)
			s.NoError(err)
			s.Equal(&HibernationResult{
				DeploymentID: "test-id-1",
				Name:         deploymentResponse.JSON200.Name,
				Override:     &HibernationOverride{IsHibernating: tt.IsHibernating},
			}, res, "the override the API reports, with no end")
			mockV1Client.AssertExpectations(s.T())
		})

		s.Run(fmt.Sprintf("%s with OverrideUntil", tt.command), func() {
			overrideUntil := time.Now().Add(time.Hour)
			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &overrideUntil,
					IsActive:      &isActive,
				},
			}

			mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(s.T(), "y")()

			_, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", tt.IsHibernating, &overrideUntil, false, mockV1Client)
			s.NoError(err)
			mockV1Client.AssertExpectations(s.T())
		})
	}

	s.Run("returns an error if deployment is not in development mode", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse2, nil).Once()

		_, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", true, nil, false, mockV1Client)
		s.Error(err)
		s.Equal(err, errNotADevelopmentDeployment)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns an error if none of the deployments are in development mode", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)

		var deploymentList []astrov1.Deployment
		for i := range mockCoreDeploymentResponse {
			deployment := &mockCoreDeploymentResponse[i]
			if deployment.IsDevelopmentMode != nil && *deployment.IsDevelopmentMode == true {
				continue
			}
			deploymentList = append(deploymentList, *deployment)
		}
		mockDeploymentListResponse := astrov1.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrov1.DeploymentsPaginated{
				Deployments: deploymentList,
			},
		}

		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeploymentListResponse, nil).Once()

		_, err := UpdateDeploymentHibernationOverride("", ws, "", true, nil, false, mockV1Client)
		s.Error(err)
		s.Equal(err.Error(), fmt.Sprintf("%s %s", NoDeploymentInWSMsg, ws))
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("cancels if requested", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "n")()

		_, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", true, nil, false, mockV1Client)
		s.NoError(err)
		mockV1Client.AssertExpectations(s.T())
	})

	// Nothing to hibernate is a failure: it used to print that and exit 0.
	s.Run("fails if no deployments were found in the workspace", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		res, err := UpdateDeploymentHibernationOverride("", ws, "", true, nil, false, mockV1Client)
		s.EqualError(err, fmt.Sprintf("%s %s to hibernate", NoDeploymentInWSMsg, ws))
		s.Nil(res)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("fails for a named Deployment in a workspace with none", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		_, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", false, nil, true, mockV1Client)
		s.ErrorIs(err, errInvalidDeployment)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestUpdateDeploymentHibernationOverrideWaits() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	origTick, origGet := TickNum, GetDeploymentByID
	TickNum = 1
	s.T().Cleanup(func() { TickNum, GetDeploymentByID = origTick, origGet })

	// The first status answers the lookup that picks the Deployment; the rest
	// answer the wait, one per tick.
	reports := func(statuses ...astrov1.DeploymentStatus) *int {
		polls := 0
		GetDeploymentByID = func(_, _ string, _ astrov1.APIClient) (astrov1.Deployment, error) {
			status := statuses[min(polls, len(statuses)-1)]
			polls++
			isDevelopmentMode := true
			return astrov1.Deployment{Id: "test-id-1", Name: "test", Status: status, IsDevelopmentMode: &isDevelopmentMode}, nil
		}
		return &polls
	}
	override := func(isHibernating bool, waitTime time.Duration) error {
		isActive := true
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateDeploymentHibernationOverrideResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.DeploymentHibernationOverride{IsHibernating: &isHibernating, IsActive: &isActive},
		}, nil).Once()
		res, err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", isHibernating, nil, true, mockV1Client)
		if err != nil {
			return err
		}
		return WaitForHibernationOverride(nil, res.DeploymentID, isHibernating, waitTime, mockV1Client)
	}

	s.Run("wake-up waits past HIBERNATING until HEALTHY", func() {
		polls := reports(astrov1.DeploymentStatusHIBERNATING, astrov1.DeploymentStatusHIBERNATING, astrov1.DeploymentStatusDEPLOYING, astrov1.DeploymentStatusHEALTHY)
		s.NoError(override(false, time.Minute))
		s.Equal(4, *polls)
	})

	s.Run("hibernate waits until HIBERNATING", func() {
		polls := reports(astrov1.DeploymentStatusHEALTHY, astrov1.DeploymentStatusHEALTHY, astrov1.DeploymentStatusHIBERNATING)
		s.NoError(override(true, time.Minute))
		s.Equal(3, *polls)
	})

	s.Run("hibernate times out", func() {
		reports(astrov1.DeploymentStatusHEALTHY)
		s.ErrorIs(override(true, time.Second), ErrTimedOutHibernating)
	})

	s.Run("wake-up names the Airflow when only it is missing at the timeout", func() {
		origAnswering := airflowAnswering
		s.T().Cleanup(func() { airflowAnswering = origAnswering })
		airflowAnswering = func(astrov1.Deployment, string) bool { return false }
		reports(astrov1.DeploymentStatusHIBERNATING, astrov1.DeploymentStatusHEALTHY)
		s.ErrorIs(override(false, 2*time.Second), errAirflowNotAnswering)
	})

	s.Run("wake-up waits past HEALTHY until the Airflow answers", func() {
		origAnswering := airflowAnswering
		s.T().Cleanup(func() { airflowAnswering = origAnswering })
		probes := 0
		airflowAnswering = func(astrov1.Deployment, string) bool {
			probes++
			return probes == 2
		}
		polls := reports(astrov1.DeploymentStatusHIBERNATING, astrov1.DeploymentStatusHEALTHY)
		s.NoError(override(false, time.Minute))
		s.Equal(3, *polls)
		s.Equal(2, probes)
	})
}

func (s *Suite) TestAirflowAnswering() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	login, err := context.GetCurrentContext()
	s.Require().NoError(err)
	s.Require().NotEmpty(login.Token)
	for status, want := range map[int]bool{
		http.StatusOK:                 true,
		http.StatusUnauthorized:       true,
		http.StatusNotFound:           true,
		http.StatusBadGateway:         false,
		http.StatusServiceUnavailable: false,
		http.StatusGatewayTimeout:     false,
	} {
		var path, auth string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path, auth = r.URL.Path, r.Header.Get("Authorization")
			w.WriteHeader(status)
		}))
		got := airflowAnswering(astrov1.Deployment{WebServerAirflowApiUrl: server.URL + "/abc123/api/v2"}, "")
		server.Close()
		s.Equal(want, got, "status %d", status)
		s.Equal("/abc123/api/v2/version", path, "status %d", status)
		s.Equal(login.Token, auth, "status %d", status)
	}
	s.True(airflowAnswering(astrov1.Deployment{}, ""), "a Deployment with no Airflow API URL should not hold up the wait")

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	s.True(airflowAnswering(astrov1.Deployment{WebServerAirflowApiUrl: unreachable.URL}, ""), "an Airflow host this machine cannot reach should not hold up the wait")

	var auth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
	}))
	defer other.Close()
	airflowAnswering(astrov1.Deployment{WebServerAirflowApiUrl: other.URL}, "Bearer other-login")
	s.Equal("Bearer other-login", auth, "a given token should replace the current context's")
}

func (s *Suite) TestDeleteDeploymentHibernationOverride() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("remove override", func() {
		mockResponse := astrov1.DeleteDeploymentHibernationOverrideResponse{
			HTTPResponse: &http.Response{
				StatusCode: http.StatusNoContent,
			},
		}

		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "y")()

		res, err := DeleteDeploymentHibernationOverride("test-id-1", ws, "", false, mockV1Client)
		s.NoError(err)
		s.Equal(&HibernationResult{DeploymentID: "test-id-1", Name: deploymentResponse.JSON200.Name}, res, "no override once removed")
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("remove override with deployment selection", func() {
		mockResponse := astrov1.DeleteDeploymentHibernationOverrideResponse{
			HTTPResponse: &http.Response{
				StatusCode: http.StatusNoContent,
			},
		}

		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockV1Client.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "1")()

		_, err := DeleteDeploymentHibernationOverride("", ws, "", true, mockV1Client)
		s.NoError(err)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("returns an error if deployment is not in development mode", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse2, nil).Once()

		_, err := DeleteDeploymentHibernationOverride("test-id-2", ws, "", false, mockV1Client)
		s.Error(err)
		s.Equal(err, errNotADevelopmentDeployment)
		mockV1Client.AssertExpectations(s.T())
	})

	s.Run("cancels if requested", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "n")()

		_, err := DeleteDeploymentHibernationOverride("test-id-1", ws, "", false, mockV1Client)
		s.NoError(err)
		mockV1Client.AssertExpectations(s.T())
	})

	// Nothing to remove an override from is a failure: it used to print that
	// and exit 0.
	s.Run("fails if no deployments were found in the workspace", func() {
		mockV1Client = new(astrov1_mocks.ClientWithResponsesInterface)
		mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		_, err := DeleteDeploymentHibernationOverride("", ws, "", false, mockV1Client)
		s.EqualError(err, "no Deployments with a hibernation override that can be removed found in Workspace "+ws)
		mockV1Client.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCreateDefaultTaskPodCPU() {
	s.Run("creates default task pod CPU if set", func() {
		cpu := createDefaultTaskPodCPU("1CPU", false, nil)
		s.Equal("1CPU", *cpu)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		cpu := createDefaultTaskPodCPU("", true, nil)
		s.Nil(cpu)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		cpu := createDefaultTaskPodCPU("", false, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{DefaultPodSize: astrov1.ResourceOption{Cpu: astrov1.ResourceRange{Default: "0.5CPU"}}}})
		s.Equal("0.5CPU", *cpu)
	})
}

func (s *Suite) TestCreateDefaultTaskPodMemory() {
	s.Run("creates default task pod memory if set", func() {
		memory := createDefaultTaskPodMemory("1Gi", false, nil)
		s.Equal("1Gi", *memory)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		memory := createDefaultTaskPodMemory("", true, nil)
		s.Nil(memory)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		memory := createDefaultTaskPodMemory("", false, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{DefaultPodSize: astrov1.ResourceOption{Memory: astrov1.ResourceRange{Default: "2Gi"}}}})
		s.Equal("2Gi", *memory)
	})
}

func (s *Suite) TestCreateResourceQuotaCPU() {
	s.Run("creates CPU if set", func() {
		cpu := createResourceQuotaCPU("1CPU", false, nil)
		s.Equal("1CPU", *cpu)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		cpu := createResourceQuotaCPU("", true, nil)
		s.Nil(cpu)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		cpu := createResourceQuotaCPU("", false, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{ResourceQuota: astrov1.ResourceOption{Cpu: astrov1.ResourceRange{Default: "0.5CPU"}}}})
		s.Equal("0.5CPU", *cpu)
	})
}

func (s *Suite) TestCreateResourceQuotaMemory() {
	s.Run("creates memory if set", func() {
		memory := createResourceQuotaMemory("1Gi", false, nil)
		s.Equal("1Gi", *memory)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		memory := createResourceQuotaMemory("", true, nil)
		s.Nil(memory)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		memory := createResourceQuotaMemory("", false, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{ResourceQuota: astrov1.ResourceOption{Memory: astrov1.ResourceRange{Default: "2Gi"}}}})
		s.Equal("2Gi", *memory)
	})
}

func (s *Suite) TestUpdateDefaultTaskPodCPU() {
	s.Run("updates CPU if set", func() {
		cpu := updateDefaultTaskPodCPU("2CPU", nil, nil)
		s.Equal("2CPU", *cpu)
	})

	s.Run("keeps existing CPU if set", func() {
		existingDefaultTaskPodCPU := "1CPU"
		cpu := updateDefaultTaskPodCPU("1CPU", &astrov1.Deployment{DefaultTaskPodCpu: &existingDefaultTaskPodCPU}, nil)
		s.Equal("1CPU", *cpu)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		cpu := updateDefaultTaskPodCPU("", &astrov1.Deployment{RemoteExecution: &astrov1.DeploymentRemoteExecution{Enabled: true}}, nil)
		s.Nil(cpu)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		cpu := updateDefaultTaskPodCPU("", &astrov1.Deployment{}, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{DefaultPodSize: astrov1.ResourceOption{Cpu: astrov1.ResourceRange{Default: "0.5CPU"}}}})
		s.Equal("0.5CPU", *cpu)
	})

	s.Run("returns nil if CPU is not set, Remote Execution is disabled and config is not available", func() {
		cpu := updateDefaultTaskPodCPU("", &astrov1.Deployment{}, nil)
		s.Nil(cpu)
	})
}

func (s *Suite) TestUpdateDefaultTaskPodMemory() {
	s.Run("updates memory if set", func() {
		memory := updateDefaultTaskPodMemory("2Gi", nil, nil)
		s.Equal("2Gi", *memory)
	})

	s.Run("keeps existing memory if set", func() {
		existingDefaultTaskPodMemory := "1Gi"
		memory := updateDefaultTaskPodMemory("1Gi", &astrov1.Deployment{DefaultTaskPodMemory: &existingDefaultTaskPodMemory}, nil)
		s.Equal("1Gi", *memory)
	})

	s.Run("defaults to nil if Remote Execution is enabled", func() {
		memory := updateDefaultTaskPodMemory("", &astrov1.Deployment{RemoteExecution: &astrov1.DeploymentRemoteExecution{Enabled: true}}, nil)
		s.Nil(memory)
	})

	s.Run("defaults to config option if Remote Execution is disabled", func() {
		memory := updateDefaultTaskPodMemory("", &astrov1.Deployment{}, &astrov1.DeploymentOptions{ResourceQuotas: astrov1.ResourceQuotaOptions{DefaultPodSize: astrov1.ResourceOption{Memory: astrov1.ResourceRange{Default: "2Gi"}}}})
		s.Equal("2Gi", *memory)
	})

	s.Run("returns nil if memory is not set, Remote Execution is disabled and config is not available", func() {
		memory := updateDefaultTaskPodMemory("", &astrov1.Deployment{}, nil)
		s.Nil(memory)
	})
}

func (s *Suite) TestUpdateResourceQuotaCPU() {
	s.Run("updates CPU if set", func() {
		cpu := updateResourceQuotaCPU("2CPU", nil)
		s.Equal("2CPU", *cpu)
	})

	s.Run("keeps existing CPU", func() {
		existingResourceQuotaCPU := "1CPU"
		cpu := updateResourceQuotaCPU("1CPU", &astrov1.Deployment{ResourceQuotaCpu: &existingResourceQuotaCPU})
		s.Equal("1CPU", *cpu)
	})
}

func (s *Suite) TestUpdateResourceQuotaMemory() {
	s.Run("updates memory if set", func() {
		memory := updateResourceQuotaMemory("2Gi", nil)
		s.Equal("2Gi", *memory)
	})

	s.Run("keeps existing memory", func() {
		existingResourceQuotaMemory := "1Gi"
		memory := updateResourceQuotaMemory("1Gi", &astrov1.Deployment{ResourceQuotaMemory: &existingResourceQuotaMemory})
		s.Equal("1Gi", *memory)
	})
}
