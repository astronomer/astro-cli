package cloud

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/astro-client-v1"
	astrov1alpha1 "github.com/astronomer/astro-cli/astro-client-v1alpha1"
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/config"
	v2deploy "github.com/astronomer/astro-cli/internal/deploy"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

const v2ManifestForRouting = `[project]
name = "demo"

[tool.astro]
airflow = "3.1"
`

// resetDeployFlagVars zeroes the package-level deploy flag vars so a routing
// test does not inherit flag state a prior test left behind (cobra binds these
// vars once and never clears them between runs).
func resetDeployFlagVars() {
	dags = false
	image = false
	imageName = ""
	v2Deployment = ""
	v2Workspace = ""
	noDagsBaseDir = false
	waitForDeploy = false
	forceDeploy = false
	forcePrompt = false
	workspaceID = ""
	deploymentName = ""
	deployDescription = ""
	nonDags = false
}

func TestDeployRoutesV2Project(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(v2ManifestForRouting), 0o600))

	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	// A plain `astro deploy` on a v2 project is an image-and-dag deploy, which
	// this slice does not ship. Routing to the v2 path surfaces that refusal,
	// which is how we know it did not take the v1 path.
	err := execDeployCmd()
	require.Error(t, err)
	assert.ErrorIs(t, err, v2deploy.ErrImageDeploy)
}

func TestDeployRoutesV1Project(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	resetDeployFlagVars()

	dir := t.TempDir() // no pyproject.toml, so not a v2 project

	orig := config.WorkingPath
	config.WorkingPath = dir
	t.Cleanup(func() { config.WorkingPath = orig })

	origEnsure := EnsureProjectDir
	origDeploy := DeployImage
	t.Cleanup(func() {
		EnsureProjectDir = origEnsure
		DeployImage = origDeploy
	})

	EnsureProjectDir = func(cmd *cobra.Command, args []string) error { return nil }
	called := false
	DeployImage = func(cloud.InputDeploy, astrov1.APIClient, astrov1alpha1.APIClient) error {
		called = true
		return nil
	}

	err := execDeployCmd("test-deployment-id", "-f", "--workspace-id", "test-ws")
	require.NoError(t, err)
	assert.True(t, called, "a v1 project should run the v1 deploy path")
}
