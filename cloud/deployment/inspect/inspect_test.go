package inspect

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	astroplatformcore_mocks "github.com/astronomer/astro-cli/astro-client-platform-core/mocks"
	"github.com/astronomer/astro-cli/context"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	mockPlatformCoreClient     = new(astroplatformcore_mocks.ClientWithResponsesInterface)
	executorKubernetes         = astroplatformcore.DeploymentExecutorKUBERNETES
	errGetDeployment           = errors.New("test get deployment error")
	errMarshal                 = errors.New("test error")
	workloadIdentity           = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	deploymentID               = "test-deployment-id"
	standardType               = astroplatformcore.DeploymentTypeSTANDARD
	hybridType                 = astroplatformcore.DeploymentTypeHYBRID
	region                     = "us-central1"
	astroMachine               = "a5"
	mockCoreDeploymentResponse = []astroplatformcore.Deployment{
		{
			Status:           "HEALTHY",
			WorkloadIdentity: &workloadIdentity,
			Id:               deploymentID,
		},
	}
	mockCoreDeploymentResponseAlpha = []astrocore.Deployment{
		{
			Status:           "HEALTHY",
			Id:               deploymentID,
			WorkloadIdentity: &workloadIdentity,
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
	mockListDeploymentsResponseAlpha = astrocore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrocore.DeploymentsPaginated{
			Deployments: mockCoreDeploymentResponseAlpha,
		},
	}
	executorCelery                = astroplatformcore.DeploymentExecutorCELERY
	description                   = "description"
	workspaceName                 = "test-ws"
	clusterID                     = "cluster-id"
	ClusterName                   = "test-cluster"
	contactEmails                 = []string{"email1", "email2"}
	schedulerAU                   = 5
	schedulerReplicas             = 3
	envValue                      = "bar"
	envValue2                     = "baz"
	nodePoolID                    = "test-wq-id"
	nodePoolID1                   = "test-pool-id-1"
	deploymentEnvironmentVariable = []astroplatformcore.DeploymentEnvironmentVariable{
		{
			Key:       "foo",
			Value:     &envValue,
			IsSecret:  false,
			UpdatedAt: "NOW",
		},
		{
			Key:       "bar",
			Value:     &envValue2,
			IsSecret:  true,
			UpdatedAt: "NOW+1",
		},
	}
	workerqueue = []astroplatformcore.WorkerQueue{
		{
			Id:                "test-wq-id",
			Name:              "default",
			IsDefault:         true,
			MaxWorkerCount:    130,
			MinWorkerCount:    12,
			WorkerConcurrency: 110,
			NodePoolId:        &nodePoolID,
			AstroMachine:      &astroMachine,
			PodCpu:            "SmallCPU",
			PodMemory:         "megsOfRam",
		},
		{
			Id:                "test-wq-id-1",
			Name:              "test-queue-1",
			IsDefault:         false,
			MaxWorkerCount:    175,
			MinWorkerCount:    8,
			WorkerConcurrency: 150,
			NodePoolId:        &nodePoolID1,
			AstroMachine:      &astroMachine,
			PodCpu:            "LotsOfCPU",
			PodMemory:         "gigsOfRam",
		},
	}
	nodePools = []astroplatformcore.NodePool{
		{
			Id:               nodePoolID,
			IsDefault:        false,
			NodeInstanceType: "test-instance-type",
			CreatedAt:        time.Now(),
		},
		{
			Id:               nodePoolID1,
			IsDefault:        true,
			NodeInstanceType: "test-instance-type-1",
			CreatedAt:        time.Now(),
		},
	}
	cluster = astroplatformcore.Cluster{
		Id:        clusterID,
		Name:      "test-cluster",
		NodePools: &nodePools,
	}
	mockGetClusterResponse = astroplatformcore.GetClusterResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &cluster,
	}
	defaultTaskPodCPU      = "defaultTaskPodCPU"
	defaultTaskPodMemory   = "defaultTaskPodMemory"
	resourceQuotaCPU       = "resourceQuotaCPU"
	resourceQuotaMemory    = "ResourceQuotaMemory"
	cloudProvider          = astroplatformcore.DeploymentCloudProviderAWS
	schedulerTestSize      = astroplatformcore.DeploymentSchedulerSizeSMALL
	hibernationDescription = "hibernation schedule 1"
	hibernationSchedules   = []astroplatformcore.DeploymentHibernationSchedule{
		{
			HibernateAtCron: "1 * * * *",
			WakeAtCron:      "2 * * * *",
			Description:     &hibernationDescription,
			IsEnabled:       true,
		},
	}
	hibernationIsActive      = true
	hibernationIsHibernating = true
	hibernationOverrideUntil = time.Now()
	hibernationOverride      = astroplatformcore.DeploymentHibernationOverride{
		IsActive:      &hibernationIsActive,
		IsHibernating: &hibernationIsHibernating,
		OverrideUntil: &hibernationOverrideUntil,
	}
	isDevelopmentMode = true
	sourceDeployment  = astroplatformcore.Deployment{
		Id:                     deploymentID,
		Name:                   "test-deployment-label",
		Description:            &description,
		WorkspaceName:          &workspaceName,
		WorkspaceId:            "test-ws-id",
		Namespace:              "great-release-name",
		ClusterId:              &clusterID,
		ClusterName:            &ClusterName,
		ContactEmails:          &contactEmails,
		Type:                   &hybridType,
		Executor:               &executorCelery,
		Region:                 &region,
		RuntimeVersion:         "6.0.0",
		AirflowVersion:         "2.4.0",
		SchedulerAu:            &schedulerAU,
		SchedulerReplicas:      schedulerReplicas,
		WebServerAirflowApiUrl: "some-url/api/v1",
		EnvironmentVariables:   &deploymentEnvironmentVariable,
		WorkerQueues:           &workerqueue,
		UpdatedAt:              time.Now(),
		Status:                 "UNHEALTHY",
		IsDagDeployEnabled:     true,
		DefaultTaskPodCpu:      &defaultTaskPodCPU,
		DefaultTaskPodMemory:   &defaultTaskPodMemory,
		ResourceQuotaCpu:       &resourceQuotaCPU,
		ResourceQuotaMemory:    &resourceQuotaMemory,
		SchedulerSize:          &schedulerTestSize,
		CloudProvider:          &cloudProvider,
		IsDevelopmentMode:      &isDevelopmentMode,
		ScalingSpec: &astroplatformcore.DeploymentScalingSpec{
			HibernationSpec: &astroplatformcore.DeploymentHibernationSpec{
				Override:  &hibernationOverride,
				Schedules: &hibernationSchedules,
			},
		},
	}

	getDeploymentResponse = astroplatformcore.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &sourceDeployment,
	}
	emptyListDeploymentsResponse = astroplatformcore.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200: &astroplatformcore.DeploymentsPaginated{
			Deployments: []astroplatformcore.Deployment{},
		},
	}
)

func errReturningYAMLMarshal(v interface{}) ([]byte, error) {
	return []byte{}, errMarshal
}

func errReturningJSONMarshal(v interface{}, prefix, indent string) ([]byte, error) {
	return []byte{}, errMarshal
}

func errorReturningDecode(input, output interface{}) error {
	return errMarshal
}

func restoreDecode(replace func(input, output interface{}) error) {
	decodeToStruct = replace
}

func restoreJSONMarshal(replace func(v interface{}, prefix, indent string) ([]byte, error)) {
	jsonMarshal = replace
}

func restoreYAMLMarshal(replace func(v interface{}) ([]byte, error)) {
	yamlMarshal = replace
}

func TestInspect(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	workspaceID := "test-ws-id"
	deploymentName := "test-deployment-label"
	deploymentResponse := sourceDeployment
	t.Run("prints a deployment in yaml format to stdout", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), deploymentResponse.Namespace)
		assert.Contains(t, out.String(), deploymentName)
		assert.Contains(t, out.String(), deploymentResponse.RuntimeVersion)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prints a deployment template in yaml format to stdout", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", true, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), deploymentResponse.RuntimeVersion)
		assert.NotContains(t, out.String(), deploymentResponse.Namespace)
		assert.NotContains(t, out.String(), deploymentName)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prints a deployment in json format to stdout", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), deploymentResponse.Namespace)
		assert.Contains(t, out.String(), deploymentName)
		assert.Contains(t, out.String(), deploymentResponse.RuntimeVersion)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prints a deployment template in json format to stdout", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", true, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), deploymentResponse.RuntimeVersion)
		assert.NotContains(t, out.String(), deploymentResponse.Namespace)
		assert.NotContains(t, out.String(), deploymentName)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prints a deployment's specific field to stdout", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "configuration.cluster_name", false, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), *deploymentResponse.ClusterName)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("prompts for a deployment to inspect if no deployment name or id was provided", func(t *testing.T) {
		out := new(bytes.Buffer)
		defer testUtil.MockUserInput(t, "1")() // selecting test-deployment-id
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", "", "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), deploymentName)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if core deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, errGetDeployment).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.ErrorIs(t, err, errGetDeployment)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if listing deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errGetDeployment).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.ErrorIs(t, err, errGetDeployment)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if requested field is not found in deployment", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "no-exist-information", false, false)
		assert.ErrorIs(t, err, errKeyNotFound)
		assert.Equal(t, "", out.String())
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if formatting deployment fails", func(t *testing.T) {
		out := new(bytes.Buffer)
		originalMarshal := yamlMarshal
		yamlMarshal = errReturningYAMLMarshal
		defer restoreYAMLMarshal(originalMarshal)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.ErrorIs(t, err, errMarshal)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if getting context fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		out := new(bytes.Buffer)

		err := Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.ErrorContains(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("Display Cluster Region and hide Release Name if an org is hosted", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		out := new(bytes.Buffer)
		sourceDeployment.Type = &standardType
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		err = Inspect(workspaceID, "", deploymentID, "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "N/A")
		assert.Contains(t, out.String(), deploymentName)
		assert.Contains(t, out.String(), sourceDeployment.RuntimeVersion)
		assert.Contains(t, out.String(), "us-central1")
		assert.Contains(t, out.String(), "a5")
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
	t.Run("when no deployments in workspace", func(t *testing.T) {
		out := new(bytes.Buffer)
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&emptyListDeploymentsResponse, nil).Once()
		err := Inspect(workspaceID, "", "", "yaml", mockPlatformCoreClient, mockCoreClient, out, "", false, false)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
		mockPlatformCoreClient.AssertExpectations(t)
	})
}

func TestGetDeploymentInspectInfo(t *testing.T) {
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	description = ""
	cloudProvider := astroplatformcore.DeploymentCloudProviderGCP
	sourceDeployment.Description = &description
	sourceDeployment.CloudProvider = &cloudProvider
	sourceDeployment.Executor = &executorCelery
	sourceDeployment.ImageTag = "some-tag"
	sourceDeployment.CreatedAt = time.Now()
	sourceDeployment.UpdatedAt = time.Now()
	sourceDeployment.Status = "HEALTHY"

	t.Run("returns deployment metadata for the requested cloud deployment", func(t *testing.T) {
		var actualDeploymentMeta deploymentMetadata
		testUtil.InitTestConfig(testUtil.CloudPlatform)
		rawDeploymentInfo, err := getDeploymentInfo(sourceDeployment)
		assert.NoError(t, err)
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns deployment metadata for the requested local deployment", func(t *testing.T) {
		var actualDeploymentMeta deploymentMetadata
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		rawDeploymentInfo, err := getDeploymentInfo(sourceDeployment)
		assert.NoError(t, err)
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns error if getting context fails", func(t *testing.T) {
		var actualDeploymentMeta deploymentMetadata
		// get an error from GetCurrentContext()
		testUtil.InitTestConfig(testUtil.ErrorReturningContext)
		expectedDeploymentMetadata := deploymentMetadata{}
		rawDeploymentInfo, err := getDeploymentInfo(mockCoreDeploymentResponse[0])
		assert.ErrorContains(t, err, "no context set, have you authenticated to Astro or Astronomer Software? Run astro login and try again")
		err = decodeToStruct(rawDeploymentInfo, &actualDeploymentMeta)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentMetadata, actualDeploymentMeta)
		mockCoreClient.AssertExpectations(t)
	})
}

func TestGetDeploymentConfig(t *testing.T) {
	t.Run("returns deployment config for the requested cloud deployment", func(t *testing.T) {
		sourceDeployment.Type = &hybridType
		sourceDeployment.WorkloadIdentity = &workloadIdentity
		cloudProvider := astroplatformcore.DeploymentCloudProviderAWS
		sourceDeployment.CloudProvider = &cloudProvider
		var actualDeploymentConfig deploymentConfig
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedDeploymentConfig := deploymentConfig{
			Name:              sourceDeployment.Name,
			Description:       *sourceDeployment.Description,
			WorkspaceName:     *sourceDeployment.WorkspaceName,
			ClusterName:       *sourceDeployment.ClusterName,
			RunTimeVersion:    sourceDeployment.RuntimeVersion,
			SchedulerAU:       *sourceDeployment.SchedulerAu,
			SchedulerCount:    sourceDeployment.SchedulerReplicas,
			DagDeployEnabled:  &sourceDeployment.IsDagDeployEnabled,
			Executor:          string(*sourceDeployment.Executor),
			Region:            *sourceDeployment.Region,
			DeploymentType:    string(*sourceDeployment.Type),
			CloudProvider:     string(*sourceDeployment.CloudProvider),
			IsDevelopmentMode: *sourceDeployment.IsDevelopmentMode,
		}
		rawDeploymentConfig, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		err = decodeToStruct(rawDeploymentConfig, &actualDeploymentConfig)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentConfig, actualDeploymentConfig)

		// clear workload identity
		sourceDeployment.WorkloadIdentity = nil
	})

	t.Run("returns deployment config for the requested cloud standard deployment", func(t *testing.T) {
		sourceDeployment.Type = &standardType
		taskPodNodePoolID := "task_node_id"
		sourceDeployment.TaskPodNodePoolId = &taskPodNodePoolID
		cloudProvider := astroplatformcore.DeploymentCloudProviderAZURE
		sourceDeployment.CloudProvider = &cloudProvider
		var actualDeploymentConfig deploymentConfig
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedDeploymentConfig := deploymentConfig{
			Name:                 sourceDeployment.Name,
			Description:          *sourceDeployment.Description,
			WorkspaceName:        *sourceDeployment.WorkspaceName,
			RunTimeVersion:       sourceDeployment.RuntimeVersion,
			SchedulerAU:          *sourceDeployment.SchedulerAu,
			SchedulerCount:       sourceDeployment.SchedulerReplicas,
			DagDeployEnabled:     &sourceDeployment.IsDagDeployEnabled,
			SchedulerSize:        string(*sourceDeployment.SchedulerSize),
			Executor:             string(*sourceDeployment.Executor),
			Region:               *sourceDeployment.Region,
			DeploymentType:       string(*sourceDeployment.Type),
			CloudProvider:        string(*sourceDeployment.CloudProvider),
			DefaultTaskPodCPU:    *sourceDeployment.DefaultTaskPodCpu,
			DefaultTaskPodMemory: *sourceDeployment.DefaultTaskPodMemory,
			ResourceQuotaCPU:     *sourceDeployment.ResourceQuotaCpu,
			ResourceQuotaMemory:  *sourceDeployment.ResourceQuotaMemory,
			IsDevelopmentMode:    *sourceDeployment.IsDevelopmentMode,
		}
		rawDeploymentConfig, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		err = decodeToStruct(rawDeploymentConfig, &actualDeploymentConfig)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeploymentConfig, actualDeploymentConfig)
		sourceDeployment.Type = &hybridType
	})
}

func TestGetPrintableDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	sourceDeployment.Type = &hybridType
	taskPodNodePoolID := "task_node_id"
	sourceDeployment.TaskPodNodePoolId = &taskPodNodePoolID
	t.Run("returns a deployment map", func(t *testing.T) {
		info, _ := getDeploymentInfo(sourceDeployment)
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		expectedDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}
		actualDeployment := getPrintableDeployment(info, config, additional)
		assert.Equal(t, expectedDeployment, actualDeployment)
	})
}

func TestGetAdditionalNullableFields(t *testing.T) {
	sourceDeployment.Type = &hybridType
	sourceDeployment.TaskPodNodePoolId = nil
	t.Run("returns alert emails, queues and variables for the requested deployment with CeleryExecutor", func(t *testing.T) {
		var expectedAdditional, actualAdditional orderedPieces
		qList := []map[string]interface{}{
			{
				"name":               "default",
				"max_worker_count":   130,
				"min_worker_count":   12,
				"worker_concurrency": 110,
				"worker_type":        "test-instance-type",
			},
			{
				"name":               "test-queue-1",
				"max_worker_count":   175,
				"min_worker_count":   8,
				"worker_concurrency": 150,
				"worker_type":        "test-instance-type-1",
			},
		}
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		rawExpected := map[string]interface{}{
			"alert_emails":          sourceDeployment.ContactEmails,
			"worker_queues":         qList,
			"environment_variables": getVariablesMap(*sourceDeployment.EnvironmentVariables), // API only returns values when !EnvironmentVariablesObject.isSecret
			"hibernation_schedules": getHibernationSchedulesMap(*sourceDeployment.ScalingSpec.HibernationSpec.Schedules),
		}
		rawAdditional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		err := decodeToStruct(rawAdditional, &actualAdditional)
		assert.NoError(t, err)
		err = decodeToStruct(rawExpected, &expectedAdditional)
		assert.NoError(t, err)
		assert.Equal(t, expectedAdditional, actualAdditional)
	})
	t.Run("returns alert emails, queues and variables for the requested deployment with KubernetesExecutor", func(t *testing.T) {
		var expectedAdditional, actualAdditional orderedPieces
		sourceDeployment.Executor = &executorKubernetes
		qList := []map[string]interface{}{
			{
				"name":        "default",
				"pod_cpu":     "SmallCPU",
				"pod_ram":     "megsOfRam",
				"worker_type": "test-instance-type",
			},
			{
				"name":        "test-queue-1",
				"pod_cpu":     "LotsOfCPU",
				"pod_ram":     "gigsOfRam",
				"worker_type": "test-instance-type-1",
			},
		}
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		rawExpected := map[string]interface{}{
			"alert_emails":          sourceDeployment.ContactEmails,
			"worker_queues":         qList,
			"environment_variables": getVariablesMap(*sourceDeployment.EnvironmentVariables), // API only returns values when !EnvironmentVariablesObject.isSecret
			"hibernation_schedules": getHibernationSchedulesMap(*sourceDeployment.ScalingSpec.HibernationSpec.Schedules),
		}
		rawAdditional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		err := decodeToStruct(rawAdditional, &actualAdditional)
		assert.NoError(t, err)
		err = decodeToStruct(rawExpected, &expectedAdditional)
		assert.NoError(t, err)
		assert.Equal(t, expectedAdditional, actualAdditional)
	})
}

func TestFormatPrintableDeployment(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	var expectedPrintableDeployment []byte
	sourceDeployment.Type = &hybridType

	t.Run("returns a yaml formatted printable deployment", func(t *testing.T) {
		info, _ := getDeploymentInfo(sourceDeployment)
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)

		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}
		expectedDeployment := `deployment:
    environment_variables:
        - is_secret: false
          key: foo
          updated_at: NOW
          value: bar
        - is_secret: true
          key: bar
          updated_at: NOW+1
          value: baz
    configuration:
        name: test-deployment-label
        description: description
        runtime_version: 6.0.0
        scheduler_au: 5
        scheduler_count: 3
        cluster_id: cluster-id
    worker_queues:
        - name: default
          id: test-wq-id
          is_default: true
          max_worker_count: 130
          min_worker_count: 12
          worker_concurrency: 110
          node_pool_id: test-pool-id
        - name: test-queue-1
          id: test-wq-id-1
          is_default: false
          max_worker_count: 175
          min_worker_count: 8
          worker_concurrency: 150
          node_pool_id: test-pool-id-1
    metadata:
        deployment_id: test-deployment-id
        workspace_id: test-ws-id
        cluster_id: cluster-id
        release_name: great-release-name
        airflow_version: 2.4.0
        dag_deploy_enabled: true
		ci_cd_enforcement: true
        status: UNHEALTHY
        created_at: 2022-11-17T13:25:55.275697-08:00
        updated_at: 2022-11-17T13:25:55.275697-08:00
        deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
        webserver_url: some-url
        hibernation_override:
            is_hibernating: true
            override_until: 2022-11-17T13:25:55.275697-08:00
    alert_emails:
        - email1
        - email2
`
		var orderedAndTaggedDeployment, unorderedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("", false, printableDeployment)
		assert.NoError(t, err)
		// testing we get valid yaml
		err = yaml.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		assert.NoError(t, err)
		// update time and create time are not equal here so can not do equality check
		assert.NotEqual(t, expectedDeployment, string(actualPrintableDeployment), "tag and order should match")

		unordered, err := yaml.Marshal(printableDeployment)
		assert.NoError(t, err)
		err = yaml.Unmarshal(unordered, &unorderedDeployment)
		assert.NoError(t, err)
		// testing the structs are equal regardless of order
		assert.Equal(t, orderedAndTaggedDeployment, unorderedDeployment, "structs should match")
		// testing the order is not equal
		assert.NotEqual(t, string(unordered), string(actualPrintableDeployment), "order should not match")
	})
	t.Run("returns a yaml formatted template deployment", func(t *testing.T) {
		sourceDeployment2 := sourceDeployment
		sourceDeployment2.Type = &hybridType
		sourceDeployment2.Executor = &executorCelery
		description = "description"
		sourceDeployment2.Description = &description
		empty := ""
		sourceDeployment2.CloudProvider = nil
		sourceDeployment2.Region = &empty

		info, _ := getDeploymentInfo(sourceDeployment2)
		config, err := getDeploymentConfig(&sourceDeployment2, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment2, nodePools)

		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}
		expectedDeployment := `deployment:
    environment_variables:
        - is_secret: false
          key: foo
          updated_at: NOW
          value: bar
    configuration:
        name: ""
        description: description
        runtime_version: 6.0.0
        dag_deploy_enabled: true
        ci_cd_enforcement: false
        is_high_availability: false
        is_development_mode: true
        executor: CELERY
        scheduler_au: 5
        scheduler_count: 3
        cluster_name: test-cluster
        workspace_name: test-ws
        deployment_type: HYBRID
        cloud_provider: ""
        region: ""
        workload_identity: ""
    worker_queues:
        - name: default
          max_worker_count: 130
          min_worker_count: 12
          worker_concurrency: 110
          worker_type: test-instance-type
        - name: test-queue-1
          max_worker_count: 175
          min_worker_count: 8
          worker_concurrency: 150
          worker_type: test-instance-type-1
    alert_emails:
        - email1
        - email2
    hibernation_schedules:
        - hibernate_at: 1 * * * *
          wake_at: 2 * * * *
          description: hibernation schedule 1
          enabled: true
`
		var orderedAndTaggedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("", true, printableDeployment)
		assert.NoError(t, err)
		// testing we get valid yaml
		err = yaml.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeployment, string(actualPrintableDeployment), "tag and order should match")
	})

	t.Run("returns a yaml formatted standard deployment", func(t *testing.T) {
		sourceDeployment2 := sourceDeployment
		sourceDeployment2.Type = &standardType
		sourceDeployment2.Executor = &executorCelery
		description = "description"
		sourceDeployment2.Description = &description
		sourceDeployment2.ClusterId = nil
		one := "1"
		sourceDeployment2.DefaultTaskPodCpu = &one
		sourceDeployment2.DefaultTaskPodMemory = &one
		sourceDeployment2.ResourceQuotaCpu = &one
		sourceDeployment2.ResourceQuotaMemory = &one
		sourceDeployment2.WebServerUrl = "some-url"
		sourceDeployment2.UpdatedAt = time.Date(2023, time.February, 1, 12, 0, 0, 0, time.UTC)
		sourceDeployment2.CreatedAt = time.Date(2023, time.February, 1, 12, 0, 0, 0, time.UTC)
		provider := astroplatformcore.DeploymentCloudProviderAZURE
		sourceDeployment2.CloudProvider = &provider
		sourceDeployment2.ImageTag = "some-tag"
		sourceDeployment2.Status = "UNHEALTHY"
		overrideUntil := time.Date(2023, time.February, 1, 12, 0, 0, 0, time.UTC)
		sourceDeployment2.ScalingSpec.HibernationSpec.Override.OverrideUntil = &overrideUntil

		info, _ := getDeploymentInfo(sourceDeployment2)
		config, err := getDeploymentConfig(&sourceDeployment2, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment2, nodePools)

		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}
		expectedDeployment := `deployment:
    environment_variables:
        - is_secret: false
          key: foo
          updated_at: NOW
          value: bar
        - is_secret: true
          key: bar
          updated_at: NOW+1
          value: baz
    configuration:
        name: test-deployment-label
        description: description
        runtime_version: 6.0.0
        dag_deploy_enabled: true
        ci_cd_enforcement: false
        scheduler_size: SMALL
        is_high_availability: false
        is_development_mode: true
        executor: CELERY
        scheduler_au: 5
        scheduler_count: 3
        workspace_name: test-ws
        deployment_type: STANDARD
        cloud_provider: AZURE
        region: us-central1
        default_task_pod_cpu: "1"
        default_task_pod_memory: "1"
        resource_quota_cpu: "1"
        resource_quota_memory: "1"
        workload_identity: ""
    worker_queues:
        - name: default
          max_worker_count: 130
          min_worker_count: 12
          worker_concurrency: 110
          worker_type: a5
        - name: test-queue-1
          max_worker_count: 175
          min_worker_count: 8
          worker_concurrency: 150
          worker_type: a5
    metadata:
        deployment_id: test-deployment-id
        workspace_id: test-ws-id
        cluster_id: N/A
        release_name: N/A
        airflow_version: 2.4.0
        current_tag: some-tag
        status: UNHEALTHY
        created_at: 2023-02-01T12:00:00Z
        updated_at: 2023-02-01T12:00:00Z
        deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
        webserver_url: some-url
        airflow_api_url: some-url/api/v1
        hibernation_override:
            is_hibernating: true
            override_until: 2023-02-01T12:00:00Z
    alert_emails:
        - email1
        - email2
    hibernation_schedules:
        - hibernate_at: 1 * * * *
          wake_at: 2 * * * *
          description: hibernation schedule 1
          enabled: true
`
		var orderedAndTaggedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("", false, printableDeployment)
		assert.NoError(t, err)
		// testing we get valid yaml
		err = yaml.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeployment, string(actualPrintableDeployment), "tag and order should match")
	})
	t.Run("returns a json formatted printable deployment", func(t *testing.T) {
		sourceDeployment2 := sourceDeployment
		description = "description"
		sourceDeployment2.Description = &description
		empty := ""
		sourceDeployment2.CloudProvider = nil
		sourceDeployment2.Region = &empty

		info, _ := getDeploymentInfo(sourceDeployment2)
		config, err := getDeploymentConfig(&sourceDeployment2, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment2, nodePools)
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}
		expectedDeployment := `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            },
            {
                "is_secret": true,
                "key": "bar",
                "updated_at": "NOW+1",
                "value": "baz"
            }
        ],
        "configuration": {
            "name": "test-deployment-label",
            "description": "description",
            "runtime_version": "6.0.0",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_id": "cluster-id"
        },
        "worker_queues": [
            {
                "name": "default",
                "id": "test-wq-id",
                "is_default": true,
                "max_worker_count": 130,
                "min_worker_count": 12,
                "worker_concurrency": 110,
                "node_pool_id": "test-pool-id"
            },
            {
                "name": "test-queue-1",
                "id": "test-wq-id-1",
                "is_default": false,
                "max_worker_count": 175,
                "min_worker_count": 8,
                "worker_concurrency": 150,
                "node_pool_id": "test-pool-id-1"
            }
        ],
        "metadata": {
            "deployment_id": "test-deployment-id",
            "workspace_id": "test-ws-id",
            "cluster_id": "cluster-id",
            "release_name": "great-release-name",
            "airflow_version": "2.4.0",
            "dag_deploy_enabled": true,
            "ci_cd_enforcement": true,
            "status": "UNHEALTHY",
            "created_at": "2022-11-17T12:26:45.362983-08:00",
            "updated_at": "2022-11-17T12:26:45.362983-08:00",
            "deployment_url": "cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview",
            "webserver_url": "some-url",
            "airflow_api_url": "some-url/api/v1"
			"hibernation_override": {
				"is_hibernating": true,
				"override_until": "2022-11-17T12:26:45.362983-08:00"
			}
        },
        "alert_emails": [
            "email1",
            "email2"
        ]
    }
}`
		var orderedAndTaggedDeployment, unorderedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("json", false, printableDeployment)
		assert.NoError(t, err)
		// testing we get valid json
		err = json.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		assert.NoError(t, err)
		// update time and create time are not equal here so can not do equality check
		assert.NotEqual(t, expectedDeployment, string(actualPrintableDeployment), "tag and order should match")

		unordered, err := json.MarshalIndent(printableDeployment, "", "    ")
		assert.NoError(t, err)
		err = json.Unmarshal(unordered, &unorderedDeployment)
		assert.NoError(t, err)
		// testing the structs are equal regardless of order
		assert.Equal(t, orderedAndTaggedDeployment, unorderedDeployment, "structs should match")
		// testing the order is not equal
		assert.NotEqual(t, string(unordered), string(actualPrintableDeployment), "order should not match")
	})
	t.Run("returns a json formatted template deployment", func(t *testing.T) {
		sourceDeployment.Executor = &executorKubernetes
		sourceDeployment.WorkerQueues = nil
		empty := ""
		sourceDeployment.CloudProvider = nil
		sourceDeployment.Region = &empty

		info, _ := getDeploymentInfo(sourceDeployment)
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
				"hibernation_schedules": additional["hibernation_schedules"],
			},
		}

		expectedDeployment := `{
    "deployment": {
        "environment_variables": [
            {
                "is_secret": false,
                "key": "foo",
                "updated_at": "NOW",
                "value": "bar"
            }
        ],
        "configuration": {
            "name": "",
            "description": "description",
            "runtime_version": "6.0.0",
            "dag_deploy_enabled": true,
            "ci_cd_enforcement": false,
            "is_high_availability": false,
            "is_development_mode": true,
            "executor": "KUBERNETES",
            "scheduler_au": 5,
            "scheduler_count": 3,
            "cluster_name": "test-cluster",
            "workspace_name": "test-ws",
            "deployment_type": "HYBRID",
            "cloud_provider": "",
            "region": "",
            "workload_identity": ""
        },
        "worker_queues": [],
        "alert_emails": [
            "email1",
            "email2"
        ],
        "hibernation_schedules": [
            {
                "hibernate_at": "1 * * * *",
                "wake_at": "2 * * * *",
                "description": "hibernation schedule 1",
                "enabled": true
            }
        ]
    }
}`
		var orderedAndTaggedDeployment FormattedDeployment
		actualPrintableDeployment, err := formatPrintableDeployment("json", true, printableDeployment)
		assert.NoError(t, err)
		// testing we get valid json
		err = json.Unmarshal(actualPrintableDeployment, &orderedAndTaggedDeployment)
		assert.NoError(t, err)
		assert.Equal(t, expectedDeployment, string(actualPrintableDeployment), "tag and order should match")
	})
	t.Run("returns an error if decoding to struct fails", func(t *testing.T) {
		originalDecode := decodeToStruct
		decodeToStruct = errorReturningDecode
		defer restoreDecode(originalDecode)
		info, _ := getDeploymentInfo(sourceDeployment)
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("", false, getPrintableDeployment(info, config, additional))
		assert.ErrorIs(t, err, errMarshal)
		assert.Contains(t, string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
	t.Run("returns an error if marshaling yaml fails", func(t *testing.T) {
		originalMarshal := yamlMarshal
		yamlMarshal = errReturningYAMLMarshal
		defer restoreYAMLMarshal(originalMarshal)
		info, _ := getDeploymentInfo(sourceDeployment)
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("", false, getPrintableDeployment(info, config, additional))
		assert.ErrorIs(t, err, errMarshal)
		assert.Contains(t, string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
	t.Run("returns an error if marshaling json fails", func(t *testing.T) {
		originalMarshal := jsonMarshal
		jsonMarshal = errReturningJSONMarshal
		defer restoreJSONMarshal(originalMarshal)
		info, _ := getDeploymentInfo(sourceDeployment)
		config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
		assert.NoError(t, err)
		additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
		expectedPrintableDeployment = []byte{}
		actualPrintableDeployment, err := formatPrintableDeployment("json", false, getPrintableDeployment(info, config, additional))
		assert.ErrorIs(t, err, errMarshal)
		assert.Contains(t, string(actualPrintableDeployment), string(expectedPrintableDeployment))
	})
}

func TestGetSpecificField(t *testing.T) {
	sourceDeployment.Status = "UNHEALTHY"
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	info, _ := getDeploymentInfo(sourceDeployment)
	config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
	assert.NoError(t, err)
	additional := getAdditionalNullableFields(&sourceDeployment, nodePools)
	printableDeployment := map[string]interface{}{
		"deployment": map[string]interface{}{
			"metadata":              info,
			"configuration":         config,
			"alert_emails":          additional["alert_emails"],
			"worker_queues":         additional["worker_queues"],
			"environment_variables": additional["environment_variables"],
			"hibernation_schedules": additional["hibernation_schedules"],
		},
	}
	t.Run("returns a value if key is found in deployment.metadata", func(t *testing.T) {
		requestedField := "metadata.workspace_id"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, sourceDeployment.WorkspaceId, actual)
	})
	t.Run("returns a value if key is found in deployment.configuration", func(t *testing.T) {
		requestedField := "configuration.scheduler_count"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, sourceDeployment.SchedulerReplicas, actual)
	})
	t.Run("returns a value if key is alert_emails", func(t *testing.T) {
		requestedField := "alert_emails"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, sourceDeployment.ContactEmails, actual)
	})
	t.Run("returns a value if key is environment_variables", func(t *testing.T) {
		requestedField := "environment_variables"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, getVariablesMap(*sourceDeployment.EnvironmentVariables), actual)
	})
	t.Run("returns a value if key is worker_queues", func(t *testing.T) {
		requestedField := "worker_queues"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, getQMap(&sourceDeployment, nodePools), actual)
	})
	t.Run("returns a value if key is hibernation_schedules", func(t *testing.T) {
		requestedField := "hibernation_schedules"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, getHibernationSchedulesMap(*sourceDeployment.ScalingSpec.HibernationSpec.Schedules), actual)
	})
	t.Run("returns a value if key is metadata", func(t *testing.T) {
		requestedField := "metadata"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, info, actual)
	})
	t.Run("returns value regardless of upper or lower case key", func(t *testing.T) {
		requestedField := "Configuration.Cluster_NAME"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, *sourceDeployment.ClusterName, actual)
	})
	t.Run("returns correct boolean value", func(t *testing.T) {
		requestedField := "configuration.is_development_mode"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.NoError(t, err)
		assert.Equal(t, *sourceDeployment.IsDevelopmentMode, actual)
	})
	t.Run("returns error if no value is found", func(t *testing.T) {
		requestedField := "does-not-exist"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.ErrorContains(t, err, "requested key "+requestedField+" not found in deployment")
		assert.Equal(t, nil, actual)
	})
	t.Run("returns error if incorrect field is requested", func(t *testing.T) {
		requestedField := "configuration.does-not-exist"
		actual, err := getSpecificField(printableDeployment, requestedField)
		assert.ErrorIs(t, err, errKeyNotFound)
		assert.Equal(t, nil, actual)
	})
}

func TestGetWorkerTypeFromNodePoolID(t *testing.T) {
	var (
		expectedWorkerType, poolID, actualWorkerType string
		existingPools                                []astroplatformcore.NodePool
	)
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	expectedWorkerType = "worker-1"
	poolID = "test-pool-id"
	existingPools = []astroplatformcore.NodePool{
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-1",
		},
		{
			Id:               "test-pool-id",
			IsDefault:        false,
			NodeInstanceType: "worker-2",
		},
	}
	t.Run("returns a worker type from cluster for pool with matching nodepool id", func(t *testing.T) {
		actualWorkerType = getWorkerTypeFromNodePoolID(poolID, existingPools)
		assert.Equal(t, expectedWorkerType, actualWorkerType)
	})
	t.Run("returns an empty worker type if no pool with matching node pool id exists in the cluster", func(t *testing.T) {
		poolID = "test-pool-id-1"
		actualWorkerType = getWorkerTypeFromNodePoolID(poolID, existingPools)
		assert.Equal(t, "", actualWorkerType)
	})
}

func TestGetTemplate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	info, _ := getDeploymentInfo(sourceDeployment)
	config, err := getDeploymentConfig(&sourceDeployment, false, mockPlatformCoreClient)
	assert.NoError(t, err)
	additional := getAdditionalNullableFields(&sourceDeployment, nodePools)

	t.Run("returns a formatted template", func(t *testing.T) {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"alert_emails":          additional["alert_emails"],
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		assert.NoError(t, err)
		err = decodeToStruct(printableDeployment, &expected)
		assert.NoError(t, err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		expected.Deployment.EnvVars = newEnvVars
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = "NOW"
		}

		actual := getTemplate(&decoded)
		assert.Equal(t, expected, actual)
	})
	t.Run("returns a template without env vars if they are empty", func(t *testing.T) {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":      info,
				"configuration": config,
				"alert_emails":  additional["alert_emails"],
				"worker_queues": additional["worker_queues"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		assert.NoError(t, err)
		err = decodeToStruct(printableDeployment, &expected)
		assert.NoError(t, err)
		err = decodeToStruct(printableDeployment, &decoded)
		assert.NoError(t, err)
		err = decodeToStruct(printableDeployment, &expected)
		assert.NoError(t, err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		expected.Deployment.EnvVars = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = "NOW"
		}
		expected.Deployment.EnvVars = newEnvVars
		actual := getTemplate(&decoded)
		assert.Equal(t, expected, actual)
	})
	t.Run("returns a template without alert emails if they are empty", func(t *testing.T) {
		printableDeployment := map[string]interface{}{
			"deployment": map[string]interface{}{
				"metadata":              info,
				"configuration":         config,
				"worker_queues":         additional["worker_queues"],
				"environment_variables": additional["environment_variables"],
			},
		}
		var decoded, expected FormattedDeployment
		err := decodeToStruct(printableDeployment, &decoded)
		assert.NoError(t, err)
		err = decodeToStruct(printableDeployment, &expected)
		assert.NoError(t, err)
		expected.Deployment.Configuration.Name = ""
		expected.Deployment.Metadata = nil
		expected.Deployment.AlertEmails = nil
		newEnvVars := []EnvironmentVariable{}
		for i := range expected.Deployment.EnvVars {
			if !expected.Deployment.EnvVars[i].IsSecret {
				newEnvVars = append(newEnvVars, expected.Deployment.EnvVars[i])
			}
		}
		for i := range expected.Deployment.EnvVars {
			expected.Deployment.EnvVars[i].UpdatedAt = ""
		}
		expected.Deployment.EnvVars = newEnvVars
		actual := getTemplate(&decoded)
		assert.Equal(t, expected, actual)
	})
}

func TestReturnSpecifiedValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	workspaceID := "test-ws-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	deploymentName := "test-deployment-label"

	t.Run("run function successfully", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, nil).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		value, err := ReturnSpecifiedValue(workspaceID, "", deploymentID, mockPlatformCoreClient, mockCoreClient, "configuration.name")
		assert.NoError(t, err)
		assert.Contains(t, value, deploymentName)
	})
	t.Run("get deployment error", func(t *testing.T) {
		mockPlatformCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, errMarshal).Once()
		mockPlatformCoreClient.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&getDeploymentResponse, errMarshal).Once()
		mockPlatformCoreClient.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()

		_, err := ReturnSpecifiedValue(workspaceID, "", deploymentID, mockPlatformCoreClient, mockCoreClient, "configuration.name")
		assert.ErrorIs(t, err, errMarshal)
	})
}
