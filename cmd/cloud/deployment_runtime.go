package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	runtimeVersionInput string
)

func newDeploymentUpgradeCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"upg"},
		Short:   "Upgrade a Deployment's runtime version",
		Long:    "Upgrade the Astro Runtime version for an API-driven Astro Deployment. This triggers an image build with the specified version.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentUpgradeRuntime(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&runtimeVersionInput, "version", "v", "", "The Astro Runtime version to upgrade to (e.g. \"3.1-13\").")
	return cmd
}

func deploymentUpgradeRuntime(cmd *cobra.Command, out io.Writer) error {
	if runtimeVersionInput == "" {
		return errors.New("--version is required")
	}

	cmd.SilenceUsage = true
	return deployment.RuntimeVersionSet(workspaceID, deploymentID, deploymentName, runtimeVersionInput, platformCoreClient, astroCoreClient, out)
}
