package cloud

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
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

	// var / airflow-var create + update inputs
	envVarKey    string
	envVarValue  string
	envVarSecret bool
	envVarStrict bool

	// shared auto-link toggle for create + update across all four types
	envAutoLink bool

	// connection create + update inputs
	envConnKey      string
	envConnType     string
	envConnHost     string
	envConnLogin    string
	envConnPassword string
	envConnSchema   string
	envConnPort     int
	envConnExtra    string

	// metrics-export create + update inputs
	envMetricsKey            string
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

func newEnvRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "env",
		Aliases: []string{"environment"},
		Short:   "Manage platform environment objects (variables, connections, etc.)",
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
	)
	return cmd
}
