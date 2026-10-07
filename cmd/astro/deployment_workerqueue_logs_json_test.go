package astro

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
)

// What `astro deployment worker-queue create`, `update` and `delete`, and
// `astro deployment logs`, print in both formats.
//
// The json shapes are pinned once, by the goldens in testdata/schema
// (deployment-worker-queue.json, deployment-log-entry.json). These tests
// decode what a run printed and assert what it means; in text they assert
// the messages a person reads.

const wqDeploymentID = "cldep00000000000000000wq1"

func wqPtr[T any](v T) *T { return &v }

// wqDeployment is a standard Celery Deployment with the default queue and one
// more, both on A5 machines.
func wqDeployment() astrov1.Deployment {
	return astrov1.Deployment{
		Id:                   wqDeploymentID,
		Name:                 "etl-dev",
		WorkspaceId:          "workspace-id",
		WorkspaceName:        wqPtr("test-workspace"),
		OrganizationId:       "test-org-id",
		RuntimeVersion:       "12.1.0",
		AirflowVersion:       "2.10.5",
		Status:               astrov1.DeploymentStatusHEALTHY,
		Type:                 wqPtr(astrov1.DeploymentTypeSTANDARD),
		CloudProvider:        wqPtr(astrov1.DeploymentCloudProviderAWS),
		Region:               wqPtr("us-east-1"),
		Executor:             wqPtr(astrov1.DeploymentExecutorCELERY),
		SchedulerSize:        wqPtr(astrov1.DeploymentSchedulerSizeSMALL),
		IsHighAvailability:   wqPtr(false),
		IsDevelopmentMode:    wqPtr(true),
		DefaultTaskPodCpu:    wqPtr("0.25"),
		DefaultTaskPodMemory: wqPtr("0.5Gi"),
		ResourceQuotaCpu:     wqPtr("10"),
		ResourceQuotaMemory:  wqPtr("20Gi"),
		WorkerQueues: &[]astrov1.WorkerQueue{
			{Id: "wq-default", Name: "default", IsDefault: true, AstroMachine: wqPtr("A5"), MinWorkerCount: 1, MaxWorkerCount: 10, WorkerConcurrency: 5},
			{Id: "wq-etl", Name: "etl-queue", AstroMachine: wqPtr("A5"), MinWorkerCount: 0, MaxWorkerCount: 4, WorkerConcurrency: 5},
		},
	}
}

// wqMock answers every read these commands make, and the update a queue
// change is sent as.
func wqMock(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	d := wqDeployment()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{d}, TotalCount: 1, Limit: 1000},
	}, nil).Maybe()
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID).Return(&astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &d}, nil).Maybe()
	m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseOK, nil).Maybe()
	return m
}

func expectQueueUpdate(m *astrov1_mocks.ClientWithResponsesInterface) {
	d := wqDeployment()
	m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&astrov1.UpdateDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &d}, nil).Once()
}

// logsMock answers the Deployment lookup and one page of logs holding
// entries.
func logsMock(t *testing.T, entries ...astrov1.DeploymentLogEntry) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := wqMock(t)
	if entries == nil {
		entries = []astrov1.DeploymentLogEntry{}
	}
	m.On("GetDeploymentLogsWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&astrov1.GetDeploymentLogsResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &astrov1.DeploymentLog{Results: entries, ResultCount: len(entries), Limit: 500, SearchId: "search-id"},
	}, nil).Once()
	return m
}

var twoLogEntries = []astrov1.DeploymentLogEntry{
	{Raw: "[2026-01-02 03:04:05] INFO - Scheduler started", Source: astrov1.DeploymentLogEntrySourceScheduler, Timestamp: 1767323045},
	{Raw: "[2026-01-02 03:04:06] ERROR - Task failed", Source: astrov1.DeploymentLogEntrySourceWorker, Timestamp: 1767323046},
}

// The worker-queue flags every case passes: the Deployment, and its Workspace.
var wqTarget = []string{"--deployment-id", wqDeploymentID, "--workspace-id", "workspace-id"}

func wqArgs(args ...string) []string { return append(args, wqTarget...) }

// What each command prints in text: the same messages, in the same order, as
// before it gained --output.
func TestDeploymentWorkerQueueAndLogsText(t *testing.T) {
	cases := []struct {
		name    string
		client  func(t *testing.T) astrov1.APIClient
		answers string
		args    []string
		says    []string
	}{
		{
			name: "worker-queue create",
			client: func(t *testing.T) astrov1.APIClient {
				m := wqMock(t)
				expectQueueUpdate(m)
				return m
			},
			args: wqArgs("worker-queue", "create", "--name", "reports", "--worker-type", "a5"),
			says: []string{"worker queue reports for etl-dev in workspace-id workspace created"},
		},
		{
			name: "worker-queue update --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := wqMock(t)
				expectQueueUpdate(m)
				return m
			},
			args: wqArgs("worker-queue", "update", "--name", "etl-queue", "--max-count", "8", "--yes"),
			says: []string{"worker queue etl-queue for etl-dev in workspace-id workspace updated"},
		},
		{
			name:    "worker-queue update, declined",
			client:  func(t *testing.T) astrov1.APIClient { return wqMock(t) },
			answers: "n\n",
			args:    wqArgs("worker-queue", "update", "--name", "etl-queue", "--max-count", "8"),
			says:    []string{"Are you sure you want to update the", "etl-queue", "Canceling worker queue update"},
		},
		{
			name: "worker-queue delete --yes",
			client: func(t *testing.T) astrov1.APIClient {
				m := wqMock(t)
				expectQueueUpdate(m)
				return m
			},
			args: wqArgs("worker-queue", "delete", "--name", "etl-queue", "--yes"),
			says: []string{"worker queue etl-queue for etl-dev in workspace-id workspace deleted"},
		},
		{
			name:    "worker-queue delete, declined",
			client:  func(t *testing.T) astrov1.APIClient { return wqMock(t) },
			answers: "n\n",
			args:    wqArgs("worker-queue", "delete", "--name", "etl-queue"),
			says:    []string{"Are you sure you want to delete the", "etl-queue", "Canceling worker queue deletion"},
		},
		{
			name:   "logs",
			client: func(t *testing.T) astrov1.APIClient { return logsMock(t, twoLogEntries...) },
			args:   []string{"logs", wqDeploymentID, "--workspace-id", "workspace-id"},
			says: []string{
				"[2026-01-02 03:04:05] INFO - Scheduler started scheduler\n",
				"[2026-01-02 03:04:06] ERROR - Task failed worker\n",
			},
		},
		{
			name:   "logs, none",
			client: func(t *testing.T) astrov1.APIClient { return logsMock(t) },
			args:   []string{"logs", wqDeploymentID, "--workspace-id", "workspace-id"},
			says:   []string{"No matching logs have been recorded in the past 24 hours for Deployment etl-dev\n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.client(t)
			run := execAstroCmd(t, client, tc.answers, newDeploymentRootCmd, append([]string{"deployment"}, tc.args...)...)
			require.NoError(t, run.err, "stdout:\n%s", run.stdout)
			assert.Equal(t, 0, run.code)
			requireInOrder(t, run.terminal(), tc.says...)
			client.(*astrov1_mocks.ClientWithResponsesInterface).AssertExpectations(t)
		})
	}
}

// The parts of the published shapes these tests read, decoded on their own
// so a renamed key fails here as well as in its golden.
type (
	wqQueueJSON struct {
		Name              string `json:"name"`
		IsDefault         bool   `json:"is_default"`
		WorkerType        string `json:"worker_type"`
		MinWorkerCount    int    `json:"min_worker_count"`
		MaxWorkerCount    int    `json:"max_worker_count"`
		WorkerConcurrency int    `json:"worker_concurrency"`
	}
	wqResultJSON struct {
		DeploymentID   string      `json:"deployment_id"`
		DeploymentName string      `json:"deployment_name"`
		WorkspaceID    string      `json:"workspace_id"`
		Action         string      `json:"action"`
		WorkerQueue    wqQueueJSON `json:"worker_queue"`
	}
	logEntryJSON struct {
		Source  string `json:"source"`
		Message string `json:"message"`
	}
)

// Each worker-queue command's one json object, and what it says.
func TestDeploymentWorkerQueueJSON(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want wqResultJSON
	}{
		{
			name: "create",
			args: wqArgs("worker-queue", "create", "--name", "reports", "--worker-type", "a5", "--min-count", "1", "--max-count", "6", "--concurrency", "4"),
			want: wqResultJSON{
				DeploymentID: wqDeploymentID, DeploymentName: "etl-dev", WorkspaceID: "workspace-id", Action: "created",
				WorkerQueue: wqQueueJSON{Name: "reports", WorkerType: "a5", MinWorkerCount: 1, MaxWorkerCount: 6, WorkerConcurrency: 4},
			},
		},
		{
			// Only --max-count changes; the queue keeps the rest, on its
			// machine as the Deployment's options spell it.
			name: "update",
			args: wqArgs("worker-queue", "update", "--name", "etl-queue", "--max-count", "8", "--yes"),
			want: wqResultJSON{
				DeploymentID: wqDeploymentID, DeploymentName: "etl-dev", WorkspaceID: "workspace-id", Action: "updated",
				WorkerQueue: wqQueueJSON{Name: "etl-queue", WorkerType: "a5", MinWorkerCount: 0, MaxWorkerCount: 8, WorkerConcurrency: 5},
			},
		},
		{
			// The queue as it was before it was deleted.
			name: "delete",
			args: wqArgs("worker-queue", "delete", "--name", "etl-queue", "--yes"),
			want: wqResultJSON{
				DeploymentID: wqDeploymentID, DeploymentName: "etl-dev", WorkspaceID: "workspace-id", Action: "deleted",
				WorkerQueue: wqQueueJSON{Name: "etl-queue", WorkerType: "A5", MinWorkerCount: 0, MaxWorkerCount: 4, WorkerConcurrency: 5},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := wqMock(t)
			expectQueueUpdate(m)
			r := execAstroCmd(t, m, "", newDeploymentRootCmd, append([]string{"deployment"}, append(tc.args, "-o", "json")...)...)
			require.NoError(t, r.err)
			var got wqResultJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, tc.want, got)
			assert.Empty(t, r.stderr)
			m.AssertExpectations(t)
		})
	}
}

// `deployment logs -o json` is a stream: one object per entry, one line
// each, in the order the API returned them. None is no lines at all, and the
// note that says so goes to stderr.
func TestDeploymentLogsJSON(t *testing.T) {
	t.Run("entries", func(t *testing.T) {
		r := execAstroCmd(t, logsMock(t, twoLogEntries...), "", newDeploymentRootCmd, "deployment", "logs", wqDeploymentID, "--workspace-id", "workspace-id", "-o", "json")
		require.NoError(t, r.err)
		lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
		require.Len(t, lines, 2, "one line per entry:\n%s", r.stdout)
		var got []logEntryJSON
		for _, line := range lines {
			var e logEntryJSON
			decodeOne(t, line, &e)
			got = append(got, e)
		}
		assert.Equal(t, []logEntryJSON{
			{Source: "scheduler", Message: "[2026-01-02 03:04:05] INFO - Scheduler started"},
			{Source: "worker", Message: "[2026-01-02 03:04:06] ERROR - Task failed"},
		}, got)
	})
	t.Run("none", func(t *testing.T) {
		r := execAstroCmd(t, logsMock(t), "", newDeploymentRootCmd, "deployment", "logs", wqDeploymentID, "--workspace-id", "workspace-id", "-o", "json")
		require.NoError(t, r.err)
		assert.Equal(t, 0, r.code)
		assert.Empty(t, r.stdout, "no entries is no records")
		assert.Contains(t, r.stderr, "No matching logs have been recorded in the past 24 hours for Deployment etl-dev")
	})
}

// Under --output json a command that would ask something fails as
// input_required, naming the flag that answers it, with that object as the
// whole of stdout: the table a pick would have been made from goes to
// stderr, not stdout. The client mocks no update, so a refused question that
// went on to act would panic.
func TestDeploymentWorkerQueueJSONNeverAsks(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		answered string
	}{
		{"create naming no queue", wqArgs("worker-queue", "create", "--worker-type", "a5"), "pass --name"},
		{"create naming no worker type", wqArgs("worker-queue", "create", "--name", "reports"), "pass --worker-type"},
		{"update without --yes", wqArgs("worker-queue", "update", "--name", "etl-queue", "--max-count", "8"), "pass --yes"},
		{"delete without --yes", wqArgs("worker-queue", "delete", "--name", "etl-queue"), "pass --yes"},
		{"delete naming no queue", wqArgs("worker-queue", "delete", "--yes"), "pass --name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An answer is waiting, so a prompt that did read would go on.
			r := execAstroCmd(t, wqMock(t), "y\n1\nreports\n", newDeploymentRootCmd, append([]string{"deployment"}, append(tc.args, "-o", "json")...)...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, tc.answered)
		})
	}
}

// A queue change in a Workspace with no Deployments used to print that there
// were none and exit 0, having done nothing. It now fails, exit 1, in both
// formats.
func TestDeploymentWorkerQueueWithNoDeploymentFails(t *testing.T) {
	empty := func() *astrov1_mocks.ClientWithResponsesInterface {
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{}},
		}, nil)
		return m
	}
	for _, args := range [][]string{
		{"worker-queue", "create", "--name", "reports", "--worker-type", "a5"},
		{"worker-queue", "update", "--name", "etl-queue", "--yes"},
		{"worker-queue", "delete", "--name", "etl-queue", "--yes"},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(strings.Join(args[:2], " ")+" "+format, func(t *testing.T) {
				r := execAstroCmd(t, empty(), "", newDeploymentRootCmd, append([]string{"deployment"}, append(args, "--workspace-id", "ws-empty", "-o", format)...)...)
				assert.Equal(t, cliout.ExitFailure, r.code)
				require.Error(t, r.err)
				assert.Contains(t, r.err.Error(), "no Deployments found in workspace ws-empty")
				if format == "text" {
					assert.Empty(t, r.stdout)
				}
			})
		}
	}
}

// --output takes text or json: anything else is a usage error, exit 2, before
// anything is asked or done.
func TestDeploymentWorkerQueueAndLogsOutputUsage(t *testing.T) {
	for _, args := range [][]string{
		wqArgs("worker-queue", "create", "--name", "reports", "-o", "yaml"),
		wqArgs("worker-queue", "update", "--name", "etl-queue", "-o", "yaml"),
		wqArgs("worker-queue", "delete", "--name", "etl-queue", "-o", "yaml"),
		{"logs", wqDeploymentID, "-o", "yaml"},
	} {
		r := execAstroCmd(t, new(astrov1_mocks.ClientWithResponsesInterface), "", newDeploymentRootCmd, append([]string{"deployment"}, args...)...)
		require.Error(t, r.err, args)
		assert.Equal(t, cliout.ExitUsage, r.code, args)
	}
}

// A hybrid KubernetesExecutor queue has no counts of its own (its pods are
// sized by the worker type), so create and update publish the same: 0 for
// each, never the CLI's -1 "unset" that the create request carries.
func TestDeploymentWorkerQueueKubernetesCounts(t *testing.T) {
	// KubernetesExecutor takes only the default queue: create makes it on a
	// Deployment that has none, and update changes it.
	k8s := func(queues ...astrov1.WorkerQueue) astrov1.Deployment {
		d := wqDeployment()
		d.Type = wqPtr(astrov1.DeploymentTypeHYBRID)
		d.Executor = wqPtr(astrov1.DeploymentExecutorKUBERNETES)
		d.ClusterId = wqPtr(csID)
		d.SchedulerAu, d.SchedulerReplicas = wqPtr(10), 1
		d.WorkerQueues = &queues
		return d
	}
	run := func(t *testing.T, d astrov1.Deployment, args ...string) wqResultJSON {
		t.Helper()
		m := new(astrov1_mocks.ClientWithResponsesInterface)
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&astrov1.ListDeploymentsResponse{
			HTTPResponse: &http.Response{StatusCode: http.StatusOK},
			JSON200:      &astrov1.DeploymentsPaginated{Deployments: []astrov1.Deployment{d}, TotalCount: 1, Limit: 1000},
		}, nil).Maybe()
		m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID).Return(&astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &d}, nil).Maybe()
		// Options with no queue defaults, as a Kubernetes Deployment has.
		m.On("GetDeploymentOptionsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&GetDeploymentOptionsResponseAlphaOK, nil).Maybe()
		m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Maybe()
		m.On("UpdateDeploymentWithResponse", mock.Anything, mock.Anything, wqDeploymentID, mock.Anything).Return(&astrov1.UpdateDeploymentResponse{HTTPResponse: &http.Response{StatusCode: http.StatusOK}, JSON200: &d}, nil).Once()
		r := execAstroCmd(t, m, "", newDeploymentRootCmd, append([]string{"deployment"}, append(args, "-o", "json")...)...)
		require.NoError(t, r.err, r.stdout)
		var got wqResultJSON
		decodeOne(t, r.stdout, &got)
		return got
	}
	created := run(t, k8s(), wqArgs("worker-queue", "create", "--name", "default", "--worker-type", "test-worker-1")...)
	updated := run(t, k8s(astrov1.WorkerQueue{Id: "wq-default", Name: "default", IsDefault: true, NodePoolId: wqPtr("test-pool-id")}), wqArgs("worker-queue", "update", "--name", "default", "--worker-type", "test-worker-1", "--yes")...)
	counts := func(q wqQueueJSON) [3]int { return [3]int{q.MinWorkerCount, q.MaxWorkerCount, q.WorkerConcurrency} }
	assert.Equal(t, [3]int{0, 0, 0}, counts(created.WorkerQueue), "create")
	assert.Equal(t, counts(updated.WorkerQueue), counts(created.WorkerQueue), "create and update publish the same counts")
	assert.Equal(t, "test-worker-1", created.WorkerQueue.WorkerType)
}
