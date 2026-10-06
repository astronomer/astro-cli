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
	template                     bool
	cleanOutput                  bool
	showWorkloadIdentity         bool
)

// formatYAML is the --output value inspect offers beyond text and json, kept
// because a deployment file is YAML: `inspect --template` output is what
// `astro deployment create --deployment-file` reads, and
// astronomer/deploy-action round-trips it. text renders the same YAML, so the
// default prints what it always has.
const formatYAML cliout.Format = "yaml"

func newDeploymentInspectCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "inspect",
		Aliases: []string{"in"},
		Short:   "Inspect a deployment configuration",
		Long:    "Inspect an Astro Deployment configuration, which can be useful if you manage deployments as code or use Deployment configuration templates. This command returns the Deployment's configuration as YAML (the default, and the format `astro deployment create --deployment-file` reads) or JSON, which includes information about resources, such as cluster ID, region, and Airflow API URL, as well as scheduler and worker queue configurations.",
		Example: `
  $ astro deployment inspect <deployment-id>
  $ astro deployment inspect <deployment-id> --output json
  $ astro deployment inspect --deployment-name my-deployment --key configuration.cluster_id
  $ astro deployment inspect <deployment-id> --template > deployment.yaml
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deploymentInspect(cmd, args, out)
		},
	}
	cmd.Flags().StringVarP(&deploymentName, "deployment-name", "n", "", "Name of the deployment to inspect.")
	cliout.AddOutputFlag(cmd, &outputFormat, formatYAML)
	cmd.PersistentFlags().Lookup("output").Usage += ". text prints the deployment as YAML, the same as yaml."
	cmd.Flags().BoolVarP(&template, "template", "t", false, "Create a template from the deployment being inspected.")
	cmd.Flags().StringVarP(&requestedField, "key", "k", "", "A specific key for the deployment. Use --key configuration.cluster_id to get a deployment's cluster id.")
	cmd.Flags().BoolVarP(&cleanOutput, "clean-output", "c", false, "clean output to only include inspect yaml or json file in any situation.")
	cmd.Flags().BoolVarP(&showWorkloadIdentity, "show-workload-identity", "", false, "Include the workload identity configured for the deployment in the output.")
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

	return inspect.Inspect(wsID, deploymentName, deploymentID, string(format), astroV1Client, out, requestedField, template, showWorkloadIdentity)
}
