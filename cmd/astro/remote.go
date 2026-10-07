package astro

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	astrodeploy "github.com/astronomer/astro-cli/internal/platform/astro/deploy"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	remotePlatform     string
	remoteImageName    string
	remoteBuildSecrets = []string{}
	remoteDeploymentID string
)

const (
	remoteDeployExample = `  # Build this project's client image and push it to the remote registry
  astro remote deploy

  # Build it for several platforms
  astro remote deploy --platform linux/amd64,linux/arm64

  # Push an image already built on this machine, checking its runtime against a Deployment's
  astro remote deploy --image-name <IMAGE_NAME> --deployment <DEPLOYMENT_ID>`
)

// newRemoteRootCmd creates the root command for remote operations
func newRemoteRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote deploys and images",
		Long:  "Commands for interacting with remote registries and deploying client images",
	}

	cmd.AddCommand(newRemoteDeployCmd())
	applyPreferredFlagsIn(cmd)
	return cmd
}

// newRemoteDeployCmd creates the remote deploy command
func newRemoteDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a client image to the remote registry",
		Long:  "Build and deploy a client image to the configured remote registry. This command assumes you have already authenticated with the registry.",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// A project with a pyproject.toml has no .astro/config.yaml, so the 1.x check would
			// reject it. astro deploy grew this bypass and its sibling here
			// never did, which left remote deploy refusing every such project
			// with advice to run astro dev init, a command v2 removed.
			if project.HasManifest(config.WorkingPath) {
				return nil
			}
			return utils.EnsureProjectDir(cmd, args)
		},
		RunE:    remoteDeploy,
		Example: remoteDeployExample,
	}

	cmd.Flags().StringVar(&remotePlatform, "platform", "", "Target platform for client image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVarP(&remoteImageName, "image-name", "i", "", "Name of a custom image to deploy, or image name with custom tag; the image must be present on this machine")
	utils.AddBuildSecretFlag(cmd.Flags(), &remoteBuildSecrets)
	cmd.Flags().StringVar(&remoteDeploymentID, "deployment-id", "", "Deployment ID to validate client image runtime version against deployment runtime version")
	addDeploymentFlag(cmd.Flags(), "Deployment whose runtime version the client image is validated against")

	return cmd
}

// remoteDeploy handles the remote deploy functionality
func remoteDeploy(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	deployInput := astrodeploy.InputClientDeploy{
		Path:         config.WorkingPath,
		ImageName:    remoteImageName,
		Platform:     remotePlatform,
		BuildSecrets: util.ResolveBuildSecrets(remoteBuildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		DeploymentID: remoteDeploymentID,
	}

	return astrodeploy.DeployClientImage(deployInput, astroV1Client)
}
