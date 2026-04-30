package cloud

import (
	"io"

	"github.com/spf13/cobra"
)

// shared flag values for `astro env` subcommands.
var (
	envWorkspaceID    string
	envDeploymentID   string
	envFormat         string
	envOutputPath     string
	envIncludeSecrets bool
	envResolveLinked  bool
	envYes            bool

	// var-create / var-update inputs
	envVarKey    string
	envVarValue  string
	envVarSecret bool
	envVarStrict bool
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
	)
	return cmd
}
