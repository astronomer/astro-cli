//go:build !windows

package apc

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// A dags directory the deploy cannot look at fails the deploy: it is not
// skipped with a notice as a missing one is, and the exit is not 0.
func TestDeployFailsOnADagsDirectoryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory modes this failure is injected with")
	}
	project := inProject(t, true)

	deployMocks(t, deploy.Deployed{DeploymentID: "dep-ac", Dags: deploy.DagsFromUpload}, nil)
	uploads := realDagsOnlyDeploy(t)
	dags := filepath.Join(project, "dags")
	require.NoError(t, os.Chmod(dags, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dags, 0o755) })
	run := runAPC(t, dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads)), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
	assert.NotEqual(t, 0, run.code)
	require.ErrorIs(t, run.err, syscall.EACCES)
	assert.NotErrorIs(t, run.err, deploy.ErrNoDagsDirectory)
	assert.Equal(t, 1, *uploads)
	assert.NotContains(t, run.stdout, `"deployment"`, "no result for a deploy that failed")
}
