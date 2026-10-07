package astro

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// What `astro deployment create`, `update`, `delete`, `hibernate` and
// `wake-up` print, in both formats.
//
// The json shapes are pinned once, by the goldens in testdata/schema
// (deployment-inspect.json for the Deployment a create or an update leaves,
// deployment-removal.json, deployment-hibernation.json). These tests decode
// what a run printed and assert what it means. In text they assert the
// messages, in the order a person reads them, and each table cell under its
// header, not how the table pads it.

const coreDeploymentID = "test-id-1"

func corePtr[T any](v T) *T { return &v }

// coreDeployment is a hybrid Celery Deployment as the API returns it. Hybrid
// is what a create outside a hosted organization makes, so the same fixture
// serves create, update and delete.
func coreDeployment() astrov1.Deployment {
	return astrov1.Deployment{
		Id:                     coreDeploymentID,
		Name:                   "etl-prod",
		Namespace:              "fiery-nebula-1234",
		WorkspaceId:            "workspace-id",
		WorkspaceName:          corePtr("test-workspace"),
		OrganizationId:         "test-org-id",
		RuntimeVersion:         "12.1.0",
		AirflowVersion:         "2.10.5",
		ImageTag:               "12.1.0",
		Status:                 astrov1.DeploymentStatusHEALTHY,
		Type:                   corePtr(astrov1.DeploymentTypeHYBRID),
		ClusterId:              corePtr(csID),
		ClusterName:            corePtr(testCluster),
		CloudProvider:          corePtr(astrov1.DeploymentCloudProviderAWS),
		Region:                 corePtr("us-east-1"),
		Executor:               corePtr(astrov1.DeploymentExecutorCELERY),
		SchedulerAu:            corePtr(10),
		SchedulerReplicas:      1,
		Description:            corePtr("Runs the nightly ETL"),
		WebServerUrl:           "etl-prod.astronomer.run/d1234",
		WebServerAirflowApiUrl: "etl-prod.astronomer.run/d1234/api/v2",
		IsDagDeployEnabled:     true,
		WorkerQueues:           &[]astrov1.WorkerQueue{},
		CreatedAt:              time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt:              time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

// devDeployment is a standard development Deployment, the only kind that
// hibernates.
func devDeployment() astrov1.Deployment {
	d := coreDeployment()
	d.Name = "etl-dev"
	d.Type = corePtr(astrov1.DeploymentTypeSTANDARD)
	d.ClusterId, d.ClusterName = nil, nil
	d.IsDevelopmentMode = corePtr(true)
	return d
}

func listDeploymentsResp(ds ...astrov1.Deployment) *astrov1.ListDeploymentsResponse {
	if ds == nil {
		ds = []astrov1.Deployment{}
	}
	return &astrov1.ListDeploymentsResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: ds, TotalCount: len(ds), Limit: 1000},
	}
}

func getDeploymentResp(d astrov1.Deployment) *astrov1.GetDeploymentResponse { //nolint:gocritic // a test fixture
	return &astrov1.GetDeploymentResponse{HTTPResponse: ok200(), JSON200: &d}
}

// coreMock answers the reads every one of these commands makes: the
// Workspace's Deployments, each of them by id, and their cluster.
func coreMock(t *testing.T, ds ...astrov1.Deployment) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listDeploymentsResp(ds...), nil).Maybe()
	for i := range ds {
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, ds[i].Id).Return(getDeploymentResp(ds[i]), nil).Maybe()
	}
	m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Maybe()
	return m
}

// expectCreate adds what a hybrid create asks for: the options, the
// Workspaces, the clusters, and the create, which returns created.
func expectCreate(m *astrov1_mocks.ClientWithResponsesInterface, created astrov1.Deployment) { //nolint:gocritic // a test fixture
	m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
	m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Once()
	m.On("ListClustersWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockListClustersResponse, nil).Once()
	m.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.CreateDeploymentResponse{HTTPResponse: ok200(), JSON200: &created}, nil).Once()
}

// expectUpdate adds the options and the update, which returns updated.
func expectUpdate(m *astrov1_mocks.ClientWithResponsesInterface, updated astrov1.Deployment) { //nolint:gocritic // a test fixture
	m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
	m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.UpdateDeploymentResponse{HTTPResponse: ok200(), JSON200: &updated}, nil).Once()
}

func expectDelete(m *astrov1_mocks.ClientWithResponsesInterface) {
	m.On("DeleteDeploymentWithResponse", mock.Anything, mock.Anything, coreDeploymentID).Return(&astrov1.DeleteDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil).Once()
}

func expectOverride(m *astrov1_mocks.ClientWithResponsesInterface, isHibernating bool, until *time.Time) {
	m.On("UpdateDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, coreDeploymentID, mock.Anything).Return(&astrov1.UpdateDeploymentHibernationOverrideResponse{
		HTTPResponse: ok200(),
		JSON200:      &astrov1.DeploymentHibernationOverride{IsActive: corePtr(true), IsHibernating: &isHibernating, OverrideUntil: until},
	}, nil).Once()
}

func expectRemoveOverride(m *astrov1_mocks.ClientWithResponsesInterface) {
	m.On("DeleteDeploymentHibernationOverrideWithResponse", mock.Anything, mock.Anything, coreDeploymentID).Return(&astrov1.DeleteDeploymentHibernationOverrideResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil).Once()
}

// createArgs creates coreDeployment's twin outside a hosted organization: a
// hybrid Deployment on the fixture cluster.
var createArgs = []string{"create", "--name", "etl-prod", "--workspace-id", "workspace-id", "--cluster-id", csID, "--runtime-version", "12.1.0", "--dag-deploy", "enable"}

// deploymentRow is the row create and update print for coreDeployment, by
// column.
func deploymentRow(name string) map[string]string {
	return map[string]string{
		"NAME": name, "NAMESPACE": "fiery-nebula-1234", "CLUSTER": testCluster,
		"CLOUD PROVIDER": "AWS", "REGION": "us-east-1", "DEPLOYMENT ID": coreDeploymentID,
		"RUNTIME VERSION": "12.1.0 (based on Airflow 2.10.5)", "DAG DEPLOY ENABLED": "true",
		"CI-CD ENFORCEMENT": "false", "DEPLOYMENT TYPE": "HYBRID", "REMOTE EXECUTION": "false",
	}
}

// What each command prints in text: the same messages, in the same order, as
// before it gained --output.
func TestDeploymentCoreText(t *testing.T) {
	until := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)

	says := func(parts ...string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) { requireInOrder(t, out, parts...) }
	}
	tableThen := func(name string, parts ...string) func(t *testing.T, out string) {
		return func(t *testing.T, out string) {
			assert.Equal(t, []map[string]string{deploymentRow(name)}, tableRows(t, out, "NAME"))
			requireInOrder(t, out, parts...)
		}
	}

	cases := []struct {
		name    string
		client  func(t *testing.T) astrov1.APIClient
		answers string
		args    []string
		check   func(t *testing.T, stdout string)
		wantErr string
	}{
		{
			name: "create",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t)
				expectCreate(m, coreDeployment())
				return m
			},
			args: createArgs,
			check: tableThen("etl-prod",
				"Current Workspace: test-workspace",
				"Successfully created Deployment: ", "etl-prod",
				"Deployment can be accessed at the following URLs",
				"Deployment Dashboard: ", coreDeploymentID,
				"Airflow Dashboard: ", "etl-prod.astronomer.run/d1234"),
		},
		{
			name: "update",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, coreDeployment())
				updated := coreDeployment()
				updated.Name = "etl-prod-2"
				expectUpdate(m, updated)
				return m
			},
			args:  []string{"update", coreDeploymentID, "--name", "etl-prod-2"},
			check: tableThen("etl-prod-2", "Successfully updated Deployment"),
		},
		{
			name:   "update to dag deploys it already has",
			client: func(t *testing.T) astrov1.APIClient { return coreMock(t, coreDeployment()) },
			args:   []string{"update", coreDeploymentID, "--dag-deploy", "enable"},
			check:  says("DAG deploys are already enabled for this Deployment. Your DAGs will continue to run as scheduled."),
		},
		{
			name: "update, declining an executor change",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, coreDeployment())
				m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Once()
				return m
			},
			answers: "n\n",
			args:    []string{"update", coreDeploymentID, "--executor", "KubernetesExecutor"},
			check:   says("Are you sure you want to update the", "etl-prod", "Deployment?", "Canceling Deployment update"),
		},
		{
			name: "delete --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, coreDeployment())
				expectDelete(m)
				return m
			},
			args:  []string{"delete", coreDeploymentID, "--yes"},
			check: says("Successfully deleted deployment ", "etl-prod"),
		},
		{
			name: "delete, confirmed",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, coreDeployment())
				expectDelete(m)
				return m
			},
			answers: "y\n",
			args:    []string{"delete", coreDeploymentID},
			check:   says("Are you sure you want to delete the", "etl-prod", "Deployment?", "Successfully deleted deployment ", "etl-prod"),
		},
		{
			name:    "delete, declined",
			client:  func(t *testing.T) astrov1.APIClient { return coreMock(t, coreDeployment()) },
			answers: "n\n",
			args:    []string{"delete", coreDeploymentID},
			check:   says("Are you sure you want to delete the", "etl-prod", "Canceling deployment deletion"),
		},
		{
			name: "hibernate --yes, until further notice",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, devDeployment())
				expectOverride(m, true, nil)
				return m
			},
			args: []string{"hibernate", coreDeploymentID, "--yes"},
			check: says("Successfully overrode to ", "hibernate", " until further notice",
				"Any configured hibernation schedules will not resume until override is removed."),
		},
		{
			name: "wake-up --until",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, devDeployment())
				expectOverride(m, false, &until)
				return m
			},
			args: []string{"wake-up", coreDeploymentID, "--until", until.Format(time.RFC3339), "--yes"},
			check: says("Successfully overrode to ", "wake up", " until ", until.Format(time.RFC3339),
				"If set, hibernation schedule will resume in "),
		},
		{
			name: "hibernate, confirmed",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, devDeployment())
				expectOverride(m, true, nil)
				return m
			},
			answers: "y\n",
			args:    []string{"hibernate", coreDeploymentID},
			check:   says("Are you sure you want to override to", "hibernate", "etl-dev", "Successfully overrode to "),
		},
		{
			name:    "wake-up, declined",
			client:  func(t *testing.T) astrov1.APIClient { return coreMock(t, devDeployment()) },
			answers: "n\n",
			args:    []string{"wake-up", coreDeploymentID},
			check:   says("Are you sure you want to override to", "wake up", "etl-dev", "Canceling wake up override"),
		},
		{
			name: "hibernate --remove-override --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := coreMock(t, devDeployment())
				expectRemoveOverride(m)
				return m
			},
			args:  []string{"hibernate", coreDeploymentID, "--remove-override", "--yes"},
			check: says("Successfully removed hibernation override", "If set, hibernation schedule will resume immediately."),
		},
		{
			name:    "wake-up --remove-override, declined",
			client:  func(t *testing.T) astrov1.APIClient { return coreMock(t, devDeployment()) },
			answers: "n\n",
			args:    []string{"wake-up", coreDeploymentID, "--remove-override"},
			check:   says("Are you sure you want to remove the hibernation override", "etl-dev", "Canceling hibernation override removal"),
		},
		{
			name:    "hibernate a Deployment that is not for development",
			client:  func(t *testing.T) astrov1.APIClient { return coreMock(t, coreDeployment()) },
			args:    []string{"hibernate", coreDeploymentID, "--yes"},
			wantErr: "the Deployment specified is not a development Deployment",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client(t)
			run := execDeploymentRun(t, client, tc.answers, tc.args...)
			if tc.wantErr != "" {
				assert.Equal(t, 1, run.code)
				assert.ErrorContains(t, run.err, tc.wantErr)
				return
			}
			require.NoError(t, run.err, "stdout:\n%s", run.stdout)
			assert.Equal(t, 0, run.code)
			tc.check(t, run.stdout)
			if m, ok := client.(*astrov1_mocks.ClientWithResponsesInterface); ok {
				m.AssertExpectations(t)
			}
		})
	}
}

// execDeploymentRun runs `astro deployment <args>` the way the CLI does.
func execDeploymentRun(t *testing.T, client astrov1.APIClient, answers string, args ...string) tokenRun {
	t.Helper()
	return execAstroCmd(t, client, answers, newDeploymentRootCmd, append([]string{"deployment"}, args...)...)
}

// The parts of a published Deployment (inspect.FormattedDeployment) these
// tests read. The golden pins the rest.
type (
	formattedJSON struct {
		Deployment struct {
			Configuration struct {
				Name          string `json:"name"`
				WorkspaceName string `json:"workspace_name"`
			} `json:"configuration"`
			Metadata struct {
				DeploymentID string `json:"deployment_id"`
				WorkspaceID  string `json:"workspace_id"`
				Status       string `json:"status"`
			} `json:"metadata"`
		} `json:"deployment"`
	}
	removalJSON struct {
		DeploymentID string `json:"deployment_id"`
		Name         string `json:"name"`
		WorkspaceID  string `json:"workspace_id"`
		Action       string `json:"action"`
	}
	hibernationJSON struct {
		DeploymentID string `json:"deployment_id"`
		Name         string `json:"name"`
		Override     *struct {
			IsHibernating bool       `json:"is_hibernating"`
			OverrideUntil *time.Time `json:"override_until"`
		} `json:"hibernation_override"`
	}
)

// Each command's one json object, and what it says.
func TestDeploymentCoreJSON(t *testing.T) {
	until := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)

	t.Run("create publishes the Deployment it made, as inspect shows it", func(t *testing.T) {
		m := coreMock(t, coreDeployment())
		expectCreate(m, coreDeployment())
		r := execDeploymentRun(t, m, "", append(createArgs, "-o", "json")...)
		require.NoError(t, r.err)
		var got formattedJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, coreDeploymentID, got.Deployment.Metadata.DeploymentID)
		assert.Equal(t, "workspace-id", got.Deployment.Metadata.WorkspaceID)
		assert.Equal(t, "etl-prod", got.Deployment.Configuration.Name)
		assert.Equal(t, "test-workspace", got.Deployment.Configuration.WorkspaceName)
		assert.Empty(t, r.stderr)
		m.AssertExpectations(t)
	})

	t.Run("update publishes the Deployment as the update left it", func(t *testing.T) {
		updated := coreDeployment()
		updated.Name = "etl-prod-2"
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listDeploymentsResp(coreDeployment()), nil).Once()
		// The update reads the Deployment before it, and the json reads it
		// after.
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, coreDeploymentID).Return(getDeploymentResp(coreDeployment()), nil).Once()
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, coreDeploymentID).Return(getDeploymentResp(updated), nil).Once()
		m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil)
		expectUpdate(m, updated)
		r := execDeploymentRun(t, m, "", "update", coreDeploymentID, "--name", "etl-prod-2", "-o", "json")
		require.NoError(t, r.err)
		var got formattedJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, coreDeploymentID, got.Deployment.Metadata.DeploymentID)
		assert.Equal(t, "etl-prod-2", got.Deployment.Configuration.Name)
		m.AssertExpectations(t)
	})

	t.Run("an update with nothing to send publishes the Deployment as it is", func(t *testing.T) {
		m := coreMock(t, coreDeployment())
		r := execDeploymentRun(t, m, "", "update", coreDeploymentID, "--dag-deploy", "enable", "-o", "json")
		require.NoError(t, r.err)
		var got formattedJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, "etl-prod", got.Deployment.Configuration.Name)
		m.AssertNotCalled(t, "UpdateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("delete publishes what it deleted", func(t *testing.T) {
		m := coreMock(t, coreDeployment())
		expectDelete(m)
		r := execDeploymentRun(t, m, "", "delete", coreDeploymentID, "--yes", "-o", "json")
		require.NoError(t, r.err)
		var got removalJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, removalJSON{DeploymentID: coreDeploymentID, Name: "etl-prod", WorkspaceID: "workspace-id", Action: "deleted"}, got)
		m.AssertExpectations(t)
	})

	t.Run("hibernate until further notice", func(t *testing.T) {
		m := coreMock(t, devDeployment())
		expectOverride(m, true, nil)
		r := execDeploymentRun(t, m, "", "hibernate", coreDeploymentID, "--yes", "-o", "json")
		require.NoError(t, r.err)
		var got hibernationJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, coreDeploymentID, got.DeploymentID)
		assert.Equal(t, "etl-dev", got.Name)
		require.NotNil(t, got.Override)
		assert.True(t, got.Override.IsHibernating)
		assert.Nil(t, got.Override.OverrideUntil)
		assert.JSONEq(t, `{"is_hibernating":true,"override_until":null}`, string(fields["hibernation_override"]),
			"an override with no end says so, rather than leaving the key out")
	})

	t.Run("wake-up until a time", func(t *testing.T) {
		m := coreMock(t, devDeployment())
		expectOverride(m, false, &until)
		r := execDeploymentRun(t, m, "", "wake-up", coreDeploymentID, "--until", until.Format(time.RFC3339), "--yes", "-o", "json")
		require.NoError(t, r.err)
		var got hibernationJSON
		decodeOne(t, r.stdout, &got)
		require.NotNil(t, got.Override)
		assert.False(t, got.Override.IsHibernating)
		require.NotNil(t, got.Override.OverrideUntil)
		assert.True(t, until.Equal(*got.Override.OverrideUntil))
	})

	t.Run("--remove-override leaves no override", func(t *testing.T) {
		m := coreMock(t, devDeployment())
		expectRemoveOverride(m)
		r := execDeploymentRun(t, m, "", "wake-up", coreDeploymentID, "--remove-override", "--yes", "-o", "json")
		require.NoError(t, r.err)
		var got hibernationJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, coreDeploymentID, got.DeploymentID)
		assert.Equal(t, "null", string(fields["hibernation_override"]))
	})
}

// A --wait that runs out fails the run, exit 1, after the Deployment was made
// or its override set. That result is still what happened, so it is still
// published: in text as it always was, ahead of the error; in json as the one
// object on stdout, with the error on stderr.
func TestDeploymentCoreWaitThatRunsOut(t *testing.T) {
	origSleep, origTick := deployment.SleepTime, deployment.TickNum
	deployment.SleepTime, deployment.TickNum = 0, 1
	t.Cleanup(func() { deployment.SleepTime, deployment.TickNum = origSleep, origTick })

	// Never healthy, never hibernating: each wait runs out.
	deploying := coreDeployment()
	deploying.Status = astrov1.DeploymentStatusDEPLOYING

	for _, format := range []string{"text", "json"} {
		t.Run("create "+format, func(t *testing.T) {
			m := coreMock(t, deploying)
			expectCreate(m, coreDeployment())
			r := execDeploymentRun(t, m, "", append(createArgs, "--wait", "--wait-time", "1s", "-o", format)...)
			assert.Equal(t, cliout.ExitFailure, r.code)
			assert.ErrorIs(t, r.err, deployment.ErrTimedOut)
			if format == "text" {
				assert.Equal(t, []map[string]string{deploymentRow("etl-prod")}, tableRows(t, r.stdout, "NAME"))
				return
			}
			var got formattedJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, coreDeploymentID, got.Deployment.Metadata.DeploymentID, "the new Deployment's id, to wait again or clean up")
			assert.Equal(t, "DEPLOYING", got.Deployment.Metadata.Status, "as it is when the wait ran out")
			assert.Contains(t, r.stderr, "Error: "+deployment.ErrTimedOut.Error())
		})
		t.Run("hibernate "+format, func(t *testing.T) {
			m := coreMock(t, devDeployment())
			expectOverride(m, true, nil)
			r := execDeploymentRun(t, m, "", "hibernate", coreDeploymentID, "--yes", "--wait", "--wait-time", "1s", "-o", format)
			assert.Equal(t, cliout.ExitFailure, r.code)
			assert.ErrorIs(t, r.err, deployment.ErrTimedOutHibernating)
			if format == "text" {
				requireInOrder(t, r.stdout, "Successfully overrode to ", "Waiting for the Deployment to hibernate")
				return
			}
			var got hibernationJSON
			decodeOne(t, r.stdout, &got)
			require.NotNil(t, got.Override)
			assert.True(t, got.Override.IsHibernating)
			assert.Contains(t, r.stderr, "Error: "+deployment.ErrTimedOutHibernating.Error())
		})
	}
}

// Under --output json a command that would ask something fails as
// input_required, naming the flag that answers it, with that object as the
// whole of stdout. The client mocks nothing that changes a Deployment, so a
// refused question that went on to act would panic.
func TestDeploymentCoreJSONNeverAsks(t *testing.T) {
	withOptions := func(m *astrov1_mocks.ClientWithResponsesInterface) *astrov1_mocks.ClientWithResponsesInterface {
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Maybe()
		m.On("ListWorkspacesWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&ListWorkspacesResponseOK, nil).Maybe()
		return m
	}
	cases := []struct {
		name     string
		client   func(t *testing.T) astrov1.APIClient
		args     []string
		answered string
	}{
		{"delete without --yes", func(t *testing.T) astrov1.APIClient { return coreMock(t, coreDeployment()) }, []string{"delete", coreDeploymentID}, "pass --yes"},
		{"delete naming no Deployment", func(t *testing.T) astrov1.APIClient { return coreMock(t, coreDeployment()) }, []string{"delete", "--yes"}, "pass --deployment"},
		{"update changing the executor without --yes", func(t *testing.T) astrov1.APIClient { return withOptions(coreMock(t, coreDeployment())) }, []string{"update", coreDeploymentID, "--executor", "KubernetesExecutor"}, "pass --yes"},
		{"hibernate without --yes", func(t *testing.T) astrov1.APIClient { return coreMock(t, devDeployment()) }, []string{"hibernate", coreDeploymentID}, "pass --yes"},
		{"wake-up without --yes", func(t *testing.T) astrov1.APIClient { return coreMock(t, devDeployment()) }, []string{"wake-up", coreDeploymentID, "--for", "1h"}, "pass --yes"},
		{"--remove-override without --yes", func(t *testing.T) astrov1.APIClient { return coreMock(t, devDeployment()) }, []string{"hibernate", coreDeploymentID, "--remove-override"}, "pass --yes"},
		{"create naming no name", func(t *testing.T) astrov1.APIClient { return withOptions(coreMock(t)) }, []string{"create", "--workspace-id", "workspace-id", "--cluster-id", csID, "--runtime-version", "12.1.0"}, "pass --name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execDeploymentRun(t, tc.client(t), "y\n1\nname\n", append(tc.args, "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Equal(t, cliout.ExitFailure, got.Code, "the code it reports is the one it exits with")
			assert.Contains(t, got.Error, tc.answered)
			assert.Empty(t, r.stderr)
		})
	}
}

// Asked to act on a Deployment in a Workspace that has none, each of these
// used to print that there were none and exit 0, having done nothing. Now it
// fails, exit 1, in both formats: a Deployment named by id or name is not
// found, as it is not in a Workspace that has others, and with none named
// there is nothing to act on.
func TestDeploymentCoreNothingToActOnFails(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"delete, naming none", []string{"delete", "--yes"}, "no Deployments found in workspace ws-empty to delete"},
		{"delete, naming one", []string{"delete", coreDeploymentID, "--yes"}, "the Deployment specified was not found in this workspace"},
		{"hibernate, naming none", []string{"hibernate", "--yes"}, "no Deployments found in workspace ws-empty to hibernate"},
		{"wake-up, naming one by name", []string{"wake-up", "--deployment-name", "etl-dev", "--yes"}, "the Deployment specified was not found in this workspace"},
		{"--remove-override, naming none", []string{"wake-up", "--remove-override", "--yes"}, "no Deployments with a hibernation override that can be removed found in Workspace ws-empty"},
	}
	for _, tc := range cases {
		for _, format := range []string{"text", "json"} {
			t.Run(tc.name+" "+format, func(t *testing.T) {
				r := execDeploymentRun(t, coreMock(t), "", append(tc.args, "--workspace-id", "ws-empty", "-o", format)...)
				assert.Equal(t, cliout.ExitFailure, r.code)
				require.Error(t, r.err)
				assert.Contains(t, r.err.Error(), tc.want)
				if format == "json" {
					var got errorJSON
					decodeOne(t, r.stdout, &got)
					assert.Contains(t, got.Error, tc.want)
				} else {
					assert.Empty(t, r.stdout)
				}
			})
		}
	}
}

// --output takes text or json: anything else is a usage error, exit 2, before
// anything is asked or done.
func TestDeploymentCoreOutputUsage(t *testing.T) {
	for _, args := range [][]string{
		append(append([]string{}, createArgs...), "-o", "yaml"),
		{"update", coreDeploymentID, "-o", "yaml"},
		{"delete", coreDeploymentID, "--yes", "-o", "yaml"},
		{"hibernate", coreDeploymentID, "--yes", "-o", "yaml"},
		{"wake-up", coreDeploymentID, "--yes", "-o", "yaml"},
	} {
		r := execDeploymentRun(t, new(astrov1_mocks.ClientWithResponsesInterface), "", args...)
		require.Error(t, r.err, args)
		assert.Equal(t, cliout.ExitUsage, r.code, args)
	}
}

// --deployment-file takes -o too. A file that fails validation under -o json
// is the json error object, exit 1, like any other failure; nothing is asked
// of the API, so the client mocks nothing.
func TestDeploymentFromFileJSONError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-name.yaml")
	require.NoError(t, os.WriteFile(path, []byte("deployment:\n  configuration:\n    description: no name\n"), 0o600))
	for _, verb := range []string{"create", "update"} {
		t.Run(verb, func(t *testing.T) {
			r := execDeploymentRun(t, new(astrov1_mocks.ClientWithResponsesInterface), "", verb, "--deployment-file", path, "-o", "json")
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, cliout.ExitFailure, got.Code)
			assert.Contains(t, got.Error, "name")
			assert.NotContains(t, got.Error, "cannot be used with other arguments", "-o is allowed beside the file")
			assert.Empty(t, r.stderr)
		})
	}
}
