package apc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	"github.com/astronomer/astro-cli/pkg/input"
)

// apcAt is newAPCClient on a platform at version, with the dag_deploy feature
// on: the create and update commands then take --dag-deployment-type.
func apcAt(version string) *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	api.On("GetPlatformVersion", mock.Anything).Return(version, nil).Maybe()
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: version, Flags: houston.FeatureFlags{
		AstroRuntimeEnabled: true, TriggererEnabled: true, DagOnlyDeployment: true,
	}}, nil).Maybe()
	return api
}

// A confirmation of the deployment commands of Astro Private Cloud that
// stops when it is declined, each one a "do you want to continue?" or "are
// you sure?" and none a choice between two things to do: no, for each, is
// "do nothing", so it is input.ErrAborted.
type apcConfirmCase struct {
	name string
	// version is what the platform reports; the commands that a version
	// gates are taken out as the root takes them out.
	version  string
	args     []string
	question string
	// client is the platform for this command; mutates is the Houston call a
	// yes makes and a no must not.
	client  func() *mocks.ClientInterface
	mutates string
}

func apcConfirmCases() []apcConfirmCase {
	dagImage := &houston.Deployment{ID: "dep-1", Workspace: houston.Workspace{ID: "ws-1"}, DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType}}
	dagOnly := &houston.Deployment{ID: "dep-1", Workspace: houston.Workspace{ID: "ws-1"}, DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}
	removed := &houston.Deployment{ID: "dep-1", Label: "alpha", ReleaseName: "rel-1"}
	return []apcConfirmCase{
		{
			name: "deployment delete", version: "2.1.0",
			args:     []string{"deployment", "delete", "dep-1"},
			question: "Proceed with delete?",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.1.0")
				api.On("DeleteDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "DeleteDeployment",
		},
		{
			name: "deployment delete, on the oldest platform with the hard-delete question", version: "0.25.0",
			args:     []string{"deployment", "delete", "dep-1"},
			question: "Proceed with delete?",
			client: func() *mocks.ClientInterface {
				api := apcAt("0.25.0")
				api.On("DeleteDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "DeleteDeployment",
		},
		{
			name: "deployment unadopt", version: "2.1.0",
			args:     []string{"deployment", "unadopt", "--deployment", "dep-1"},
			question: "Proceed with unadopt?",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.1.0")
				api.On("UnadoptDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "UnadoptDeployment",
		},
		{
			name: "deployment unadopt, on a later platform", version: "2.2.1",
			args:     []string{"deployment", "unadopt", "--deployment", "dep-1"},
			question: "Proceed with unadopt?",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.2.1")
				api.On("UnadoptDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "UnadoptDeployment",
		},
		{
			name: "deployment hibernate", version: "2.2.1",
			args:     []string{"deployment", "hibernate", "dep-hb"},
			question: "Are you sure you want to hibernate this Deployment?",
			client:   func() *mocks.ClientInterface { return newHibernationClient(withOverride(true, nil)) },
			mutates:  "HibernateOverrideDeployment",
		},
		{
			name: "deployment wake-up", version: "2.2.1",
			args:     []string{"deployment", "wake-up", "dep-hb"},
			question: "Are you sure you want to wake up this Deployment?",
			client:   func() *mocks.ClientInterface { return newHibernationClient(withOverride(false, nil)) },
			mutates:  "HibernateOverrideDeployment",
		},
		{
			name: "deployment wake-up --remove-override", version: "2.2.1",
			args:     []string{"deployment", "wake-up", "dep-hb", "--remove-override"},
			question: "Are you sure you want to remove the hibernation override on this Deployment?",
			client:   func() *mocks.ClientInterface { return newHibernationClient(withOverride(false, nil)) },
			mutates:  "HibernateOverrideDeployment",
		},
		{
			name: "deployment hibernate --remove-override", version: "2.2.1",
			args:     []string{"deployment", "hibernate", "dep-hb", "--remove-override"},
			question: "Are you sure you want to remove the hibernation override on this Deployment?",
			client:   func() *mocks.ClientInterface { return newHibernationClient(withOverride(true, nil)) },
			mutates:  "HibernateOverrideDeployment",
		},
		{
			name: "deployment create as dag_deploy", version: "2.1.0",
			args:     []string{"deployment", "create", "--cluster-id", "cl-1", "--name", "alpha", "--dag-deployment-type", "dag_deploy"},
			question: "this is an experimental feature",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.1.0")
				api.On("CreateDeployment", mock.Anything).Return(&runtimeDep, nil).Maybe()
				return api
			},
			mutates: "CreateDeployment",
		},
		{
			name: "deployment update onto dag_deploy", version: "2.1.0",
			args:     []string{"deployment", "update", "dep-1", "--dag-deployment-type", "dag_deploy"},
			question: "Changing to a Dag-only Deployment will erase all of the currently deployed Dags",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.1.0")
				api.On("GetDeployment", "dep-1").Return(dagImage, nil)
				api.On("UpdateDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "UpdateDeployment",
		},
		{
			name: "deployment update off dag_deploy", version: "2.1.0",
			args:     []string{"deployment", "update", "dep-1", "--dag-deployment-type", "image"},
			question: "changing from a Dag-only deployment will erase all of the currently deployed Dags",
			client: func() *mocks.ClientInterface {
				api := apcAt("2.1.0")
				api.On("GetDeployment", "dep-1").Return(dagOnly, nil)
				api.On("UpdateDeployment", mock.Anything).Return(removed, nil).Maybe()
				return api
			},
			mutates: "UpdateDeployment",
		},
	}
}

// A no stops the command: exit 1, kind aborted, nothing on stdout, nothing
// changed on the platform, and the question was asked (on stderr).
func TestDeploymentConfirmationsDeclined(t *testing.T) {
	for _, c := range apcConfirmCases() {
		for _, answer := range []string{"n\n", "no\n", "\n", "maybe\n"} {
			t.Run(c.name+" answered "+answer, func(t *testing.T) {
				api := c.client()
				run := runAPCGated(t, api, true, answer, c.args...)

				assert.Equal(t, 1, run.code, "stderr:\n%s", run.stderr)
				assert.Empty(t, run.stdout)
				require.ErrorIs(t, run.err, input.ErrAborted)
				assert.Equal(t, cliout.KindAborted, cliout.Kinds(nil).Of(run.err))
				assert.Contains(t, run.stderr, c.question, "the question was asked")
				api.AssertNotCalled(t, c.mutates, mock.Anything)
			})
		}
	}
}

// A yes, in any case, goes on.
func TestDeploymentConfirmationsAccepted(t *testing.T) {
	for _, c := range apcConfirmCases() {
		for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n"} {
			t.Run(c.name+" answered "+answer, func(t *testing.T) {
				api := c.client()
				run := runAPCGated(t, api, true, answer, c.args...)

				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				api.AssertCalled(t, c.mutates, mock.Anything)
			})
		}
	}
}

// Input that ends before any answer is nobody saying no: it fails
// input_required, naming --yes, and changes nothing.
func TestDeploymentConfirmationsEndedInput(t *testing.T) {
	for _, c := range apcConfirmCases() {
		t.Run(c.name, func(t *testing.T) {
			api := c.client()
			run := runAPCGated(t, api, true, "", c.args...)

			assert.Equal(t, 1, run.code, "stderr:\n%s", run.stderr)
			assert.Empty(t, run.stdout)
			require.Error(t, run.err)
			assert.True(t, input.IsRequired(run.err), "%v", run.err)
			assert.NotErrorIs(t, run.err, input.ErrAborted)
			assert.Equal(t, cliout.KindInputRequired, cliout.Kinds(nil).Of(run.err))
			assert.Contains(t, run.err.Error(), "--yes")
			api.AssertNotCalled(t, c.mutates, mock.Anything)
		})
	}
}

// --yes and -y answer it: the question is not asked, and nothing is read.
func TestDeploymentConfirmationsSkippedByYes(t *testing.T) {
	for _, c := range apcConfirmCases() {
		for _, yes := range []string{"--yes", "-y"} {
			t.Run(c.name+" "+yes, func(t *testing.T) {
				api := c.client()
				run := runAPCGated(t, api, true, "n\n", append(append([]string{}, c.args...), yes)...)

				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				assert.NotContains(t, run.stderr, c.question)
				assert.False(t, run.stdinRead, "--yes left the question to stdin")
				api.AssertCalled(t, c.mutates, mock.Anything)
			})
		}
	}
}

// Under --output json nothing is asked: the question is refused as
// input_required naming --yes, before anything is read, and the error is the
// only thing on stdout.
func TestDeploymentConfirmationsRefusedUnderJSON(t *testing.T) {
	for _, c := range apcConfirmCases() {
		t.Run(c.name, func(t *testing.T) {
			api := c.client()
			run := runAPCGated(t, api, true, "y\n", append(append([]string{}, c.args...), "-o", "json")...)

			assert.Equal(t, 1, run.code)
			var got struct {
				Error string `json:"error"`
				Kind  string `json:"kind"`
			}
			decodeOne(t, run.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, "--yes")
			assert.False(t, run.stdinRead, "a refused prompt read stdin")
			api.AssertNotCalled(t, c.mutates, mock.Anything)
		})
	}
}

// A no to a question the command has to ask on a platform that serves it
// only from a version is the version's business first: before the version
// that adds unadopt it is refused, and asks nothing.
func TestUnadoptBeforeItsVersionAsksNothing(t *testing.T) {
	api := apcAt("2.0.9")
	run := runAPCGated(t, api, true, "n\n", "deployment", "unadopt", "--deployment", "dep-1")

	assert.Equal(t, 1, run.code)
	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "astro deployment unadopt needs Astro Private Cloud 2.1.0 or newer")
	assert.NotErrorIs(t, run.err, input.ErrAborted)
	assert.NotContains(t, run.stderr, "Proceed with unadopt?")
	assert.False(t, run.stdinRead)
	api.AssertNotCalled(t, "UnadoptDeployment", mock.Anything)
}

// Hibernation is served from 2.2.0, and a platform before it is refused
// before the question, not after a yes.
func TestHibernationBeforeItsVersionAsksNothing(t *testing.T) {
	api := newAPCClient()
	api.On("GetDeployment", "dep-hb").Return(&hibernatingDep, nil)
	api.On("GetAppConfig", mock.Anything).Unset()
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.27"}, nil)

	run := runAPCGated(t, api, true, "n\n", "deployment", "hibernate", "dep-hb")

	assert.Equal(t, 1, run.code)
	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "needs Astro Private Cloud 2.2.0 or later")
	assert.NotErrorIs(t, run.err, input.ErrAborted)
	assert.False(t, run.stdinRead)
	api.AssertNotCalled(t, "HibernateOverrideDeployment", mock.Anything)
}

// A create or update that moves nothing onto or off dag_deploy has no
// question to decline.
func TestDagDeployQuestionOnlyForDagDeploy(t *testing.T) {
	t.Run("create as image", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("CreateDeployment", mock.Anything).Return(&runtimeDep, nil)
		run := runAPCGated(t, api, true, "", "deployment", "create", "--cluster-id", "cl-1", "--name", "alpha", "--dag-deployment-type", "image")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.False(t, run.stdinRead)
	})
	t.Run("update from image to image", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType}}, nil)
		api.On("UpdateDeployment", mock.Anything).Return(&certifiedDep, nil)
		run := runAPCGated(t, api, true, "", "deployment", "update", "dep-1", "--dag-deployment-type", "image")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.False(t, run.stdinRead)
	})
}
