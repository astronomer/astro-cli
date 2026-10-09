package apc

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			assert.Equal(t, "image", got.Type)
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
