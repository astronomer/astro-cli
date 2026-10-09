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
	"github.com/astronomer/astro-cli/config"
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
	// err is what the tree returned, which main prints.
	err error
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
	LoadPlatform(api) // as the root does for a line that runs one of these commands
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
		err:       runErr,
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

// deployCalls is what deploy handed its platform calls in one test.
type deployCalls struct {
	// opts is the options the last platform call was handed.
	opts deploy.Options
	// dagUploads is how many times the DAG deploy was called.
	dagUploads int
}

// deployMocks replaces deploy's platform calls for one test. deployed is
// what the image deploy reports, its Dags what --remote's update reports
// too; dags is what the DAG deploy returns.
func deployMocks(t *testing.T, deployed deploy.Deployed, dags error) *deployCalls {
	t.Helper()
	prevImage, prevDags, prevRemote := DeployAirflowImage, DagsOnlyDeploy, UpdateDeploymentImage
	t.Cleanup(func() {
		DeployAirflowImage, DagsOnlyDeploy, UpdateDeploymentImage = prevImage, prevDags, prevRemote
	})
	seen := new(deployCalls)
	DeployAirflowImage = func(_ houston.ClientInterface, deploymentID, _ string, _, _ bool, _ string, opts deploy.Options) (deploy.Deployed, error) {
		seen.opts = opts
		fmt.Fprintln(opts.Progress, "Deploying: rel-ac")
		return deployed, nil
	}
	DagsOnlyDeploy = func(_ houston.ClientInterface, _, deploymentID, _ string, _ *string, _ bool, _ string, opts deploy.Options) (string, error) {
		seen.opts = opts
		seen.dagUploads++
		if deploymentID == "" {
			// As the picker would, or the project's saved Deployment.
			deploymentID = "dep-picked"
		}
		return deploymentID, dags
	}
	UpdateDeploymentImage = func(_ houston.ClientInterface, deploymentID, _, _, imageName string, opts deploy.Options) (deploy.Deployed, error) {
		seen.opts = opts
		fmt.Fprintln(opts.Progress, "Image successfully updated")
		return deploy.Deployed{DeploymentID: deploymentID, Image: imageName, Dags: deployed.Dags}, nil
	}
	return seen
}

// inWorkingDir points the deploy at a fresh directory for one test, with a
// dags directory in it or not.
func inWorkingDir(t *testing.T, withDags bool) string {
	t.Helper()
	dir := t.TempDir()
	if withDags {
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o755))
	}
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
	return dir
}

// inProject is inWorkingDir with a pyproject.toml project in the directory,
// the one kind of project whose DAGs --dags uploads.
func inProject(t *testing.T, withDags bool) string {
	t.Helper()
	dir := inWorkingDir(t, withDags)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	return dir
}

// warningsOff turns show_warnings off for the deploy's run. runAPC resets
// the config before it runs the tree, so this is done from the first
// platform call.
func warningsOff(t *testing.T) {
	t.Helper()
	image := DeployAirflowImage
	DeployAirflowImage = func(c houston.ClientInterface, deploymentID, wsID string, prompt, imageOnly bool, imageName string, opts deploy.Options) (deploy.Deployed, error) {
		require.NoError(t, config.CFG.ShowWarnings.SetHomeString("false"))
		return image(c, deploymentID, wsID, prompt, imageOnly, imageName, opts)
	}
	remote := UpdateDeploymentImage
	UpdateDeploymentImage = func(c houston.ClientInterface, deploymentID, wsID, runtimeVersion, imageName string, opts deploy.Options) (deploy.Deployed, error) {
		require.NoError(t, config.CFG.ShowWarnings.SetHomeString("false"))
		return remote(c, deploymentID, wsID, runtimeVersion, imageName, opts)
	}
}

// realDagsOnlyDeploy puts the real DAG deploy back for one test, counting
// the times it is asked.
func realDagsOnlyDeploy(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	DagsOnlyDeploy = func(c houston.ClientInterface, wsID, deploymentID, dagsParentPath string, dagDeployURL *string, cleanUpFiles bool, description string, opts deploy.Options) (string, error) {
		*calls++
		return deploy.DagsOnlyDeploy(c, wsID, deploymentID, dagsParentPath, dagDeployURL, cleanUpFiles, description, opts)
	}
	return calls
}

// deployWorkspace is the workspace runAPC's config deploys to.
const deployWorkspace = "ck05r3bor07h40d02y2hw4n4v"

// dagsDeployment is the Deployment dagsAPI holds, dep-ac, of DAG deployment
// type typ, read with its type or not (as before 0.29.0).
func dagsDeployment(typ string, read bool) *houston.Deployment {
	return &houston.Deployment{ID: "dep-ac", ReleaseName: "rel-ac", ClusterID: "cl-1", DagDeployment: houston.DagDeploymentConfig{Type: typ}, DagDeploymentRead: read}
}

// takesDagUploads is the cluster config of a cluster with DAG-only deploys.
var takesDagUploads = &houston.AppConfig{Version: "1.0.0", Flags: houston.FeatureFlags{DagOnlyDeployment: true}}

// dagsAPI is a Houston mock for the real update and DAG deploy: a
// workspace holding the one Deployment dep, whose cluster config each read
// in turn answers with the next of cfgs, the last for any read after.
func dagsAPI(dep *houston.Deployment, cfgs ...func() (*houston.AppConfig, error)) *mocks.ClientInterface {
	api := new(mocks.ClientInterface)
	req := houston.GetAppConfigRequest{ClusterID: dep.ClusterID, WorkspaceUUID: deployWorkspace, DeploymentUUID: dep.ID}
	for i, cfg := range cfgs {
		c, err := cfg()
		call := api.On("GetAppConfig", req).Return(c, err)
		if i < len(cfgs)-1 {
			call.Once()
		}
	}
	api.On("GetAppConfig", mock.Anything).Return(&houston.AppConfig{}, nil).Maybe()
	api.On("GetPlatformVersion", mock.Anything).Return("1.0.0", nil).Maybe()
	api.On("GetWorkspace", deployWorkspace).Return(&houston.Workspace{ID: deployWorkspace}, nil).Maybe()
	api.On("ListDeployments", mock.Anything).Return([]houston.Deployment{*dep}, nil).Maybe()
	api.On("GetDeployment", dep.ID).Return(dep, nil).Maybe()
	api.On("UpdateDeploymentImage", mock.Anything).Return(&houston.UpdateDeploymentImageResp{}, nil).Maybe()
	return api
}

// cfgOf answers a cluster config read with cfg.
func cfgOf(cfg *houston.AppConfig) func() (*houston.AppConfig, error) {
	return func() (*houston.AppConfig, error) { return cfg, nil }
}

// cfgFails answers a cluster config read with err.
func cfgFails(err error) func() (*houston.AppConfig, error) {
	return func() (*houston.AppConfig, error) { return nil, err }
}

// inImageWarning is the warning for an --image-name image that is all an
// image Deployment's DAGs.
func inImageWarning(image string) string {
	return "this Deployment runs the Dags inside the image " + image + "; the dags folder is not uploaded. An image astro package built without a dockerfile declared under [tool.astro] contains none."
}

// noDagsNotice is the notice for a DAG upload skipped for want of a dags
// directory in the pyproject.toml project at dir.
func noDagsNotice(dir string) string {
	return "no Dags were uploaded: there is no dags directory in " + dir + ", and the Deployment keeps the Dags it had. To upload Dags, create a dags directory in the project."
}

// deployPushed is what the image deploy reports in these tests.
var deployPushed = deploy.Deployed{DeploymentID: "dep-ac", Image: "registry/rel-ac/airflow:deploy-2", URL: "https://airflow"}

func TestDeployJSON(t *testing.T) {
	pushed := deployPushed

	t.Run("image and dags", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "image_and_dags", Image: pushed.Image, URL: pushed.URL}, got)
		assert.Contains(t, run.stderr, "Deploying: rel-ac", "the progress goes to stderr")
	})

	t.Run("a Deployment that takes no DAG-only deploy is an image deploy", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, pushed, deploy.ErrDagOnlyDeployNotEnabledForDeployment)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image", got.Type)
	})

	// An image Deployment runs the DAGs in its image. A build from the project
	// baked them in; an --image-name image was built elsewhere and may carry
	// none, so the deploy says so, and still succeeds.
	toImage := pushed
	toImage.Dags = deploy.DagsFromImage
	// refused is the DAG deploy failing: none is made for a Deployment that
	// takes no DAG uploads, so a call would fail the deploy.
	refused := errors.New("no DAG deploy is made for this Deployment")
	inImage := inImageWarning
	noDags := noDagsNotice
	t.Run("--image-name to an image Deployment warns that only the image's DAGs run", func(t *testing.T) {
		deployMocks(t, toImage, refused)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "astro-package/proj:latest", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{
			Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "image", Image: pushed.Image, URL: pushed.URL,
			Warnings: []string{inImage("astro-package/proj:latest")},
		}, got)
		assert.Equal(t, 1, strings.Count(run.stderr, "Warning: "+inImage("astro-package/proj:latest")+"\n"), "once, with the progress on stderr:\n%s", run.stderr)
	})

	t.Run("--image-name --remote to an image Deployment warns too", func(t *testing.T) {
		deployMocks(t, toImage, refused)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "registry/img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image", got.Type)
		assert.Equal(t, []string{inImage("registry/img:1")}, got.Warnings)
		assert.Contains(t, run.stderr, "Warning: "+inImage("registry/img:1"))
	})

	// In text the warning goes with the deploy's progress, to stdout, once.
	t.Run("--image-name warns in text on stdout", func(t *testing.T) {
		deployMocks(t, toImage, refused)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "astro-package/proj:latest")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "Deploying: rel-ac\nWarning: "+inImage("astro-package/proj:latest")+"\n", run.stdout)
		assert.NotContains(t, run.stderr, "Warning")
	})

	t.Run("show_warnings off silences it", func(t *testing.T) {
		deployMocks(t, toImage, refused)
		warningsOff(t)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "astro-package/proj:latest", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Empty(t, got.Warnings)
		assert.NotContains(t, run.stderr, "Warning")
	})

	// No warning where it would mislead: a git-sync or volume Deployment gets
	// its DAGs from elsewhere, and --image says it deploys no DAGs itself.
	toGitSync := pushed
	toGitSync.Dags = deploy.DagsFromElsewhere
	for _, tc := range []struct {
		name     string
		deployed deploy.Deployed
		dags     error
		args     []string
	}{
		{"--image-name to a git-sync Deployment", toGitSync, refused, []string{"deploy", "dep-ac", "--image-name", "img:1", "-o", "json"}},
		{"--image-name --remote to a git-sync Deployment", toGitSync, refused, []string{"deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json"}},
		{"--image --image-name to an image Deployment", toImage, errors.New("must not deploy DAGs"), []string{"deploy", "dep-ac", "--image", "--image-name", "img:1", "-o", "json"}},
	} {
		t.Run(tc.name+" does not warn", func(t *testing.T) {
			inWorkingDir(t, false)
			seen := deployMocks(t, tc.deployed, tc.dags)
			run := runAPC(t, newAPCClient(), "", tc.args...)
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Zero(t, seen.dagUploads)
			var got deployJSON
			decodeOne(t, run.stdout, &got)
			assert.Empty(t, got.Warnings)
			assert.NotContains(t, run.stderr, "Warning")
		})
	}

	// With no dags directory to upload, a Deployment that takes DAG uploads
	// keeps the DAGs it has: DagsOnlyDeploy refuses the empty upload that
	// would delete them. The deploy says so whatever show_warnings is, as it
	// is something asked for that did not happen.
	toUpload := pushed
	toUpload.Dags = deploy.DagsFromUpload
	t.Run("no dags directory skips the DAG upload, and says so", func(t *testing.T) {
		dir := inProject(t, false)
		for _, args := range [][]string{
			{"deploy", "dep-ac", "--image-name", "img:1", "-o", "json"},
			{"deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json"},
		} {
			noDags := noDags(dir)
			for _, quiet := range []bool{false, true} {
				deployMocks(t, toUpload, nil)
				uploads := realDagsOnlyDeploy(t)
				if quiet {
					warningsOff(t)
				}
				run := runAPC(t, dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads)), "", args...)
				require.Equal(t, 0, run.code, "%v quiet=%v stderr:\n%s", args, quiet, run.stderr)
				assert.Equal(t, 1, *uploads, "%v: asked", args)
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, "image", got.Type, "%v", args)
				assert.Equal(t, []string{noDags}, got.Warnings, "%v quiet=%v", args, quiet)
				assert.Equal(t, 1, strings.Count(run.stderr, "Warning: "+noDags+"\n"), "%v quiet=%v:\n%s", args, quiet, run.stderr)
			}
		}

		deployMocks(t, toUpload, nil)
		realDagsOnlyDeploy(t)
		warningsOff(t)
		run := runAPC(t, dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads)), "", "deploy", "dep-ac", "--image-name", "img:1")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "Deploying: rel-ac\nWarning: "+noDags(dir)+"\n", run.stdout, "text: on stdout, once, show_warnings off too")
	})

	// A Deployment the image deploy did not place (the zero value) is
	// uploaded to as before: DagsOnlyDeploy is called, and its refusals
	// decide. With no dags directory, one that refuses (every Deployment on
	// a Houston before 0.29.0) says nothing, as before; one that takes the
	// upload says it was skipped, as a placed one does.
	t.Run("a Deployment not placed is uploaded to as before", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, 1, seen.dagUploads)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image_and_dags", got.Type)
		assert.Empty(t, got.Warnings)

		for _, refusal := range []error{deploy.ErrDagOnlyDeployNotEnabledForDeployment, deploy.ErrDagOnlyDeployDisabledInConfig} {
			seen := deployMocks(t, pushed, refusal)
			run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
			require.Equal(t, 0, run.code, "%v stderr:\n%s", refusal, run.stderr)
			assert.Equal(t, 1, seen.dagUploads, "%v", refusal)
			var got deployJSON
			decodeOne(t, run.stdout, &got)
			assert.Equal(t, "image", got.Type, "%v", refusal)
			assert.Empty(t, got.Warnings, "%v", refusal)
		}

		dir := inProject(t, false)
		for _, tc := range []struct {
			name     string
			api      func() *mocks.ClientInterface
			warnings []string
		}{
			{"on 0.25.0, which reads no type and has no DAG-only deploys", func() *mocks.ClientInterface {
				return dagsAPI(dagsDeployment("", false), cfgOf(&houston.AppConfig{Version: "0.25.0"}))
			}, nil},
			{"an image Deployment", func() *mocks.ClientInterface {
				return dagsAPI(dagsDeployment(houston.ImageDeploymentType, true), cfgOf(takesDagUploads))
			}, nil},
			{"a DAG-only Deployment", func() *mocks.ClientInterface {
				return dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads))
			}, []string{noDags(dir)}},
		} {
			for _, quiet := range []bool{false, true} {
				deployMocks(t, pushed, nil)
				uploads := realDagsOnlyDeploy(t)
				if quiet {
					warningsOff(t)
				}
				run := runAPC(t, tc.api(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
				require.Equal(t, 0, run.code, "%s quiet=%v stderr:\n%s", tc.name, quiet, run.stderr)
				assert.Equal(t, 1, *uploads, tc.name)
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, "image", got.Type, tc.name)
				assert.Equal(t, tc.warnings, got.Warnings, "%s quiet=%v", tc.name, quiet)
				if tc.warnings == nil {
					assert.NotContains(t, run.stderr, "Warning", tc.name)
				}
			}
		}
	})

	t.Run("a dags directory is uploaded as before", func(t *testing.T) {
		inProject(t, true)
		seen := deployMocks(t, toUpload, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, 1, seen.dagUploads)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image_and_dags", got.Type)
		assert.Empty(t, got.Warnings)
	})

	t.Run("--image", func(t *testing.T) {
		deployMocks(t, pushed, errors.New("must not deploy DAGs"))
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "image", got.Type)
		assert.Contains(t, run.stderr, "Dags in the project will not be deployed")
	})

	t.Run("--dags", func(t *testing.T) {
		deployMocks(t, pushed, nil)
		houstonVersion = "1.0.0"
		inProject(t, true)
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
		inProject(t, true)
		run := runAPC(t, newAPCClient(), "", "deploy", "--dags", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, "dep-picked", got.Deployment)
	})

	t.Run("--yes answers the deploy's confirmations", func(t *testing.T) {
		seen := deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "--yes", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.True(t, seen.opts.Yes)

		run = runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
		require.Equal(t, 0, run.code)
		assert.False(t, seen.opts.Yes, "only when given")
	})

	t.Run("--image-name --remote", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "my/image:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		var got deployJSON
		decodeOne(t, run.stdout, &got)
		assert.Equal(t, deployJSON{Deployment: "dep-ac", Workspace: "ck05r3bor07h40d02y2hw4n4v", Type: "image_and_dags", Image: "my/image:1", RuntimeVersion: "12.1.1"}, got)
		assert.Contains(t, run.stderr, "Image successfully updated")
	})

	t.Run("text prints only the deploy's own progress", func(t *testing.T) {
		inProject(t, true)
		deployMocks(t, pushed, nil)
		run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1")
		require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
		assert.Equal(t, "Deploying: rel-ac\n", run.stdout)
	})
}

func strp(s string) *string { return &s }

// --dags asks for nothing but the upload, so with no dags directory the
// deploy fails, having sent nothing: from a directory that is not a
// project (--image-name skips the check), or a project without one. A
// Deployment or cluster that takes no upload is refused as such first,
// as that is what there is to change.
func TestDeployDagsNoDagsDirectoryJSON(t *testing.T) {
	toUpload := deployPushed
	toUpload.Dags = deploy.DagsFromUpload
	dir := inProject(t, false)
	for _, args := range [][]string{
		{"deploy", "dep-ac", "--dags", "--image-name", "img:1", "-o", "json"},
		{"deploy", "dep-ac", "--dags"},
	} {
		deployMocks(t, toUpload, nil)
		realDagsOnlyDeploy(t)
		run := runAPC(t, dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads)), "", args...)
		assert.NotEqual(t, 0, run.code, "%v", args)
		require.ErrorIs(t, run.err, deploy.ErrNoDagsDirectory, "%v", args)
		assert.ErrorContains(t, run.err, filepath.Join(dir, "dags")+" is not a directory. Nothing was uploaded, and the Deployment keeps the Dags it had", "%v", args)
		assert.NotContains(t, run.stdout, `"deployment"`, "%v: no result for a deploy that did not happen", args)
	}

	for _, tc := range []struct {
		name string
		api  *mocks.ClientInterface
		want error
	}{
		{"a git-sync Deployment", dagsAPI(dagsDeployment(houston.GitSyncDeploymentType, true), cfgOf(takesDagUploads)), deploy.ErrDagOnlyDeployNotEnabledForDeployment},
		{"a cluster without DAG-only deploys", dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(&houston.AppConfig{Version: "2.0.0"})), deploy.ErrDagOnlyDeployDisabledInConfig},
	} {
		deployMocks(t, toUpload, nil)
		realDagsOnlyDeploy(t)
		run := runAPC(t, tc.api, "", "deploy", "dep-ac", "--dags", "--image-name", "img:1", "-o", "json")
		assert.NotEqual(t, 0, run.code, tc.name)
		require.ErrorIs(t, run.err, tc.want, tc.name)
		assert.NotErrorIs(t, run.err, deploy.ErrNoDagsDirectory, tc.name)
	}
}

// --image-name --remote end to end through the real update and DAG
// deploy, the Deployment placed from what Houston says of it.
func TestDeployRemoteDagsJSON(t *testing.T) {
	toUpload := deployPushed
	toUpload.Dags = deploy.DagsFromUpload
	down := errors.New("houston is down")
	undecided := fmt.Sprintf(noticeDagsUndecided, "failed to get app config: houston is down", "dep-ac")
	for _, tc := range []struct {
		name string
		dep  *houston.Deployment
		cfgs []func() (*houston.AppConfig, error)
		// uploads is how many times the DAG deploy is called.
		uploads int
		// warnings are with show_warnings on; quiet those it still gives.
		warnings, quiet []string
	}{
		// Houston deploys a Deployment with no type by its image.
		{"no type read warns", dagsDeployment("", true), nil, 0, []string{inImageWarning("img:1")}, nil},
		// Before 0.29.0 GetDeployment reads no type, so no type says
		// nothing of the Deployment: it is uploaded to as before, and
		// DagsOnlyDeploy refuses it, so no upload was skipped.
		{"no type before 0.29.0 says nothing", dagsDeployment("", false), []func() (*houston.AppConfig, error){cfgOf(&houston.AppConfig{Version: "0.25.0"})}, 1, nil, nil},
		{"git-sync says nothing", dagsDeployment(houston.GitSyncDeploymentType, true), nil, 0, nil, nil},
		// A cluster config it cannot read leaves a DAG-only Deployment
		// not placed: DagsOnlyDeploy reads the config again, and decides.
		{"dag_deploy whose cluster config is read the second time skips the upload, and says so", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down), cfgOf(takesDagUploads)}, 1, []string{noDagsNotice("")}, []string{noDagsNotice("")}},
		{"dag_deploy whose cluster config is read the second time and refuses says nothing", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down), cfgOf(&houston.AppConfig{Version: "2.0.0"})}, 1, nil, nil},
		// Read neither time: the image update stands, and the deploy says
		// the DAGs were not updated, whatever show_warnings is.
		{"dag_deploy whose cluster config cannot be read says so, and succeeds", dagsDeployment(houston.DagOnlyDeploymentType, true), []func() (*houston.AppConfig, error){cfgFails(down)}, 1, []string{undecided}, []string{undecided}},
	} {
		for _, quiet := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s quiet=%v", tc.name, quiet), func(t *testing.T) {
				dir := inProject(t, false)
				deployMocks(t, toUpload, nil)
				uploads := realDagsOnlyDeploy(t)
				UpdateDeploymentImage = deploy.UpdateDeploymentImage
				if quiet {
					warningsOff(t)
				}
				cfgs := tc.cfgs
				if cfgs == nil {
					cfgs = []func() (*houston.AppConfig, error){cfgOf(takesDagUploads)}
				}
				api := dagsAPI(tc.dep, cfgs...)
				run := runAPC(t, api, "", "deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
				require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
				api.AssertCalled(t, "UpdateDeploymentImage", mock.Anything)
				assert.Contains(t, run.stderr, "Image successfully updated")
				var got deployJSON
				decodeOne(t, run.stdout, &got)
				assert.Equal(t, "image", got.Type)
				assert.Equal(t, tc.uploads, *uploads)
				want := append([]string(nil), tc.warnings...)
				if quiet {
					want = append([]string(nil), tc.quiet...)
				}
				for i, w := range want {
					if w == noDagsNotice("") {
						want[i] = noDagsNotice(dir)
					}
				}
				assert.Equal(t, want, got.Warnings)
			})
		}
	}
}
