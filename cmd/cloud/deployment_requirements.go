package cloud

import (
	"io"
	"os"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	requirementsFile  string
	requirementsInput string
)

func newDeploymentRequirementsRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "requirements",
		Aliases: []string{"req"},
		Short:   "Manage deployment Python requirements",
		Long:    "Manage Python pip requirements for an API-driven Astro Deployment. Setting requirements triggers an image build that installs the specified packages.",
	}
	cmd.AddCommand(
		newDeploymentRequirementsGetCmd(out),
		newDeploymentRequirementsSetCmd(out),
	)
	return cmd
}

func newDeploymentRequirementsGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"g"},
		Short:   "Get a Deployment's requirements",
		Long:    "Get the current Python pip requirements for an API-driven Astro Deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentRequirementsGet(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	return cmd
}

func newDeploymentRequirementsSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set",
		Aliases: []string{"s"},
		Short:   "Set a Deployment's requirements",
		Long:    "Set the Python pip requirements for an API-driven Astro Deployment. This triggers an image build that installs the specified packages.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentRequirementsSet(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentID, "deployment-id", "d", "", "The ID of the Deployment.")
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "The name of the Deployment.")
	cmd.Flags().StringVarP(&requirementsFile, "file", "f", "", "Path to a requirements.txt file.")
	cmd.Flags().StringVarP(&requirementsInput, "input", "i", "", "Inline requirements string (e.g. \"pandas==2.0\\nrequests\").")
	return cmd
}

func deploymentRequirementsGet(cmd *cobra.Command, out io.Writer) error {
	cmd.SilenceUsage = true
	return deployment.RequirementsGet(workspaceID, deploymentID, deploymentName, platformCoreClient, astroCoreClient, out)
}

func deploymentRequirementsSet(cmd *cobra.Command, out io.Writer) error {
	if requirementsFile == "" && requirementsInput == "" {
		return errors.New("one of --file or --input is required")
	}
	if requirementsFile != "" && requirementsInput != "" {
		return errors.New("only one of --file or --input can be specified")
	}

	var content string
	if requirementsFile != "" {
		data, err := os.ReadFile(requirementsFile)
		if err != nil {
			return errors.Wrap(err, "failed to read requirements file")
		}
		content = string(data)
	} else {
		content = requirementsInput
	}

	cmd.SilenceUsage = true
	return deployment.RequirementsSet(workspaceID, deploymentID, deploymentName, content, platformCoreClient, astroCoreClient, out)
}
