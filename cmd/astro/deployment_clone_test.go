package astro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// `astro deployment create --clone`. The request it builds from each kind of
// source is pinned in internal/platform/astro/deployment/clone; these tests
// cover the command: which flags it takes, how it finds the source, and what
// it prints.

// cloneSourceID is a CUID, so --clone reads it as an id.
const cloneSourceID = "clh1rai0g000008l50d5hahbc"

// cloneSrc is the Deployment copied: coreDeployment, under a CUID, with a
// secret variable a copy cannot carry.
func cloneSrc() astrov1.Deployment {
	d := coreDeployment()
	d.Id = cloneSourceID
	d.EnvironmentVariables = &[]astrov1.DeploymentEnvironmentVariable{
		{Key: "PLAIN", Value: corePtr("1")},
		{Key: "API_KEY", IsSecret: true},
	}
	return d
}

// cloneMade is the Deployment the create returns: coreDeployment, under the
// new name.
func cloneMade() astrov1.Deployment {
	d := coreDeployment()
	d.Name = "etl-preview"
	return d
}

// cloneMock answers the reads a clone makes: the source by id, the
// Workspace's Deployments, the new Deployment by id, and its cluster. The
// create records the request it was sent in sent.
func cloneMock(t *testing.T, sent *astrov1.CreateDeploymentRequest, made astrov1.Deployment, workspace ...astrov1.Deployment) *astrov1_mocks.ClientWithResponsesInterface { //nolint:gocritic // a test fixture
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	src := cloneSrc()
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, cloneSourceID).Return(getDeploymentResp(src), nil).Maybe()
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, made.Id).Return(getDeploymentResp(made), nil).Maybe()
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listDeploymentsResp(workspace...), nil).Maybe()
	m.On("GetClusterWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(&mockGetClusterResponse, nil).Maybe()
	m.On("CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if sent != nil {
				*sent = args.Get(2).(astrov1.CreateDeploymentRequest)
			}
		}).
		Return(&astrov1.CreateDeploymentResponse{HTTPResponse: ok200(), JSON200: &made}, nil).Once()
	return m
}

// sentFields decodes the create request's body.
func sentFields(t *testing.T, req *astrov1.CreateDeploymentRequest) map[string]any {
	t.Helper()
	b, err := json.Marshal(req)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	return got
}

const secretNote = "Note: Not copied: secret environment variables API_KEY."

// Invoked wrongly, --clone is a usage error, exit 2, before anything is read:
// the client mocks nothing, so a call would panic.
func TestDeploymentCloneUsage(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no --name":           {[]string{"--clone", cloneSourceID}, "--clone needs --name"},
		"no source":           {[]string{"--clone", "", "--name", "x"}, "--clone needs a Deployment id or name"},
		"a flag it copies":    {[]string{"--clone", cloneSourceID, "--name", "x", "--runtime-version", "12.1.0"}, "cannot be used with --runtime-version"},
		"a cluster":           {[]string{"--clone", cloneSourceID, "--name", "x", "--cluster-id", csID, "--dag-deploy", "enable"}, "cannot be used with --cluster-id, --dag-deploy"},
		"an unanswered --yes": {[]string{"--clone", cloneSourceID, "--name", "x", "--yes"}, "cannot be used with --yes"},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(name+" "+format, func(t *testing.T) {
				r := execDeploymentRun(t, new(astrov1_mocks.ClientWithResponsesInterface), "", append(append([]string{"create"}, tc.args...), "-o", format)...)
				require.Error(t, r.err)
				assert.Equal(t, cliout.ExitUsage, r.code)
				assert.Contains(t, r.err.Error(), tc.want)
				if format == "json" {
					var got errorJSON
					decodeOne(t, r.stdout, &got)
					assert.Equal(t, string(cliout.KindUsage), got.Kind)
					assert.Equal(t, cliout.ExitUsage, got.Code)
				}
			})
		}
	}
}

// A name has to name one Deployment in the Workspace. None, or several, fails
// the run, exit 1, having created nothing; several are listed by id rather
// than one being taken.
func TestDeploymentCloneSourceByName(t *testing.T) {
	other := cloneSrc()
	other.Id = "clh1rai0g000008l50d5hahbd"
	unrelated := coreDeployment()
	unrelated.Name = "something-else"

	t.Run("several share the name", func(t *testing.T) {
		m := cloneMock(t, nil, cloneMade(), cloneSrc(), other, unrelated)
		r := execDeploymentRun(t, m, "", "create", "--clone", "etl-prod", "--name", "etl-preview", "--workspace-id", "workspace-id", "-o", "json")
		assert.Equal(t, cliout.ExitFailure, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Contains(t, got.Error, `2 Deployments are named "etl-prod"`)
		assert.Contains(t, got.Error, cloneSourceID)
		assert.Contains(t, got.Error, other.Id)
		m.AssertNotCalled(t, "CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("none has it", func(t *testing.T) {
		m := cloneMock(t, nil, cloneMade(), unrelated)
		r := execDeploymentRun(t, m, "", "create", "--clone", "etl-prod", "--name", "etl-preview", "--workspace-id", "workspace-id")
		assert.Equal(t, cliout.ExitFailure, r.code)
		require.Error(t, r.err)
		assert.Contains(t, r.err.Error(), `no Deployment named "etl-prod" in Workspace workspace-id`)
		assert.Empty(t, r.stdout)
		m.AssertNotCalled(t, "CreateDeploymentWithResponse", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("one has it", func(t *testing.T) {
		var sent astrov1.CreateDeploymentRequest
		m := cloneMock(t, &sent, cloneMade(), cloneSrc(), unrelated)
		r := execDeploymentRun(t, m, "", "create", "--clone", "etl-prod", "--name", "etl-preview", "--workspace-id", "workspace-id", "-o", "json")
		require.NoError(t, r.err)
		assert.Equal(t, "etl-preview", sentFields(t, &sent)["name"])
		m.AssertExpectations(t)
	})
}

// Under -o json the run publishes the new Deployment as inspect shows it, the
// one object on stdout; what the copy could not carry goes to stderr.
func TestDeploymentCloneJSON(t *testing.T) {
	var sent astrov1.CreateDeploymentRequest
	m := cloneMock(t, &sent, cloneMade())
	r := execDeploymentRun(t, m, "", "create", "--clone", cloneSourceID, "--name", "etl-preview", "--description", "Preview", "-o", "json")
	require.NoError(t, r.err)

	var got formattedJSON
	decodeOne(t, r.stdout, &got)
	assert.Equal(t, coreDeploymentID, got.Deployment.Metadata.DeploymentID)
	assert.Equal(t, "etl-preview", got.Deployment.Configuration.Name)
	assert.Equal(t, "workspace-id", got.Deployment.Metadata.WorkspaceID)
	assert.Contains(t, r.stderr, secretNote)

	body := sentFields(t, &sent)
	assert.Equal(t, "HYBRID", body["type"])
	assert.Equal(t, "workspace-id", body["workspaceId"], "the source's Workspace, with no --workspace")
	assert.Equal(t, "Preview", body["description"])
	assert.Equal(t, csID, body["clusterId"])
	m.AssertExpectations(t)
}

// --workspace puts the copy in another Workspace; an id is read wherever it is.
func TestDeploymentCloneIntoAnotherWorkspace(t *testing.T) {
	var sent astrov1.CreateDeploymentRequest
	m := cloneMock(t, &sent, cloneMade())
	r := execDeploymentRun(t, m, "", "create", "--clone", cloneSourceID, "--name", "etl-preview", "--workspace", "ws-preview", "-o", "json")
	require.NoError(t, r.err)
	assert.Equal(t, "ws-preview", sentFields(t, &sent)["workspaceId"])
}

// In text the run prints what a plain create prints: the new Deployment's row
// and where to reach it.
func TestDeploymentCloneText(t *testing.T) {
	m := cloneMock(t, nil, cloneMade())
	r := execDeploymentRun(t, m, "", "create", "--clone", cloneSourceID, "--name", "etl-preview")
	require.NoError(t, r.err)
	assert.Equal(t, []map[string]string{deploymentRow("etl-preview")}, tableRows(t, r.stdout, "NAME"))
	requireInOrder(t, r.stdout,
		"Successfully created Deployment: ", "etl-preview",
		"Deployment can be accessed at the following URLs",
		"Deployment Dashboard: ", coreDeploymentID,
		"Airflow Dashboard: ", "etl-prod.astronomer.run/d1234")
	assert.Contains(t, r.stderr, secretNote)
	assert.NotContains(t, r.stdout, "Not copied", "notes are not the result")
}

// A --wait that runs out fails the run, exit 1, after the copy was made, and
// the copy is still published: its id is what a script needs to wait again
// or clean up.
func TestDeploymentCloneWaitThatRunsOut(t *testing.T) {
	origSleep, origTick := deployment.SleepTime, deployment.TickNum
	deployment.SleepTime, deployment.TickNum = 0, 1
	t.Cleanup(func() { deployment.SleepTime, deployment.TickNum = origSleep, origTick })

	deploying := cloneMade()
	deploying.Status = astrov1.DeploymentStatusDEPLOYING
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			m := cloneMock(t, nil, deploying)
			r := execDeploymentRun(t, m, "", "create", "--clone", cloneSourceID, "--name", "etl-preview", "--wait", "--wait-time", "1s", "-o", format)
			assert.Equal(t, cliout.ExitFailure, r.code)
			assert.ErrorIs(t, r.err, deployment.ErrTimedOut)
			if format == "text" {
				assert.Equal(t, []map[string]string{deploymentRow("etl-preview")}, tableRows(t, r.stdout, "NAME"))
				return
			}
			var got formattedJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, coreDeploymentID, got.Deployment.Metadata.DeploymentID)
			assert.Equal(t, "DEPLOYING", got.Deployment.Metadata.Status)
			assert.Contains(t, r.stderr, "Error: "+deployment.ErrTimedOut.Error())
		})
	}
}
