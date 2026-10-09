package apc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// apcRun is one run of the APC tree the way main runs it: through
// cliout.Execute, with stdout and stderr apart and stdin under the test's
// control.
type apcRun struct {
	stdout, stderr string
	code           int
	// stdinRead is whether anything read stdin: a refused prompt must not.
	stdinRead bool
}

// newAPCClient is a Houston mock that answers what building the tree asks.
func newAPCClient() *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{
		AstroRuntimeEnabled: true, TriggererEnabled: true,
	}}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Maybe()
	return api
}

// runAPC builds the APC tree on a file standing in for stdout, and runs args.
func runAPC(t *testing.T, api houston.ClientInterface, answers string, args ...string) apcRun {
	t.Helper()
	testUtil.InitTestConfig(testUtil.SoftwarePlatform)
	houstonVersion = "1.0.0"
	// These flags are registered only when Houston's feature flags allow
	// them, so a tree built without them never resets what an earlier run
	// set: a dag_deploy type left behind would make the next create ask.
	dagDeploymentType, nfsLocation = "", ""
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	require.NoError(t, err)
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	require.NoError(t, err)
	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(answers)
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	prevOut, prevErr, prevIn := os.Stdout, os.Stderr, os.Stdin
	os.Stdout, os.Stderr, os.Stdin = stdout, stderr, inR
	t.Cleanup(func() { os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn })

	root := &cobra.Command{Use: "astro", SilenceErrors: true}
	root.AddCommand(AddCmds(api, stdout)...)
	root.SetOut(stdout)
	root.SetErr(stderr)
	ctx := context.Background()
	runErr := cliout.Execute(ctx, root, args, stdout, nil)
	os.Stdout, os.Stderr, os.Stdin = prevOut, prevErr, prevIn

	// Whatever is left in the pipe was not read.
	left, _ := io.ReadAll(inR)
	require.NoError(t, stdout.Close())
	require.NoError(t, stderr.Close())
	outBytes, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	errBytes, err := os.ReadFile(stderr.Name())
	require.NoError(t, err)
	return apcRun{
		stdout:    string(outBytes),
		stderr:    string(errBytes),
		code:      cliout.ExitCode(ctx, runErr),
		stdinRead: len(left) < len(answers),
	}
}

// decodeOne decodes stdout as exactly one json value into v.
func decodeOne(t *testing.T, stdout string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	require.NoError(t, dec.Decode(v), "stdout:\n%s", stdout)
	var extra json.RawMessage
	require.ErrorIs(t, dec.Decode(&extra), io.EOF, "more than one value on stdout:\n%s", stdout)
}

// decodeLines decodes stdout as one json object per line.
func decodeLines[T any](t *testing.T, stdout string) []T {
	t.Helper()
	var out []T
	sc := bufio.NewScanner(strings.NewReader(stdout))
	for sc.Scan() {
		var v T
		require.NoError(t, json.Unmarshal(sc.Bytes(), &v), "line %q", sc.Text())
		out = append(out, v)
	}
	return out
}

// errorObject is the failure cliout.Execute publishes under json.
type errorObject struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

var (
	certifiedDep = houston.Deployment{
		ID: "dep-ac", Label: "alpha", ReleaseName: "rel-ac", Version: "0.29.0", AirflowVersion: "2.0.0",
		Workspace: houston.Workspace{ID: "ws-1"}, DeploymentInfo: houston.DeploymentInfo{Current: "deploy-1"},
		DagDeployment: houston.DagDeploymentConfig{Type: "image"},
	}
	runtimeDep = houston.Deployment{
		ID: "dep-rt", Label: "beta", ReleaseName: "rel-rt", Version: "0.29.0", RuntimeVersion: "4.2.0", ClusterID: "cl-1",
		Urls: []houston.DeploymentURL{{Type: "airflow", URL: "https://airflow"}},
	}
)

func TestDeploymentListJSON(t *testing.T) {
	t.Run("the deployments, in the table's order", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeployments", mock.Anything).Return([]houston.Deployment{certifiedDep, runtimeDep}, nil)

		run := runAPC(t, api, "", "deployment", "list", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentListJSON
		decodeOne(t, run.stdout, &got)
		require.Len(t, got.Deployments, 2)
		assert.Equal(t, "beta", got.Deployments[0].Label, "ordered by label, last first, as the table is")
		assert.Equal(t, deploymentJSON{
			DeploymentID: "dep-ac", Label: "alpha", ReleaseName: "rel-ac", WorkspaceID: strp("ws-1"), ChartVersion: strp("0.29.0"),
			AirflowVersion: strp("2.0.0"), ImageTag: strp("deploy-1"), DagDeploymentType: strp("image"), URLs: []deploymentURLJSON{},
		}, got.Deployments[1])
		assert.Equal(t, strp("4.2.0"), got.Deployments[0].RuntimeVersion)
		assert.Nil(t, got.Deployments[0].AirflowVersion, "a Runtime Deployment has no Airflow version: null, not \"\"")
		assert.Nil(t, got.Deployments[1].ClusterID, "a value Houston did not give is null")
		assert.Equal(t, []deploymentURLJSON{{Type: "airflow", URL: "https://airflow"}}, got.Deployments[0].URLs)
		assert.Empty(t, run.stderr)
	})

	t.Run("none is an empty list", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeployments", mock.Anything).Return([]houston.Deployment{}, nil)

		run := runAPC(t, api, "", "deployment", "list", "-o", "json")
		require.Equal(t, 0, run.code)
		assert.JSONEq(t, `{"deployments":[]}`, run.stdout)
	})

	t.Run("a failure is the error object", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeployments", mock.Anything).Return(nil, errors.New("houston is down"))

		run := runAPC(t, api, "", "deployment", "list", "-o", "json")
		assert.Equal(t, 1, run.code)
		var got errorObject
		decodeOne(t, run.stdout, &got)
		assert.Contains(t, got.Error, "houston is down")
	})
}

func TestDeploymentCreateUpdateAdoptJSON(t *testing.T) {
	t.Run("create publishes the Deployment", func(t *testing.T) {
		api := newAPCClient()
		api.On("CreateDeployment", mock.Anything).Return(&runtimeDep, nil)

		run := runAPC(t, api, "", "deployment", "create", "--label", "beta", "--cluster-id", "cl-1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "dep-rt", got.DeploymentID)
		assert.Equal(t, strp("cl-1"), got.ClusterID)
		assert.Equal(t, []deploymentURLJSON{{Type: "airflow", URL: "https://airflow"}}, got.URLs)
		assert.NotContains(t, run.stdout, "Successfully created")
	})

	// Where the platform asks for a namespace, --namespace answers it, and
	// without it the run refuses, naming it.
	t.Run("create where the platform picks namespaces", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{ManualNamespaceNames: true}}, nil)
		api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil)
		api.On("GetAvailableNamespaces", mock.Anything).Return([]houston.Namespace{{Name: "ns-a"}}, nil)
		api.On("CreateDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool { return vars["namespace"] == "ns-a" })).Return(&runtimeDep, nil)

		run := runAPC(t, api, "1\n", "deployment", "create", "--label", "beta", "--cluster-id", "cl-1", "-o", "json")
		assert.Equal(t, 1, run.code)
		var refused errorObject
		decodeOne(t, run.stdout, &refused)
		assert.Contains(t, refused.Error, "--namespace")
		assert.False(t, run.stdinRead)

		run = runAPC(t, api, "", "deployment", "create", "--label", "beta", "--cluster-id", "cl-1", "--namespace", "ns-a", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "dep-rt", got.DeploymentID)
	})

	// Namespace management is resolved per cluster and workspace. Off by
	// default, on for this cluster: --namespace is judged by the cluster's
	// settings.
	t.Run("create reads the cluster's settings for --namespace", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil)
		api.On("GetAppConfig", houston.GetAppConfigRequest{}).Return(&houston.AppConfig{}, nil)
		api.On("GetAppConfig", mock.MatchedBy(func(r houston.GetAppConfigRequest) bool { return r.ClusterID == "cl-1" })).
			Return(&houston.AppConfig{Flags: houston.FeatureFlags{NamespaceFreeFormEntry: true}}, nil)
		api.On("CreateDeployment", mock.MatchedBy(func(vars map[string]interface{}) bool { return vars["namespace"] == "my-ns" })).Return(&runtimeDep, nil)

		run := runAPC(t, api, "", "deployment", "create", "--label", "beta", "--cluster-id", "cl-1", "--namespace", "my-ns", "-o", "json")
		require.Equal(t, 0, run.code, "stdout:\n%s", run.stdout)
	})

	// A failed settings lookup used to be dropped, and create read settings
	// that were not there.
	t.Run("create reports a failed settings lookup", func(t *testing.T) {
		api := new(mocks.ClientInterface)
		api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil)
		api.On("GetAppConfig", houston.GetAppConfigRequest{}).Return(&houston.AppConfig{}, nil)
		api.On("GetAppConfig", mock.MatchedBy(func(r houston.GetAppConfigRequest) bool { return r.ClusterID == "cl-1" })).
			Return(nil, errors.New("Insufficient permissions."))

		run := runAPC(t, api, "", "deployment", "create", "--label", "beta", "--cluster-id", "cl-1", "-o", "json")
		assert.Equal(t, 1, run.code)
		var got errorObject
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "failed to get app config: Insufficient permissions.", got.Error)
		api.AssertNotCalled(t, "CreateDeployment", mock.Anything)
	})

	t.Run("update publishes the Deployment as it now is", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&certifiedDep, nil)
		updated := certifiedDep
		updated.Label = "renamed"
		api.On("UpdateDeployment", mock.Anything).Return(&updated, nil)

		run := runAPC(t, api, "", "deployment", "update", "dep-ac", "--label", "renamed", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "renamed", got.Label)
		assert.Equal(t, strp("deploy-1"), got.ImageTag)
	})

	t.Run("adopt publishes the Deployment it created", func(t *testing.T) {
		api := newAPCClient()
		api.On("AdoptDeployment", mock.Anything).Return(&houston.Deployment{ID: "adopted", Label: "cr", ReleaseName: "cr", Namespace: "ns", ClusterID: "cl-1"}, nil)

		run := runAPC(t, api, "", "deployment", "adopt", "--cluster-id", "cl-1", "--name", "cr", "--namespace", "ns", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "adopted", got.DeploymentID)
		assert.Equal(t, strp("ns"), got.Namespace)
	})
}

// A confirmation is never asked under json: the run refuses, names the flag
// that answers it, and reads nothing from stdin.
func TestDeploymentConfirmationsRefuseUnderJSON(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
	}{
		{"delete", []string{"deployment", "delete", "dep-ac", "-o", "json"}},
		{"unadopt", []string{"deployment", "unadopt", "--deployment-id", "dep-ac", "-o", "json"}},
		{"create of a dag_deploy Deployment", []string{"deployment", "create", "--label", "x", "--cluster-id", "cl-1", "--dag-deployment-type", "dag_deploy", "-o", "json"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			api := newAPCClient()
			api.On("GetAppConfig", mock.Anything).Unset()
			api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{Flags: houston.FeatureFlags{DagOnlyDeployment: true}}, nil).Maybe()

			run := runAPC(t, api, "y\n", c.args...)
			assert.Equal(t, 1, run.code)
			var got errorObject
			decodeOne(t, run.stdout, &got)
			assert.Contains(t, got.Error, "--yes")
			assert.False(t, run.stdinRead, "a refused prompt read stdin")
			api.AssertNotCalled(t, "DeleteDeployment", mock.Anything)
			api.AssertNotCalled(t, "UnadoptDeployment", mock.Anything)
			api.AssertNotCalled(t, "CreateDeployment", mock.Anything)
		})
	}
}

func TestDeploymentRemovalJSON(t *testing.T) {
	t.Run("delete --yes publishes what it deleted", func(t *testing.T) {
		api := newAPCClient()
		api.On("DeleteDeployment", houston.DeleteDeploymentRequest{DeploymentID: "dep-ac", HardDelete: true}).Return(&certifiedDep, nil)

		run := runAPC(t, api, "", "deployment", "delete", "dep-ac", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentRemovalJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deploymentRemovalJSON{DeploymentID: "dep-ac", Label: strp("alpha"), ReleaseName: strp("rel-ac"), WorkspaceID: strp("ws-1"), Action: "deleted"}, got)
	})

	t.Run("delete --yes in text skips the question", func(t *testing.T) {
		api := newAPCClient()
		api.On("DeleteDeployment", mock.Anything).Return(&certifiedDep, nil)

		run := runAPC(t, api, "", "deployment", "delete", "dep-ac", "--yes")
		require.Equal(t, 0, run.code)
		assert.Equal(t, "\n Successfully deleted deployment\n", run.stdout)
		assert.NotContains(t, run.stderr, "Proceed with delete?")
	})

	// Houston may answer with no record; the id given is what is known.
	t.Run("unadopt with no Deployment back names the id", func(t *testing.T) {
		api := newAPCClient()
		api.On("UnadoptDeployment", mock.Anything).Return(nil, nil)

		run := runAPC(t, api, "", "deployment", "unadopt", "--deployment-id", "dep-ac", "--yes")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "\n Successfully unadopted deployment dep-ac\n", run.stdout)

		run = runAPC(t, api, "", "deployment", "unadopt", "--deployment-id", "dep-ac", "--yes", "-o", "json")
		require.Equal(t, 0, run.code)
		var got deploymentRemovalJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deploymentRemovalJSON{DeploymentID: "dep-ac", Action: "unadopted"}, got)
	})

	t.Run("unadopt --yes publishes what it released", func(t *testing.T) {
		api := newAPCClient()
		api.On("UnadoptDeployment", houston.UnadoptDeploymentRequest{DeploymentID: "dep-ac"}).Return(&houston.Deployment{ID: "dep-ac", ReleaseName: "rel-ac"}, nil)

		run := runAPC(t, api, "", "deployment", "unadopt", "--deployment-id", "dep-ac", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deploymentRemovalJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "unadopted", got.Action)
		assert.Equal(t, strp("rel-ac"), got.ReleaseName)
	})
}

func TestDeploymentVersionChangeJSON(t *testing.T) {
	upgrading := houston.Deployment{ID: "dep-ac", Label: "alpha", ReleaseName: "rel-ac", Version: "0.29.0", AirflowVersion: "2.0.0", DesiredAirflowVersion: "2.0.2"}

	t.Run("airflow upgrade started", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&houston.Deployment{ID: "dep-ac", AirflowVersion: "2.0.0", DesiredAirflowVersion: "2.0.0"}, nil)
		api.On("UpdateDeploymentAirflow", mock.Anything).Return(&upgrading, nil)

		run := runAPC(t, api, "", "deployment", "airflow", "upgrade", "--deployment-id", "dep-ac", "--desired-airflow-version", "2.0.2", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got versionChangeJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, versionChangeJSON{
			DeploymentID: "dep-ac", Label: "alpha", ReleaseName: "rel-ac", Action: "started",
			Current: imageVersionJSON{Image: "astronomer_certified", Version: "2.0.0"},
			Desired: &imageVersionJSON{Image: "astronomer_certified", Version: "2.0.2"},
		}, got)
	})

	t.Run("airflow upgrade canceled, then nothing to cancel", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&upgrading, nil).Once()
		api.On("UpdateDeploymentAirflow", mock.Anything).Return(&upgrading, nil)
		run := runAPC(t, api, "", "deployment", "airflow", "upgrade", "--deployment-id", "dep-ac", "--cancel", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got versionChangeJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "canceled", got.Action)
		assert.Equal(t, imageVersionJSON{Image: "astronomer_certified", Version: "2.0.0"}, got.Current)
		assert.Nil(t, got.Desired)

		settled := upgrading
		settled.DesiredAirflowVersion = settled.AirflowVersion
		api.On("GetDeployment", "dep-ac").Return(&settled, nil).Once()
		run = runAPC(t, api, "", "deployment", "airflow", "upgrade", "--deployment-id", "dep-ac", "--cancel", "-o", "json")
		require.Equal(t, 0, run.code)
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "nothing_to_cancel", got.Action)
		assert.Contains(t, run.stdout, `"desired":null`)
	})

	t.Run("runtime upgrade started", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-rt").Return(&houston.Deployment{ID: "dep-rt", RuntimeVersion: "4.2.0", RuntimeAirflowVersion: "2.2.4"}, nil)
		api.On("UpdateDeploymentRuntime", mock.Anything).Return(&houston.Deployment{ID: "dep-rt", RuntimeVersion: "4.2.0", DesiredRuntimeVersion: "4.2.1"}, nil)

		run := runAPC(t, api, "", "deployment", "runtime", "upgrade", "--deployment-id", "dep-rt", "--desired-runtime-version", "4.2.1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got versionChangeJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, imageVersionJSON{Image: "runtime", Version: "4.2.0"}, got.Current)
		assert.Equal(t, &imageVersionJSON{Image: "runtime", Version: "4.2.1"}, got.Desired)
	})

	t.Run("runtime migrate started", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&houston.Deployment{ID: "dep-ac", AirflowVersion: "2.2.4", ClusterID: "cl-1"}, nil)
		api.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{{Version: "4.2.0", AirflowVersion: "2.2.4"}}, nil)
		api.On("UpdateDeploymentRuntime", mock.Anything).Return(&houston.Deployment{ID: "dep-ac"}, nil)

		run := runAPC(t, api, "", "deployment", "runtime", "migrate", "--deployment-id", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got versionChangeJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, imageVersionJSON{Image: "astronomer_certified", Version: "2.2.4"}, got.Current)
		assert.Equal(t, &imageVersionJSON{Image: "runtime", Version: "4.2.0"}, got.Desired)
	})

	// A cancel names the image the Deployment runs. A migrate cancel on an
	// Astronomer Certified Deployment used to say it was "already running
	// Runtime " with no version.
	t.Run("a cancel names the image the Deployment runs", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&houston.Deployment{ID: "dep-ac", AirflowVersion: "2.2.4"}, nil)
		api.On("GetDeployment", "dep-rt").Return(&houston.Deployment{ID: "dep-rt", RuntimeVersion: "4.2.0", DesiredRuntimeVersion: "4.2.0"}, nil)

		run := runAPC(t, api, "", "deployment", "runtime", "migrate", "--deployment-id", "dep-ac", "--cancel")
		require.Equal(t, 0, run.code)
		assert.Equal(t, "\nNothing to cancel. You are running Airflow 2.2.4 and you have not indicated that you want to migrate to Runtime.", run.stdout)

		run = runAPC(t, api, "", "deployment", "airflow", "upgrade", "--deployment-id", "dep-rt", "--cancel", "-o", "json")
		require.Equal(t, 0, run.code)
		var got versionChangeJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, imageVersionJSON{Image: "runtime", Version: "4.2.0"}, got.Current)
	})

	// The version pickers refuse under json, naming the flag that answers them.
	t.Run("pickers refuse naming their flag", func(t *testing.T) {
		api := newAPCClient()
		api.On("GetDeployment", "dep-ac").Return(&houston.Deployment{ID: "dep-ac", AirflowVersion: "2.0.0"}, nil)
		api.On("GetDeploymentConfig", mock.Anything).Return(&houston.DeploymentConfig{AirflowVersions: []string{"2.0.2"}}, nil)
		api.On("GetDeployment", "dep-rt").Return(&houston.Deployment{ID: "dep-rt", RuntimeVersion: "4.2.0", RuntimeAirflowVersion: "2.2.4"}, nil)
		api.On("GetRuntimeReleases", mock.Anything).Return(houston.RuntimeReleases{{Version: "4.2.1", AirflowVersion: "2.2.4"}}, nil)

		for flag, args := range map[string][]string{
			"--desired-airflow-version": {"deployment", "airflow", "upgrade", "--deployment-id", "dep-ac", "-o", "json"},
			"--desired-runtime-version": {"deployment", "runtime", "upgrade", "--deployment-id", "dep-rt", "-o", "json"},
		} {
			run := runAPC(t, api, "1\n", args...)
			assert.Equal(t, 1, run.code)
			var got errorObject
			decodeOne(t, run.stdout, &got)
			assert.Contains(t, got.Error, flag)
			assert.False(t, run.stdinRead)
		}
		api.AssertNotCalled(t, "UpdateDeploymentAirflow", mock.Anything)
		api.AssertNotCalled(t, "UpdateDeploymentRuntime", mock.Anything)
	})
}

func TestDeploymentLogsJSON(t *testing.T) {
	t.Run("one record per line", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{
			{ID: "1", CreatedAt: "2026-01-01T00:00:00Z", Log: "first"},
			{ID: "2", CreatedAt: "2026-01-01T00:00:01Z", Log: "second <html>"},
		}, nil)

		run := runAPC(t, api, "", "deployment", "logs", "scheduler", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, []logEntryJSON{
			{Component: "scheduler", Timestamp: "2026-01-01T00:00:00Z", Message: "first"},
			{Component: "scheduler", Timestamp: "2026-01-01T00:00:01Z", Message: "second <html>"},
		}, decodeLines[logEntryJSON](t, run.stdout))
	})

	// Houston refuses a window over 2 days; the CLI says so first, as a usage
	// error, without asking.
	t.Run("--since over 48h is a usage error", func(t *testing.T) {
		api := newAPCClient()
		run := runAPC(t, api, "", "deployment", "logs", "scheduler", "dep-ac", "--since", "49h", "-o", "json")
		assert.Equal(t, 2, run.code)
		var got errorObject
		decodeOne(t, run.stdout, &got)
		assert.Contains(t, got.Error, "48h")
		api.AssertNotCalled(t, "ListDeploymentLogs", mock.Anything)

		api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{}, nil)
		run = runAPC(t, api, "", "deployment", "logs", "scheduler", "dep-ac", "--since", "48h", "-o", "json")
		assert.Equal(t, 0, run.code, "48h itself is allowed")
	})

	t.Run("none prints nothing", func(t *testing.T) {
		api := newAPCClient()
		api.On("ListDeploymentLogs", mock.Anything).Return([]houston.DeploymentLog{}, nil)

		run := runAPC(t, api, "", "deployment", "logs", "workers", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code)
		assert.Empty(t, run.stdout)
	})

	t.Run("follow streams each record, with its note on stderr", func(t *testing.T) {
		prev := subscribeLogs
		t.Cleanup(func() { subscribeLogs = prev })
		subscribeLogs = func(_, component, _ string, _ time.Duration, notes io.Writer, onLog func(houston.DeploymentLog) error) error {
			fmt.Fprintln(notes, "Waiting for logs...")
			for _, l := range []string{"a", "b"} {
				if err := onLog(houston.DeploymentLog{CreatedAt: "t", Log: l}); err != nil {
					return err
				}
			}
			return houston.ErrLogStreamClosed
		}

		run := runAPC(t, newAPCClient(), "", "deployment", "logs", "webserver", "dep-ac", "--follow", "-o", "json")
		assert.Equal(t, 1, run.code, "the server closing the stream is a failure, not a 0")
		lines := strings.Split(strings.TrimSpace(run.stdout), "\n")
		require.Len(t, lines, 3, "two records, then the error object as one more line:\n%s", run.stdout)
		assert.Equal(t, []logEntryJSON{{Component: "webserver", Timestamp: "t", Message: "a"}, {Component: "webserver", Timestamp: "t", Message: "b"}},
			decodeLines[logEntryJSON](t, strings.Join(lines[:2], "\n")))
		var got errorObject
		require.NoError(t, json.Unmarshal([]byte(lines[2]), &got))
		assert.Contains(t, got.Error, "log stream was closed")
		assert.Contains(t, run.stderr, "Waiting for logs...")
	})

	t.Run("follow in text is the records as they come", func(t *testing.T) {
		prev := subscribeLogs
		t.Cleanup(func() { subscribeLogs = prev })
		subscribeLogs = func(_, _, _ string, _ time.Duration, notes io.Writer, onLog func(houston.DeploymentLog) error) error {
			fmt.Fprintln(notes, "Waiting for logs...")
			return onLog(houston.DeploymentLog{Log: "a line\n"})
		}

		run := runAPC(t, newAPCClient(), "", "deployment", "logs", "webserver", "dep-ac", "--follow")
		require.Equal(t, 0, run.code)
		assert.Equal(t, "Waiting for logs...\na line\n", run.stdout)
	})
}

// deployMocks replaces deploy's platform calls for one test. deployed is
// what the image deploy reports; dags is what the DAG deploy returns.
func deployMocks(t *testing.T, deployed deploy.Deployed, dags error) *deploy.Options {
	t.Helper()
	prevImage, prevDags, prevRemote, prevEnsure := DeployAirflowImage, DagsOnlyDeploy, UpdateDeploymentImage, EnsureProjectDir
	t.Cleanup(func() {
		DeployAirflowImage, DagsOnlyDeploy, UpdateDeploymentImage, EnsureProjectDir = prevImage, prevDags, prevRemote, prevEnsure
	})
	// seen is the options the last platform call was handed.
	seen := new(deploy.Options)
	EnsureProjectDir = func(*cobra.Command, []string) error { return nil }
	DeployAirflowImage = func(_ houston.ClientInterface, _, deploymentID, _ string, _, _ bool, _ string, _ bool, _ string, opts deploy.Options) (deploy.Deployed, error) {
		*seen = opts
		fmt.Fprintln(opts.Progress, "Deploying: rel-ac")
		return deployed, nil
	}
	DagsOnlyDeploy = func(_ houston.ClientInterface, _, deploymentID, _ string, _ *string, _ bool, _ string, opts deploy.Options) (string, error) {
		*seen = opts
		if deploymentID == "" {
			// As the picker would, or the project's saved Deployment.
			deploymentID = "dep-picked"
		}
		return deploymentID, dags
	}
	UpdateDeploymentImage = func(_ houston.ClientInterface, deploymentID, _, _, _ string, opts deploy.Options) (string, error) {
		*seen = opts
		fmt.Fprintln(opts.Progress, "Image successfully updated")
		return deploymentID, nil
	}
	return seen
}

func TestDeployJSON(t *testing.T) {
	pushed := deploy.Deployed{DeploymentID: "dep-ac", Image: "registry/rel-ac/airflow:deploy-2", URL: "https://airflow"}

	t.Run("image and dags", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "image_and_dags", Image: pushed.Image, URL: pushed.URL}, got)
		assert.Contains(t, run.stderr, "Deploying: rel-ac", "the progress goes to stderr")
	})

	t.Run("a Deployment that takes no DAG-only deploy is an image deploy", func(t *testing.T) {
		deployMocks(t, pushed, deploy.ErrDagOnlyDeployNotEnabledForDeployment)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image", got.Type)
	})

	t.Run("--image", func(t *testing.T) {
		deployMocks(t, pushed, errors.New("must not deploy DAGs"))
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image", got.Type)
		assert.Contains(t, run.stderr, "Dags in the project will not be deployed")
	})

	t.Run("--dags", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		houstonVersion = "1.0.0"
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--dags", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "dags"}, got)
	})

	// The Deployment published is the one deployed to, not the argument:
	// with none, the project's saved Deployment or the picker's choice.
	t.Run("--dags with no argument publishes the Deployment it resolved", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		houstonVersion = "1.0.0"
		run := runAPC(t, newAPCClient(), "", "deploy", "--dags", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "dep-picked", got.Deployment)
	})

	t.Run("--yes answers the deploy's confirmations", func(t *testing.T) {
		seen := deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.True(t, seen.Yes)

		run = runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "-o", "json")
		require.Equal(t, 0, run.code)
		assert.False(t, seen.Yes, "only when given")
	})

	t.Run("--image-name --remote", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "my/image:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "image_and_dags", Image: "my/image:1", RuntimeVersion: "12.1.1"}, got)
		assert.Contains(t, run.stderr, "Image successfully updated")
	})

	t.Run("text prints only the deploy's own progress", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "Deploying: rel-ac\n", run.stdout)
	})
}

func strp(s string) *string { return &s }
