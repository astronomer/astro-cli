package deployment

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestDeployment(t *testing.T) {
	suite.Run(t, new(Suite))
}

var (
	hybridQueueList                = []astroplatformcore.HybridWorkerQueueRequest{}
	workerQueueRequest             = []astroplatformcore.WorkerQueueRequest{}
	newEnvironmentVariables        = []astroplatformcore.DeploymentEnvironmentVariableRequest{}
	errMock                        = errors.New("mock error")
	errCreateFailed                = errors.New("failed to create deployment")
	nodePools                      = []astroplatformcore.NodePool{}
	mockListClustersResponse       = astroplatformcore.ListClustersResponse{}
	cluster                        = astroplatformcore.Cluster{}
	mockGetClusterResponse         = astroplatformcore.GetClusterResponse{}
	standardType                   = astroplatformcore.DeploymentTypeSTANDARD
	dedicatedType                  = astroplatformcore.DeploymentTypeDEDICATED
	hybridType                     = astroplatformcore.DeploymentTypeHYBRID
	testRegion                     = "region"
	testProvider                   = astroplatformcore.DeploymentCloudProviderGCP
	testCluster                    = "cluster"
	testWorkloadIdentity           = "test-workload-identity"
	mockCoreDeploymentResponse     = []astroplatformcore.Deployment{}
	mockListDeploymentsResponse    = astroplatformcore.ListDeploymentsResponse{}
	emptyListDeploymentsResponse   = astroplatformcore.ListDeploymentsResponse{}
	schedulerAU                    = 0
	clusterID                      = "cluster-id"
	executorCelery                 = astroplatformcore.DeploymentExecutorCELERY
	executorKubernetes             = astroplatformcore.DeploymentExecutorKUBERNETES
	highAvailability               = true
	isDevelopmentMode              = true
	resourceQuotaCPU               = "1cpu"
	ResourceQuotaMemory            = "1"
	schedulerSize                  = astroplatformcore.DeploymentSchedulerSizeSMALL
	deploymentResponse             = astroplatformcore.GetDeploymentResponse{}
	deploymentResponse2            = astroplatformcore.GetDeploymentResponse{}
	GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{}
	workspaceTestDescription       = "test workspace"
	workspace1                     = astrocore.Workspace{}
	workspaces                     = []astrocore.Workspace{}
	ListWorkspacesResponseOK       = astrocore.ListWorkspacesResponse{}
)

func MockResponseInit() {
	nodePools = []astroplatformcore.NodePool{
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
	mockListClustersResponse = astroplatformcore.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.ClustersPaginated{
			Clusters: []astroplatformcore.Cluster{
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
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
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
	mockListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponse,
		},
	}
	emptyListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{},
		},
	}
	deploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
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
	deploymentResponse2 = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:                  "test-id-2",
			RuntimeVersion:      "4.2.5",
			Namespace:           "test-name",
			WorkspaceId:         ws,
			WebServerUrl:        "test-url",
			IsDagDeployEnabled:  false,
			Name:                "test",
			Status:              "HEALTHY",
			Type:                &hybridType,
			SchedulerAu:         &schedulerAU,
			ClusterId:           &clusterID,
			Executor:            &executorCelery,
			IsHighAvailability:  &highAvailability,
			ResourceQuotaCpu:    &resourceQuotaCPU,
			ResourceQuotaMemory: &ResourceQuotaMemory,
			SchedulerSize:       &schedulerSize,
			WorkerQueues:        &[]astroplatformcore.WorkerQueue{},
			WorkloadIdentity:    &testWorkloadIdentity,
		},
	}
	GetDeploymentOptionsResponseOK = astrocore.GetDeploymentOptionsResponse{
		JSON200: &astrocore.DeploymentOptions{
			DefaultValues: astrocore.DefaultValueOptions{
				WorkerMachineName: "test-machine",
			},
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
			WorkerMachines: []astrocore.WorkerMachine{
				{
					Concurrency: astrocore.Range{
						Ceiling: 1,
						Default: 1,
						Floor:   1,
					},
					Name: "test-machine",
					Spec: astrocore.MachineSpec{},
				},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	workspaceTestDescription = "test workspace"
	workspace1 = astrocore.Workspace{
		Name:                         "test-workspace",
		Description:                  &workspaceTestDescription,
		ApiKeyOnlyDeploymentsDefault: false,
		Id:                           "workspace-id",
		OrganizationId:               "test-org-id",
	}
	workspaces = []astrocore.Workspace{
		workspace1,
	}
	ListWorkspacesResponseOK = astrocore.ListWorkspacesResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.WorkspacesPaginated{
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

var (
	mockPlatformCoreClient *astroplatformcore_mocks.ClientWithResponsesInterface
	mockCoreClient         *astrocore_mocks.ClientWithResponsesInterface
)

func (s *Suite) SetupTest() {
	// init mocks
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)

	// init responses object
	MockResponseInit()
}

func (s *Suite) TearDownSubTest() {
	// assert expectations
	mockPlatformCoreClient.AssertExpectations(s.T())
	mockCoreClient.AssertExpectations(s.T())

	// reset mocks
	mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	mockCoreClient = new(astrocore_mocks.ClientWithResponsesInterface)

	// reset responses object
	MockResponseInit()
}

func (s *Suite) TestList() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("success", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with no deployments in a workspace", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		s.NoError(err)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with all true", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List("", true, mockPlatformCoreClient, buf)
		s.NoError(err)
		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(nil, errMock).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		s.ErrorIs(err, errMock)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with hidden namespace information", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		buf := new(bytes.Buffer)
		err := List(ws, false, mockPlatformCoreClient, buf)
		s.NoError(err)

		s.Contains(buf.String(), "test-id-1")
		s.Contains(buf.String(), "test-id-2")
		s.Contains(buf.String(), "N/A")
		s.Contains(buf.String(), "region")
		s.Contains(buf.String(), "cluster")

		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeployment() {
	deploymentID := "test-id-wrong"
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("invalid deployment ID", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, deploymentID, "", false, nil, mockPlatformCoreClient, nil)
		s.ErrorIs(err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	deploymentName := "test-wrong"
	deploymentID = "test-id-1"
	s.Run("error after invalid deployment Name", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, false, nil, mockPlatformCoreClient, nil)
		s.ErrorIs(err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("no deployments in workspace", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		_, err := GetDeployment(ws, "", deploymentName, true, nil, mockPlatformCoreClient, nil)
		s.ErrorIs(err, errInvalidDeployment)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	s.Run("correct deployment ID", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, "", false, nil, mockPlatformCoreClient, nil)

		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})
	deploymentName = "test"
	s.Run("correct deployment Name", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, "", deploymentName, false, nil, mockPlatformCoreClient, nil)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Name)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("two deployments with the same name", func() {
		mockCoreDeploymentResponse = []astroplatformcore.Deployment{
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

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

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

		deployment, err := GetDeployment(ws, "", deploymentName, false, nil, mockPlatformCoreClient, nil)
		s.NoError(err)
		s.Equal(deploymentName, deployment.Name)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("deployment name and deployment id", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, mockPlatformCoreClient, nil)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("create deployment error", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
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

		_, err = GetDeployment(ws, "", "", false, nil, mockPlatformCoreClient, nil)
		s.ErrorIs(err, errMock)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("get deployments after creation error", func() {
		// Assuming org, ws, and errMock are predefined
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
		) error {
			return nil
		}

		// Second ListDeployments call with an error to simulate an error after deployment creation
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Once()

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

		_, err = GetDeployment(ws, "", "", false, nil, mockPlatformCoreClient, nil)
		s.ErrorIs(err, errMock)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("test automatic deployment creation", func() {
		// Assuming org, ws, and deploymentID are predefined
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		// Mock createDeployment
		createDeployment = func(
			label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement,
			defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string,
			deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int,
			platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool,
		) error {
			return nil
		}

		mockCoreDeploymentResponse = []astroplatformcore.Deployment{
			{
				Id:            "test-id-1",
				Name:          "test",
				Status:        "HEALTHY",
				Type:          &standardType,
				Region:        &testRegion,
				CloudProvider: &testProvider,
			},
		}

		mockListOneDeploymentsResponse := astroplatformcore.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.DeploymentsPaginated{
				Deployments: mockCoreDeploymentResponse,
			},
		}

		// Second ListDeployments call after successful deployment creation
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListOneDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

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

		deployment, err := GetDeployment(ws, "", "", false, nil, mockPlatformCoreClient, nil)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)

		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCoreGetDeployment() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	s.Run("success", func() {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := CoreGetDeployment(org, deploymentID, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success from context", func() {
		ctx, err := context.GetCurrentContext()
		s.NoError(err)
		ctx.SetContextKey("organization_short_name", org)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		deployment, err := CoreGetDeployment("", deploymentID, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(deploymentID, deployment.Id)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("context error", func() {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)

		_, err := CoreGetDeployment("", deploymentID, mockPlatformCoreClient)
		s.ErrorContains(err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
	})

	s.Run("error in api response", func() {
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, errMock).Once()

		_, err := CoreGetDeployment(org, deploymentID, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestIsDeploymentDedicated() {
	s.Run("if deployment type is dedicated", func() {
		out := IsDeploymentDedicated(astroplatformcore.DeploymentTypeDEDICATED)
		s.Equal(out, true)
	})

	s.Run("if deployment type is hybrid", func() {
		out := IsDeploymentDedicated(astroplatformcore.DeploymentTypeHYBRID)
		s.Equal(out, false)
	})
}

func (s *Suite) TestSelectRegion() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("list regions failure", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(nil, errMock).Once()

		_, err := selectRegion("gcp", "", mockCoreClient)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("region via selection", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

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

		resp, err := selectRegion("gcp", "", mockCoreClient)
		s.NoError(err)
		s.Equal(region, resp)
	})

	s.Run("region via selection aws", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderAws) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

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

		resp, err := selectRegion("aws", "", mockCoreClient)
		s.NoError(err)
		s.Equal(region, resp)
	})

	s.Run("region invalid selection", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

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

		_, err = selectRegion("gcp", "", mockCoreClient)
		s.ErrorIs(err, ErrInvalidRegionKey)
	})

	s.Run("not able to find region", func() {
		provider := astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
		getSharedClusterOptionsParams := &astrocore.GetClusterOptionsParams{
			Provider: &provider,
			Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
		}

		mockOKRegionResponse := &astrocore.GetClusterOptionsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &[]astrocore.ClusterOptions{
				{Regions: []astrocore.ProviderRegion{{Name: region}}},
			},
		}

		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, getSharedClusterOptionsParams).Return(mockOKRegionResponse, nil).Once()

		_, err := selectRegion("gcp", "test-invalid-region", mockCoreClient)
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Region")
	})
}

func (s *Suite) TestLogs() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	deploymentID := "test-id-1"
	logCount := 2
	mockGetDeploymentLogsResponse := astrocore.GetDeploymentLogsResponse{
		JSON200: &astrocore.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrocore.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockGetDeploymentLogsMultipleComponentsResponse := astrocore.GetDeploymentLogsResponse{
		JSON200: &astrocore.DeploymentLog{
			Limit:         4,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
			Results: []astrocore.DeploymentLogEntry{
				{
					Raw:       "test log line",
					Timestamp: 1,
					Source:    astrocore.DeploymentLogEntrySourceWebserver,
				},
				{
					Raw:       "test log line 2",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceTriggerer,
				},
				{
					Raw:       "test log line 3",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceScheduler,
				},
				{
					Raw:       "test log line 4",
					Timestamp: 2,
					Source:    astrocore.DeploymentLogEntrySourceWorker,
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
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

		err := Logs(deploymentID, ws, "", "", true, true, true, true, true, false, false, logCount, mockPlatformCoreClient, mockCoreClient)
		s.NoError(err)

		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
	s.Run("success without deployment", func() {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse using the reusable variable
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Once()

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

		err = Logs("", ws, "", "keyword", true, true, true, true, false, false, false, 1, mockPlatformCoreClient, mockCoreClient)
		s.NoError(err)

		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failure", func() {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()

		// Mock GetDeploymentWithResponse for the first deployment in the list
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock GetDeploymentLogsWithResponse to return an error
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, errMock).Once()

		err := Logs(deploymentID, ws, "", "", true, true, true, true, false, false, false, logCount, mockPlatformCoreClient, mockCoreClient)
		s.ErrorIs(err, errMock)

		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("query for more than one log level error", func() {
		err := Logs(deploymentID, ws, "", "", true, true, true, true, true, true, true, logCount, mockPlatformCoreClient, mockCoreClient)
		s.Error(err)
		s.Equal(err.Error(), "cannot query for more than one log level and/or keyword at a time")
	})
	s.Run("multiple components", func() {
		// Mock ListDeployments
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockCoreClient.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsMultipleComponentsResponse, nil).Once()

		err := Logs(deploymentID, ws, "", "", true, true, true, true, true, false, false, logCount, mockPlatformCoreClient, mockCoreClient)
		s.NoError(err)

		mockPlatformCoreClient.AssertExpectations(s.T())
		mockCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestCreate() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	csID := "test-cluster-id"
	var (
		cloudProvider                = astroplatformcore.DeploymentCloudProviderAWS
		mockCreateDeploymentResponse = astroplatformcore.CreateDeploymentResponse{
			JSON200: &astroplatformcore.Deployment{
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
		mockCreateHostedDeploymentResponse = astroplatformcore.CreateDeploymentResponse{
			JSON200: &astroplatformcore.Deployment{
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
	GetClusterOptionsResponseOK := astrocore.GetClusterOptionsResponse{
		JSON200: &[]astrocore.ClusterOptions{
			{
				DatabaseInstances:       []astrocore.ProviderInstanceType{},
				DefaultDatabaseInstance: astrocore.ProviderInstanceType{},
				DefaultNodeInstance:     astrocore.ProviderInstanceType{},

				DefaultPodSubnetRange:      nil,
				DefaultRegion:              astrocore.ProviderRegion{Name: "us-west-2"},
				DefaultServicePeeringRange: nil,
				DefaultServiceSubnetRange:  nil,
				DefaultVpcSubnetRange:      "10.0.0.0/16",
				NodeCountDefault:           1,
				NodeCountMax:               3,
				NodeCountMin:               1,
				NodeInstances:              []astrocore.ProviderInstanceType{},
				Regions:                    []astrocore.ProviderRegion{{Name: "us-west-1"}, {Name: "us-west-2"}},
			},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("success with Celery Executor", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})


	s.Run("success with Celery Executor and different schedulers", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(4)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(4)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(4)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "small", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "large", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "extra_large", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with enabling ci-cd enforcement", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with ci-cd enforcement enabled
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "enable", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with enabling development mode", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateHostedDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		// Call the Create function with development mode enabled
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "enable", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with cloud provider and region", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with workload identity", func() {
		mockWorkloadIdentity := "workload-id-1"
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateHybridDeploymentRequest()
				return *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with a non-empty workload ID
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "", "", "", "", "", "", "", "", mockWorkloadIdentity, astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with hosted deployment with workload identity", func() {
		mockWorkloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.CreateDeploymentRequest) bool {
				request, _ := input.AsCreateDedicatedDeploymentRequest()
				return *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with a non-empty workload ID
		err := Create("test-name", ws, "test-desc", csID, "12.0.0", dagDeploy, CeleryExecutor, "aws", "us-west-2", strings.ToLower(string(astrocore.DeploymentSchedulerSizeSMALL)), "", "", "", "", "", "", "", mockWorkloadIdentity, astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with standard/dedicated type different scheduler sizes", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(11)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(11)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(11)
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(6)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()
		// Call the Create function with deployment type as STANDARD, cloud provider, and region set
		err := Create("", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "small", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KUBERNETES, gcpCloud,
			"us-west-2", "large", "enable", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KUBERNETES, gcpCloud,
			"us-west-2", "extra_large", "enable", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function with deployment type as DEDICATED, cloud provider, and region set
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "us-west-2", "small", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CELERY, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, ASTRO, azureCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", "large", "enable", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, gcpCloud, "us-west-2", "extra_large", "enable", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with region selection", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("GetClusterOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetClusterOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()

		// Mock user input for region selection
		defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function with region selection
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "aws", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with Kube Executor", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name and executor type
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "2")()

		// Call the Create function with Kube Executor
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, KubeExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success with Astro Executor on Dedicated", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// setup wait for test
		sleepTime = 1
		tickNum = 2

		// Call the Create function with Dedicated Deployment and wait for status
		err := Create("test-name", ws, "test-desc", csID, "3.0-1", dagDeploy, AstroExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, true)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("success and wait for status with Dedicated Deployment", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Mock user input for deployment name and wait for status
		defer testUtil.MockUserInput(s.T(), "test-name")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// setup wait for test
		sleepTime = 1
		tickNum = 2

		// Call the Create function with Dedicated Deployment and wait for status
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeDEDICATED, 0, 0, mockPlatformCoreClient, mockCoreClient, true)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("returns an error when creating a Hybrid deployment fails", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, errCreateFailed).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "test-name")()

		// Call the Create function with Hybrid Deployment that returns an error during creation
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.Error(err)
		s.Contains(err.Error(), "failed to create deployment")

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed to get workspaces", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, errMock).Once()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("failed to get default options", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Once()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.ErrorIs(err, errMock)
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("returns an error if cluster choice is not valid", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		// Mock invalid user input for cluster choice
		defer testUtil.MockUserInput(s.T(), "invalid-cluster-choice")()

		// Call the Create function and expect an error due to invalid cluster choice
		err := Create("test-name", ws, "test-desc", "", "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.Error(err)
		s.Contains(err.Error(), "invalid Cluster selected")

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("invalid hybrid resources", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		// Call the Create function and expect an error due to invalid cluster choice
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 10, 10, mockPlatformCoreClient, mockCoreClient, false)
		s.Error(err)
		s.ErrorIs(err, ErrInvalidResourceRequest)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("invalid workspace failure", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()

		err := Create("test-name", "wrong-workspace", "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.ErrorContains(err, "no Workspace with id")
		mockCoreClient.AssertExpectations(s.T())
	})

	s.Run("success for standard deployment with Celery Executor and different scheduler sizes", func() {
		// Set up mock responses and expectations
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(4)
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Times(4)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(4)

		// Mock user input for deployment name
		// defer testUtil.MockUserInput(s.T(), "test-name")()
		// defer testUtil.MockUserInput(s.T(), "1")()

		// Call the Create function
		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "small", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "medium", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "large", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Call the Create function
		err = Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, awsCloud, "us-west-2", "extra_large", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeSTANDARD, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("no node pools on hybrid cluster", func() {
		mockGetClusterResponseWithNoNodePools := mockGetClusterResponse
		mockGetClusterResponseWithNoNodePools.JSON200.NodePools = nil

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockPlatformCoreClient.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockCreateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponseWithNoNodePools, nil).Once()

		defer testUtil.MockUserInput(s.T(), "test-name")()

		err := Create("test-name", ws, "test-desc", csID, "4.2.5", dagDeploy, CeleryExecutor, "", "", "", "", "", "", "", "", "", "", "", astroplatformcore.DeploymentTypeHYBRID, 0, 0, mockPlatformCoreClient, mockCoreClient, false)
		s.NoError(err)

		// Assert expectations
		mockCoreClient.AssertExpectations(s.T())
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestSelectCluster() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	csID := "test-cluster-id"
	s.Run("list cluster failure", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astroplatformcore.ListClustersResponse{}, errMock).Once()

		_, err := selectCluster("", mockOrgID, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cluster id via selection", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

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

		resp, err := selectCluster("", mockOrgID, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(csID, resp)
	})

	s.Run("cluster id invalid selection", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

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

		_, err = selectCluster("", mockOrgID, mockPlatformCoreClient)
		s.ErrorIs(err, ErrInvalidClusterKey)
	})

	s.Run("not able to find cluster", func() {
		mockPlatformCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()

		_, err := selectCluster("test-invalid-id", mockOrgID, mockPlatformCoreClient)
		s.Error(err)
		s.Contains(err.Error(), "unable to find specified Cluster")
	})
}

func (s *Suite) TestCanCiCdDeploy() {
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

func (s *Suite) TestUpdate() { //nolint
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	cloudProvider := astroplatformcore.DeploymentCloudProviderAZURE
	astroMachine := "test-machine"
	nodeID := "test-id"
	varValue := "VALUE"
	deploymentResponse.JSON200.WorkerQueues = &[]astroplatformcore.WorkerQueue{
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
	deploymentResponse.JSON200.EnvironmentVariables = &[]astroplatformcore.DeploymentEnvironmentVariable{
		{
			IsSecret: false,
			Key:      "KEY",
			Value:    &varValue,
		},
	}
	deploymentResponse.JSON200.Executor = &executorCelery
	mockUpdateDeploymentResponse := astroplatformcore.UpdateDeploymentResponse{
		JSON200: &astroplatformcore.Deployment{
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
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Twice()

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with hybrid type in this test nothing is being change just ensuring that dag deploy stays true. Addtionally no deployment id/name is given so user input is needed to select one
		err := Update("", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, true, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// success updating the kubernetes executor on hybrid type. deployment name is given
		err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with standard type and deployment name input and dag deploy stays the same
		err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success updating to kubernetes executor on standard type
		err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType
		// mock os.Stdin
		// Mock user input for deployment name
		// defer testUtil.MockUserInput(t, "1")()

		// success with dedicated type no changes made asserts that dag deploy stays the same
		err = Update("test-id-1", "", ws, "", "", "", CeleryExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
		s.Equal(deploymentResponse.JSON200.IsDagDeployEnabled, dagDeployEnabled)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with dedicated updating to kubernetes executor
		err = Update("test-id-1", "", ws, "", "", "", KubeExecutor, "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})
	s.Run("successfully update schedulerSize and highAvailability and CICDEnforement", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(6)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(6)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(2)

		// mock os.Stdin
		// Mock user input for deployment name

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()
		// success with standard type with name
		err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to dedicatd
		deploymentResponse.JSON200.Type = &dedicatedType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type
		err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// success with large scheduler size
		err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "large", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// success with extra large scheduler size
		err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "extra_large", "enable", "", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// success with hybrid type with id
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "y")()

		// success with hybrid type with id
		deploymentResponse.JSON200.Executor = &executorKubernetes
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("successfully update developmentMode", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(2)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(2)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with standard type with name
		err := Update("", "test", ws, "", "", "enable", CeleryExecutor, "medium", "disable", "disable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to dedicatd and set development mode to false
		deploymentResponse.JSON200.Type = &dedicatedType

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "1")()

		// success with dedicated type
		err = Update("", "", ws, "", "test-1", "enable", CeleryExecutor, "medium", "disable", "enable", "disable", "", "", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("failed to validate resources", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, errMock).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "10Gi", "2CPU", "10Gi", "", 100, 100, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorIs(err, ErrInvalidResourceRequest)
	})

	s.Run("list deployments failure", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
	})

	s.Run("invalid deployment id", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)
		// list deployment error
		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)

		// invalid id
		err = Update("invalid-id", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorContains(err, "the Deployment specified was not found in this workspace.")
		// invalid name
		err = Update("", "", ws, "update", "invalid-name", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorContains(err, "the Deployment specified was not found in this workspace.")

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "0")()

		// invalid selection
		err = Update("", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorContains(err, "invalid Deployment selected")
	})

	s.Run("cancel update", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		// mock os.Stdin
		// Mock user input for deployment name
		defer testUtil.MockUserInput(s.T(), "n")()

		err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("update deployment failure", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, errMock).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
		s.NotContains(err.Error(), organization.AstronomerConnectionErrMsg)
	})

	s.Run("do not update deployment to enable dag deploy if already enabled", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		deploymentResponse.JSON200.IsDagDeployEnabled = true
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("throw warning to enable dag deploy if ci-cd enforcement is enabled", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		canCiCdDeploy = func(astroAPIToken string) bool {
			return false
		}

		defer testUtil.MockUserInput(s.T(), "n")()
		err := Update("test-id-1", "", ws, "update", "", "enable", CeleryExecutor, "medium", "enable", "", "enable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("do not update deployment to disable dag deploy if already disabled", func() {
		deploymentResponse.JSON200.IsDagDeployEnabled = false
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)

		err := Update("test-id-1", "", ws, "update", "", "disable", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("update deployment to change executor to KubernetesExecutor", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				// converting to hybrid deployment request type works for all three tests because the executor and worker queues are only being checked and
				// it's common in all three deployment types, if we have to test more than we should break this into multiple test scenarios
				request, err := input.AsUpdateHybridDeploymentRequest()
				s.NoError(err)
				return request.Executor == KUBERNETES && request.WorkerQueues == nil
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		defer testUtil.MockUserInput(s.T(), "y")()

		err := Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", KubeExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)

		s.NoError(err)
	})

	s.Run("update deployment to change executor to CeleryExecutor", func() {
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(3)
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		deploymentResponse.JSON200.Executor = &executorKubernetes

		// change type to standard
		deploymentResponse.JSON200.Type = &standardType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		// change type to standard
		deploymentResponse.JSON200.Type = &dedicatedType

		defer testUtil.MockUserInput(s.T(), "y")()
		// test update with standard type
		err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)

		defer testUtil.MockUserInput(s.T(), "y")()

		// test update with hybrid type
		deploymentResponse.JSON200.Type = &hybridType
		err = Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("do not update deployment if user says no to the executor change", func() {
		// change type to hybrid
		deploymentResponse.JSON200.Type = &hybridType
		deploymentResponse.JSON200.Executor = &executorKubernetes

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Times(1)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

		defer testUtil.MockUserInput(s.T(), "n")()

		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("no node pools on hybrid cluster", func() {
		mockGetClusterResponseWithNoNodePools := astroplatformcore.GetClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.Cluster{
				Id:        "test-cluster-id",
				Name:      "test-cluster",
				NodePools: nil,
			},
		}

		// change type to hybrid
		deploymentResponse.JSON200.Type = &hybridType
		deploymentResponse.JSON200.Executor = &executorKubernetes

		defer testUtil.MockUserInput(s.T(), "y")()

		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponseWithNoNodePools, nil).Once()

		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "medium", "enable", "", "disable", "2CPU", "2Gi", "2CPU", "2Gi", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("update workload identity for a hosted deployment", func() {
		mockWorkloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"

		// change type to dedicated
		deploymentResponse.JSON200.Type = &dedicatedType

		// Set up mock responses and expectations
		mockPlatformCoreClient.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.MatchedBy(
			func(input astroplatformcore.UpdateDeploymentRequest) bool {
				request, err := input.AsUpdateStandardDeploymentRequest()
				assert.NoError(s.T(), err)
				return request.WorkloadIdentity != nil && *request.WorkloadIdentity == mockWorkloadIdentity
			},
		)).Return(&mockUpdateDeploymentResponse, nil).Once()
		mockCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Once()
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		// Call the Update function with a non-empty workload ID
		err := Update("test-id-1", "", ws, "update", "", "", CeleryExecutor, "small", "enable", "", "disable", "", "", "", "", mockWorkloadIdentity, 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, true, mockCoreClient, mockPlatformCoreClient)
		s.NoError(err)
	})
}

func (s *Suite) TestDelete() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockDeleteDeploymentResponse := astroplatformcore.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	s.Run("success", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "y")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("list deployments failure", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMock).Times(1)

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("no deployments in a workspace", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Times(1)

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("invalid deployment id", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "0")()

		err := Delete("", ws, "", false, mockPlatformCoreClient)
		s.ErrorContains(err, "invalid Deployment selected")
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cancel update", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "n")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("delete deployment failure", func() {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
		mockPlatformCoreClient.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, errMock).Times(1)

		// mock os.Stdin
		defer testUtil.MockUserInput(s.T(), "y")()

		err := Delete("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.ErrorIs(err, errMock)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestGetDeploymentURL() {
	deploymentID := "deployment-id"
	workspaceID := "workspace-id"

	s.Run("returns deploymentURL for dev environment", func() {
		testUtil.InitTestConfig(testUtil.CloudDevPlatform)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for stage environment", func() {
		testUtil.InitTestConfig(testUtil.CloudStagePlatform)
		expectedURL := "cloud.astronomer-stage.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for perf environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPerfPlatform)
		expectedURL := "cloud.astronomer-perf.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for cloud (prod) environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		expectedURL := "cloud.astronomer.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for pr preview environment", func() {
		testUtil.InitTestConfig(testUtil.CloudPrPreview)
		expectedURL := "cloud.astronomer-dev.io/workspace-id/deployments/deployment-id/overview"
		actualURL, err := GetDeploymentURL(deploymentID, workspaceID)
		s.NoError(err)
		s.Equal(expectedURL, actualURL)
	})
	s.Run("returns deploymentURL for local environment", func() {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedURL := "localhost:5000/workspace-id/deployments/deployment-id/overview"
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

func (s *Suite) TestPrintWarning() {
	s.Run("when KubernetesExecutor is requested", func() {
		s.Run("returns true > 1 queues exist", func() {
			actual := printWarning(KubeExecutor, 3)
			s.True(actual)
		})
		s.Run("returns false if only 1 queue exists", func() {
			actual := printWarning(KubeExecutor, 1)
			s.False(actual)
		})
	})
	s.Run("returns true when CeleryExecutor is requested", func() {
		actual := printWarning(CeleryExecutor, 2)
		s.True(actual)
	})
	s.Run("returns false for any other executor is requested", func() {
		actual := printWarning("non-existent", 2)
		s.False(actual)
	})
}

func (s *Suite) TestGetPlatformDeploymentOptions() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	s.Run("function success", func() {
		GetDeploymentOptionsResponse := astroplatformcore.GetDeploymentOptionsResponse{
			JSON200: &astroplatformcore.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, nil).Times(1)

		_, err := GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, mockPlatformCoreClient)
		s.NoError(err)
	})

	s.Run("function failure", func() {
		GetDeploymentOptionsResponse := astroplatformcore.GetDeploymentOptionsResponse{
			JSON200: &astroplatformcore.DeploymentOptions{},
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
		}

		mockPlatformCoreClient.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponse, errMock).Times(1)

		_, err := GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, mockPlatformCoreClient)
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
			mockResponse := astroplatformcore.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus200,
				},
				JSON200: &astroplatformcore.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					IsActive:      &isActive,
				},
			}

			mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
			mockPlatformCoreClient.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(s.T(), "y")()

			err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", tt.IsHibernating, nil, false, mockPlatformCoreClient)
			s.NoError(err)
			mockPlatformCoreClient.AssertExpectations(s.T())
		})

		s.Run(fmt.Sprintf("%s with OverrideUntil", tt.command), func() {
			overrideUntil := time.Now().Add(time.Hour)
			isActive := true
			mockResponse := astroplatformcore.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: astrocore.HTTPStatus200,
				},
				JSON200: &astroplatformcore.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &overrideUntil,
					IsActive:      &isActive,
				},
			}

			mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
			mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
			mockPlatformCoreClient.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(s.T(), "y")()

			err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", tt.IsHibernating, &overrideUntil, false, mockPlatformCoreClient)
			s.NoError(err)
			mockPlatformCoreClient.AssertExpectations(s.T())
		})
	}

	s.Run("returns an error if deployment is not in development mode", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse2, nil).Once()

		err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", true, nil, false, mockPlatformCoreClient)
		s.Error(err)
		s.Equal(err, ErrNotADevelopmentDeployment)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("returns an error if none of the deployments are in development mode", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)

		var deploymentList []astroplatformcore.Deployment
		for i := range mockCoreDeploymentResponse {
			deployment := &mockCoreDeploymentResponse[i]
			if deployment.IsDevelopmentMode != nil && *deployment.IsDevelopmentMode == true {
				continue
			}
			deploymentList = append(deploymentList, *deployment)
		}
		mockDeploymentListResponse := astroplatformcore.ListDeploymentsResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astroplatformcore.DeploymentsPaginated{
				Deployments: deploymentList,
			},
		}

		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeploymentListResponse, nil).Once()

		err := UpdateDeploymentHibernationOverride("", ws, "", true, nil, false, mockPlatformCoreClient)
		s.Error(err)
		s.Equal(err.Error(), fmt.Sprintf("%s %s", NoDeploymentInWSMsg, ws))
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cancels if requested", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "n")()

		err := UpdateDeploymentHibernationOverride("test-id-1", ws, "", true, nil, false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cancels if no deployments were found in the workspace", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		err := UpdateDeploymentHibernationOverride("", ws, "", true, nil, false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestDeleteDeploymentHibernationOverride() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	s.Run("remove override", func() {
		mockResponse := astroplatformcore.DeleteDeploymentHibernationOverrideResponse{
			HTTPResponse: &http.Response{
				StatusCode: astrocore.HTTPStatus204,
			},
		}

		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "y")()

		err := DeleteDeploymentHibernationOverride("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("remove override with deployment selection", func() {
		mockResponse := astroplatformcore.DeleteDeploymentHibernationOverrideResponse{
			HTTPResponse: &http.Response{
				StatusCode: astrocore.HTTPStatus204,
			},
		}

		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		mockPlatformCoreClient.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "1")()

		err := DeleteDeploymentHibernationOverride("", ws, "", true, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("returns an error if deployment is not in development mode", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse2, nil).Once()

		err := DeleteDeploymentHibernationOverride("test-id-2", ws, "", false, mockPlatformCoreClient)
		s.Error(err)
		s.Equal(err, ErrNotADevelopmentDeployment)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cancels if requested", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()

		defer testUtil.MockUserInput(s.T(), "n")()

		err := DeleteDeploymentHibernationOverride("test-id-1", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("cancels if no deployments were found in the workspace", func() {
		mockPlatformCoreClient = new(astroplatformcore_mocks.ClientWithResponsesInterface)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()

		err := DeleteDeploymentHibernationOverride("", ws, "", false, mockPlatformCoreClient)
		s.NoError(err)
		mockPlatformCoreClient.AssertExpectations(s.T())
	})
}

func (s *Suite) TestAirflow3Blocking() {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// Set up a deployment with Airflow 3 runtime version
	airflow3Version := "3.0-1"

	// Create a local copy of the deployment response with Airflow 3
	airflow3DeploymentResponse := astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.Deployment{
			Id:             "test-id-airflow3",
			Name:           "test-airflow3",
			RuntimeVersion: airflow3Version,
			WorkspaceId:    ws,
		},
	}

	// Mock list deployments response with Airflow 3 deployment
	airflow3ListDeploymentsResponse := astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{
				{
					Id:             "test-id-airflow3",
					Name:           "test-airflow3",
					RuntimeVersion: airflow3Version,
					WorkspaceId:    ws,
				},
			},
		},
	}

	s.Run("Update blocks operation for Airflow 3 deployments", func() {
		// Create a fresh local mock clients for this subtest
		updateMockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		updateMockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		// Set expectations on the local mock client
		// Mock the ListDeploymentsWithResponse method which is called by GetDeployment
		updateMockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&airflow3ListDeploymentsResponse, nil)
		updateMockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&airflow3DeploymentResponse, nil)

		// Test updating an Airflow 3 deployment
		err := Update("test-id-airflow3", "new-name", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueueRequest, hybridQueueList, newEnvironmentVariables, false, updateMockCoreClient, updateMockPlatformCoreClient)

		s.Error(err)
		s.Contains(err.Error(), "This command is not yet supported on Airflow 3 deployments")
		updateMockPlatformCoreClient.AssertExpectations(s.T())
	})

	s.Run("Logs blocks operation for Airflow 3 deployments", func() {
		// Create a fresh local mock clients for this subtest
		logsMockPlatformCoreClient := new(astroplatformcore_mocks.ClientWithResponsesInterface)
		logsMockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

		// Set expectations on the local mock client
		// Mock the ListDeploymentsWithResponse method which is called by GetDeployment
		logsMockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&airflow3ListDeploymentsResponse, nil)
		logsMockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&airflow3DeploymentResponse, nil)

		// Test viewing logs for an Airflow 3 deployment
		// logWebserver, logScheduler, logTriggerer, logWorkers, warnLogs, errorLogs, infoLogs
		err := Logs("test-id-airflow3", ws, "", "", true, true, true, true, false, false, false, 10, logsMockPlatformCoreClient, logsMockCoreClient)

		s.Error(err)
		s.Contains(err.Error(), "This command is not yet supported on Airflow 3 deployments")
		logsMockPlatformCoreClient.AssertExpectations(s.T())
	})
}
