package astro

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

// includeSecretsWarning is printed to stderr (so it doesn't pollute piped
// dotenv/JSON output) whenever a list/export call sets --include-secrets.
const includeSecretsWarning = "Warning: --include-secrets returns secret values in the response. Treat the output as sensitive: do not commit, paste into shared channels, or leave on disk longer than necessary." //nolint:gosec // user-facing warning text, not a credential

// addAutoLinkFlag wires --auto-link onto a create/update subcommand.
// At workspace scope, this toggles the platform's "auto-link to all
// deployments" flag on the object. Has no effect at deployment scope.
func addAutoLinkFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&envAutoLink, "auto-link", false, "Workspace scope only: automatically link this object to all deployments in the workspace, including future ones.")
}

// autoLinkPtr returns nil when --auto-link was not explicitly set (so the
// platform leaves the field alone on update), otherwise &envAutoLink.
func autoLinkPtr(cmd *cobra.Command) *bool {
	if !cmd.Flags().Changed("auto-link") {
		return nil
	}
	return &envAutoLink
}

// addScopePersistentFlags wires the workspace/deployment scope flags onto a
// subroot. Used by every `astro env <type>` subroot so the scope semantics
// are uniform across types.
func addScopePersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&envWorkspaceID, "workspace-id", "", "Workspace scope (mutually exclusive with --deployment-id). Defaults to the current workspace from context.")
	cmd.PersistentFlags().StringVar(&envDeploymentID, "deployment-id", "", "Deployment scope (mutually exclusive with --workspace-id)")
	cmd.PersistentFlags().BoolVar(&envIncludeSecrets, "include-secrets", false, "Surface secret values (requires org policy to allow)")
	cmd.PersistentFlags().BoolVar(&envResolveLinked, "resolve-linked", true, "Include objects linked from another scope (e.g. workspace -> deployment). In this mode IDs are not returned, since they refer to resolved rows that aren't directly addressable. Use --resolve-linked=false to see IDs.")
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
	envFormat         string
	envOutputPath     string
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
		Short:   "List environment objects of every kind in the scope",
		Long: "List every environment object the scope holds — variables, connections, Airflow variables and metrics exports — with the kind that manages each one.\n\n" +
			"This listing never prints a value. The per-kind listings (`astro env variable list` and friends) show the type-specific columns and take --include-secrets; this one answers what exists.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvList(cmd, out)
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	// The scope flags are persistent on each noun rather than on `env`, so a
	// command sitting directly under the group has to register its own. They
	// are worded exactly as addScopePersistentFlags words them, and the
	// mutual exclusion is left to envScope for the same reason: a cobra flag
	// group would answer the same mistake with a different sentence here than
	// on the four nouns, and make env.ErrScopeAmbiguous unreachable.
	//
	// --include-secrets is deliberately absent: it asks the platform to unmask
	// values, and this listing has no column to put one in.
	cmd.Flags().StringVar(&envWorkspaceID, "workspace-id", "", "Workspace scope (mutually exclusive with --deployment-id). Defaults to the current workspace from context.")
	cmd.Flags().StringVar(&envDeploymentID, "deployment-id", "", "Deployment scope (mutually exclusive with --workspace-id)")
	cmd.Flags().BoolVar(&envResolveLinked, "resolve-linked", true, "Include objects linked from another scope (e.g. workspace -> deployment). In this mode IDs are not returned, since they refer to resolved rows that aren't directly addressable. Use --resolve-linked=false to see IDs.")
	return cmd
}

func runEnvList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	// Checked before the call, not after rendering: there is no reason to
	// fetch every object in the scope to then refuse to print it. The same
	// sentinel guards WriteInventory, so the two cannot disagree.
	if f == env.FormatDotenv {
		return env.ErrInventoryHasNoValues
	}

	items, err := env.ListInventory(scope, envResolveLinked, astroV1Client)
	if err != nil {
		return err
	}
	w, closer, err := openOutput(out)
	if err != nil {
		return err
	}
	defer closer()
	return env.WriteInventory(items, f, w)
}

func newEnvRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "env",
		Aliases:                    []string{"environment"},
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
		Short:                      "Manage a Deployment's environment objects on Astro",
		Long: `Manage Astronomer environment-manager objects: workspace- or deployment-scoped
environment variables, connections, Airflow variables, and metrics exports.

This command tree is distinct from 'astro deployment variable' (which writes to the
deployment record directly) and 'astro deployment connection' (which talks to Airflow's
metadata database). Use 'astro env' for objects that should be shared across deployments
or managed at workspace scope.`,
	}
	cmd.SetOut(out)
	cmd.AddCommand(
		newEnvVarRootCmd(out),
		newEnvConnRootCmd(out),
		newEnvAirflowVarRootCmd(out),
		newEnvMetricsExportRootCmd(out),
		newEnvListCmd(out),
	)
	return cmd
}
