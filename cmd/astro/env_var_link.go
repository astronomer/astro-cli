package astro

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

var (
	envLinkNoCreate     bool
	envLinkVariableID   string
	envLinkVariableKey  string
	envLinkDeploymentID string
	envLinkValue        string
	envLinkExclude      bool
)

const envVarLinkExamples = `
  # link a workspace variable to a deployment
  astro env variable link set --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id>

  # link with a per-deployment override value
  astro env variable link set --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id> --value postgres://prod

  # drop that override, keeping the link (one command: the link's override is
  # whatever --value says, and saying nothing means it has none)
  astro env variable link set --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id>

  # refuse to create the link if it is not already there
  astro env variable link set --variable-key DATABASE_URL --workspace-id <ws-id> --deployment-id <dep-id> --value x --no-create

  # exclude a deployment from an auto-linked variable
  astro env variable link set --variable-key LOG_LEVEL --workspace-id <ws-id> --deployment-id <dep-id> --exclude

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
		Long:    "Set, delete, or list the explicit per-deployment links (and auto-link excludes) of a workspace-scoped environment variable. Identify the variable with --variable-id or --variable-key.",
		Example: envVarLinkExamples,

		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.AddCommand(
		newEnvVarLinkSetCmd(out),
		newRemovedLinkCreateCmd(),
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

// newRemovedLinkCreateCmd is the `link create` tombstone. The nouns' stub
// cannot serve here: its guidance names `set <id-or-key>`, and a link is
// addressed by two flags rather than a positional.
func newRemovedLinkCreateCmd() *cobra.Command {
	return removedVerbStub("create", []string{"cr"}, func([]string) string {
		return "`astro env variable link create` was removed in v2.\n" +
			"  use:  astro env variable link set --variable-key <key> --deployment-id <id>\n" +
			"`set` links the deployment when it is not linked and updates the link when it is. " +
			"It also treats --value as the whole override, so omitting it now CLEARS an existing " +
			"override rather than leaving it in place — which is how one is removed. " +
			"Pass --no-create to fail instead of linking."
	})
}

func newEnvVarLinkSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a workspace variable's link to a deployment, with or without an override",
		Long: `Attach a workspace-scoped environment variable to a specific deployment.
If --value is provided, that value overrides the workspace default for the linked deployment only.
Pass --exclude to add the deployment to the excludeLinks list instead (used with --auto-link to opt specific deployments out).

Set semantics, the same as ` + "`set`" + ` on the object nouns: the link is created when it is not there and updated when it is, and --value describes the whole override. Saying nothing means the link has no override, so running this against a link that has one CLEARS it — which is how an override is removed now, in place of the old delete-then-recreate. Pass --no-create to fail instead of creating a link that does not exist.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarLinkSet(cmd, out)
		},
	}
	addLinkVariableFlags(cmd)
	cmd.Flags().StringVar(&envLinkDeploymentID, "deployment-id", "", "ID of the deployment to link (required)")
	cmd.Flags().StringVar(&envLinkValue, "value", "", "Override value for the linked deployment only. Omit it and the link has no override, clearing any it had.")
	cmd.Flags().BoolVar(&envLinkExclude, "exclude", false, "Add to excludeLinks instead of links (auto-link only)")
	cmd.Flags().BoolVar(&envLinkNoCreate, "no-create", false, "Fail if the deployment is not already linked, instead of linking it")
	// --exclude takes a different path entirely (the platform's exclude-linking
	// endpoint), which has no create/update distinction for --no-create to
	// govern. Accepting the pair would have silently ignored the guard.
	cmd.MarkFlagsMutuallyExclusive("exclude", "no-create")
	_ = cmd.MarkFlagRequired("deployment-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
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
	_ = cmd.MarkFlagRequired("deployment-id") //nolint:errcheck // the flag is defined just above; this only errors on an unknown flag name
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

func runEnvVarLinkSet(cmd *cobra.Command, out io.Writer) error {
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
	if err := env.LinkVar(idOrKey, scope, envLinkDeploymentID, override, envLinkNoCreate, astroV1Client); err != nil {
		return err
	}
	if override != nil {
		fmt.Fprintf(out, "Linked %s to deployment %s (override value applied)\n", idOrKey, envLinkDeploymentID)
	} else {
		fmt.Fprintf(out, "Linked %s to deployment %s (no override)\n", idOrKey, envLinkDeploymentID)
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
