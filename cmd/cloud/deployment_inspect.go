package cloud

import (
	"io"

	"github.com/astronomer/astro-cli/cloud/deployment/inspect"

	"github.com/spf13/cobra"
)

var outputFormat, requestedField string

func newDeploymentInspectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect",
		Aliases: []string{"in"},
		Short:   "Inspect a deployment",
		Long:    "Inspect an Astro Deployment.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentInspect(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to inspect.")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "yaml", "Output format can be one of: yaml or json. By default the inspected deployment will be in YAML format.")
	cmd.Flags().StringVarP(&requestedField, "key", "k", "", "A specific key for the deployment. Use --key configuration.cluster_id to get a deployment's cluster id.")
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
	return inspect.Inspect(wsID, deploymentName, deploymentID, outputFormat, astroClient, out, requestedField)
}
