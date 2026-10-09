package astro

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astrov1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1/mocks"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
)

const (
	dbtTestDeploymentID = "clxdbtdeployment000000000"
	dbtTestDeployID     = "clxdbtdeploy0000000000000"
	dbtTestVersion      = "2026-10-07T00:00:00.0000000Z"
)

// fakeBlobStore answers the bundle upload the way Azure does: every request
// succeeds, and the commit reports the version the upload created.
func fakeBlobStore(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("x-ms-version-id", dbtTestVersion)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/container/bundle.tar.gz?sig=x"
}

// dbtProject writes a dbt project named jaffle_shop in a fresh directory outside
// any Astro project, and makes a second fresh directory the working path, so
// the bundle's tarball is written beside it rather than inside it.
func dbtProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dbt_project.yml"), []byte("name: jaffle_shop\n"), 0o600))
	prev := config.WorkingPath
	config.WorkingPath = t.TempDir()
	t.Cleanup(func() { config.WorkingPath = prev })
	return dir
}

func dbtTestDeployment() astrov1.Deployment {
	return astrov1.Deployment{
		Id:                 dbtTestDeploymentID,
		Name:               "analytics",
		WorkspaceId:        "clxdbtworkspace0000000000",
		IsDagDeployEnabled: true,
		Status:             astrov1.DeploymentStatusHEALTHY,
	}
}

// dbtBundleMock is the API a bundle deploy or delete talks to.
func dbtBundleMock(t *testing.T, uploadURL string) *astrov1_mocks.ClientWithResponsesInterface {
	t.Helper()
	m := new(astrov1_mocks.ClientWithResponsesInterface)
	t.Cleanup(func() { m.AssertExpectations(t) })
	d := dbtTestDeployment()
	m.On("GetDeploymentWithResponse", mock.Anything, mock.Anything, dbtTestDeploymentID).Return(&astrov1.GetDeploymentResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &d,
	}, nil).Maybe()
	deploy := astrov1.Deploy{Id: dbtTestDeployID}
	if uploadURL != "" {
		deploy.BundleUploadUrl = &uploadURL
	}
	m.On("CreateDeployWithResponse", mock.Anything, mock.Anything, dbtTestDeploymentID, mock.Anything).Return(&astrov1.CreateDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &deploy,
	}, nil).Once()
	m.On("FinalizeDeployWithResponse", mock.Anything, mock.Anything, dbtTestDeploymentID, dbtTestDeployID, mock.Anything).Return(&astrov1.FinalizeDeployResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &deploy,
	}, nil).Once()
	return m
}

func execDbtRun(t *testing.T, client astrov1.APIClient, args ...string) tokenRun {
	t.Helper()
	return execAstroCmd(t, client, "", newDbtCmd, append([]string{"dbt"}, args...)...)
}

// What v2 printed on stdout in text mode, recorded before --output json
// existed on these commands. Text output may not change, so these are pinned
// byte for byte.
func TestDbtTextOutputUnchanged(t *testing.T) {
	t.Run("deploy", func(t *testing.T) {
		project := dbtProject(t)
		r := execDbtRun(t, dbtBundleMock(t, fakeBlobStore(t)), "deploy", dbtTestDeploymentID, "--project-path", project)
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Initiating dbt deploy for deployment ID: "+dbtTestDeploymentID+"\n"+
			"Generated mount path from dbt project name: /usr/local/airflow/dbt/jaffle_shop\n"+
			"Successfully uploaded bundle with version "+dbtTestVersion+" to Astro.\n", r.stdout)
	})
	t.Run("deploy --mount-path", func(t *testing.T) {
		project := dbtProject(t)
		r := execDbtRun(t, dbtBundleMock(t, fakeBlobStore(t)), "deploy", dbtTestDeploymentID, "--project-path", project, "--mount-path", "/usr/local/airflow/dbt/custom")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Initiating dbt deploy for deployment ID: "+dbtTestDeploymentID+"\n"+
			"Successfully uploaded bundle with version "+dbtTestVersion+" to Astro.\n", r.stdout)
	})
	t.Run("delete", func(t *testing.T) {
		project := dbtProject(t)
		r := execDbtRun(t, dbtBundleMock(t, ""), "delete", dbtTestDeploymentID, "--project-path", project)
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Initiating dbt delete deploy for deployment ID: "+dbtTestDeploymentID+"\n"+
			"Successfully requested bundle delete for mount path /usr/local/airflow/dbt/jaffle_shop from Astro.\n", r.stdout)
	})
	// astro deploy --non-dags uploads a bundle the same way, and prints the
	// same line.
	t.Run("astro deploy --non-dags", func(t *testing.T) {
		bundle := dbtProject(t)
		r := execAstroCmd(t, dbtBundleMock(t, fakeBlobStore(t)), "", func(io.Writer) *cobra.Command { return NewDeployCmd() },
			"deploy", dbtTestDeploymentID, "--non-dags", "--non-dags-mount-path", "/usr/local/airflow/x", "--non-dags-local-path", bundle)
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Successfully uploaded bundle with version "+dbtTestVersion+" to Astro.\n", r.stdout)
	})
	t.Run("cleanup", func(t *testing.T) {
		dir := t.TempDir()
		artifact := filepath.Join(dir, ".astro", "dbt_metadata.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(artifact), 0o755))
		require.NoError(t, os.WriteFile(artifact, []byte(`{"generated_by": {"application": "astro"}}`), 0o600))
		r := execDbtRun(t, nil, "cleanup", dir)
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Removed the Cosmos Boost artifacts\n", r.stdout)
	})
}

// stubBundleWait stands in for the wait a --wait does, writing one line of
// progress where the real one writes its own and answering err.
func stubBundleWait(t *testing.T, err error) *[]string {
	t.Helper()
	var waited []string
	prev := waitForBundle
	waitForBundle = func(progress io.Writer, deploymentID string, _ time.Duration, _ astrov1.APIClient) error {
		waited = append(waited, deploymentID)
		fmt.Fprintln(progress, "Waiting for the deployment to become healthy…")
		return err
	}
	t.Cleanup(func() { waitForBundle = prev })
	return &waited
}

func TestDbtDeployJSON(t *testing.T) {
	project := dbtProject(t)
	r := execDbtRun(t, dbtBundleMock(t, fakeBlobStore(t)), "deploy", dbtTestDeploymentID, "--project-path", project, "-o", "json")
	require.NoError(t, r.err, "stderr:\n%s", r.stderr)

	var got dbtDeployJSON
	decodeOne(t, r.stdout, &got)
	assert.Equal(t, dbtTestDeploymentID, got.Deployment)
	assert.Equal(t, "analytics", got.DeploymentName)
	assert.Equal(t, "clxdbtworkspace0000000000", got.Workspace)
	assert.Equal(t, dbtTestDeployID, got.DeployID)
	assert.Equal(t, "jaffle_shop", got.Project)
	assert.Equal(t, project, got.ProjectPath)
	assert.Equal(t, "/usr/local/airflow/dbt/jaffle_shop", got.MountPath)
	assert.Equal(t, dbtTestVersion, got.BundleVersion)
	assert.False(t, got.Waited)
	assert.Nil(t, got.Git, "the project is in no git checkout")
}

// A Deployment found by name is read once, by the lookup, and the deploy
// uses what the lookup read rather than reading it again.
func TestDbtDeployReadsANamedDeploymentOnce(t *testing.T) {
	project := dbtProject(t)
	m := dbtBundleMock(t, fakeBlobStore(t))
	m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listDeploymentsResp(dbtTestDeployment()), nil).Once()

	r := execDbtRun(t, m, "deploy", "--deployment-name", "analytics", "--project-path", project, "-o", "json")
	require.NoError(t, r.err, "stderr:\n%s", r.stderr)
	var got dbtDeployJSON
	decodeOne(t, r.stdout, &got)
	assert.Equal(t, dbtTestDeploymentID, got.Deployment)
	m.AssertNumberOfCalls(t, "GetDeploymentWithResponse", 1)
}

// A --wait's progress is on stderr in both formats. In text the result comes
// first, as it always has, so stdout is what it is without --wait; under json
// the object comes after the wait it reports, and says it waited.
func TestDbtWait(t *testing.T) {
	cases := []struct {
		name   string
		upload bool
	}{
		{"deploy", true},
		{"delete", false},
	}
	for _, tc := range cases {
		args := func(project string, more ...string) []string {
			return append([]string{tc.name, dbtTestDeploymentID, "--project-path", project}, more...)
		}
		client := func(t *testing.T) *astrov1_mocks.ClientWithResponsesInterface {
			if tc.upload {
				return dbtBundleMock(t, fakeBlobStore(t))
			}
			return dbtBundleMock(t, "")
		}
		t.Run(tc.name+" text", func(t *testing.T) {
			project := dbtProject(t)
			without := execDbtRun(t, client(t), args(project)...)
			require.NoError(t, without.err)

			waited := stubBundleWait(t, nil)
			r := execDbtRun(t, client(t), args(project, "--wait")...)
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			assert.Equal(t, []string{dbtTestDeploymentID}, *waited)
			assert.Equal(t, without.stdout, r.stdout, "stdout is the result, the same with --wait as without")
			assert.Contains(t, r.stderr, "Waiting for the deployment to become healthy…")
		})
		t.Run(tc.name+" text, the wait failing", func(t *testing.T) {
			project := dbtProject(t)
			stubBundleWait(t, errors.New("timed out"))
			r := execDbtRun(t, client(t), args(project, "--wait")...)
			require.Error(t, r.err)
			assert.Contains(t, r.stdout, "Successfully", "the result is printed before the wait begins")
		})
		t.Run(tc.name+" json", func(t *testing.T) {
			project := dbtProject(t)
			waited := stubBundleWait(t, nil)
			r := execDbtRun(t, client(t), args(project, "--wait", "-o", "json")...)
			require.NoError(t, r.err, "stderr:\n%s", r.stderr)
			assert.Equal(t, []string{dbtTestDeploymentID}, *waited)
			var got struct {
				Waited bool `json:"waited"`
			}
			fields := decodeOne(t, r.stdout, &got)
			assert.True(t, got.Waited)
			assert.NotContains(t, fields, "wait_error", "a wait that succeeded has no error")
			assert.Contains(t, r.stderr, "Waiting for the deployment to become healthy…")
		})
		t.Run(tc.name+" json, the wait failing", func(t *testing.T) {
			project := dbtProject(t)
			stubBundleWait(t, errors.New("timed out"))
			r := execDbtRun(t, client(t), args(project, "--wait", "-o", "json")...)
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			// The upload happened, so the result is still published, saying
			// it waited and why the wait failed, and the run fails.
			var got struct {
				DeployID  string `json:"deploy_id"`
				Waited    bool   `json:"waited"`
				WaitError string `json:"wait_error"`
			}
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, dbtTestDeployID, got.DeployID)
			assert.True(t, got.Waited, "the wait was attempted")
			assert.Equal(t, "timed out", got.WaitError)
			assert.Contains(t, r.stderr, "Error: timed out")
		})
	}
}

func TestDbtDeleteJSON(t *testing.T) {
	// Named by id, the Deployment is never read, so its Workspace is not
	// known and not published.
	t.Run("by id", func(t *testing.T) {
		r := execDbtRun(t, dbtBundleMock(t, ""), "delete", dbtTestDeploymentID, "--mount-path", "/usr/local/airflow/dbt/custom", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)

		var got dbtDeleteJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, dbtDeleteJSON{
			Action:     "deleted",
			Deployment: dbtTestDeploymentID,
			DeployID:   dbtTestDeployID,
			MountPath:  "/usr/local/airflow/dbt/custom",
		}, got)
		assert.NotContains(t, fields, "workspace")
		assert.NotContains(t, fields, "wait_error")
	})
	// Named by name, it was read from the Workspace, and its Workspace is
	// published.
	t.Run("by name", func(t *testing.T) {
		m := dbtBundleMock(t, "")
		m.On("ListDeploymentsWithResponse", mock.Anything, mock.Anything, mock.Anything).Return(listDeploymentsResp(dbtTestDeployment()), nil).Once()
		r := execDbtRun(t, m, "delete", "--deployment-name", "analytics", "--mount-path", "/usr/local/airflow/dbt/custom", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)

		var got dbtDeleteJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, dbtTestDeploymentID, got.Deployment)
		assert.Equal(t, "clxdbtworkspace0000000000", got.Workspace)
	})
}

// A wait that fails and a result that then cannot be written are both
// reported: neither error hides the other.
func TestPublishThenWaitReportsBothFailures(t *testing.T) {
	timedOut := errors.New("timed out")
	stubBundleWait(t, timedOut)
	writeFailed := errors.New("broken pipe")

	var stdout, stderr bytes.Buffer
	root := &cobra.Command{Use: "astro"}
	var format cliout.Format
	cmd := &cobra.Command{Use: "x", RunE: func(cmd *cobra.Command, _ []string) error {
		return publishThenWait(cmd, cliout.FormatJSON, true, dbtTestDeploymentID, time.Second, func(waitErr error) error {
			assert.Same(t, timedOut, waitErr)
			return writeFailed
		})
	}}
	cliout.AddOutputFlag(cmd, &format)
	root.AddCommand(cmd)
	root.SetOut(&stdout)
	root.SetErr(&stderr)

	err := cliout.Execute(context.Background(), root, []string{"x", "-o", "json"}, &stdout, nil)

	require.ErrorIs(t, err, writeFailed)
	require.ErrorIs(t, err, timedOut)
	assert.Empty(t, stdout.String(), "the write to stdout failed, so no error object is written there after it")
	assert.Equal(t, "Waiting for the deployment to become healthy…\nError: broken pipe\ntimed out\n", stderr.String(), "after the wait's progress, the write's failure first, then the wait's")
	assert.Equal(t, cliout.ExitFailure, cliout.ExitCode(context.Background(), err))
}

func TestDbtCleanupJSON(t *testing.T) {
	dir := t.TempDir()
	// The cleanup walks the directory with symlinks resolved (macOS keeps
	// temp dirs under /private/var), and reports the files at those paths.
	walked, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	ours := filepath.Join(walked, ".astro", "dbt_metadata.json")
	foreign := filepath.Join(walked, "other", ".astro", "dbt_metadata.json")
	for path, app := range map[string]string{ours: "astro", foreign: "someone-else"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(`{"generated_by": {"application": "`+app+`"}}`), 0o600))
	}

	r := execDbtRun(t, nil, "cleanup", dir, "-o", "json")
	require.NoError(t, r.err, "stderr:\n%s", r.stderr)
	var got dbtCleanupJSON
	decodeOne(t, r.stdout, &got)
	assert.Equal(t, dbtCleanupJSON{Action: "removed", Paths: []string{dir}, Removed: []string{ours}, Kept: []string{foreign}}, got)

	t.Run("nothing to remove", func(t *testing.T) {
		r := execDbtRun(t, nil, "cleanup", t.TempDir(), "-o", "json")
		require.NoError(t, r.err)
		var got dbtCleanupJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.JSONEq(t, "[]", string(fields["removed"]))
		assert.JSONEq(t, "[]", string(fields["kept"]))
	})
	t.Run("no path: the current directory, absolute", func(t *testing.T) {
		here := t.TempDir()
		t.Chdir(here)
		r := execDbtRun(t, nil, "cleanup", "-o", "json")
		require.NoError(t, r.err)
		var got dbtCleanupJSON
		decodeOne(t, r.stdout, &got)
		wd, err := os.Getwd()
		require.NoError(t, err)
		assert.Equal(t, []string{wd}, got.Paths)
	})
	t.Run("a path that does not exist", func(t *testing.T) {
		r := execDbtRun(t, nil, "cleanup", filepath.Join(t.TempDir(), "missing"), "-o", "json")
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitFailure, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Contains(t, got.Error, "missing")
	})
}

// Under --output json the deployment picker is refused as input_required,
// naming --deployment, before anything is uploaded or deleted. A Workspace
// with no Deployment fails instead of walking through a create, whose name
// question would name a --name these commands do not have.
func TestDbtJSONNeverAsks(t *testing.T) {
	two := []astrov1.Deployment{dbtTestDeployment(), dbtTestDeployment()}
	two[1].Id, two[1].Name = "clxdbtother00000000000000", "other"
	for _, sub := range []string{"deploy", "delete"} {
		t.Run(sub+" with several Deployments", func(t *testing.T) {
			project := dbtProject(t)
			r := execAstroCmd(t, coreMock(t, two...), "1\n", newDbtCmd, "dbt", sub, "--project-path", project, "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Equal(t, string(cliout.KindInputRequired), got.Kind)
			assert.Contains(t, got.Error, "pass --deployment")
		})
		t.Run(sub+" with no Deployment", func(t *testing.T) {
			project := dbtProject(t)
			r := execAstroCmd(t, coreMock(t), "", newDbtCmd, "dbt", sub, "--project-path", project, "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Contains(t, got.Error, "no Deployments found in workspace")
		})
		// Named, it is a Deployment that was not found, as in a Workspace
		// that has others.
		t.Run(sub+" naming a Deployment, with none", func(t *testing.T) {
			project := dbtProject(t)
			r := execAstroCmd(t, coreMock(t), "", newDbtCmd, "dbt", sub, "--project-path", project, "--deployment-name", "analytics", "-o", "json")
			require.Error(t, r.err)
			assert.Equal(t, cliout.ExitFailure, r.code)
			var got errorJSON
			decodeOne(t, r.stdout, &got)
			assert.Contains(t, got.Error, "the Deployment specified was not found")
		})
	}
}

// remote deploy's closing lines, as v2 printed them when the push succeeded.
const remoteDeployTail = "Successfully pushed client image to registry.example.com/agents/client:deploy-2026-10-07T14-53\n" +
	"\n--------------------------------\n" +
	"The client image has been pushed to your private registry.\n" +
	"Your next step would be to update the agent component to use the new client image.\n" +
	"For that you would either need to update the helm chart values.yaml file or update your CI/CD pipeline to use the new client image.\n" +
	"If you are using Astronomer provided Agent Helm chart, you would need to update the `image` field for each of the workers, dagProcessor, and triggerer component sections to the new image: registry.example.com/agents/client:deploy-2026-10-07T14-53\n" +
	"Once you have updated the helm chart values.yaml file, you can run 'helm upgrade' or update via your CI/CD pipeline to update the agent components\n"

// stubClientDeploy stands in for the build and the push, printing one of the
// progress lines the real one prints on the way.
func stubClientDeploy(t *testing.T, res *astrodeploy.ClientDeploy, err error) {
	t.Helper()
	prev := deployClientImage
	deployClientImage = func(astrodeploy.InputClientDeploy, astrov1.APIClient) (astrodeploy.ClientDeploy, error) {
		fmt.Println("Pushing client image to configured remote registry")
		return *res, err
	}
	t.Cleanup(func() { deployClientImage = prev })
}

func execRemoteRun(t *testing.T, args ...string) tokenRun {
	t.Helper()
	// remote deploy runs in a project.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
	return execAstroCmd(t, nil, "", newRemoteRootCmd, append([]string{"remote"}, args...)...)
}

func TestRemoteDeployOutput(t *testing.T) {
	pushed := astrodeploy.ClientDeploy{
		Image:     "registry.example.com/agents/client:deploy-2026-10-07T14-53",
		Registry:  "registry.example.com/agents/client",
		Tag:       "deploy-2026-10-07T14-53",
		Platforms: []string{"linux/amd64", "linux/arm64"},
		RuntimeCheck: &astrodeploy.ClientRuntimeCheck{
			DeploymentID: dbtTestDeploymentID, ClientRuntimeVersion: "3.1-1", DeploymentRuntimeVersion: "3.1-2",
		},
	}
	t.Run("text", func(t *testing.T) {
		stubClientDeploy(t, &pushed, nil)
		r := execRemoteRun(t, "deploy")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Pushing client image to configured remote registry\n"+remoteDeployTail, r.stdout)
	})
	t.Run("json", func(t *testing.T) {
		stubClientDeploy(t, &pushed, nil)
		r := execRemoteRun(t, "deploy", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got remoteDeployJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, "registry.example.com/agents/client:deploy-2026-10-07T14-53", got.Image)
		assert.Equal(t, "deploy-2026-10-07T14-53", got.Tag)
		assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, got.Platforms)
		require.NotNil(t, got.RuntimeCheck)
		assert.Equal(t, remoteRuntimeCheckJSON{Deployment: dbtTestDeploymentID, ClientRuntimeVersion: "3.1-1", DeploymentRuntimeVersion: "3.1-2"}, *got.RuntimeCheck)
	})
	t.Run("json, a prebuilt image", func(t *testing.T) {
		stubClientDeploy(t, &astrodeploy.ClientDeploy{Image: "r/c:deploy-x", Registry: "r/c", Tag: "deploy-x", SourceImage: "local:1"}, nil)
		r := execRemoteRun(t, "deploy", "--image-name", "local:1", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got remoteDeployJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, "local:1", got.SourceImage)
		assert.JSONEq(t, "[]", string(fields["platforms"]))
		assert.NotContains(t, fields, "runtime_check")
	})
	t.Run("json, the push failing", func(t *testing.T) {
		stubClientDeploy(t, &astrodeploy.ClientDeploy{}, errors.New("failed to push client image: denied"))
		r := execRemoteRun(t, "deploy", "-o", "json")
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitFailure, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Contains(t, got.Error, "denied")
	})
}
