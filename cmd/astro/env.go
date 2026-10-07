package astro

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

// includeSecretsWarning is printed to stderr (so it doesn't pollute piped
// dotenv/JSON output) whenever a list/export call sets --include-secrets.
const includeSecretsWarning = "Warning: --include-secrets returns secret values in the response. Treat the output as sensitive: do not commit, paste into shared channels, or leave on disk longer than necessary." //nolint:gosec // user-facing warning text, not a credential

// addAutoLinkFlag wires --auto-link onto a create/update subcommand.
// At workspace scope, this toggles the platform's "auto-link to all
// deployments" flag on the object. Has no effect at deployment scope.
func addAutoLinkFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&envAutoLink, "auto-link", false, "Link to every deployment in the workspace, including future ones (workspace scope only)")
}

// autoLinkPtr returns nil when --auto-link was not explicitly set (so the
// platform leaves the field alone on update), otherwise &envAutoLink.
func autoLinkPtr(cmd *cobra.Command) *bool {
	if !cmd.Flags().Changed("auto-link") {
		return nil
	}
	return &envAutoLink
}

// formatDotenv is the -o value the environment-variable reads offer beyond
// text and json, declared to cliout as an extra on those commands only: an
// environment variable is the one kind with the KEY=VALUE shape a .env file
// is made of. -o is the format here, as everywhere else in the CLI. It used
// to be a file path, with the format on --format, which made `--output json`
// quietly write a file named json.
const formatDotenv cliout.Format = "dotenv"

// addScopePersistentFlags wires the workspace/deployment scope flags onto a
// subroot. Used by every `astro env <type>` subroot so the scope semantics
// are uniform across types.
func addScopePersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&envWorkspaceID, "workspace-id", "", "Workspace to use (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	cmd.PersistentFlags().StringVar(&envDeploymentID, "deployment-id", "", "Deployment to use instead of a workspace: an id, or a link name inside a project with a pyproject.toml")
	addWorkspaceFlag(cmd.PersistentFlags(), "", "Workspace to use (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	addDeploymentFlag(cmd.PersistentFlags(), "Deployment to use instead of a workspace: an id, or a link name inside a project with a pyproject.toml")
	cmd.PersistentFlags().BoolVar(&envIncludeSecrets, "include-secrets", false, "Show secret values (org policy must allow it)")
	cmd.PersistentFlags().BoolVar(&envResolveLinked, "resolve-linked", true, "Include objects linked from another scope; set to false to see IDs")
}

// envScope resolves the active scope, validating mutual exclusivity and falling
// back to the current workspace from context when neither flag is set.
func envScope() (env.Scope, error) {
	if envWorkspaceID != "" && envDeploymentID != "" {
		return env.Scope{}, env.ErrScopeAmbiguous
	}
	if envWorkspaceID == "" && envDeploymentID == "" {
		ws, err := coalesceWorkspace()
		if err != nil {
			return env.Scope{}, fmt.Errorf("%w (and falling back to workspace context: %s)", env.ErrScopeNotSpecified, err.Error())
		}
		return env.Scope{WorkspaceID: ws}, nil
	}
	return env.Scope{WorkspaceID: envWorkspaceID, DeploymentID: envDeploymentID}, nil
}

// shared flag values for `astro env` subcommands.
var (
	envWorkspaceID    string
	envDeploymentID   string
	envOutput         string
	envIncludeSecrets bool
	envResolveLinked  bool
	envYes            bool

	// var / airflow-var set inputs
	envVarValue    string
	envVarSecret   bool
	envVarNoCreate bool
	envVarFromFile string

	// shared auto-link toggle for set across all four types
	envAutoLink bool

	// connection set inputs
	envConnNoCreate bool
	envConnValue    string
	envConnType     string
	envConnHost     string
	envConnLogin    string
	envConnPassword string
	envConnSchema   string
	envConnPort     int
	envConnExtra    string

	// metrics-export set inputs
	envMetricsNoCreate       bool
	envMetricsEndpoint       string
	envMetricsExporterType   string
	envMetricsAuthType       string
	envMetricsBasicToken     string
	envMetricsUsername       string
	envMetricsPassword       string
	envMetricsSigV4AssumeArn string
	envMetricsSigV4StsRegion string
	envMetricsHeaders        map[string]string
	envMetricsLabels         map[string]string
)

// newEnvListCmd is the cross-kind listing that sits beside the nouns: every
// environment object in the scope, whatever its type, with a KIND column
// telling them apart. `astro local env list` is the same idea on the other
// side, and this is what makes the two trees answer "what is in here?" the
// same way.
//
// It is one API call, not four: the list endpoint documents objectType as a
// filter, so omitting it returns every type.
//
// The columns are what the four per-type listings have left in common once
// their type-specific ones are removed, which means no value column — so this
// is value-free by construction rather than by redaction, and takes no
// --include-secrets.
func newEnvListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List every environment object in the scope",
		Long: "List every environment object in the scope, grouped by kind. Values are not\n" +
			"shown; use a kind's own list to see them.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvList(cmd, out)
		},
		Example: `  # List every environment object in the workspace
  astro env list

  # List one Deployment's, as JSON
  astro env list --deployment <DEPLOYMENT_ID> -o json`,
	}
	cliout.AddOutputFlag(cmd, &envOutput)
	// The scope flags are persistent on each noun rather than on `env`, so a
	// command sitting directly under the group has to register its own. They
	// are worded exactly as addScopePersistentFlags words them, and the
	// mutual exclusion is left to envScope for the same reason: a cobra flag
	// group would answer the same mistake with a different sentence here than
	// on the four nouns, and make env.ErrScopeAmbiguous unreachable.
	//
	// --include-secrets is deliberately absent: it asks the platform to unmask
	// values, and this listing has no column to put one in.
	cmd.Flags().StringVar(&envWorkspaceID, "workspace-id", "", "Workspace to use (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	cmd.Flags().StringVar(&envDeploymentID, "deployment-id", "", "Deployment to use instead of a workspace: an id, or a link name inside a project with a pyproject.toml")
	addWorkspaceFlag(cmd.Flags(), "", "Workspace to use (default: the project's workspace inside a project with a pyproject.toml, else the current one)")
	addDeploymentFlag(cmd.Flags(), "Deployment to use instead of a workspace: an id, or a link name inside a project with a pyproject.toml")
	cmd.Flags().BoolVar(&envResolveLinked, "resolve-linked", true, "Include objects linked from another scope; set to false to see IDs")
	return cmd
}

func runEnvList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	// Checked before the parse, which would refuse it with the generic
	// wording, and before the call: there is no reason to fetch every object
	// in the scope to then refuse to print it.
	if envOutput == string(formatDotenv) {
		cmd.SilenceUsage = true
		return cliout.Usage(env.ErrInventoryHasNoValues)
	}
	f, err := cliout.ParseFormat(envOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	items, err := env.ListInventory(scope, envResolveLinked, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteInventory(items, cliout.Renderer{Format: f, Out: out})
}

func newEnvRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "env",
		Aliases:                    []string{"environment"},
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
		Short:                      "Manage environment objects on Astro",
		Long: `Manage environment objects on Astro: environment variables, connections,
Airflow variables, and metrics exports, scoped to a workspace or a deployment.

Objects here can be shared across deployments from a workspace, or set on
one deployment with --deployment.`,
	}
	cmd.PersistentPreRunE = followProjectPreRun(cmd)
	cmd.SetOut(out)
	// One -o for the whole group, text or json: a set publishes the object
	// as it now is, a delete the object as it was, a link change the links
	// as it left them. The reads that offer dotenv register their own.
	cliout.AddOutputFlag(cmd, &envOutput)
	cmd.AddCommand(
		newEnvVarRootCmd(out),
		newEnvConnRootCmd(out),
		newEnvAirflowVarRootCmd(out),
		newEnvMetricsExportRootCmd(out),
		newEnvListCmd(out),
	)
	applyPreferredFlagsIn(cmd)
	return cmd
}
