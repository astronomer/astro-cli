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
	astro_mocks "github.com/astronomer/astro-cli/astro-client/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func execDeploymentCmd(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	cmd := newDeploymentRootCmd(buf)
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	_, err := cmd.ExecuteC()
	return buf.String(), err
}

func TestDeploymentRootCommand(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)
	buf := new(bytes.Buffer)
	deplyCmd := newDeploymentRootCmd(os.Stdout)
	deplyCmd.SetOut(buf)
	_, err := deplyCmd.ExecuteC()
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "deployment")
}

func TestDeploymentList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything).Return([]astro.Deployment{{ID: "test-id-1"}, {ID: "test-id-2"}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"list", "-a"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-id-1")
	assert.Contains(t, resp, "test-id-2")
	mockClient.AssertExpectations(t)
}

func TestDeploymentLogs(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	deploymentID := "test-id"
	logLevels := []string{"WARN", "ERROR", "INFO"}
	mockInput := map[string]interface{}{
		"deploymentId":  deploymentID,
		"logCountLimit": logCount,
		"start":         "-24hrs",
		"logLevels":     logLevels,
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything).Return([]astro.Deployment{{ID: "test-id"}, {ID: "test-id-2"}}, nil).Once()
	mockClient.On("GetDeploymentHistory", mockInput).Return(astro.DeploymentHistory{DeploymentID: deploymentID, SchedulerLogs: []astro.SchedulerLog{{Raw: "test log line"}}}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"logs", "test-id", "-w", "-e", "-i"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentCreate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "test-ws-id"
	csID := "test-cluster-id"

	mockClient := new(astro_mocks.Client)
	mockClient.On("GetDeploymentConfig").Return(astro.DeploymentConfig{RuntimeReleases: []astro.RuntimeRelease{{Version: "4.2.5"}}}, nil).Once()
	mockClient.On("ListWorkspaces", "test-org-id").Return([]astro.Workspace{{ID: ws, OrganizationID: "test-org-id"}}, nil).Once()
	mockClient.On("ListClusters", "test-org-id").Return([]astro.Cluster{{ID: csID}}, nil).Once()
	mockClient.On("CreateDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()
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

	cmdArgs := []string{"create", "--name", "test-name", "--workspace-id", ws, "--cluster-id", csID}
	_, err = execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentUpdate(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	ws := "test-ws-id"
	deploymentResp := astro.Deployment{
		ID:             "test-id",
		Label:          "test-name",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Workers: astro.Workers{AU: 10}, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", astro.DeploymentsInput{WorkspaceID: ws}).Return([]astro.Deployment{deploymentResp}, nil).Once()
	mockClient.On("UpdateDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"update", "test-id", "--name", "test-name", "--workspace-id", ws, "--force"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentDelete(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

	deploymentResp := astro.Deployment{
		ID:             "test-id",
		RuntimeRelease: astro.RuntimeRelease{Version: "4.2.5"},
		DeploymentSpec: astro.DeploymentSpec{Workers: astro.Workers{AU: 10}, Scheduler: astro.Scheduler{AU: 5, Replicas: 3}},
	}

	mockClient := new(astro_mocks.Client)
	mockClient.On("ListDeployments", mock.Anything).Return([]astro.Deployment{deploymentResp}, nil).Once()
	mockClient.On("DeleteDeployment", mock.Anything).Return(astro.Deployment{ID: "test-id"}, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"delete", "test-id", "--force"}
	_, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestDeploymentVariableList(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	mockClient.On("ListDeployments", mock.Anything).Return(mockResponse, nil).Once()
	astroClient = mockClient

	cmdArgs := []string{"variable", "list", "--deployment-id", "test-id-1"}
	resp, err := execDeploymentCmd(cmdArgs...)
	assert.NoError(t, err)
	assert.Contains(t, resp, "test-key-1")
	assert.Contains(t, resp, "test-value-1")
	mockClient.AssertExpectations(t)
}

func TestDeploymentVariableModify(t *testing.T) {
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	mockClient.On("ListDeployments", mock.Anything).Return(mockListResponse, nil).Once()
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
	testUtil.InitTestConfig(testUtil.CloudPlatform)

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
	mockClient.On("ListDeployments", mock.Anything).Return(mockListResponse, nil).Once()
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
