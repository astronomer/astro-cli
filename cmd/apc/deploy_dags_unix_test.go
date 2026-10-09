//go:build !windows

package apc

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
)

// A dags directory the deploy cannot look at fails the deploy: it is not
// skipped with a notice as a missing one is, and the exit is not 0.
func TestDeployFailsOnADagsDirectoryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory modes this failure is injected with")
	}
	project := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(project, "dags"), 0o755))
	prev := config.WorkingPath
	config.WorkingPath = project
	t.Cleanup(func() { config.WorkingPath = prev })

	deployMocks(t, deploy.Deployed{DeploymentID: "dep-ac", Dags: deploy.DagsFromUpload}, nil)
	uploads := realDagsOnlyDeploy(t)
	require.NoError(t, os.Chmod(project, 0o000))
	t.Cleanup(func() { _ = os.Chmod(project, 0o755) })
	api := newAPCClient()
	run := runAPC(t, api, "", "deploy", "dep-ac", "-o", "json")
	assert.NotEqual(t, 0, run.code)
	require.ErrorIs(t, run.err, syscall.EACCES)
	assert.NotErrorIs(t, run.err, deploy.ErrNoDagsDirectory)
	assert.Equal(t, 1, *uploads)
	assert.NotContains(t, run.stdout, `"deployment"`, "no result for a deploy that failed")
	assertNoHoustonDagsCalls(t, api)
}
