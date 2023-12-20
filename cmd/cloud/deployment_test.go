package cloud

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astrocore_mocks "github.com/astronomer/astro-cli/astro-client-core/mocks"
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var mockListClustersResponse = astrocore.ListClustersResponse{
	HTTPResponse: &http.Response{
		StatusCode: 200,
	},
	JSON200: &astrocore.ClustersPaginated{
		Clusters: []astrocore.Cluster{
			{
				Id:   "test-cluster-id",
				Name: "test-cluster",
				NodePools: []astrocore.NodePool{
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
				},
			},
			{
				Id:   "test-cluster-id-1",
				Name: "test-cluster-1",
			},
		},
	},
}

func execDeploymentCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(buf)
	cmd.SetOut(buf)
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

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, "").Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"list", "-a"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	mockClient.AssertExpectations(t)
}

func TestDeploymentLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	deploymentID := "test-id"
	logLevels := []string{"WARN", "ERROR", "INFO"}
	mockInput := map[string]interface{}{
		"deploymentId":  deploymentID,
		"logCountLimit": logCount,
		"start":         "-24hrs",
		"logLevels":     logLevels,
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return([]astro.Deployment{{ID: "test-id"}, {ID: "test-id-2"}}, nil).Once()
	mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{{Raw: "test log line"}}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"logs", "test-id", "-w", "-e", "-i"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	ws := "workspace-id"
	csID := "test-cluster-id"
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)

	deploymentCreateInput := astro.CreateDeploymentInput{
		WorkspaceID:           ws,
		ClusterID:             csID,
		Label:                 "test-name",
		Description:           "",
		RuntimeReleaseVersion: "4.2.5",
		DagDeployEnabled:      true,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 1,
			},
		},
	}

	deploymentCreateInput1 := astro.CreateDeploymentInput{
		WorkspaceID:           ws,
		ClusterID:             csID,
		Label:                 "test-name",
		Description:           "",
		RuntimeReleaseVersion: "4.2.5",
		DagDeployEnabled:      false,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 1,
			},
		},
	}

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
		RuntimeReleases: []astro.RuntimeRelease{
			{
				Version: "4.2.5",
			},
		},
	}, nil).Times(7)
	mockClient.On("GetDeploymentConfigWithOrganization", mock.Anything).Return(astro.DeploymentConfig{
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
	}, nil).Times(7)
	mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Times(5)
	mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Times(4)
	mockClient.On("CreateDeployment", &deploymentCreateInput).Return(astro.Deployment{ID: "test-id"}, nil).Twice()
	mockClient.On("CreateDeployment", &deploymentCreateInput1).Return(astro.Deployment{ID: "test-id"}, nil).Times(6)
	deploymentCreateInput2 := astro.CreateDeploymentInput{
		WorkspaceID:           ws,
		ClusterID:             csID,
		Label:                 "test-name",
		Description:           "",
		RuntimeReleaseVersion: "4.2.5",
		DagDeployEnabled:      false,
		SchedulerSize:         "small",
		IsHighAvailability:    false,
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor: "CeleryExecutor",
			Scheduler: astro.Scheduler{
				AU:       5,
				Replicas: 1,
			},
		},
	}
	mockClient.On("CreateDeployment", &deploymentCreateInput2).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	astroClient = mockClient

	mockResponse := &airflowversions.Response{
		RuntimeVersions: map[string]airflowversions.RuntimeVersion{
			"4.2.5": {Metadata: airflowversions.RuntimeVersionMetadata{AirflowVersion: "2.2.5", Channel: "stable"}, Migrations: airflowversions.RuntimeVersionMigrations{}},
		},
	}
	jsonResponse, err := json.Marshal(mockResponse)
	assert.NoError(t, err)

	httpClient = testUtil.NewTestClient(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBuffer(jsonResponse)),
			Header:     make(http.Header),
		}
	})

	t.Run("creates a deployment when dag-deploy is disabled", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		astroCoreClient = mockCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("creates a deployment when dag deploy is enabled", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "enable"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("creates a deployment when executor is specified", func(t *testing.T) {
		deploymentCreateInput1.DeploymentSpec.Executor = "KubernetesExecutor"
		defer func() { deploymentCreateInput1.DeploymentSpec.Executor = "CeleryExecutor" }()
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubernetesExecutor"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("creates a deployment with default executor", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("returns an error if dag-deploy flag has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "some-value"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID, "--dag-deploy", "disable", "--executor", "KubeExecutor"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("creates a deployment from file", func(t *testing.T) {
		orgID := "test-org-id"
		filePath := "./test-deployment.yaml"
		data := `
deployment:
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
    executor: CeleryExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		createdDeployment := astro.Deployment{
			ID:    "test-deployment-id",
			Label: "test-deployment-label",
		}
		mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		mockClient = new(astro_mocks.Client)
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
		mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{}, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("CreateDeployment", mock.Anything).Return(createdDeployment, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
		mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, nil)
		mockClient.On("ListDeployments", orgID, ws).Return([]astro.Deployment{createdDeployment}, nil)
		origClient := astroClient
		astroClient = mockClient
		fileutil.WriteStringToFile(filePath, data)
		defer func() {
			astroClient = origClient
			afero.NewOsFs().Remove(filePath)
		}()
		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml"}
		astroCoreClient = mockCoreClient
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if creating a deployment from file fails", func(t *testing.T) {
		cmdArgs := []string{"create", "--deployment-file", "test-file-name.json"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "open test-file-name.json: no such file or directory")
	})
	t.Run("returns an error if from-file is specified with any other flags", func(t *testing.T) {
		cmdArgs := []string{"create", "--deployment-file", "test-deployment.yaml", "--description", "fail"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errFlag)
	})
	t.Run("creates a deployment with cloud provider and region", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("organization_short_name", "test-org")
		ctx.SetContextKey("workspace", ws)
		mockOKResponse := &astrocore.GetSharedClusterResponse{
			HTTPResponse: &http.Response{
				StatusCode: 200,
			},
			JSON200: &astrocore.SharedCluster{Id: csID},
		}
		astroCoreClient = mockCoreClient
		mockCoreClient.On("GetSharedClusterWithResponse", mock.Anything, mock.Anything).Return(mockOKResponse, nil).Once()
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--cloud-provider", "gcp", "--region", "us-central1",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization_short_name", "test-org")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "gcp", "--region", "us-east1", "--high-availability", "some-value",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	t.Run("returns an error if cloud provider is not valid", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--dag-deploy", "disable",
			"--executor", "KubernetesExecutor", "--cloud-provider", "azure",
		}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "azure is not a valid cloud provider. It can only be gcp")
	})
	t.Run("creates a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		ctx.SetContextKey("organization_short_name", "test-org")
		mockCoreClient.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		astroCoreClient = mockCoreClient
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--cluster-type", "dedicated",
		}

		// mock os.Stdin
		input := []byte("1")
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, err = w.Write(input)
		if err != nil {
			t.Error(err)
		}
		w.Close()
		stdin := os.Stdin
		// Restore stdin right after the test.
		defer func() { os.Stdin = stdin }()
		os.Stdin = r

		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockCoreClient.AssertExpectations(t)
	})
	t.Run("returns an error if incorrect cluster type is passed for a hosted dedicated deployment", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		astroCoreClient = mockCoreClient
		cmdArgs := []string{
			"create", "--name", "test-name", "--workspace-id", ws, "--cluster-type", "wrong-value",
		}

		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --cluster-type value")
		mockCoreClient.AssertExpectations(t)
	})
	mockClient.AssertExpectations(t)
	mockCoreClient.AssertExpectations(t)
}

func TestDeploymentUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	mockCoreClient := new(astrocore_mocks.ClientWithResponsesInterface)
	ws := "test-ws-id"
	deploymentResp := astro.Deployment{
		ID:             "test-id",
		Label:          "test-name",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Executor: "CeleryExecutor", Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}
	deploymentUpdateInput := astro.UpdateDeploymentInput{
		ID:          "test-id",
		ClusterID:   "",
		Label:       "test-name",
		Description: "",
		DeploymentSpec: astro.DeploymentCreateSpec{
			Executor:  "CeleryExecutor",
			Scheduler: astro.Scheduler{AU: 5, Replicas: 3},
		},
		APIKeyOnlyDeployments: false,
		WorkerQueues:          nil,
	}

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
		RuntimeReleases: []astro.RuntimeRelease{
			{
				Version: "4.2.5",
			},
		},
	}, nil).Times(3)
	mockClient.On("ListDeployments", mock.Anything, ws).Return([]astro.Deployment{deploymentResp}, nil).Times(3)
	mockClient.On("UpdateDeployment", &deploymentUpdateInput).Return(astro.Deployment{ID: "test-id"}, nil).Times(3)
	astroClient = mockClient

	t.Run("updates the deployment successfully", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("updates the deployment successfully to enable ci-cd enforcement", func(t *testing.T) {
		deploymentUpdateInput.APIKeyOnlyDeployments = true
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("updates the deployment successfully to disable ci-cd enforcement", func(t *testing.T) {
		deploymentUpdateInput.APIKeyOnlyDeployments = false
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--enforce-cicd=false"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
	})
	t.Run("returns an error if dag-deploy has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--dag-deploy", "some-value"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.Error(t, err)
	})
	t.Run("returns an error if executor has an incorrect value", func(t *testing.T) {
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force", "--executor", "KubeExecutor"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "KubeExecutor is not a valid executor")
	})
	t.Run("returns an error when getting workspace fails", func(t *testing.T) {
		testUtil.InitTestConfig(testUtil.LocalPlatform)
		ctx, err := config.GetCurrentContext()
		assert.NoError(t, err)
		ctx.Workspace = ""
		err = ctx.SetContext()
		assert.NoError(t, err)
		defer testUtil.InitTestConfig(testUtil.LocalPlatform)
		expectedOut := "Usage:\n"
		cmdArgs := []string{"update", "-n", "doesnotexist"}
		resp, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "failed to find a valid Workspace")
		assert.Contains(t, resp, expectedOut)
	})
	t.Run("updates a deployment from file", func(t *testing.T) {
		orgID := "test-org-id"
		filePath := "./test-deployment.yaml"
		data := `
deployment:
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
    executor: CeleryExecutor
    scheduler_au: 5
    scheduler_count: 3
    cluster_name: test-cluster
    workspace_name: test-workspace
    deployment_type: HYBRID
  worker_queues:
    - name: default
      is_default: true
      max_worker_count: 130
      min_worker_count: 12
      worker_concurrency: 180
      worker_type: test-worker-1
    - name: test-queue-1
      is_default: false
      max_worker_count: 175
      min_worker_count: 8
      worker_concurrency: 176
      worker_type: test-worker-2
  metadata:
    deployment_id: test-deployment-id
    workspace_id: test-ws-id
    cluster_id: cluster-id
    release_name: great-release-name
    airflow_version: 2.4.0
    status: UNHEALTHY
    created_at: 2022-11-17T13:25:55.275697-08:00
    updated_at: 2022-11-17T13:25:55.275697-08:00
    deployment_url: cloud.astronomer.io/test-ws-id/deployments/test-deployment-id/overview
    webserver_url: some-url
  alert_emails:
    - test1@test.com
    - test2@test.com
`
		updatedDeployment := astro.Deployment{
			ID:      "test-deployment-id",
			Label:   "test-deployment-label",
			Cluster: astro.Cluster{ID: "test-cluster-id", Name: "test-cluster"},
		}
		mockWorkerQueueDefaultOptions := astro.WorkerQueueDefaultOptions{
			MinWorkerCount: astro.WorkerQueueOption{
				Floor:   1,
				Ceiling: 20,
				Default: 5,
			},
			MaxWorkerCount: astro.WorkerQueueOption{
				Floor:   16,
				Ceiling: 200,
				Default: 125,
			},
			WorkerConcurrency: astro.WorkerQueueOption{
				Floor:   175,
				Ceiling: 275,
				Default: 180,
			},
		}
		mockCoreClient.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
		mockCoreClient.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, deploymentListParams).Return(&mockListDeploymentsResponse, nil).Once()
		mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil).Once()
		mockClient.On("GetWorkerQueueOptions").Return(mockWorkerQueueDefaultOptions, nil).Once()
		mockClient.On("UpdateDeployment", mock.Anything).Return(updatedDeployment, nil)
		mockClient.On("ModifyDeploymentVariable", mock.Anything).Return([]astro.EnvironmentVariablesObject{}, nil)
		mockClient.On("UpdateAlertEmails", mock.Anything).Return(astro.DeploymentAlerts{}, nil)
		mockClient.On("ListDeployments", orgID, "").Return([]astro.Deployment{updatedDeployment}, nil)
		origClient := astroClient
		astroClient = mockClient
		fileutil.WriteStringToFile(filePath, data)
		defer func() {
			astroClient = origClient
			afero.NewOsFs().Remove(filePath)
		}()
		cmdArgs := []string{"update", "--deployment-file", "test-deployment.yaml"}
		astroCoreClient = mockCoreClient
		_, err := execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error if updating a deployment from file fails", func(t *testing.T) {
		cmdArgs := []string{"update", "--deployment-file", "test-file-name.json"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "open test-file-name.json: no such file or directory")
	})
	t.Run("returns an error if from-file is specified with any other flags", func(t *testing.T) {
		cmdArgs := []string{"update", "--deployment-file", "test-deployment.yaml", "--description", "fail"}
		_, err := execDeploymentCmd(cmdArgs...)
		assert.ErrorIs(t, err, errFlag)
	})
	t.Run("updates a deployment with small scheduler size", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		deploymentResp := astro.Deployment{
			ID:             "test-id",
			Label:          "test-name",
			RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
			DeploymentSpec: astro.DeploymentSpec{Executor: "CeleryExecutor", Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
		}

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
		mockClient.On("ListDeployments", mock.Anything, ws).Return([]astro.Deployment{deploymentResp}, nil).Once()
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--scheduler-size", "small", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})
	t.Run("returns an error with incorrect high-availability value", func(t *testing.T) {
		ctx, err := context.GetCurrentContext()
		assert.NoError(t, err)
		ctx.SetContextKey("organization_product", "HOSTED")
		ctx.SetContextKey("organization", "test-org-id")
		ctx.SetContextKey("workspace", ws)
		cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--high-availability", "some-value", "--force"}
		_, err = execDeploymentCmd(cmdArgs...)
		assert.ErrorContains(t, err, "Invalid --high-availability value")
	})
	mockClient.AssertExpectations(t)
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return([]astro.Deployment{deploymentResp}, nil).Once()
	mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"delete", "test-id", "--force"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockResponse := []astro.Deployment{
		{
			ID: "test-id-1",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
			},
		},
		{
			ID: "test-id-2",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-2", Value: "test-value-2"}},
			},
		},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockResponse, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"variable", "list", "--deployment-id", "test-id-1"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	mockClient.AssertExpectations(t)
}

func TestDeploymentVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockListResponse := []astro.Deployment{
		{
			ID: "test-id-1",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
			},
		},
		{
			ID: "test-id-2",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{},
			},
		},
	}

	mockCreateResponse := []astro.EnvironmentVariablesObject{
		{
			Key:   "test-key-1",
			Value: "test-value-1",
		},
		{
			Key:   "test-key-2",
			Value: "test-value-2",
		},
		{
			Key:   "test-key-3",
			Value: "test-value-3",
		},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockListResponse, nil).Once()
	mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockCreateResponse, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"variable", "create", "test-key-3=test-value-3", "--deployment-id", "test-id-1", "--key", "test-key-2", "--value", "test-value-2"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2")
	assert.Contains(t, resp, "test-key-3")
	assert.Contains(t, resp, "test-value-3")
	mockClient.AssertExpectations(t)
}

func TestDeploymentVariableUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	mockListResponse := []astro.Deployment{
		{
			ID: "test-id-1",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{{Key: "test-key-1", Value: "test-value-1"}},
			},
		},
		{
			ID: "test-id-2",
			DeploymentSpec: astro.DeploymentSpec{
				EnvironmentVariablesObjects: []astro.EnvironmentVariablesObject{},
			},
		},
	}

	mockUpdateResponse := []astro.EnvironmentVariablesObject{
		{
			Key:   "test-key-1",
			Value: "test-value-update",
		},
		{
			Key:   "test-key-2",
			Value: "test-value-2-update",
		},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything, mock.Anything).Return(mockListResponse, nil).Once()
	mockClient.On("ModifyDeploymentVariable", mock.Anything).Return(mockUpdateResponse, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"variable", "update", "test-key-2=test-value-2-update", "--deployment-id", "test-id-1", "--key", "test-key-1", "--value", "test-value-update"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-update")
	assert.Contains(t, resp, "test-key-2")
	assert.Contains(t, resp, "test-value-2-update")
	mockClient.AssertExpectations(t)
}

func TestIsValidExecutor(t *testing.T) {
	t.Run("returns true for Kubernetes Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.KubeExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true for Celery Executor", func(t *testing.T) {
		actual := isValidExecutor(deployment.CeleryExecutor)
		assert.True(t, actual)
	})
	t.Run("returns true when no Executor is requested", func(t *testing.T) {
		actual := isValidExecutor("")
		assert.True(t, actual)
	})
	t.Run("returns false for any invalid executor", func(t *testing.T) {
		actual := isValidExecutor("KubeExecutor")
		assert.False(t, actual)
	})
}

func TestIsValidCloudProvider(t *testing.T) {
	t.Run("returns true if cloudProvider is gcp", func(t *testing.T) {
		actual := isValidCloudProvider("gcp")
		assert.True(t, actual)
	})
	t.Run("returns true if cloudProvider is aws", func(t *testing.T) {
		actual := isValidCloudProvider("aws")
		assert.True(t, actual)
	})
	t.Run("returns false if cloudProvider is not gcp", func(t *testing.T) {
		actual := isValidCloudProvider("azure")
		assert.False(t, actual)
	})
}
