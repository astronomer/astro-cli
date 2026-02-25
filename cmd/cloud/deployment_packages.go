package cloud

import (
	"io"
	"os"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	packagesFile  string
	packagesInput string
)

func newDeploymentPackagesRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "packages",
		Aliases: []string{"pkg"},
		Short:   "Manage deployment system packages",
		Long:    "Manage system-level OS packages for an API-driven Astro Deployment. Setting packages triggers an image build that installs the specified packages.",
	}
	cmd.AddCommand(
		newDeploymentPackagesGetCmd(out),
		newDeploymentPackagesSetCmd(out),
	)
	return cmd
}

func newDeploymentPackagesGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Get a Deployment's packages",
		Long:    "Get the current system packages for an API-driven Astro Deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPackagesGet(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	return cmd
}

func newDeploymentPackagesSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set",
		Aliases: []string{"s"},
		Short:   "Set a Deployment's packages",
		Long:    "Set the system packages for an API-driven Astro Deployment. This triggers an image build that installs the specified packages.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentPackagesSet(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&packagesFile, "file", "f", "", "Path to a packages file (one package per line).")
	cmd.Flags().StringVarP(&packagesInput, "input", "i", "", "Inline packages string (e.g. \"git\\ncurl\").")
	return cmd
}

func deploymentPackagesGet(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return deployment.PackagesGet(workspaceID, deploymentID, deploymentName, platformCoreClient, astroCoreClient, out)
}

func deploymentPackagesSet(cmd *cobra.Command, out io.Writer) error {
	if packagesFile == "" && packagesInput == "" {
		return errors.New("one of --file or --input is required")
	}
	if packagesFile != "" && packagesInput != "" {
		return errors.New("only one of --file or --input can be specified")
	}

	var content string
	if packagesFile != "" {
		data, err := os.ReadFile(packagesFile)
		if err != nil {
			return errors.Wrap(err, "failed to read packages file")
		}
		content = string(data)
	} else {
		content = packagesInput
	}

	cmd.SilenceUsage = true
	return deployment.PackagesSet(workspaceID, deploymentID, deploymentName, content, platformCoreClient, astroCoreClient, out)
}
