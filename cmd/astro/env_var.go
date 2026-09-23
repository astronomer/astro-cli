//nolint:dupl // Cobra wiring per env-object type is intentionally parallel; sharing across types via callbacks would obscure the per-type flag set.
package astro

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

const envVarExamples = `
  # list workspace variables (table)
  astro env variable list --workspace-id <workspace-id>

  # export workspace variables as a dotenv file
  astro env variable export --workspace-id <workspace-id> > .env

  # show actual secret values (requires org policy to allow)
  astro env variable export --workspace-id <workspace-id> --include-secrets > .env

  # list deployment-resolved variables, including those linked from the workspace
  astro env variable list --deployment-id <deployment-id> --resolve-linked

  # set (creates the variable when missing, updates it when present)
  astro env variable set --workspace-id <ws-id> DBT_PROFILES_DIR --value /opt/profiles
  astro env variable set --workspace-id <ws-id> API_TOKEN --value $TOKEN --secret

  # refuse to create, so a mistyped key fails instead of making a second variable
  astro env variable set --workspace-id <ws-id> DBT_PROFILES_DIR --value /etc/profiles --no-create

  # delete
  astro env variable delete --workspace-id <ws-id> DBT_PROFILES_DIR --yes

  # bulk set from a dotenv file (round-trips with 'astro env variable export')
  astro env variable set --workspace-id <ws-id> --from-file .env

  # manage per-deployment links (see 'astro env variable link --help')
  astro env variable link set --variable-key DBT_PROFILES_DIR --workspace-id <ws-id> --deployment-id <dep-id> --value /etc/profiles
  astro env variable link list --variable-key DBT_PROFILES_DIR --workspace-id <ws-id>
`

func newEnvVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "variable",
		Aliases:                    []string{"var", "variables", "vars"},
		Short:                      "Manage environment-manager environment variables",
		Long:                       "List, set, delete, or export environment variables managed through the platform's environment manager. `set` creates a variable when it does not exist and updates it when it does. Variables can be scoped to a workspace or a deployment.",
		Example:                    envVarExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	// Reads, then the write, then the destructive one, then the sub-group.
	// The same order as every other noun here and as `astro local env`, so
	// help reads the same wherever you are; TestEnvVerbOrderIsUniform pins it.
	cmd.AddCommand(
		newEnvVarListCmd(out),
		newEnvVarGetCmd(out),
		newEnvVarExportCmd(out),
		newEnvVarSetCmd(out),
		newRemovedVerbCmd("create", "variable"),
		newRemovedVerbCmd("update", "variable"),
		newEnvVarDeleteCmd(out),
		newEnvVarLinkRootCmd(out),
	)
	return cmd
}

func newEnvVarListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List environment variables",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarList(cmd, out, "")
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml|dotenv")
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	return cmd
}

func newEnvVarExportCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export environment variables in dotenv format",
		Long:  "Export environment variables for the given scope as KEY=VALUE lines suitable for a .env file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarList(cmd, out, env.FormatDotenv)
		},
	}
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	return cmd
}

func newEnvVarGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Get a single environment variable by ID or key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml|dotenv")
	return cmd
}

func newEnvVarSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [<id-or-key>]",
		Short: "Set a variable's value, creating it if it does not exist",
		Long:  "Set the value of an environment variable. The object is created when the key does not exist and updated when it does, so one verb covers both. Pass --no-create to fail instead of creating, which is the guard against a mistyped key quietly becoming a second variable. Use --from-file to bulk-set from a dotenv file. The platform API does not allow toggling the secret flag on an existing variable; delete and recreate to change it.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envVarFromFile != "" {
				if len(args) > 0 {
					return errors.New("cannot pass an <id-or-key> together with --from-file; --from-file sets every entry in the file")
				}
				return runEnvVarSetFromFile(cmd, out)
			}
			if len(args) == 0 {
				return errors.New("set requires <id-or-key> or --from-file")
			}
			return runEnvVarSet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "New variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "When the variable does not exist and is created, mark it secret. No effect on an existing variable; the platform API does not allow toggling the secret flag.")
	cmd.Flags().BoolVar(&envVarNoCreate, "no-create", false, "Fail if the variable does not exist, instead of creating it")
	// --strict was this flag's name while the verb was `update`, where its job
	// was to take away the create half. Against `set` the name contradicts the
	// verb, so it was renamed — but a rename that answers "unknown flag" tells
	// a stale script nothing, and this is the path that survived, unlike
	// `create`, which got a whole tombstone. Deprecated, hidden, still works.
	cmd.Flags().BoolVar(&envVarNoCreate, "strict", false, "")
	_ = cmd.Flags().MarkDeprecated("strict", "use --no-create") //nolint:errcheck // the flag is registered on the line above; this only errors on an unknown name
	cmd.Flags().StringVar(&envVarFromFile, "from-file", "", "Bulk-set variables from a dotenv file. Pass '-' to read from stdin. Mutually exclusive with --value and the positional <id-or-key>.")
	addAutoLinkFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("value", "from-file")
	return cmd
}

func newEnvVarDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id-or-key>",
		Aliases: []string{"rm"},
		Short:   "Delete an environment variable",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarDelete(cmd, out, args[0])
		},
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runEnvVarList(cmd *cobra.Command, out io.Writer, formatOverride env.Format) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f := formatOverride
	if f == "" {
		f, err = env.ParseFormat(envFormat)
		if err != nil {
			return err
		}
	}
	cmd.SilenceUsage = true

	objs, err := env.ListVars(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, includeSecretsWarning)
	}
	w, closer, err := openOutput(out)
	if err != nil {
		return err
	}
	defer closer()
	return env.WriteVarList(objs, f, envIncludeSecrets, w)
}

func runEnvVarGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetVar(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteVar(obj, f, envIncludeSecrets, out)
}

func runEnvVarSetFromFile(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runFromFileSet(out, scope, autoLinkPtr(cmd), envVarSecret, envVarNoCreate, envVarFromFile, env.CreateVar, env.UpdateVar)
}

func runEnvVarSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	value, err := readSetValue(cmd, "value", envVarValue, fmt.Sprintf("new value for %s", idOrKey))
	if err != nil {
		return err
	}
	autoLink := autoLinkPtr(cmd)
	obj, err := env.UpdateVar(idOrKey, scope, value, autoLink, astroV1Client)
	if err != nil {
		// Upsert: absent and creating is allowed, so fall through to create.
		if errors.Is(err, env.ErrNotFound) && !envVarNoCreate {
			if cerr := refuseCreateByID("environment variable", idOrKey); cerr != nil {
				return cerr
			}
			obj, err = env.CreateVar(scope, idOrKey, value, envVarSecret, autoLink, astroV1Client)
			if err != nil {
				return err
			}
			printCreated(out, obj)
			return nil
		}
		if errors.Is(err, env.ErrNotFound) && envVarNoCreate {
			return setNotFound("environment variable", idOrKey, err)
		}
		return err
	}
	fmt.Fprintf(out, "Updated %s\n", obj.ObjectKey)
	return nil
}

func runEnvVarDelete(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if !envYes && !confirmTTY(fmt.Sprintf("Delete environment variable %q?", idOrKey)) {
		return errors.New("aborted: pass --yes (or confirm interactively) to delete")
	}
	if err := env.DeleteVar(idOrKey, scope, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}

func openOutput(out io.Writer) (io.Writer, func(), error) {
	if envOutputPath == "" || envOutputPath == "-" {
		return out, func() {}, nil
	}
	f, err := os.Create(envOutputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening output file: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}
