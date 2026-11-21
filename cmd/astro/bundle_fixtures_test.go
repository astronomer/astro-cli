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

// stubBundleWait stands in for the wait a --wait does, writing one line of
// progress where the real one writes its own and answering err.
func stubBundleWait(t *testing.T, err error) *[]string {
	t.Helper()
	var waited []string
	prev := waitForBundle
	waitForBundle = func(progress io.Writer, deploymentID string, _ time.Duration, _ astrov1.APIClient) error {
		waited = append(waited, deploymentID)
		fmt.Fprintln(progress, "Waiting for the Deployment to become healthy…")
		return err
	}
	t.Cleanup(func() { waitForBundle = prev })
	return &waited
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
		wait := bundleWait(cmd, &deployLogin{current: true}, dbtTestDeploymentID, time.Second)
		return publishThenWaitWith(cmd, cliout.FormatJSON, true, wait, func(waitErr error) error {
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
	assert.Equal(t, "Waiting for the Deployment to become healthy…\nError: broken pipe\ntimed out\n", stderr.String(), "after the wait's progress, the write's failure first, then the wait's")
	assert.Equal(t, cliout.ExitFailure, cliout.ExitCode(context.Background(), err))
}

// deploy agent's closing lines, when the push succeeded.
const deployAgentTail = "Pushed agent image to registry.example.com/agents/client:deploy-2026-10-07T14-53\n" +
	"Next, set image: in your agent Helm chart's values.yaml to registry.example.com/agents/client:deploy-2026-10-07T14-53 and run helm upgrade, or have your CI/CD pipeline do it.\n"

// stubAgentDeploy stands in for the build and the push, printing one of the
// progress lines the real one prints on the way.
func stubAgentDeploy(t *testing.T, res *astrodeploy.AgentDeploy, err error) {
	t.Helper()
	prev := deployAgentImage
	deployAgentImage = func(astrodeploy.InputAgentDeploy, astrov1.APIClient) (astrodeploy.AgentDeploy, error) {
		fmt.Println("Pushing agent image to the agent registry")
		return *res, err
	}
	t.Cleanup(func() { deployAgentImage = prev })
}

// execAgentRun runs astro followed by args in a project that sets
// agent-registry; args begin with the command path, deploy agent or 1.x's
// remote deploy.
func execAgentRun(t *testing.T, args ...string) tokenRun {
	t.Helper()
	// deploy agent runs in a project.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\ndependencies = [\"apache-airflow==3.1.*\"]\n\n[tool.astro]\n\n[tool.astro.deploy]\nagent-registry = \"registry.example.com/agents/client\"\n"), 0o600))
	prev := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = prev })
	newRoot := NewDeployCmd
	if args[0] == "remote" {
		newRoot = newRemoteRootCmd
	}
	return execAstroCmd(t, nil, "", func(out io.Writer) *cobra.Command {
		c := newRoot()
		c.SetOut(out)
		return c
	}, args...)
}

func TestDeployAgentOutput(t *testing.T) {
	pushed := astrodeploy.AgentDeploy{
		Image:     "registry.example.com/agents/client:deploy-2026-10-07T14-53",
		Registry:  "registry.example.com/agents/client",
		Tag:       "deploy-2026-10-07T14-53",
		Platforms: []string{"linux/amd64", "linux/arm64"},
		RuntimeCheck: &astrodeploy.AgentRuntimeCheck{
			DeploymentID: dbtTestDeploymentID, AgentRuntimeVersion: "3.1-1", DeploymentRuntimeVersion: "3.1-2",
		},
	}
	t.Run("text", func(t *testing.T) {
		stubAgentDeploy(t, &pushed, nil)
		r := execAgentRun(t, "deploy", "agent")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Pushing agent image to the agent registry\n"+deployAgentTail, r.stdout)
	})
	t.Run("1.x's remote deploy, which still pushes and says what to type", func(t *testing.T) {
		stubAgentDeploy(t, &pushed, nil)
		r := execAgentRun(t, "remote", "deploy", "--deployment-id", dbtTestDeploymentID)
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		assert.Equal(t, "Pushing agent image to the agent registry\n"+deployAgentTail, r.stdout)
		assert.Contains(t, r.stderr, "remote deploy is deprecated: use astro deploy agent")
	})
	t.Run("json", func(t *testing.T) {
		stubAgentDeploy(t, &pushed, nil)
		r := execAgentRun(t, "deploy", "agent", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got deployAgentJSON
		decodeOne(t, r.stdout, &got)
		assert.Equal(t, "registry.example.com/agents/client:deploy-2026-10-07T14-53", got.Image)
		assert.Equal(t, "deploy-2026-10-07T14-53", got.Tag)
		assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, got.Platforms)
		require.NotNil(t, got.RuntimeCheck)
		assert.Equal(t, agentRuntimeCheckJSON{Deployment: dbtTestDeploymentID, AgentRuntimeVersion: "3.1-1", DeploymentRuntimeVersion: "3.1-2"}, *got.RuntimeCheck)
	})
	t.Run("json, a prebuilt image", func(t *testing.T) {
		stubAgentDeploy(t, &astrodeploy.AgentDeploy{Image: "r/c:deploy-x", Registry: "r/c", Tag: "deploy-x", SourceImage: "local:1"}, nil)
		r := execAgentRun(t, "deploy", "agent", "--image-name", "local:1", "-o", "json")
		require.NoError(t, r.err, "stderr:\n%s", r.stderr)
		var got deployAgentJSON
		fields := decodeOne(t, r.stdout, &got)
		assert.Equal(t, "local:1", got.SourceImage)
		assert.JSONEq(t, "[]", string(fields["platforms"]))
		assert.NotContains(t, fields, "runtime_check")
	})
	t.Run("json, the push failing", func(t *testing.T) {
		stubAgentDeploy(t, &astrodeploy.AgentDeploy{}, errors.New("failed to push agent image: denied"))
		r := execAgentRun(t, "deploy", "agent", "-o", "json")
		require.Error(t, r.err)
		assert.Equal(t, cliout.ExitFailure, r.code)
		var got errorJSON
		decodeOne(t, r.stdout, &got)
		assert.Contains(t, got.Error, "denied")
	})
}
