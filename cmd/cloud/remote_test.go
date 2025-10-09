package cloud

import (
	"testing"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
	"github.com/stretchr/testify/assert"
)

func TestRemoteRootCmd(t *testing.T) {
	cmd := newRemoteRootCmd()
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

	t.Run("command accepts build-secrets flag", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		err := cmd.ParseFlags([]string{"--build-secrets", "id=mysecret,src=secrets.txt"})
		assert.NoError(t, err)

		buildSecretsValue, err := cmd.Flags().GetStringArray("build-secrets")
		assert.NoError(t, err)
		// StringArrayVar preserves the full secret string without comma splitting
		assert.Equal(t, []string{"id=mysecret,src=secrets.txt"}, buildSecretsValue)
	})

	t.Run("command accepts multiple build-secrets properly", func(t *testing.T) {
		cmd := newRemoteDeployCmd()

		// Proper way to pass multiple secrets (each as a separate flag)
		err := cmd.ParseFlags([]string{
			"--build-secrets", "id=secret1,src=file1.txt",
			"--build-secrets", "id=secret2,src=file2.txt",
		})
		assert.NoError(t, err)

		buildSecretsValue, err := cmd.Flags().GetStringArray("build-secrets")
		assert.NoError(t, err)
		assert.Equal(t, []string{"id=secret1,src=file1.txt", "id=secret2,src=file2.txt"}, buildSecretsValue)
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

	t.Run("build-secrets flag configuration", func(t *testing.T) {
		buildSecretsFlag := cmd.Flags().Lookup("build-secrets")
		assert.NotNil(t, buildSecretsFlag)
		assert.Equal(t, "stringArray", buildSecretsFlag.Value.Type())
		assert.Equal(t, "[]", buildSecretsFlag.DefValue)
		assert.Contains(t, buildSecretsFlag.Usage, "docker build --secret")
	})
}

// Test that ensures the remote command integrates properly with the root command
func TestRemoteCommandIntegration(t *testing.T) {
	rootCmd := newRemoteRootCmd()

	t.Run("remote root has deploy subcommand", func(t *testing.T) {
		deployCmd, _, err := rootCmd.Find([]string{"deploy"})
		assert.NoError(t, err)
		assert.Equal(t, "deploy", deployCmd.Use)
	})

	t.Run("help text is appropriate", func(t *testing.T) {
		assert.Contains(t, rootCmd.Long, "remote registries")
		assert.Contains(t, rootCmd.Long, "client images")
	})
}
