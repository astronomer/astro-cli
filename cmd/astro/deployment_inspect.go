package astro

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
)

var (
	outputFormat         cliout.Format
	requestedField       string
	cleanOutput          bool
	showWorkloadIdentity bool
)

// formatYAML is the --output value inspect offers beyond text and json, kept
// because inspect has always printed YAML and scripts read it. text renders
// the same YAML, so the default prints what it always has.
const formatYAML cliout.Format = "yaml"

func newDeploymentInspectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect [DEPLOYMENT_ID]",
		Aliases: []string{"in"},
		Short:   "Inspect a deployment configuration",
		Long:    "Inspect an Astro Deployment configuration. This command returns the Deployment's configuration as YAML (the default) or JSON, which includes information about resources, such as cluster ID, region, and Airflow API URL, as well as scheduler and worker queue configurations.",
		Example: `  # Show a Deployment's configuration as YAML
  astro deployment inspect <DEPLOYMENT_ID>

  # Show it as JSON
  astro deployment inspect <DEPLOYMENT_ID> --output json

  # Print one value from it
  astro deployment inspect --deployment my-deployment --key configuration.cluster_id`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentInspect(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to inspect")
	addDeploymentFlag(cmd.Flags(), "Deployment to inspect: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cliout.AddOutputFlag(cmd, &outputFormat, formatYAML)
	cmd.PersistentFlags().Lookup("output").Usage += " (text prints the deployment as YAML, the same as yaml)"
	cmd.Flags().StringVarP(&requestedField, "key", "k", "", "Print only this key of the configuration, such as configuration.cluster_id")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "c", false, "Print only the Deployment's YAML or JSON, with no other output")
	cmd.Flags().BoolVarP(&showWorkloadIdentity, "show-workload-identity", "", false, "Include the workload identity configured for the deployment in the output")
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

	// json goes through the CLI's one encoder, so it is pretty on a terminal
	// and compact when piped like every other result; yaml is text here, the
	// renderer's text being the YAML. --key bypasses both and prints the bare
	// value to out, as it always has.
	format := outputFormat
	if format == formatYAML {
		format = cliout.FormatText
	}
	r := cliout.Renderer{Format: format, Out: out}
	return inspect.Print(wsID, deploymentName, deploymentID, astroV1Client, out, r, requestedField, showWorkloadIdentity)
}
