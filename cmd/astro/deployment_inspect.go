package astro

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment/inspect"
)

var (
	outputFormat, requestedField string
	cleanOutput                  bool
	showWorkloadIdentity         bool
)

// formatYAML is the --output value inspect offers beyond text and json, kept
// because inspect has always printed YAML and scripts read it. text renders
// the same YAML, so the default prints what it always has.
const formatYAML cliout.Format = "yaml"

func newDeploymentInspectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect",
		Aliases: []string{"in"},
		Short:   "Inspect a deployment configuration",
		Long:    "Inspect an Astro Deployment configuration. This command returns the Deployment's configuration as YAML (the default) or JSON, which includes information about resources, such as cluster ID, region, and Airflow API URL, as well as scheduler and worker queue configurations.",
		Example: `
  $ astro deployment inspect <deployment-id>
  $ astro deployment inspect <deployment-id> --output json
  $ astro deployment inspect --deployment my-deployment --key configuration.cluster_id
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentInspect(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to inspect.")
	addDeploymentFlag(cmd.Flags(), "Deployment to inspect: a link name from pyproject.toml, a Deployment id, or a Deployment name")
	cliout.AddOutputFlag(cmd, &outputFormat, formatYAML)
	cmd.PersistentFlags().Lookup("output").Usage += ". text prints the deployment as YAML, the same as yaml."
	cmd.Flags().StringVarP(&requestedField, "key", "k", "", "A specific key for the deployment. Use --key configuration.cluster_id to get a deployment's cluster id.")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "c", false, "clean output to only include inspect yaml or json file in any situation.")
	cmd.Flags().BoolVarP(&showWorkloadIdentity, "show-workload-identity", "", false, "Include the workload identity configured for the deployment in the output.")
	addRemovedFlag(cmd, "template", "t", true, errTemplateRemoved)
	return cmd
}

func deploymentInspect(cmd *cobra.Command, args []string, out io.Writer) error {
	cmd.SilenceUsage = true

	format, err := cliout.ParseFormat(outputFormat, formatYAML)
	if err != nil {
		return err
	}
	if format == cliout.FormatText {
		format = formatYAML
	}

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
	// and compact when piped like every other result; yaml (and text, which
	// is yaml) is the renderer's text. --key bypasses both and prints the bare
	// value to out, as it always has.
	r := cliout.Renderer{Format: format, Out: out}
	return inspect.Print(wsID, deploymentName, deploymentID, astroV1Client, out, r, requestedField, showWorkloadIdentity)
}
