package apc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
		AstroRuntimeEnabled: true, TriggererEnabled: true, DagOnlyDeployment: true, GitSyncEnabled: true,
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

// An update that names no type does not touch the type, so a dag_deploy
// Deployment is renamed or resized without a question about erasing its Dags.
func TestUpdateWithoutATypeAsksNothingOnDagDeploy(t *testing.T) {
	api := apcAt("2.1.0")
	api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}, nil)
	api.On("UpdateDeployment", mock.Anything).Return(&certifiedDep, nil)

	run := runAPCGated(t, api, true, "", "deployment", "update", "dep-1", "--name", "renamed")

	require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
	assert.False(t, run.stdinRead)
	assert.NotContains(t, run.stderr, "erase all of the currently deployed Dags")
	api.AssertCalled(t, "UpdateDeployment", mock.Anything)
}

// What was asked for is checked before the question: a line that is going to
// be refused asks nothing.
func TestDagDeployQuestionFollowsValidation(t *testing.T) {
	t.Run("update with an executor that is not one", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.ImageDeploymentType}}, nil)

		run := runAPCGated(t, api, true, "y\n", "deployment", "update", "dep-1", "--dag-deployment-type", "dag_deploy", "--executor", "bogus")

		assert.Equal(t, 1, run.code)
		require.ErrorIs(t, run.err, errInvalidExecutorType)
		assert.False(t, run.stdinRead)
		assert.NotContains(t, run.stderr, "experimental feature")
		api.AssertNotCalled(t, "UpdateDeployment", mock.Anything)
	})
	t.Run("update to git_sync with an ssh key that is not there", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}, nil)

		run := runAPCGated(t, api, true, "y\n", "deployment", "update", "dep-1", "--dag-deployment-type", "git_sync",
			"--git-repository-url", "git@example.com:o/r.git", "--ssh-key", filepath.Join(t.TempDir(), "missing"))

		assert.Equal(t, 1, run.code)
		require.Error(t, run.err)
		assert.NotErrorIs(t, run.err, input.ErrAborted)
		assert.False(t, run.stdinRead, "asked before the key was read")
		assert.NotContains(t, run.stderr, "erase all of the currently deployed Dags")
		api.AssertNotCalled(t, "UpdateDeployment", mock.Anything)
	})
	t.Run("update to volume with no mount", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}, nil)

		run := runAPCGated(t, api, true, "y\n", "deployment", "update", "dep-1", "--dag-deployment-type", "volume")

		assert.Equal(t, 1, run.code)
		require.Error(t, run.err)
		assert.NotErrorIs(t, run.err, input.ErrAborted)
		assert.False(t, run.stdinRead)
		api.AssertNotCalled(t, "UpdateDeployment", mock.Anything)
	})
}

// A picker is a question: it is drawn on stderr, so a command whose stdout is
// redirected to a file captures no table and no prompt.
func TestPickersAreNotDrawnOnStdout(t *testing.T) {
	t.Run("create asks for a namespace", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetPlatformVersion", mock.Anything).Return("2.1.0", nil).Maybe()
		api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Version: "2.1.0", Flags: houston.FeatureFlags{
			AstroRuntimeEnabled: true, TriggererEnabled: true, ManualNamespaceNames: true,
		}}, nil).Maybe()
		api.On("GetAvailableNamespaces", mock.Anything).Return([]houston.Namespace{{Name: "ns-a"}, {Name: "ns-b"}}, nil)
		api.On("CreateDeployment", mock.MatchedBy(func(v map[string]interface{}) bool { return v["namespace"] == "ns-b" })).Return(&runtimeDep, nil)

		run := runAPCGated(t, api, true, "2\n", "deployment", "create", "--cluster-id", "cl-1", "--name", "alpha")

		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stderr, "AVAILABLE KUBERNETES NAMESPACES")
		assert.NotContains(t, run.stdout, "AVAILABLE KUBERNETES NAMESPACES")
		assert.NotContains(t, run.stdout, "ns-a")
		assert.NotContains(t, run.stdout, "> ")
		api.AssertCalled(t, "CreateDeployment", mock.Anything)
	})
	t.Run("airflow upgrade asks for a version", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", AirflowVersion: "1.10.7"}, nil)
		api.On("GetDeploymentConfig", mock.Anything).Return(&houston.DeploymentConfig{AirflowVersions: []string{"1.10.7", "1.10.10", "1.10.12"}}, nil)
		api.On("UpdateDeploymentAirflow", mock.Anything).Return(&houston.Deployment{ID: "dep-1", AirflowVersion: "1.10.7", DesiredAirflowVersion: "1.10.12"}, nil)

		run := runAPCGated(t, api, true, "2\n", "deployment", "airflow", "upgrade", "--deployment", "dep-1")

		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stderr, "AIRFLOW VERSION")
		assert.NotContains(t, run.stdout, "AIRFLOW VERSION")
		assert.NotContains(t, run.stdout, "Astronomer-Certified-1.10.10")
	})
	t.Run("runtime upgrade asks for a version", func(t *testing.T) {
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", RuntimeVersion: "12.1.0", RuntimeAirflowVersion: "2.10.5"}, nil)
		api.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{
			{Version: "12.1.1", AirflowVersion: "2.10.5"},
			{Version: "12.2.0", AirflowVersion: "2.10.5"},
		}, nil)
		api.On("UpdateDeploymentRuntime", mock.Anything).Return(&houston.Deployment{ID: "dep-1", RuntimeVersion: "12.1.0"}, nil)

		run := runAPCGated(t, api, true, "2\n", "deployment", "runtime", "upgrade", "--deployment", "dep-1")

		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Contains(t, run.stderr, "RUNTIME VERSION")
		assert.NotContains(t, run.stdout, "RUNTIME VERSION")
		assert.NotContains(t, run.stdout, "Runtime-12.2.0")
	})
}

// oneShot returns a path that gives content once: a pipe, as a process
// substitution (--ssh-key <(cat key)) is. A second read of it gets nothing.
func oneShot(t *testing.T, content string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/fd on windows")
	}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	t.Cleanup(func() { r.Close() })
	return fmt.Sprintf("/dev/fd/%d", r.Fd())
}

// What the update reads from --ssh-key and --known-hosts is what it sends:
// each is read once, however the run goes, so a file that gives its content
// once (a pipe) reaches Houston whole after the question is answered.
func TestUpdateSendsWhatItReadOnce(t *testing.T) {
	update := func(t *testing.T, answers string, extra ...string) (apcRun, map[string]interface{}) {
		t.Helper()
		var sent map[string]interface{}
		api := apcAt("2.1.0")
		api.On("GetDeployment", "dep-1").Return(&houston.Deployment{ID: "dep-1", DagDeployment: houston.DagDeploymentConfig{Type: houston.DagOnlyDeploymentType}}, nil)
		api.On("UpdateDeployment", mock.Anything).Run(func(args mock.Arguments) {
			sent, _ = args.Get(0).(map[string]interface{})
		}).Return(&certifiedDep, nil).Maybe()
		args := append([]string{"deployment", "update", "dep-1", "--dag-deployment-type", "git_sync", "--git-repository-url", "git@example.com:o/r.git"}, extra...)
		return runAPCGated(t, api, true, answers, args...), sent
	}

	for _, c := range []struct {
		name    string
		answers string
		yes     []string
	}{
		{"after a yes", "y\n", nil},
		{"with --yes", "", []string{"--yes"}},
	} {
		t.Run("a key and known hosts from pipes "+c.name, func(t *testing.T) {
			key := oneShot(t, "KEY\n")
			hosts := oneShot(t, "example.com ssh-rsa AAAA\n")
			run, sent := update(t, c.answers, append([]string{"--ssh-key", key, "--known-hosts", hosts}, c.yes...)...)

			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			dag, ok := sent["dagDeployment"].(map[string]interface{})
			require.True(t, ok, "%v", sent)
			assert.Equal(t, "KEY\n", dag["sshKey"])
			assert.Equal(t, "example.com ssh-rsa AAAA", dag["knownHosts"])
		})
	}

	// Under --output json nothing is asked: the run is refused before any
	// file is read, so a bad path is not what it reports and a pipe is not
	// consumed.
	t.Run("-o json without --yes reads nothing", func(t *testing.T) {
		key := oneShot(t, "KEY\n")
		run, sent := update(t, "y\n", "--ssh-key", filepath.Join(t.TempDir(), "missing"), "--known-hosts", key, "-o", "json")

		assert.Equal(t, 1, run.code)
		var got struct {
			Error string `json:"error"`
			Kind  string `json:"kind"`
		}
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
		assert.Contains(t, got.Error, "--yes")
		assert.Nil(t, sent)
		b, err := os.ReadFile(key)
		require.NoError(t, err)
		assert.Equal(t, "KEY\n", string(b), "the pipe was not consumed")
	})
}
