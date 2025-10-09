package cloud

import (
	cloud "github.com/astronomer/astro-cli/cloud/deploy"
	"github.com/astronomer/astro-cli/cmd/utils"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/spf13/cobra"
)

var (
	remotePlatform     string
	remoteImageName    string
	remoteBuildSecrets = []string{}
)

const (
	remoteDeployExample = `
Deploy a client image to the remote registry:

  $ astro remote deploy

Deploy with a specific platform:

  $ astro remote deploy --platform linux/amd64,linux/arm64

Deploy a pre-built image:

  $ astro remote deploy --image-name my-custom-image:tag

Deploy with build secrets:

  $ astro remote deploy --build-secrets id=mysecret,src=secrets.txt
`
)

// newRemoteRootCmd creates the root command for remote operations
func newRemoteRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote deploys and images",
		Long:  "Commands for interacting with remote registries and deploying client images",
	}

	cmd.AddCommand(newRemoteDeployCmd())
	return cmd
}

// newRemoteDeployCmd creates the remote deploy command
func newRemoteDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy",
		Short:   "Deploy a client image to the remote registry",
		Long:    "Build and deploy a client image to the configured remote registry. This command assumes you have already authenticated with the registry.",
		PreRunE: utils.EnsureProjectDir,
		RunE:    remoteDeploy,
		Example: remoteDeployExample,
	}

	cmd.Flags().StringVar(&remotePlatform, "platform", "", "Target platform for client image build (e.g., linux/amd64,linux/arm64). Defaults to host machine platform")
	cmd.Flags().StringVarP(&remoteImageName, "image-name", "i", "", "Name of a custom image to deploy, or image name with custom tag. The image should be present on the local machine.")
	cmd.Flags().StringArrayVar(&remoteBuildSecrets, "build-secrets", []string{}, "Mimics docker build --secret flag. See https://docs.docker.com/build/building/secrets/ for more information. Example input id=mysecret,src=secrets.txt")

	return cmd
}

// remoteDeploy handles the remote deploy functionality
func remoteDeploy(cmd *cobra.Command, args []string) error {
	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	buildSecretString := util.GetbuildSecretString(remoteBuildSecrets)

	deployInput := cloud.InputClientDeploy{
		Path:              config.WorkingPath,
		ImageName:         remoteImageName,
		Platform:          remotePlatform,
		BuildSecretString: buildSecretString,
	}

	return cloud.DeployClientImage(deployInput)
}
