//nolint:dupl // Cobra wiring per env-object type is intentionally parallel; sharing across types via callbacks would obscure the per-type flag set.
package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
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

  # create / update / delete
  astro env variable create --workspace-id <ws-id> --key DBT_PROFILES_DIR --value /opt/profiles
  astro env variable create --workspace-id <ws-id> --key API_TOKEN --value $TOKEN --secret
  astro env variable update --workspace-id <ws-id> DBT_PROFILES_DIR --value /etc/profiles
  astro env variable delete --workspace-id <ws-id> DBT_PROFILES_DIR --yes

  # bulk import from a dotenv file (round-trips with 'astro env variable export')
  astro env variable create --workspace-id <ws-id> --from-file .env
  astro env variable update --workspace-id <ws-id> --from-file .env
`

func newEnvVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "variable",
		Aliases: []string{"var", "variables", "vars"},
		Short:   "Manage environment-manager environment variables",
		Long:    "List, create, update, delete, or export environment variables managed through the platform's environment manager. Variables can be scoped to a workspace or a deployment.",
		Example: envVarExamples,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvVarListCmd(out),
		newEnvVarGetCmd(out),
		newEnvVarCreateCmd(out),
		newEnvVarUpdateCmd(out),
		newEnvVarDeleteCmd(out),
		newEnvVarExportCmd(out),
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

func newEnvVarCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create one or more environment variables",
		Long:    "Create a single variable via --key/--value, or bulk-create from a dotenv file via --from-file. --secret and --auto-link apply uniformly to every entry in the file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&envVarKey, "key", "k", "", "Variable key (required unless --from-file is used)")
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "Variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "Mark this variable as secret")
	cmd.Flags().StringVar(&envVarFromFile, "from-file", "", "Bulk-create variables from a dotenv file (KEY=VALUE per line; supports quoted and multi-line values). Pass '-' to read from stdin. Mutually exclusive with --key/--value.")
	addAutoLinkFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("key", "from-file")
	cmd.MarkFlagsMutuallyExclusive("value", "from-file")
	return cmd
}

func newEnvVarUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [<id-or-key>]",
		Aliases: []string{"up"},
		Short:   "Set a variable's value (creates it if missing; use --strict to require existing)",
		Long:    "Set the value of an environment variable. By default this upserts: if the key does not exist it is created. Pass --strict to fail when the key is missing. Use --from-file to bulk-upsert from a dotenv file. The platform API does not allow toggling the secret flag on an existing variable; delete and recreate to change it.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envVarFromFile != "" {
				if len(args) > 0 {
					return errors.New("cannot pass an <id-or-key> together with --from-file; --from-file upserts every entry in the file")
				}
				return runEnvVarUpdateFromFile(cmd, out)
			}
			if len(args) == 0 {
				return errors.New("update requires <id-or-key> or --from-file")
			}
			return runEnvVarUpdate(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "New variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "If the variable does not exist (upsert path), mark it as secret on create. Has no effect when updating an existing variable; the platform API does not allow toggling the secret flag.")
	cmd.Flags().BoolVar(&envVarStrict, "strict", false, "Fail if the variable does not exist (default: upsert)")
	cmd.Flags().StringVar(&envVarFromFile, "from-file", "", "Bulk-upsert variables from a dotenv file. Pass '-' to read from stdin. Mutually exclusive with --value and the positional <id-or-key>.")
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

	objs, err := env.ListVars(scope, envResolveLinked, envIncludeSecrets, astroCoreClient)
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

	obj, err := env.GetVar(idOrKey, scope, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	return env.WriteVar(obj, f, envIncludeSecrets, out)
}

func runEnvVarCreate(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if envVarFromFile != "" {
		return runFromFileCreate(out, scope, autoLinkPtr(cmd), envVarSecret, envVarFromFile, env.CreateVar)
	}
	if envVarKey == "" {
		return errors.New("--key is required (or use --from-file)")
	}

	value, err := readSecretValue(envVarValue, fmt.Sprintf("Value for %s", envVarKey))
	if err != nil {
		return err
	}
	obj, err := env.CreateVar(scope, envVarKey, value, envVarSecret, autoLinkPtr(cmd), astroCoreClient)
	if err != nil {
		return err
	}
	id := ""
	if obj.Id != nil {
		id = *obj.Id
	}
	fmt.Fprintf(out, "Created %s (id: %s)\n", obj.ObjectKey, id)
	return nil
}

func runEnvVarUpdateFromFile(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runFromFileUpdate(out, scope, autoLinkPtr(cmd), envVarSecret, envVarStrict, envVarFromFile, env.CreateVar, env.UpdateVar)
}

func runEnvVarUpdate(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	value, err := readSecretValue(envVarValue, fmt.Sprintf("New value for %s", idOrKey))
	if err != nil {
		return err
	}
	autoLink := autoLinkPtr(cmd)
	obj, err := env.UpdateVar(idOrKey, scope, value, autoLink, astroCoreClient)
	if err != nil {
		// Upsert: if not found and not strict, fall through to create.
		if errors.Is(err, env.ErrNotFound) && !envVarStrict {
			obj, err = env.CreateVar(scope, idOrKey, value, envVarSecret, autoLink, astroCoreClient)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Created %s\n", obj.ObjectKey)
			return nil
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
	if err := env.DeleteVar(idOrKey, scope, astroCoreClient); err != nil {
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
