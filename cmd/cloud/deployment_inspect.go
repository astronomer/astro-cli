package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/deployment/inspect"

	"github.com/spf13/cobra"
)

var (
	outputFormat, requestedField string
	template                     bool
	cleanOutput                  bool
)

func newDeploymentInspectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect",
		Aliases: []string{"in"},
		Short:   "Inspect a deployment configuration",
		Long:    "Inspect an Astro Deployment configuration, which can be useful if you manage deployments as code or use Deployment configuration templates. This command returns the Deployment's configuration as a YAML or JSON output, which includes information about resources, such as cluster ID, region, and Airflow API URL, as well as scheduler and worker queue configurations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentInspect(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to inspect.")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "yaml", "Output format can be one of: yaml or json. By default the inspected deployment will be in YAML format.")
	cmd.Flags().BoolVarP(&template, "template", "t", false, "Create a template from the deployment being inspected.")
	cmd.Flags().StringVarP(&requestedField, "key", "k", "", "A specific key for the deployment. Use --key configuration.cluster_id to get a deployment's cluster id.")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "c", false, "clean output to only include inspect yaml or json file in any situation.")
	return cmd
}

func deploymentInspect(cmd *cobra.Command, args []string, out io.Writer) error {
	cmd.SilenceUsage = true

	wsID, err := coalesceWorkspace()
	if err != nil {
		return err
	}

	if len(args) > 0 {
		deploymentID = args[0]
	}

	// clean output
	deployment.CleanOutput = cleanOutput

	return inspect.Inspect(wsID, deploymentName, deploymentID, outputFormat, platformCoreClient, astroCoreClient, out, requestedField, template)
}
