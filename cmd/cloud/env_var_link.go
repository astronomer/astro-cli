package cloud

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
)

var (
	envLinkVariableID   string
	envLinkVariableKey  string
	envLinkDeploymentID string
	envLinkValue        string
	envLinkExclude      bool
)

const envVarLinkExamples = `
  # link a workspace variable to a deployment
  astro env variable link create --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id>

  # link with a per-deployment override value
  astro env variable link create --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id> --value postgres://prod

  # exclude a deployment from an auto-linked variable
  astro env variable link create --variable-key LOG_LEVEL --workspace-id <ws-id> --deployment-id <dep-id> --exclude

  # remove a link (or an exclude, with --exclude)
  astro env variable link delete --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id>

  # show every deployment a variable is linked to or excluded from
  astro env variable link list --variable-key DATABASE_URL --workspace-id <ws-id>
`

func newEnvVarLinkRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "link",
		Aliases: []string{"links"},
		Short:   "Manage deployment links for workspace variables",
		Long:    "Create, delete, or list the explicit per-deployment links (and auto-link excludes) of a workspace-scoped environment variable. Identify the variable with --variable-id or --variable-key.",
		Example: envVarLinkExamples,
	}
	cmd.AddCommand(
		newEnvVarLinkCreateCmd(out),
		newEnvVarLinkDeleteCmd(out),
		newEnvVarLinkListCmd(out),
	)
	return cmd
}

// addLinkVariableFlags wires the parent-variable identifier flags onto a link
// subcommand: exactly one of --variable-id / --variable-key is required.
func addLinkVariableFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&envLinkVariableID, "variable-id", "", "ID of the workspace variable")
	cmd.Flags().StringVar(&envLinkVariableKey, "variable-key", "", "Key of the workspace variable")
	cmd.MarkFlagsMutuallyExclusive("variable-id", "variable-key")
	cmd.MarkFlagsOneRequired("variable-id", "variable-key")
}

// linkVariableIDOrKey returns whichever variable identifier flag was set; the
// env package resolves IDs and keys uniformly. Errors when the set flag is
// empty (cobra's one-required group counts --variable-id "" as set).
func linkVariableIDOrKey() (string, error) {
	if envLinkVariableID != "" {
		return envLinkVariableID, nil
	}
	if envLinkVariableKey != "" {
		return envLinkVariableKey, nil
	}
	return "", errors.New("--variable-id or --variable-key cannot be empty")
}

func newEnvVarLinkCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Link a workspace variable to a deployment, optionally with an override",
		Long: `Attach a workspace-scoped environment variable to a specific deployment.
If --value is provided, that value overrides the workspace default for the linked deployment only.
Pass --exclude to add the deployment to the excludeLinks list instead (used with --auto-link to opt specific deployments out).

Upsert semantics: if not already linked, the link is created; if already linked, the override is replaced when --value is passed. Re-running without --value is a no-op for an existing link's override (the platform preserves fields omitted from a link entry). To remove an existing override, delete the link then re-create it without --value.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarLinkCreate(cmd, out)
		},
	}
	addLinkVariableFlags(cmd)
	cmd.Flags().StringVar(&envLinkDeploymentID, "deployment-id", "", "ID of the deployment to link (required)")
	cmd.Flags().StringVar(&envLinkValue, "value", "", "Override value to use for the linked deployment (only the linked deployment sees this value)")
	cmd.Flags().BoolVar(&envLinkExclude, "exclude", false, "Add to excludeLinks instead of links (auto-link only)")
	_ = cmd.MarkFlagRequired("deployment-id")
	cmd.MarkFlagsMutuallyExclusive("value", "exclude")
	return cmd
}

func newEnvVarLinkDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		Short:   "Remove an explicit link or exclude between a workspace variable and a deployment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarLinkDelete(cmd, out)
		},
	}
	addLinkVariableFlags(cmd)
	cmd.Flags().StringVar(&envLinkDeploymentID, "deployment-id", "", "ID of the deployment to unlink (required)")
	cmd.Flags().BoolVar(&envLinkExclude, "exclude", false, "Remove from excludeLinks instead of links")
	_ = cmd.MarkFlagRequired("deployment-id")
	return cmd
}

func newEnvVarLinkListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Show every deployment a workspace variable is linked to or excluded from",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarLinkList(cmd, out)
		},
	}
	addLinkVariableFlags(cmd)
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func runEnvVarLinkCreate(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := linkVariableIDOrKey()
	if err != nil {
		return err
	}
	if envLinkExclude {
		if err := env.ExcludeVar(idOrKey, scope, envLinkDeploymentID, astroV1Client); err != nil {
			return err
		}
		fmt.Fprintf(out, "Excluded %s from deployment %s\n", idOrKey, envLinkDeploymentID)
		return nil
	}
	var override *string
	if cmd.Flags().Changed("value") {
		override = &envLinkValue
	}
	if err := env.LinkVar(idOrKey, scope, envLinkDeploymentID, override, astroV1Client); err != nil {
		return err
	}
	if override != nil {
		fmt.Fprintf(out, "Linked %s to deployment %s (override value applied)\n", idOrKey, envLinkDeploymentID)
	} else {
		fmt.Fprintf(out, "Linked %s to deployment %s\n", idOrKey, envLinkDeploymentID)
	}
	return nil
}

func runEnvVarLinkDelete(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := linkVariableIDOrKey()
	if err != nil {
		return err
	}
	if envLinkExclude {
		if err := env.UnexcludeVar(idOrKey, scope, envLinkDeploymentID, astroV1Client); err != nil {
			return err
		}
		fmt.Fprintf(out, "Removed exclude on %s for deployment %s\n", idOrKey, envLinkDeploymentID)
		return nil
	}
	if err := env.UnlinkVar(idOrKey, scope, envLinkDeploymentID, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Unlinked %s from deployment %s\n", idOrKey, envLinkDeploymentID)
	return nil
}

func runEnvVarLinkList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	idOrKey, err := linkVariableIDOrKey()
	if err != nil {
		return err
	}
	report, err := env.ListVarLinks(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteVarLinks(report, f, envIncludeSecrets, out)
}
