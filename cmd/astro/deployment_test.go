package astro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

var (
	mockTokenID              = "ck05r3bor07h40d02y2hw4n4t"
	csID                     = "test-cluster-id"
	testCluster              = "test-cluster"
	fixtureSchedulerAU       = 10
	fixtureSchedulerReplicas = 1
	mockListClustersResponse = astrov1.ListClustersResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.ClustersPaginated{
			Clusters: []astrov1.Cluster{
				{
					Id:        csID,
					Name:      testCluster,
					NodePools: &nodePools,
				},
				{
					Id:   "test-cluster-id-1",
					Name: "test-cluster-1",
				},
			},
		},
	}
	mockListDeploymentsCreateResponse = astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.DeploymentsPaginated{
			Deployments: mockCoreDeploymentCreateResponse,
		},
	}
	mockWorkloadIdentity             = "astro-great-release-name@provider-account.iam.gserviceaccount.com"
	mockCoreDeploymentCreateResponse = []astrov1.Deployment{
		{
			Name:                      "test-deployment-label",
			Status:                    "HEALTHY",
			EffectiveWorkloadIdentity: &mockWorkloadIdentity,
			ClusterId:                 &clusterID,
			ClusterName:               &testCluster,
			Id:                        "test-id-1",
		},
	}
	mockUpdateDeploymentResponse = astrov1.UpdateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Name:          "test-deployment-label",
			Id:            "test-id-1",
			CloudProvider: (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			ClusterId:     &clusterID,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	executorCelery       = astrov1.DeploymentExecutorCELERY
	highAvailabilityTest = true
	developmentModeTest  = true
	ResourceQuotaMemory  = "1"
	schedulerTestSize    = astrov1.DeploymentSchedulerSizeSMALL
	deploymentResponse   = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                        "test-id-1",
			RuntimeVersion:            "4.2.5",
			Namespace:                 "test-name",
			WorkspaceId:               "workspace-id",
			WebServerUrl:              "test-url",
			IsDagDeployEnabled:        false,
			Description:               &description,
			Name:                      "test-deployment-label",
			Status:                    "HEALTHY",
			Type:                      &hybridType,
			SchedulerAu:               &fixtureSchedulerAU,
			SchedulerReplicas:         fixtureSchedulerReplicas,
			ClusterId:                 &csID,
			ClusterName:               &testCluster,
			Executor:                  &executorCelery,
			IsHighAvailability:        &highAvailabilityTest,
			IsDevelopmentMode:         &developmentModeTest,
			ResourceQuotaCpu:          &resourceQuotaCPU,
			ResourceQuotaMemory:       &ResourceQuotaMemory,
			SchedulerSize:             &schedulerTestSize,
			Region:                    &region,
			WorkspaceName:             &workspaceName,
			CloudProvider:             (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:         &defaultTaskPodCPU,
			DefaultTaskPodMemory:      &defaultTaskPodMemory,
			WebServerAirflowApiUrl:    "airflow-url",
			EffectiveWorkloadIdentity: &mockWorkloadIdentity,
			WorkerQueues:              &[]astrov1.WorkerQueue{},
		},
	}
	hostedDeploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                     "test-id-1",
			RuntimeVersion:         "4.2.5",
			Namespace:              "test-name",
			WorkspaceId:            "workspace-id",
			WebServerUrl:           "test-url",
			IsDagDeployEnabled:     false,
			Description:            &description,
			Name:                   "test-deployment-label",
			Status:                 "HEALTHY",
			Type:                   &standardType,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			IsDevelopmentMode:      &developmentModeTest,
			ResourceQuotaCpu:       &resourceQuotaCPU,
			ResourceQuotaMemory:    &ResourceQuotaMemory,
			SchedulerSize:          &schedulerTestSize,
			Region:                 &region,
			WorkspaceName:          &workspaceName,
			CloudProvider:          (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			DefaultTaskPodCpu:      &defaultTaskPodCPU,
			DefaultTaskPodMemory:   &defaultTaskPodMemory,
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astrov1.WorkerQueue{},
		},
	}
	hostedDedicatedDeploymentResponse = astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
		JSON200: &astrov1.Deployment{
			Id:                     "test-id-1",
			RuntimeVersion:         "3.0-1",
			Namespace:              "test-name",
			WorkspaceId:            "workspace-id",
			WebServerUrl:           "test-url",
			IsDagDeployEnabled:     false,
			Description:            &description,
			Name:                   "test-deployment-label",
			Status:                 "HEALTHY",
			Type:                   &dedicatedType,
			ClusterId:              &csID,
			ClusterName:            &testCluster,
			Executor:               &executorCelery,
			IsHighAvailability:     &highAvailabilityTest,
			IsDevelopmentMode:      &developmentModeTest,
			SchedulerSize:          &schedulerTestSize,
			Region:                 &region,
			WorkspaceName:          &workspaceName,
			CloudProvider:          (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			WebServerAirflowApiUrl: "airflow-url",
			WorkerQueues:           &[]astrov1.WorkerQueue{},
			RemoteExecution: &astrov1.DeploymentRemoteExecution{
				Enabled: true,
			},
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
	standardType               = astrov1.DeploymentTypeSTANDARD
	dedicatedType              = astrov1.DeploymentTypeDEDICATED
	hybridType                 = astrov1.DeploymentTypeHYBRID
	testRegion                 = "region"
	testProvider               = "provider"
	mockCoreDeploymentResponse = []astrov1.Deployment{
		{
			Id:                "test-id-1",
			Name:              "test",
			Status:            "HEALTHY",
			Type:              &standardType,
			Region:            &testRegion,
			CloudProvider:     (*astrov1.DeploymentCloudProvider)(&testProvider),
			WorkspaceName:     &workspaceName,
			IsDevelopmentMode: &developmentModeTest,
		},
		{
			Id:            "test-id-2",
			Name:          "test-2",
			Status:        "HEALTHY",
			Type:          &hybridType,
			ClusterName:   &testCluster,
			WorkspaceName: &workspaceName,
		},
	}
	mockGetDeploymentLogsResponse = astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
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
	mockGetDeploymentLogsMultipleComponentsResponse = astrov1.GetDeploymentLogsResponse{
		JSON200: &astrov1.DeploymentLog{
			Limit:         logCount,
			MaxNumResults: 10,
			Offset:        0,
			ResultCount:   1,
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
			},
			SearchId: "search-id",
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	GetDeploymentOptionsResponseAlphaOK = astrov1.GetDeploymentOptionsResponse{
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
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
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
			WorkerQueues: astrov1.WorkerQueueOptions{
				MaxWorkers: astrov1.Range{
					Ceiling: 200,
					Default: 20,
					Floor:   0,
				},
				MinWorkers: astrov1.Range{
					Ceiling: 20,
					Default: 5,
					Floor:   0,
				},
				WorkerConcurrency: astrov1.Range{
					Ceiling: 200,
					Default: 100,
					Floor:   0,
				},
			},
			WorkerMachines: []astrov1.WorkerMachine{
				{
					Name: "a5",
					Concurrency: astrov1.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
				{
					Name: "a20",
					Concurrency: astrov1.Range{
						Ceiling: 10,
						Default: 5,
						Floor:   1,
					},
				},
			},
			Executors: []string{},
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
	mockCreateDeploymentResponse = astrov1.CreateDeploymentResponse{
		JSON200: &astrov1.Deployment{
			Id:            "test-id",
			CloudProvider: (*astrov1.DeploymentCloudProvider)(&cloudProvider),
			Type:          &hybridType,
			Region:        &region,
			ClusterName:   &cluster.Name,
		},
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}
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
)

func execDeploymentCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(buf)
	cmd.SetOut(buf)
	// The real CLI registers --verbosity as a persistent flag on the root command.
	// Tests invoke the deployment subcommand directly, so we stub the flag here to
	// mirror production setup and avoid "unknown flag" errors when scenarios pass it.
	var verbosity string
	cmd.PersistentFlags().StringVar(&verbosity, "verbosity", "", "")
	cmd.SetArgs(args)
	testUtil.SetupOSArgsForGinkgo()
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newDeploymentRootCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	testUtil.SetupOSArgsForGinkgo()
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "deployment")
}

func TestDeploymentList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "-a"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentListJSON(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
	astroV1Client = mockV1Client

	cmdArgs := []string{"list", "-a", "-o", "json"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	var result deployment.DeploymentList
	assert.NoError(t, json.Unmarshal([]byte(resp), &result))
	assert.Len(t, result.Deployments, 2)
	mockV1Client.AssertExpectations(t)
}

// TestDeploymentListJSONKeysAreSnakeCase pins the keys `deployment list -o
// json` publishes. Decoding into deployment.DeploymentList cannot catch a key
// rename, since the struct's own tags decode it; reading the raw object does.
// The 1.x camelCase keys (deploymentId, isDagDeployEnabled, ...) became these
// in 2.0, with the names `deployment inspect` uses for the same facts.
func TestDeploymentListJSONKeysAreSnakeCase(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// Its own fixture: the shared mock's slice is sorted in place by ListData
	// and its rows are reused by other tests, so under -shuffle their shape
	// depends on what ran first.
	standard, hybrid := astrov1.DeploymentTypeSTANDARD, astrov1.DeploymentTypeHYBRID
	region, provider, cluster, ws := "us-east-1", astrov1.DeploymentCloudProvider("AWS"), "test-cluster", "ws-name"
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: 200},
		JSON200: &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{
			{Id: "dep-standard", Name: "b", Type: &standard, Region: &region, CloudProvider: &provider, WorkspaceName: &ws},
			{Id: "dep-hybrid", Name: "a", Type: &hybrid, ClusterName: &cluster, WorkspaceName: &ws},
		}},
	}, nil).Once()
	astroV1Client = mockV1Client

	resp, err := execDeploymentCmd("list", "-a", "-o", "json")
	require.NoError(t, err)

	var got struct {
		Deployments []map[string]any `json:"deployments"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp), &got))
	require.Len(t, got.Deployments, 2) // sorted by name, descending: b, then a

	always := []string{
		"name", "workspace_name", "namespace", "deployment_id", "runtime_version", "airflow_version",
		"dag_deploy_enabled", "ci_cd_enforcement", "type", "is_remote_execution_enabled",
	}
	keys := func(m map[string]any) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	// The standard deployment has a region and provider but no cluster; the
	// hybrid one the reverse. Those three are omitempty.
	assert.ElementsMatch(t, append(append([]string{}, always...), "cloud_provider", "region"), keys(got.Deployments[0]))
	assert.ElementsMatch(t, append(append([]string{}, always...), "cluster_name"), keys(got.Deployments[1]))
	assert.Equal(t, "dep-standard", got.Deployments[0]["deployment_id"])
	assert.Equal(t, "AWS", got.Deployments[0]["cloud_provider"])
	assert.Equal(t, "ws-name", got.Deployments[0]["workspace_name"])
	assert.Equal(t, "dep-hybrid", got.Deployments[1]["deployment_id"])
	assert.Equal(t, "test-cluster", got.Deployments[1]["cluster_name"])
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentListRejectsRemovedOutputDialect(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	// No client expectations: each is rejected before any API call.
	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client

	_, err := execDeploymentCmd("list", "-o", "template")
	assert.EqualError(t, err, `unknown output format "template" (supported: text, json)`)
	// A bad --output is a usage error here as everywhere: exit 2.
	assert.True(t, cliout.IsUsage(err), "%v is not a usage error", err)

	_, err = execDeploymentCmd("list", "-o", "table")
	assert.EqualError(t, err, `unknown output format "table" (supported: text, json)`)
	assert.True(t, cliout.IsUsage(err), "%v is not a usage error", err)

	_, err = execDeploymentCmd("list", "--template", "{{.}}")
	assert.ErrorContains(t, err, "unknown flag: --template")

	mockV1Client.AssertExpectations(t)
}

// 1.x's --json is tombstoned, not aliased: alone or beside -o, it is a usage
// error naming -o json, refused before any API call, and hidden from help.
func TestDeploymentListJSONFlagIsTombstoned(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	for _, args := range [][]string{
		{"list", "-a", "--json"},
		{"list", "-a", "--json", "-o", "json"},
		{"list", "--json", "-o", "text"},
	} {
		// No call is mocked: a refused run asks the API nothing.
		mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
		astroV1Client = mockV1Client

		resp, err := execDeploymentCmd(args...)
		require.EqualError(t, err, cliout.ErrJSONFlagRemoved, args)
		assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err), args)
		assert.NotContains(t, resp, `"deployments"`, args)
		mockV1Client.AssertExpectations(t)
	}

	cmd := newDeploymentListCmd(io.Discard)
	assert.True(t, cmd.Flags().Lookup("json").Hidden, "help shows --output, not --json")
}

func TestDeploymentLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsResponse, nil).Times(3)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	cmdArgs := []string{"logs", "test-id-1", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentLogsMultipleComponents(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(3)
	mockV1Client.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockGetDeploymentLogsMultipleComponentsResponse, nil).Times(3)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	cmdArgs := []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-w"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-e"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)

	cmdArgs = []string{"logs", "test-id-1", "--webserver", "--scheduler", "--workers", "--triggerer", "-i"}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

// newCreateUpdateMock gives a subtest its own mock client, so one subtest's
// unmet or leftover expectations cannot pass or fail another's. It also
// resets the command state a previous run leaves behind: the remote-execution
// values derived from flags, and --type, which outside a hosted organization
// is not registered and so not reset by building the command.
func newCreateUpdateMock(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = m
	allowedIPAddressRanges, taskLogBucket, taskLogURLPattern = nil, nil, nil
	deploymentType = standard
	return m
}

// setHostedOrg makes the current context a hosted organization's, which
// registers the hosted-only create and update flags.
func setHostedOrg(t *testing.T, ws string) {
	t.Helper()
	ctx, err := context.GetCurrentContext()
	require.NoError(t, err)
	for k, v := range map[string]string{
		"organization_product":    "HOSTED",
		"organization":            "test-org-id",
		"organization_short_name": "test-org",
		"workspace":               ws,
	} {
		require.NoError(t, ctx.SetContextKey(k, v))
	}
}

// createRequest matches a CreateDeployment request whose variant, decoded by
// as, satisfies check.
func createRequest[T any](as func(astrov1.CreateDeploymentRequest) (T, error), check func(T) bool) any {
	return mock.MatchedBy(func(r astrov1.CreateDeploymentRequest) bool {
		v, err := as(r)
		return err == nil && check(v)
	})
}

// updateRequest is createRequest for an UpdateDeployment request.
func updateRequest[T any](as func(astrov1.UpdateDeploymentRequest) (T, error), check func(T) bool) any {
	return mock.MatchedBy(func(r astrov1.UpdateDeploymentRequest) bool {
		v, err := as(r)
		return err == nil && check(v)
	})
}

// hybridCreate matches a hybrid CreateDeployment request that satisfies check.
func hybridCreate(check func(astrov1.CreateHybridDeploymentRequest) bool) any {
	return createRequest(astrov1.CreateDeploymentRequest.AsCreateHybridDeploymentRequest, func(r astrov1.CreateHybridDeploymentRequest) bool {
		return r.Type != nil && *r.Type == astrov1.CreateHybridDeploymentRequestTypeHYBRID && check(r)
	})
}

func TestDeploymentCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	ws := "workspace-id"
	mockResponse := &airflowversions.Response{
		RuntimeVersions: map[string]airflowversions.RuntimeVersion{
			"4.2.5": {Metadata: airflowversions.RuntimeVersionMetadata{AirflowVersion: "2.2.5", Channel: "stable"}, Migrations: airflowversions.RuntimeVersionMigrations{}},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	require.NoError(t, err)

	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})

	// A hybrid create: options, the workspace, the cluster, then the create.
	expectHybridCreate := func(m *astrov1_mocks.ClientWithResponsesInterface, request any) {
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		m.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		m.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, request).Return(&mockCreateDeploymentResponse, nil).Once()
	}

	t.Run("creates a deployment when dag-deploy is disabled", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridCreate(m, hybridCreate(func(r astrov1.CreateHybridDeploymentRequest) bool {
			return r.Name == "test" && r.ClusterId != nil && *r.ClusterId == csID && r.IsDagDeployEnabled != nil && !*r.IsDagDeployEnabled
		}))
		_, err := execDeploymentCmd("create", "--name", "test", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("creates a deployment when dag deploy is enabled", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridCreate(m, hybridCreate(func(r astrov1.CreateHybridDeploymentRequest) bool {
			return r.IsDagDeployEnabled != nil && *r.IsDagDeployEnabled
		}))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "enable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("creates a deployment when executor is specified", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridCreate(m, hybridCreate(func(r astrov1.CreateHybridDeploymentRequest) bool {
			return r.Executor != nil && *r.Executor == astrov1.CreateHybridDeploymentRequestExecutorKUBERNETES
		}))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubernetesExecutor")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("creates a deployment with default executor", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridCreate(m, hybridCreate(func(r astrov1.CreateHybridDeploymentRequest) bool {
			return r.Executor != nil && *r.Executor == astrov1.CreateHybridDeploymentRequestExecutorCELERY
		}))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("creates a deployment with ci-cd enforcement", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridCreate(m, hybridCreate(func(r astrov1.CreateHybridDeploymentRequest) bool {
			return r.IsCicdEnforced != nil && *r.IsCicdEnforced
		}))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--cicd-enforcement", "enable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("returns an error if dag-deploy flag has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "some-value")
		assert.ErrorContains(t, err, "Invalid --dag-deploy value")
	})
	t.Run("returns an error if cicd-enforcement flag has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--cicd-enforcement", "some-value")
		assert.ErrorContains(t, err, "Invalid --cicd-enforcement value")
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubeExecutor")
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("returns an error if remote-execution-enabled flag is set but org is not hosted", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--remote-execution-enabled")
		assert.ErrorContains(t, err, "unknown flag: --remote-execution-enabled")
	})

	// A standard create: options, the workspace, then the create. Each case
	// gives --region, so none is selected.
	expectStandardCreate := func(m *astrov1_mocks.ClientWithResponsesInterface, check func(astrov1.CreateStandardDeploymentRequest) bool) {
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		m.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, createRequest(astrov1.CreateDeploymentRequest.AsCreateStandardDeploymentRequest, func(r astrov1.CreateStandardDeploymentRequest) bool {
			return r.Type != nil && *r.Type == astrov1.CreateStandardDeploymentRequestTypeSTANDARD && check(r)
		})).Return(&mockCreateDeploymentResponse, nil).Once()
	}
	gcpIn := func(region string) func(astrov1.CreateStandardDeploymentRequest) bool {
		return func(r astrov1.CreateStandardDeploymentRequest) bool {
			return r.CloudProvider != nil && *r.CloudProvider == astrov1.CreateStandardDeploymentRequestCloudProviderGCP &&
				r.Region != nil && *r.Region == region
		}
	}

	t.Run("creates a deployment with cloud provider and region", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectStandardCreate(m, gcpIn("us-central1"))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable", "--cloud-provider", "gcp", "--region", "us-central1")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	// The provider is validated case-insensitively, so an upper-case one has
	// to reach the request too rather than become an empty provider.
	t.Run("creates a deployment with an upper-case cloud provider", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectStandardCreate(m, gcpIn("us-central1"))
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable", "--cloud-provider", "GCP", "--region", "us-central1")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "gcp", "--region", "us-east1", "--high-availability", "some-value")
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	t.Run("returns an error with incorrect development-mode value", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "gcp", "--region", "us-east1", "--development-mode", "some-value")
		assert.ErrorContains(t, err, "Invalid --development-mode value")
	})
	t.Run("returns an error if cloud provider is not valid", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "ibm")
		assert.ErrorContains(t, err, "ibm is not a valid cloud provider. It can only be gcp")
	})
	t.Run("returns an error if cluster-id is provided with implicit standard deployment", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID)
		assert.ErrorContains(t, err, "flag --cluster-id cannot be used to create a standard deployment")
	})
	t.Run("returns an error if cluster-id is provided with explicit standard deployment", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--type", standard)
		assert.ErrorContains(t, err, "flag --cluster-id cannot be used to create a standard deployment")
	})
	t.Run("returns an error if remote execution settings are given without remote execution", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--task-log-bucket", "test-bucket")
		assert.ErrorContains(t, err, "flag --task-log-bucket cannot be used when remote execution is disabled")
	})

	// A dedicated create with no --cluster-id: options, the workspace, then a
	// cluster picked from the list.
	expectDedicatedCreate := func(m *astrov1_mocks.ClientWithResponsesInterface, check func(astrov1.CreateDedicatedDeploymentRequest) bool) {
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		m.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		m.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, createRequest(astrov1.CreateDeploymentRequest.AsCreateDedicatedDeploymentRequest, func(r astrov1.CreateDedicatedDeploymentRequest) bool {
			return r.Type != nil && *r.Type == astrov1.CreateDedicatedDeploymentRequestTypeDEDICATED &&
				r.ClusterId != nil && *r.ClusterId == csID && check(r)
		})).Return(&mockCreateDeploymentResponse, nil).Once()
	}

	t.Run("creates a hosted dedicated deployment", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectDedicatedCreate(m, func(r astrov1.CreateDedicatedDeploymentRequest) bool {
			re := r.RemoteExecution
			return re != nil && re.Enabled &&
				re.AllowedIpAddressRanges != nil && len(*re.AllowedIpAddressRanges) == 1 && (*re.AllowedIpAddressRanges)[0] == "0.0.0.0/0" &&
				re.TaskLogBucket != nil && *re.TaskLogBucket == "test-bucket" &&
				re.TaskLogUrlPattern != nil && *re.TaskLogUrlPattern == "test-url-pattern" &&
				// Remote execution leaves DAG-only deploys off by default.
				r.IsDagDeployEnabled != nil && !*r.IsDagDeployEnabled
		})
		defer testUtil.MockUserInput(t, "1")() // the first cluster
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--remote-execution-enabled",
			"--allowed-ip-address-ranges", "0.0.0.0/0", "--task-log-bucket", "test-bucket", "--task-log-url-pattern", "test-url-pattern")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("returns an error if incorrect cluster type is passed for a hosted dedicated deployment", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "wrong-value")
		assert.ErrorContains(t, err, "Invalid --type value")
	})
	t.Run("creates an extra large deployment", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectDedicatedCreate(m, func(r astrov1.CreateDedicatedDeploymentRequest) bool {
			return r.SchedulerSize != nil && *r.SchedulerSize == astrov1.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
		})
		defer testUtil.MockUserInput(t, "1")() // the first cluster
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--scheduler-size", "extra_large")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("takes a scheduler size in any case", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectStandardCreate(m, func(r astrov1.CreateStandardDeploymentRequest) bool {
			return r.SchedulerSize != nil && *r.SchedulerSize == astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE
		})
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--region", "us-central1", "--scheduler-size", "EXTRA_Large")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	// A size create does not know was dropped from the request, and the
	// create succeeded with whatever size the API chose.
	t.Run("refuses an unknown scheduler size before any request", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t) // no expectations: any API call fails the test
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "dedicated", "--scheduler-size", "extra-large")
		require.Error(t, err)
		assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err), "%v", err)
		assert.ErrorContains(t, err, `"extra-large"`)
		assert.ErrorContains(t, err, "small, medium, large, extra_large")
		m.AssertExpectations(t)
	})
	t.Run("creates a hosted deployment with workload identity", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		workloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		expectStandardCreate(m, func(r astrov1.CreateStandardDeploymentRequest) bool {
			return r.WorkloadIdentity != nil && *r.WorkloadIdentity == workloadIdentity &&
				r.CloudProvider != nil && *r.CloudProvider == astrov1.CreateStandardDeploymentRequestCloudProviderAWS
		})
		_, err := execDeploymentCmd("create", "--name", "test-name", "--workspace-id", ws, "--type", "standard", "--workload-identity", workloadIdentity, "--cloud-provider", "aws", "--region", "us-west-2")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
}

func TestDeploymentUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	ws := "test-ws-id"

	// An update by id: the Deployment is listed and fetched, then options,
	// then the update.
	expectUpdate := func(m *astrov1_mocks.ClientWithResponsesInterface, current *astrov1.GetDeploymentResponse, request any) {
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(current, nil).Once()
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
		m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, request).Return(&mockUpdateDeploymentResponse, nil).Once()
	}
	// A hybrid Deployment's update also reads its cluster.
	expectHybridUpdate := func(m *astrov1_mocks.ClientWithResponsesInterface, current *astrov1.GetDeploymentResponse, check func(astrov1.UpdateHybridDeploymentRequest) bool) {
		expectUpdate(m, current, updateRequest(astrov1.UpdateDeploymentRequest.AsUpdateHybridDeploymentRequest, check))
		m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Once()
	}
	standardUpdate := func(check func(astrov1.UpdateStandardDeploymentRequest) bool) any {
		return updateRequest(astrov1.UpdateDeploymentRequest.AsUpdateStandardDeploymentRequest, check)
	}

	t.Run("updates the deployment successfully", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridUpdate(m, &deploymentResponse, func(r astrov1.UpdateHybridDeploymentRequest) bool { return r.Name == "test" })
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test", "--workspace-id", ws, "--yes")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to enable ci-cd enforcement", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridUpdate(m, &deploymentResponse, func(r astrov1.UpdateHybridDeploymentRequest) bool { return r.IsCicdEnforced })
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--yes", "--cicd-enforcement", "enable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("updates the deployment successfully to disable ci-cd enforcement", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		enforced := *deploymentResponse.JSON200
		enforced.IsCicdEnforced = true
		current := deploymentResponse
		current.JSON200 = &enforced
		expectHybridUpdate(m, &current, func(r astrov1.UpdateHybridDeploymentRequest) bool { return !r.IsCicdEnforced })
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--yes", "--cicd-enforcement", "disable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	// Changing DAG deploys under CI/CD enforcement asks first, since the CLI
	// cannot deploy the DAGs afterwards. Declining leaves the Deployment as it
	// was; --yes answers the question.
	t.Run("declining the ci-cd enforcement question updates nothing", func(t *testing.T) {
		// A user's token, which carries no deploy permissions. The check
		// strips "Bearer " by position, so the token needs it.
		ctx, err := context.GetCurrentContext()
		require.NoError(t, err)
		require.NoError(t, ctx.SetContextKey("token", "Bearer token"))
		t.Cleanup(func() { testUtil.InitTestConfig(testUtil.LocalPlatform) })
		m := newCreateUpdateMock(t)
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Once()
		defer testUtil.MockUserInput(t, "n")()
		_, err = execDeploymentCmd("update", "test-id-1", "--workspace-id", ws, "--cicd-enforcement", "enable", "--dag-deploy", "enable")
		assert.NoError(t, err)
		m.AssertExpectations(t)
		m.AssertNotCalled(t, "UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
	t.Run("--yes answers the ci-cd enforcement question", func(t *testing.T) {
		m := newCreateUpdateMock(t)
		expectHybridUpdate(m, &deploymentResponse, func(r astrov1.UpdateHybridDeploymentRequest) bool {
			return r.IsCicdEnforced && r.IsDagDeployEnabled
		})
		_, err := execDeploymentCmd("update", "test-id-1", "--workspace-id", ws, "--cicd-enforcement", "enable", "--dag-deploy", "enable", "--yes")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("returns an error if ci-cd enforcement has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("update", "test-id", "--name", "test-name", "--workspace-id", ws, "--yes", "--cicd-enforcement", "some-value")
		assert.ErrorContains(t, err, "Invalid --cicd-enforcement value")
	})
	t.Run("returns an error if dag-deploy has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("update", "test-id", "--name", "test-name", "--workspace-id", ws, "--yes", "--dag-deploy", "some-value")
		assert.ErrorContains(t, err, "Invalid --dag-deploy value")
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("update", "test-id", "--name", "test-name", "--workspace-id", ws, "--yes", "--executor", "KubeExecutor")
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		newCreateUpdateMock(t)
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx, err := config.GetCurrentContext()
		require.NoError(t, err)
		ctx.Workspace = ""
		require.NoError(t, ctx.SetContext())
		defer testUtil.InitTestConfig(testUtil.LocalPlatform)
		resp, err := execDeploymentCmd("update", "-n", "doesnotexist")
		assert.ErrorContains(t, err, "failed to find a valid Workspace")
		assert.Contains(t, resp, "Usage:\n")
	})
	t.Run("updates a deployment with small scheduler size", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectUpdate(m, &hostedDeploymentResponse, standardUpdate(func(r astrov1.UpdateStandardDeploymentRequest) bool {
			return r.SchedulerSize == astrov1.UpdateStandardDeploymentRequestSchedulerSizeSMALL
		}))
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "small", "--yes")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("update", "test-id", "--name", "test-name", "--workspace-id", ws, "--high-availability", "some-value", "--yes")
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	t.Run("returns an error with incorrect development-mode value", func(t *testing.T) {
		setHostedOrg(t, ws)
		newCreateUpdateMock(t)
		_, err := execDeploymentCmd("update", "test-id", "--name", "test-name", "--workspace-id", ws, "--development-mode", "some-value", "--yes")
		assert.ErrorContains(t, err, "Invalid --development-mode value")
	})
	t.Run("updates a deployment with extra large scheduler size", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectUpdate(m, &hostedDeploymentResponse, standardUpdate(func(r astrov1.UpdateStandardDeploymentRequest) bool {
			return r.SchedulerSize == astrov1.UpdateStandardDeploymentRequestSchedulerSizeEXTRALARGE
		}))
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "extra_large", "--yes")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("takes a scheduler size in any case", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		expectUpdate(m, &hostedDeploymentResponse, standardUpdate(func(r astrov1.UpdateStandardDeploymentRequest) bool {
			return r.SchedulerSize == astrov1.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
		}))
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "Medium", "--yes")
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	// Update sent an empty size for one it did not know.
	t.Run("refuses an unknown scheduler size before any request", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t) // no expectations: any API call fails the test
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "extra-large", "--yes")
		require.Error(t, err)
		assert.Equal(t, cliout.ExitUsage, cliout.ExitCode(t.Context(), err), "%v", err)
		assert.ErrorContains(t, err, `"extra-large"`)
		assert.ErrorContains(t, err, "small, medium, large, extra_large")
		m.AssertExpectations(t)
	})
	t.Run("updates a hosted deployment with workload identity", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		workloadIdentity := "arn:aws:iam::1234567890:role/unit-test-1"
		expectUpdate(m, &hostedDeploymentResponse, standardUpdate(func(r astrov1.UpdateStandardDeploymentRequest) bool {
			return r.WorkloadIdentity != nil && *r.WorkloadIdentity == workloadIdentity
		}))
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--workload-identity", workloadIdentity)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
	t.Run("updates a hosted dedicated deployment with remote execution config", func(t *testing.T) {
		setHostedOrg(t, ws)
		m := newCreateUpdateMock(t)
		taskLogBucket := "test-bucket"
		taskLogURLPattern := "test-url-pattern"
		allowedIPAddressRange := "1.2.3.4/32"
		expectUpdate(m, &hostedDedicatedDeploymentResponse, updateRequest(astrov1.UpdateDeploymentRequest.AsUpdateDedicatedDeploymentRequest, func(r astrov1.UpdateDedicatedDeploymentRequest) bool {
			re := r.RemoteExecution
			return re != nil && re.Enabled &&
				re.AllowedIpAddressRanges != nil && len(*re.AllowedIpAddressRanges) == 1 && (*re.AllowedIpAddressRanges)[0] == allowedIPAddressRange &&
				re.TaskLogBucket != nil && *re.TaskLogBucket == taskLogBucket &&
				re.TaskLogUrlPattern != nil && *re.TaskLogUrlPattern == taskLogURLPattern
		}))
		_, err := execDeploymentCmd("update", "test-id-1", "--name", "test-name", "--allowed-ip-address-ranges", allowedIPAddressRange,
			"--task-log-bucket", taskLogBucket, "--task-log-url-pattern", taskLogURLPattern)
		assert.NoError(t, err)
		m.AssertExpectations(t)
	})
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	mockDeleteDeploymentResponse := astrov1.DeleteDeploymentResponse{
		HTTPResponse: &http.Response{
			StatusCode: 200,
		},
	}

	astroV1Client = mockV1Client

	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockDeleteDeploymentResponse, nil).Times(1)

	cmdArgs := []string{"delete", "test-id-1", "--yes"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)

	astroV1Client = mockV1Client

	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(1)
	value := "test-value-1"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
	}

	cmdArgs := []string{"variable", "list", "--deployment-id", "test-id-1"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client
	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &value2,
			IsSecret: false,
		},
	}

	mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "create", "test-key-3=test-value-3", "--deployment-id", "test-id-1", "--key", "test-key-2", "--value", "test-value-2"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2")
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentVariableUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
	astroV1Client = mockV1Client
	astroV1Client = mockV1Client

	value := "test-value-1"
	value2 := "test-value-2"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &value,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &value2,
			IsSecret: false,
		},
	}

	mockV1Client.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Times(1)
	mockV1Client.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockUpdateDeploymentResponse, nil).Times(1)
	mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Times(2)
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(2)
	valueUpdate := "test-value-update"
	valueUpdate2 := "test-value-2-update"
	deploymentResponse.JSON200.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{
			Key:      "test-key-1",
			Value:    &valueUpdate,
			IsSecret: false,
		},
		{
			Key:      "test-key-2",
			Value:    &valueUpdate2,
			IsSecret: false,
		},
	}
	mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&deploymentResponse, nil).Times(1)
	mockV1Client.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Times(1)

	cmdArgs := []string{"variable", "update", "test-key-2=test-value-2-update", "--deployment-id", "test-id-1", "--key", "test-key-1", "--value", "test-value-update"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-update")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2-update")
	mockV1Client.AssertExpectations(t)
	mockV1Client.AssertExpectations(t)
}

func TestDeploymentHibernateAndWakeUp(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	tests := []struct {
		IsHibernating bool
		command       string
	}{
		{true, "hibernate"},
		{false, "wake-up"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

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

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			defer testUtil.MockUserInput(t, "1")()

			cmdArgs := []string{tt.command, "", "--yes"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			until := "2022-11-17T13:25:55.275697-08:00"
			untilParsed, err := time.Parse(time.RFC3339, until)
			assert.NoError(t, err)
			isActive := true
			mockResponse := astrov1.UpdateDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusOK,
				},
				JSON200: &astrov1.DeploymentHibernationOverride{
					IsHibernating: &tt.IsHibernating,
					OverrideUntil: &untilParsed,
					IsActive:      &isActive,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--yes"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with until returns an error if invalid", tt.command), func(t *testing.T) {
			until := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--until", until, "--yes"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s with for", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			forDuration := "1h"
			forDurationParsed, err := time.ParseDuration(forDuration)
			overrideUntil := time.Now().Add(forDurationParsed)
			assert.NoError(t, err)
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

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--yes"}
			_, err = execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s with for returns an error if invalid", tt.command), func(t *testing.T) {
			forDuration := "invalid-duration"

			cmdArgs := []string{tt.command, "test-id-1", "--for", forDuration, "--yes"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.Error(t, err)
		})

		t.Run(fmt.Sprintf("%s refuses --wait-time without --wait", tt.command), func(t *testing.T) {
			_, err := execDeploymentCmd(tt.command, "test-id-1", "--wait-time", "1m", "--yes")
			assert.ErrorContains(t, err, "cannot use --wait-time with --wait=false")
		})

		t.Run(fmt.Sprintf("%s refuses --wait with --remove-override", tt.command), func(t *testing.T) {
			_, err := execDeploymentCmd(tt.command, "test-id-1", "--wait", "--remove-override", "--yes")
			assert.ErrorContains(t, err, "none of the others can be")
		})

		t.Run(fmt.Sprintf("%s with remove override", tt.command), func(t *testing.T) {
			mockV1Client := new(astrov1_mocks.ClientWithResponsesInterface)
			astroV1Client = mockV1Client

			mockResponse := astrov1.DeleteDeploymentHibernationOverrideResponse{
				HTTPResponse: &http.Response{
					StatusCode: http.StatusNoContent,
				},
			}

			mockV1Client.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListDeploymentsResponse, nil).Once()
			mockV1Client.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&hostedDeploymentResponse, nil).Once()
			mockV1Client.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&mockResponse, nil).Once()

			cmdArgs := []string{tt.command, "test-id-1", "--remove-override", "--yes"}
			_, err := execDeploymentCmd(cmdArgs...)
			assert.NoError(t, err)
			mockV1Client.AssertExpectations(t)
		})

		t.Run(fmt.Sprintf("%s returns an error when getting workspace fails", tt.command), func(t *testing.T) {
			testUtil.InitTestConfig(testUtil.LocalPlatform)
			ctx, err := config.GetCurrentContext()
			assert.NoError(t, err)
			ctx.Workspace = ""
			err = ctx.SetContext()
			assert.NoError(t, err)
			defer testUtil.InitTestConfig(testUtil.LocalPlatform)
			expectedOut := "Usage:\n"
			cmdArgs := []string{tt.command, "-n", "doesnotexist"}
			resp, err := execDeploymentCmd(cmdArgs...)
			assert.ErrorContains(t, err, "failed to find a valid workspace")
			assert.Contains(t, resp, expectedOut)
		})
	}
}

func TestIsValidExecutor(t *testing.T) {
	af3OnlyValidExecutors := []string{"astro", "astroexecutor", "ASTRO", deployment.AstroExecutor, deployment.ASTRO}
	af2ValidExecutors := []string{"celery", "celeryexecutor", "kubernetes", "kubernetesexecutor", "CELERY", "KUBERNETES", deployment.CeleryExecutor, deployment.KubeExecutor, deployment.CELERY, deployment.KUBERNETES}
	for _, executor := range af2ValidExecutors {
		t.Run(fmt.Sprintf("returns true if executor is %s isAirflow3=false", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "13.0.0", "standard")
			assert.True(t, actual)
		})
	}
	for _, executor := range af3OnlyValidExecutors {
		t.Run(fmt.Sprintf("returns false if executor is %s isAirflow3=false", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "13.0.0", "standard")
			assert.False(t, actual)
		})
	}
	t.Run("returns false if executor is invalid isAirflow3=false", func(t *testing.T) {
		actual := deployment.IsValidExecutor("invalid-executor", "13.0.0", "standard")
		assert.False(t, actual)
	})

	// Airflow 3 introduces AstroExecutor as a valid executor
	af3ValidExecutors := append(af3OnlyValidExecutors, af2ValidExecutors...) //nolint:gocritic // intentional in this shell code
	for _, executor := range af3ValidExecutors {
		t.Run(fmt.Sprintf("returns true if executor is %s isAirflow3=true", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "3.0-1", "standard")
			assert.True(t, actual)
		})
	}

	// astro exec not allowed on hybrid
	for _, executor := range af3OnlyValidExecutors {
		t.Run(fmt.Sprintf("returns false if executor is %s isAirflow3=true for hybrid", executor), func(t *testing.T) {
			actual := deployment.IsValidExecutor(executor, "3.0-1", "hybrid")
			assert.False(t, actual)
		})
	}

	t.Run("returns false if executor is invalid isAirflow3=true", func(t *testing.T) {
		actual := deployment.IsValidExecutor("invalid-executor", "3.0-1", "standard")
		assert.False(t, actual)
	})
}

func TestIsValidCloudProvider(t *testing.T) {
	t.Run("returns true if cloudProvider is gcp", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderGCP)
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is aws", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderAWS)
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is azure", func(t *testing.T) {
		actual := isValidCloudProvider(astrov1.ClusterCloudProviderAZURE)
		assert.True(t, actual)
	})
	t.Run("returns false if cloudProvider is not gcp,aws or azure", func(t *testing.T) {
		actual := isValidCloudProvider("ibm")
		assert.False(t, actual)
	})
}
