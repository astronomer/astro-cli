//go:build !windows

package apc

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

	// The image already in a registry needs no container engine, so the
	// real image deploy runs, and the DAG upload after it.
	realImageDeploy(t)
	dags := filepath.Join(project, "dags")
	require.NoError(t, os.Chmod(dags, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dags, 0o755) })
	api := dagsAPI(dagsDeployment(houston.DagOnlyDeploymentType, true), cfgOf(takesDagUploads))
	run := runAPC(t, api, "", "deploy", "dep-ac", "--image-name", "img:1", "--remote", "--runtime-version", "12.1.1", "-o", "json")
	assert.NotEqual(t, 0, run.code)
	require.ErrorIs(t, run.err, syscall.EACCES)
	assert.NotErrorIs(t, run.err, deploy.ErrNoDagsDirectory)
	var after *deploy.DagsAfterImageError
	assert.ErrorAs(t, run.err, &after, "the image was deployed, and the error says so")
	api.AssertCalled(t, "UpdateDeploymentImage", mock.Anything)
	assert.NotContains(t, run.stdout, `"deployment"`, "no result for a deploy that failed")
}
