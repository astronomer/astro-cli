package astro

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
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
func newRemoteRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote deploys and images",
		Long:  "Commands for interacting with remote registries and deploying client images",
	}
	// Bound for the same reason as dbt: see newDbtCmd.
	cmd.SetOut(out)

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
			// The rule astro deploy follows: a build needs a pyproject.toml
			// project's root, and in or below a project in the Astro CLI 1.x
			// layout, or below a pyproject.toml project's root, everything is
			// refused with the same advice. --image-name builds nothing, so
			// outside any project it pushes the image as it is. The value,
			// not whether the flag was given: an empty --image-name= names
			// no image.
			if project.HasManifest(config.WorkingPath) {
				return nil
			}
			if remoteImageName != "" && utils.Locate(config.WorkingPath) == (utils.Where{}) {
				return nil
			}
			cmd.SilenceUsage = true
			return utils.NoDeployableProject(utils.Deploy1xRefusedAstro)
		},
		RunE:    remoteDeploy,
		Example: remoteDeployExample,
	}

	cmd.Flags().StringVar(&remotePlatform, "platform", "", "Target platform for client image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVarP(&remoteImageName, "image-name", "i", "", "Name of a custom image to deploy, or image name with custom tag; the image must be present on this machine")
	utils.AddBuildSecretFlag(cmd.Flags(), &remoteBuildSecrets)
	cmd.Flags().StringVar(&remoteDeploymentID, "deployment-id", "", "Deployment ID to validate client image runtime version against deployment runtime version")
	addDeploymentFlag(cmd.Flags(), "Deployment whose runtime version the client image is validated against")
	cliout.AddOutputFlag(cmd, &remoteOutput)

	return cmd
}

// remoteOutput is --output for `astro remote deploy`.
var remoteOutput cliout.Format

// deployClientImage builds and pushes the client image. A var so a test can
// stand in for Docker and the registry.
var deployClientImage = astrodeploy.DeployClientImage

// remoteDeploy handles the remote deploy functionality
func remoteDeploy(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true
	// The build's and the push's own output, and the notes on the way, are
	// what text mode has always shown; under json they go to stderr.
	defer strayStdoutToStderr(remoteOutput)()

	deployInput := astrodeploy.InputClientDeploy{
		Path:         config.WorkingPath,
		ImageName:    remoteImageName,
		Platform:     remotePlatform,
		BuildSecrets: util.ResolveBuildSecrets(remoteBuildSecrets, os.Getenv(util.BuildSecretInputEnv)),
		DeploymentID: remoteDeploymentID,
	}

	res, err := deployClientImage(deployInput, astroV1Client)
	if err != nil {
		return err
	}
	return cliout.Renderer{Format: remoteOutput, Out: cmd.OutOrStdout()}.Emit(newRemoteDeployJSON(&res), renderRemoteDeploy(res.Image))
}
