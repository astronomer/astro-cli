package astro

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// astro remote is kept for 1.x's astro remote deploy alone: hidden, with that
// verb under it, hidden too, warning that it is astro deploy agent now.
func TestRemoteRootCmdKeepsOnlyTheDeprecatedDeploy(t *testing.T) {
	cmd := newRemoteRootCmd()
	assert.Equal(t, "remote", cmd.Use)
	assert.True(t, cmd.Hidden)
	require.Len(t, cmd.Commands(), 1)
	deploy := cmd.Commands()[0]
	assert.Equal(t, "deploy", deploy.Use)
	assert.True(t, deploy.Hidden)
	assert.Equal(t, "true", deploy.Annotations[cliout.DeprecatedCommandAnnotation])
	assert.True(t, deploy.Flags().Lookup("deployment-id").Hidden, "--deployment-id is 1.x's spelling of --deployment, kept on the 1.x path")
}

func TestDeployAgentCmd(t *testing.T) {
	cmd := newDeployAgentCmd()

	assert.Equal(t, "agent", cmd.Use)
	assert.Contains(t, cmd.Short, "Remote Execution agent image")
	assert.Contains(t, cmd.Long, "Dockerfile.client")
	assert.Contains(t, cmd.Example, "astro deploy agent")
	assert.Nil(t, cmd.Flags().Lookup("deployment-id"), "a v2 command takes no 1.x spelling")
	assert.Error(t, cmd.Args(cmd, []string{"prod"}), "the Deployment is named with --deployment")

	// Test flags exist
	platformFlag := cmd.Flags().Lookup("platform")
	assert.NotNil(t, platformFlag)
	assert.Equal(t, "", platformFlag.DefValue)
	assert.Contains(t, platformFlag.Usage, "Target platform for the agent image build")

	imageNameFlag := cmd.Flags().Lookup("image-name")
	assert.NotNil(t, imageNameFlag)
	assert.Equal(t, "", imageNameFlag.DefValue)
	assert.Contains(t, imageNameFlag.Usage, "Name of a custom image to deploy")

	deploymentFlag := cmd.Flags().Lookup("deployment")
	assert.NotNil(t, deploymentFlag)
	assert.Equal(t, "", deploymentFlag.DefValue)
	assert.Contains(t, deploymentFlag.Usage, "Deployment whose runtime version the agent image is validated against")
}

func TestDeployAgentCommandFlags(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	t.Run("command accepts platform flag", func(t *testing.T) {
		cmd := newDeployAgentCmd()
		cmd.SetArgs([]string{"--platform", "linux/amd64,linux/arm64"})

		// Parse flags without executing
		err := cmd.ParseFlags([]string{"--platform", "linux/amd64,linux/arm64"})
		assert.NoError(t, err)

		platformValue, err := cmd.Flags().GetString("platform")
		assert.NoError(t, err)
		assert.Equal(t, "linux/amd64,linux/arm64", platformValue)
	})

	t.Run("command accepts image-name flag", func(t *testing.T) {
		cmd := newDeployAgentCmd()

		err := cmd.ParseFlags([]string{"--image-name", "my-custom-image:v1.0"})
		assert.NoError(t, err)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "my-custom-image:v1.0", imageNameValue)
	})

	t.Run("command accepts both flags", func(t *testing.T) {
		cmd := newDeployAgentCmd()

		err := cmd.ParseFlags([]string{"--platform", "linux/amd64", "--image-name", "test-image:latest"})
		assert.NoError(t, err)

		platformValue, err := cmd.Flags().GetString("platform")
		assert.NoError(t, err)
		assert.Equal(t, "linux/amd64", platformValue)

		imageNameValue, err := cmd.Flags().GetString("image-name")
		assert.NoError(t, err)
		assert.Equal(t, "test-image:latest", imageNameValue)
	})

	t.Run("command takes no short flag for image name", func(t *testing.T) {
		cmd := newDeployAgentCmd()

		assert.Error(t, cmd.ParseFlags([]string{"-i", "short-flag-image:tag"}))
	})

	t.Run("command accepts build-secret flag", func(t *testing.T) {
		cmd := newDeployAgentCmd()

		err := cmd.ParseFlags([]string{"--build-secret", "id=mysecret,src=secrets.txt"})
		assert.NoError(t, err)

		buildSecretsValue, err := cmd.Flags().GetStringArray("build-secret")
		assert.NoError(t, err)
		// StringArrayVar preserves the full secret string without comma splitting
		assert.Equal(t, []string{"id=mysecret,src=secrets.txt"}, buildSecretsValue)
	})

	t.Run("command accepts multiple build-secret flags properly", func(t *testing.T) {
		cmd := newDeployAgentCmd()

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

	t.Run("1.x's remote deploy accepts deployment-id", func(t *testing.T) {
		cmd := newDeployAgentCmd(oneXDeploymentID)

		err := cmd.ParseFlags([]string{"--deployment-id", "test-deployment-123"})
		assert.NoError(t, err)

		deploymentIDValue, err := cmd.Flags().GetString("deployment-id")
		assert.NoError(t, err)
		assert.Equal(t, "test-deployment-123", deploymentIDValue)
	})

	t.Run("1.x's remote deploy accepts deployment-id with other flags", func(t *testing.T) {
		cmd := newDeployAgentCmd(oneXDeploymentID)

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

func TestDeployAgentFlags(t *testing.T) {
	cmd := newDeployAgentCmd()

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
		assert.Empty(t, imageNameFlag.Shorthand)
	})

	t.Run("build-secret flag configuration", func(t *testing.T) {
		buildSecretFlag := cmd.Flags().Lookup("build-secret")
		assert.NotNil(t, buildSecretFlag)
		assert.Equal(t, "stringArray", buildSecretFlag.Value.Type())
		assert.Equal(t, "[]", buildSecretFlag.DefValue)
		assert.Contains(t, buildSecretFlag.Usage, "Secret to expose to the build")
	})

	t.Run("deployment flag configuration", func(t *testing.T) {
		deploymentFlag := cmd.Flags().Lookup("deployment")
		assert.NotNil(t, deploymentFlag)
		assert.Equal(t, "string", deploymentFlag.Value.Type())
		assert.Equal(t, "", deploymentFlag.DefValue)
		assert.Contains(t, deploymentFlag.Usage, "runtime version")
	})
}

// Test that ensures deployment validation works correctly in the command context
func TestDeployAgentDeploymentValidation(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)

	for _, flag := range []string{"--deployment", "--deployment-id"} {
		t.Run(flag+" sets global variable", func(t *testing.T) {
			// Reset global variable
			agentDeploymentID = ""

			cmd := newDeployAgentCmd(oneXDeploymentID) // the 1.x path, which takes --deployment-id too
			err := cmd.ParseFlags([]string{flag, "validation-test-deployment"})
			assert.NoError(t, err)
			assert.NoError(t, applyPreferredFlags(cmd, nil))

			// Verify the global variable was set
			assert.Equal(t, "validation-test-deployment", agentDeploymentID)
		})
	}

	t.Run("empty deployment-id works", func(t *testing.T) {
		// Reset global variable
		agentDeploymentID = ""

		cmd := newDeployAgentCmd(oneXDeploymentID) // the 1.x path, which takes --deployment-id too
		err := cmd.ParseFlags([]string{})
		assert.NoError(t, err)

		// Verify the global variable remains empty
		assert.Equal(t, "", agentDeploymentID)
	})

	t.Run("deployment-id with all flags", func(t *testing.T) {
		// Reset global variables
		agentDeploymentID = ""
		agentPlatform = ""
		agentImageName = ""
		agentBuildSecrets = []string{}

		cmd := newDeployAgentCmd(oneXDeploymentID) // the 1.x path, which takes --deployment-id too
		err := cmd.ParseFlags([]string{
			"--deployment-id", "full-test-deployment",
			"--platform", "linux/amd64,linux/arm64",
			"--image-name", "test-image:v1.0",
			"--build-secret", "id=secret1,src=file1.txt",
		})
		assert.NoError(t, err)
		assert.NoError(t, applyPreferredFlags(cmd, nil))

		// Verify all global variables were set correctly
		assert.Equal(t, "full-test-deployment", agentDeploymentID)
		assert.Equal(t, "linux/amd64,linux/arm64", agentPlatform)
		assert.Equal(t, "test-image:v1.0", agentImageName)
		assert.Equal(t, []string{"id=secret1,src=file1.txt"}, agentBuildSecrets)
	})
}

// astro deploy agent is a subcommand of astro deploy, as bundle is.
func TestDeployAgentIsUnderDeploy(t *testing.T) {
	agent, _, err := NewDeployCmd().Find([]string{"agent"})
	require.NoError(t, err)
	assert.Equal(t, "deploy agent", agent.CommandPath())
	assert.Contains(t, agent.Example, "checking its runtime against a Deployment")
}

// deploy agent follows astro deploy's rule: building needs a pyproject.toml
// project, a project in the Astro CLI 1.x layout is refused with the same
// advice, and --image-name, which builds nothing, runs anywhere.
func TestDeployAgentProjectCheck(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prevPath, prevImage := config.WorkingPath, agentImageName
	t.Cleanup(func() { config.WorkingPath, agentImageName = prevPath, prevImage })

	check := func(dir string, args ...string) error {
		config.WorkingPath = dir
		agentImageName = ""
		cmd := newDeployAgentCmd()
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
