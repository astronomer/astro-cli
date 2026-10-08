package astro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
)

// A --wait writes its progress to stderr, in text as under --output json, so
// stdout is the command's result and nothing else: in text, exactly what the
// same command prints without --wait; in json, the one object.
func TestDeploymentWaitProgressGoesToStderr(t *testing.T) {
	origSleep, origTick := deployment.SleepTime, deployment.TickNum
	deployment.SleepTime, deployment.TickNum = 0, 1
	t.Cleanup(func() { deployment.SleepTime, deployment.TickNum = origSleep, origTick })

	// healthy is what the wait reads back: healthy, with no Airflow API URL,
	// so there is no Airflow to ask whether it answers.
	healthy := coreDeployment()
	healthy.WebServerAirflowApiUrl = ""
	hibernating := devDeployment()
	hibernating.Status = astrov1.DeploymentStatusHIBERNATING
	made := cloneMade()
	made.WebServerAirflowApiUrl = ""

	cases := []struct {
		name     string
		client   func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface
		args     []string
		progress []string
	}{
		{
			name: "create",
			client: func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
				m := coreMock(t, healthy)
				expectCreate(m, coreDeployment())
				return m
			},
			args: createArgs,
			progress: []string{
				"Current Workspace: test-workspace",
				"Waiting for the deployment to become healthy…", "This may take a few minutes",
				"Deployment etl-prod is now healthy",
			},
		},
		{
			name:   "create --clone",
			client: func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface { return cloneMock(t, nil, made) },
			args:   []string{"create", "--clone", cloneSourceID, "--name", "etl-preview"},
			progress: []string{
				"Waiting for the deployment to become healthy…", "This may take a few minutes",
				"Deployment etl-preview is now healthy",
			},
		},
		{
			name: "hibernate",
			client: func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
				m := coreMock(t, hibernating)
				expectOverride(m, true, nil)
				return m
			},
			args:     []string{"hibernate", coreDeploymentID, "--yes"},
			progress: []string{"Waiting for the Deployment to hibernate…", "Deployment etl-dev is now hibernating"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+" text", func(t *testing.T) {
			without := execDeploymentRun(t, tc.client(t), "", tc.args...)
			require.NoError(t, without.err)
			require.NotEmpty(t, without.stdout)

			m := tc.client(t)
			r := execDeploymentRun(t, m, "", append(tc.args, "--wait", "--wait-time", "10s")...)
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			assert.Equal(t, without.stdout, r.stdout, "stdout is the result, the same with --wait as without")
			requireInOrder(t, r.stderr, tc.progress...)
			m.AssertExpectations(t)
		})
		t.Run(tc.name+" json", func(t *testing.T) {
			m := tc.client(t)
			r := execDeploymentRun(t, m, "", append(tc.args, "--wait", "--wait-time", "10s", "-o", "json")...)
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			var got map[string]any
			decodeOne(t, r.stdout, &got)
			requireInOrder(t, r.stderr, tc.progress...)
			m.AssertExpectations(t)
		})
	}
}

// An update's warnings and notes go to stderr, in text as under --output
// json; the question they lead up to is asked where pkg/input asks, and
// stdout ends with the result.
func TestDeploymentUpdateWarningsGoToStderr(t *testing.T) {
	// The warning CI/CD enforcement adds is pinned in the deployment
	// package, which can stand in for the token check.
	dagDeployOff := coreDeployment()
	dagDeployOff.IsDagDeployEnabled = false
	cases := []struct {
		name      string
		dep       astrov1.Deployment
		dagDeploy string
		warnings  []string
		stdout    []string
	}{
		{
			name:      "enable DAG deploys",
			dep:       dagDeployOff,
			dagDeploy: "enable",
			warnings:  []string{"You enabled Dag-only deploys for this Deployment."},
			stdout:    []string{"Successfully updated Deployment"},
		},
		{
			name:      "disable DAG deploys",
			dep:       coreDeployment(),
			dagDeploy: "disable",
			warnings:  []string{"Warning: This command will disable Dag-only deploys for this Deployment."},
			stdout:    []string{"Are you sure you want to update the", "Successfully updated Deployment"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := coreMock(t, tc.dep)
			expectUpdate(m, coreDeployment())
			r := execDeploymentRun(t, m, "y\n", "update", coreDeploymentID, "--dag-deploy", tc.dagDeploy)
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			requireInOrder(t, r.stderr, tc.warnings...)
			for _, w := range tc.warnings {
				assert.NotContains(t, r.stdout, w)
			}
			requireInOrder(t, r.terminal(), tc.stdout...)
			m.AssertExpectations(t)
		})
	}
}
