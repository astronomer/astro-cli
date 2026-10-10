package astro

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

func TestRemoteRootCmd(t *testing.T) {
	cmd := newRemoteRootCmd(os.Stdout)
	assert.Equal(t, "remote", cmd.Use)
	assert.Equal(t, "Manage remote deploys and images", cmd.Short)
	assert.True(t, cmd.HasSubCommands())

	// Check that it has the deploy subcommand
	deployCmd := cmd.Commands()[0]
	assert.Equal(t, "deploy", deployCmd.Use)
}

func TestRemoteDeployCmd(t *testing.T) {
	cmd := newRemoteDeployCmd()

	// Test command properties
	assert.Equal(t, "deploy", cmd.Use)
	assert.Equal(t, "Deploy a client image to the remote registry", cmd.Short)
	assert.Contains(t, cmd.Long, "Build and deploy a client image")
	assert.Contains(t, cmd.Example, "astro remote deploy")

	// Test flags exist
	platformFlag := cmd.Flags().Lookup("platform")
	assert.NotNil(t, platformFlag)
	assert.Equal(t, "", platformFlag.DefValue)
	assert.Contains(t, platformFlag.Usage, "Target platform for client image build")

	imageNameFlag := cmd.Flags().Lookup("image-name")
	assert.NotNil(t, imageNameFlag)
	assert.Equal(t, "", imageNameFlag.DefValue)
	assert.Contains(t, imageNameFlag.Usage, "Name of a custom image to deploy")

	deploymentIDFlag := cmd.Flags().Lookup("deployment-id")
	assert.NotNil(t, deploymentIDFlag)
	assert.Equal(t, "", deploymentIDFlag.DefValue)
	assert.Contains(t, deploymentIDFlag.Usage, "Deployment ID to validate client image runtime version")
}

func TestRemoteDeployCommandFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("command accepts platform flag", func(t *testing.T) {
		cmd := newRemoteDeployCmd()
		cmd.SetArgs([]string{"--platform", "linux/amd64,linux/arm64"})

		// Parse flags without executing
		err := cmd.ParseFlags([]string{"--platform", "linux/amd64,linux/arm64"})
		assert.NoError(t, err)

		platformValue, err := cmd.Flags().GetString("platform")
		assert.NoError(t, err)
		assert.Equal(t, "linux/amd64,linux/arm64", platformValue)
	})

	t.Run("command accepts image-name flag", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"--image-name", "my-custom-image:v1.0"})
		assert.NoError(t, err)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "my-custom-image:v1.0", imageNameValue)
	})

	t.Run("command accepts both flags", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"--platform", "linux/amd64", "--image-name", "test-image:latest"})
		assert.NoError(t, err)

		platformValue, err := cmd.Flags().GetString("platform")
		assert.NoError(t, err)
		assert.Equal(t, "linux/amd64", platformValue)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "test-image:latest", imageNameValue)
	})

	t.Run("command accepts short flag for image name", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"-i", "short-flag-image:tag"})
		assert.NoError(t, err)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "short-flag-image:tag", imageNameValue)
	})

	t.Run("command accepts build-secret flag", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"--build-secret", "id=mysecret,src=secrets.txt"})
		assert.NoError(t, err)

		buildSecretsValue, err := cmd.Flags().GetStringArray("build-secret")
		assert.NoError(t, err)
		// StringArrayVar preserves the full secret string without comma splitting
		assert.Equal(t, []string{"id=mysecret,src=secrets.txt"}, buildSecretsValue)
	})

	t.Run("command accepts multiple build-secret flags properly", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		// Proper way to pass multiple secrets (each as a separate flag)
		err := cmd.ParseFlags([]string{
			"--build-secret", "id=secret1,src=file1.txt",
			"--build-secret", "id=secret2,src=file2.txt",
		})
		assert.NoError(t, err)

		buildSecretsValue, err := cmd.Flags().GetStringArray("build-secret")
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=secret1,src=file1.txt", "id=secret2,src=file2.txt"}, buildSecretsValue)
	})

	t.Run("command accepts deployment-id flag", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"--deployment-id", "test-deployment-123"})
		assert.NoError(t, err)

		deploymentIDValue, err := cmd.Flags().GetString("deployment-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-deployment-123", deploymentIDValue)
	})

	t.Run("command accepts deployment-id with other flags", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{
			"--deployment-id", "my-deployment",
			"--platform", "linux/amd64",
			"--image-name", "custom-image:latest",
		})
		assert.NoError(t, err)

		deploymentIDValue, err := cmd.Flags().GetString("deployment-id")
		assert.NoError(t, err)
		assert.Equal(t, "my-deployment", deploymentIDValue)

		platformValue, err := cmd.Flags().GetString("platform")
		assert.NoError(t, err)
		assert.Equal(t, "linux/amd64", platformValue)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "custom-image:latest", imageNameValue)
	})
}

func TestRemoteDeployFlags(t *testing.T) {
	cmd := newRemoteDeployCmd()

	t.Run("platform flag configuration", func(t *testing.T) {
		platformFlag := cmd.Flags().Lookup("platform")
		assert.NotNil(t, platformFlag)
		assert.Equal(t, "string", platformFlag.Value.Type())
		assert.Equal(t, "", platformFlag.DefValue)
	})

	t.Run("image-name flag configuration", func(t *testing.T) {
		imageNameFlag := cmd.Flags().Lookup("image-name")
		assert.NotNil(t, imageNameFlag)
		assert.Equal(t, "string", imageNameFlag.Value.Type())
		assert.Equal(t, "", imageNameFlag.DefValue)
		assert.Equal(t, "i", imageNameFlag.Shorthand)
	})

	t.Run("build-secret flag configuration", func(t *testing.T) {
		buildSecretFlag := cmd.Flags().Lookup("build-secret")
		assert.NotNil(t, buildSecretFlag)
		assert.Equal(t, "stringArray", buildSecretFlag.Value.Type())
		assert.Equal(t, "[]", buildSecretFlag.DefValue)
		assert.Contains(t, buildSecretFlag.Usage, "Secret to expose to the build")
	})

	t.Run("deployment-id flag configuration", func(t *testing.T) {
		deploymentIDFlag := cmd.Flags().Lookup("deployment-id")
		assert.NotNil(t, deploymentIDFlag)
		assert.Equal(t, "string", deploymentIDFlag.Value.Type())
		assert.Equal(t, "", deploymentIDFlag.DefValue)
		assert.Contains(t, deploymentIDFlag.Usage, "Deployment ID to validate")
		assert.Contains(t, deploymentIDFlag.Usage, "runtime version")
	})
}

// Test that ensures deployment validation works correctly in the command context
func TestRemoteDeployDeploymentValidation(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("deployment-id flag sets global variable", func(t *testing.T) {
		// Reset global variable
		remoteDeploymentID = ""

		cmd := newRemoteDeployCmd()
		err := cmd.ParseFlags([]string{"--deployment-id", "validation-test-deployment"})
		assert.NoError(t, err)

		// Verify the global variable was set
		assert.Equal(t, "validation-test-deployment", remoteDeploymentID)
	})

	t.Run("empty deployment-id works", func(t *testing.T) {
		// Reset global variable
		remoteDeploymentID = ""

		cmd := newRemoteDeployCmd()
		err := cmd.ParseFlags([]string{})
		assert.NoError(t, err)

		// Verify the global variable remains empty
		assert.Equal(t, "", remoteDeploymentID)
	})

	t.Run("deployment-id with all flags", func(t *testing.T) {
		// Reset global variables
		remoteDeploymentID = ""
		remotePlatform = ""
		remoteImageName = ""
		remoteBuildSecrets = []string{}

		cmd := newRemoteDeployCmd()
		err := cmd.ParseFlags([]string{
			"--deployment-id", "full-test-deployment",
			"--platform", "linux/amd64,linux/arm64",
			"--image-name", "test-image:v1.0",
			"--build-secret", "id=secret1,src=file1.txt",
		})
		assert.NoError(t, err)

		// Verify all global variables were set correctly
		assert.Equal(t, "full-test-deployment", remoteDeploymentID)
		assert.Equal(t, "linux/amd64,linux/arm64", remotePlatform)
		assert.Equal(t, "test-image:v1.0", remoteImageName)
		assert.Equal(t, []string{"id=secret1,src=file1.txt"}, remoteBuildSecrets)
	})
}

// Test that ensures the remote command integrates properly with the root command
func TestRemoteCommandIntegration(t *testing.T) {
	rootCmd := newRemoteRootCmd(os.Stdout)

	t.Run("remote root has deploy subcommand", func(t *testing.T) {
		deployCmd, _, err := rootCmd.Find([]string{"deploy"})
		assert.NoError(t, err)
		assert.Equal(t, "deploy", deployCmd.Use)
	})

	t.Run("help text is appropriate", func(t *testing.T) {
		assert.Contains(t, rootCmd.Long, "remote registries")
		assert.Contains(t, rootCmd.Long, "client images")
	})

	t.Run("deploy command includes deployment validation example", func(t *testing.T) {
		deployCmd, _, err := rootCmd.Find([]string{"deploy"})
		assert.NoError(t, err)
		assert.Contains(t, deployCmd.Example, "--deployment ")
		assert.Contains(t, deployCmd.Example, "checking its runtime against a Deployment")
	})
}

// remote deploy follows astro deploy's rule: building needs a pyproject.toml
// project, a project in the Astro CLI 1.x layout is refused with the same
// advice, and --image-name, which builds nothing, runs anywhere.
func TestRemoteDeployProjectCheck(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prevPath, prevImage := config.WorkingPath, remoteImageName
	t.Cleanup(func() { config.WorkingPath, remoteImageName = prevPath, prevImage })

	check := func(dir string, args ...string) error {
		config.WorkingPath = dir
		remoteImageName = ""
		cmd := newRemoteDeployCmd()
		require.NoError(t, cmd.ParseFlags(args))
		return cmd.PreRunE(cmd, nil)
	}

	empty := t.TempDir()
	assert.ErrorContains(t, check(empty), "not an Astro project directory")
	assert.NoError(t, check(empty, "--image-name", "img"))
	assert.Error(t, check(empty, "--image-name="), "an empty --image-name= names no image")

	oneX := make1xProject(t)
	assert.ErrorContains(t, check(oneX), "uses the Astro CLI 1.x layout")
	assert.ErrorContains(t, check(oneX, "--image-name", "img"), "uses the Astro CLI 1.x layout", "a 1.x checkout is refused, --image-name included")

	manifestProject := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(manifestProject, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n\n[tool.astro]\n"), 0o600))
	assert.NoError(t, check(manifestProject))
}
