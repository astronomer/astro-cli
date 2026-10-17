package apc

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/deploy"
)

// Outside any project --image-name ships the image alone: the working
// directory's dags/ is not uploaded, even to a Deployment that takes DAG
// uploads, and the deploy says the DAGs were not updated, whatever
// show_warnings is.
func TestDeployImageNameOutsideAProjectShipsTheImageAlone(t *testing.T) {
	toUpload := deployPushed
	toUpload.Dags = deploy.DagsFromUpload
	for _, tc := range []struct {
		name     string
		deployed deploy.Deployed
		want     string
	}{
		{"a Deployment that takes DAG uploads", toUpload, fmt.Sprintf(noticeImageAlone, "dep-ac")},
		{"a Deployment not placed", deployPushed, fmt.Sprintf(noticeImageAloneUnplaced, "dep-ac")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inWorkingDir(t, true)
			seen := deployMocks(t, tc.deployed, nil)
			run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
			require.Equal(t, 0, run.code, "stderr:\n%s", run.stderr)
			assert.Zero(t, seen.dagUploads, "no dags/ is uploaded from outside a project")
			var got deployJSON
			decodeOne(t, run.stdout, &got)
			assert.Equal(t, "image-only", got.Type)
			assert.Equal(t, []string{tc.want}, got.Warnings)
		})
	}
}

// Below a pyproject.toml project's root every deploy is refused, --image-name
// included, with the project's root to run from.
func TestDeployBelowAProjectRoot(t *testing.T) {
	deployMocks(t, deployPushed, nil)
	dir := inProject(t, true)
	prev := config.WorkingPath
	config.WorkingPath = filepath.Join(dir, "dags")
	t.Cleanup(func() { config.WorkingPath = prev })
	run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--image-name", "img:1", "-o", "json")
	require.Error(t, run.err)
	assert.Contains(t, run.err.Error(), "Run the deploy from the project directory, "+dir)
}

// --dags with --image-name is refused in astro deploy's words, before
// anything is asked: a deploy of DAGs alone has no use for an image.
func TestDeployRefusesDagsWithImageName(t *testing.T) {
	inProject(t, true)
	seen := deployMocks(t, deployPushed, nil)
	run := runAPC(t, newAPCClient(), "", "deploy", "dep-ac", "--dags", "--image-name", "img:1", "-o", "json")
	require.Error(t, run.err)
	assert.EqualError(t, run.err, "--dags deploys only your DAGs; drop --image-name")
	assert.True(t, cliout.IsUsage(run.err))
	assert.Zero(t, seen.dagUploads)

	// --remote names an image in a registry: the same mistake, said the same
	// way, from a project's root, where it used to upload the DAGs alone.
	for _, args := range [][]string{
		{"deploy", "dep-ac", "--dags", "--remote", "-o", "json"},
		{"deploy", "dep-ac", "--dags", "--remote", "--image-name", "img:1", "-o", "json"},
	} {
		run = runAPC(t, newAPCClient(), "", args...)
		require.Error(t, run.err, "%v", args)
		assert.True(t, cliout.IsUsage(run.err), "%v", args)
		assert.Contains(t, run.err.Error(), "--dags deploys only your DAGs; drop ", "%v", args)
		assert.Contains(t, run.err.Error(), "--remote", "%v", args)
	}
	assert.Zero(t, seen.dagUploads)
}
