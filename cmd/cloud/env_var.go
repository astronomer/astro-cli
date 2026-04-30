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
  astro env var list --workspace-id <workspace-id>

  # export workspace variables as a dotenv file
  astro env var export --workspace-id <workspace-id> > .env

  # show actual secret values (requires org policy to allow)
  astro env var export --workspace-id <workspace-id> --include-secrets > .env

  # list deployment-resolved variables, including those linked from the workspace
  astro env var list --deployment-id <deployment-id> --resolve-linked

  # create / update / delete
  astro env var create --workspace-id <ws-id> --key DBT_PROFILES_DIR --value /opt/profiles
  astro env var create --workspace-id <ws-id> --key API_TOKEN --value $TOKEN --secret
  astro env var update --workspace-id <ws-id> DBT_PROFILES_DIR --value /etc/profiles
  astro env var delete --workspace-id <ws-id> DBT_PROFILES_DIR --yes
`

func newEnvVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "var",
		Aliases: []string{"variables", "vars"},
		Short:   "Manage environment-manager environment variables",
		Long:    "List, create, update, delete, or export environment variables managed through the platform's environment manager. Variables can be scoped to a workspace or a deployment.",
		Example: envVarExamples,
	}
	cmd.SetOut(out)
	cmd.PersistentFlags().StringVar(&envWorkspaceID, "workspace-id", "", "Workspace scope (mutually exclusive with --deployment-id). Defaults to the current workspace from context.")
	cmd.PersistentFlags().StringVar(&envDeploymentID, "deployment-id", "", "Deployment scope (mutually exclusive with --workspace-id)")
	cmd.PersistentFlags().BoolVar(&envIncludeSecrets, "include-secrets", false, "Surface secret values (requires org policy to allow)")
	cmd.PersistentFlags().BoolVar(&envResolveLinked, "resolve-linked", true, "Include variables linked from another scope (e.g. workspace -> deployment). In this mode IDs are not returned, since they refer to resolved rows that aren't directly addressable. Use --resolve-linked=false to see IDs.")
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
			return runEnvVarList(cmd, out, string(env.FormatDotenv))
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
		Short:   "Create an environment variable",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&envVarKey, "key", "k", "", "Variable key (required)")
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "Variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "Mark this variable as secret")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

func newEnvVarUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <id-or-key>",
		Aliases: []string{"up"},
		Short:   "Update an environment variable's value",
		Long:    "Update the value of an existing environment variable. The platform API does not allow toggling the secret flag; delete and recreate to change it.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarUpdate(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "New variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "If the variable does not exist (upsert path), mark it as secret on create. Has no effect when updating an existing variable; the platform API does not allow toggling the secret flag.")
	cmd.Flags().BoolVar(&envVarStrict, "strict", false, "Fail if the variable does not exist (default: upsert)")
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

func runEnvVarList(cmd *cobra.Command, out io.Writer, formatOverride string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	format := envFormat
	if formatOverride != "" {
		format = formatOverride
	}
	f, err := env.ParseFormat(format)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	objs, err := env.ListVars(scope, envResolveLinked, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, "Warning: --include-secrets returns secret values in the response. Treat the output as sensitive: do not commit, paste into shared channels, or leave on disk longer than necessary.")
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

	value, err := readSecretValue(envVarValue, fmt.Sprintf("Value for %s", envVarKey))
	if err != nil {
		return err
	}
	obj, err := env.CreateVar(scope, envVarKey, value, envVarSecret, astroCoreClient)
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
	obj, err := env.UpdateVar(idOrKey, scope, value, astroCoreClient)
	if err != nil {
		// Upsert: if not found and not strict, fall through to create.
		if errors.Is(err, env.ErrNotFound) && !envVarStrict {
			obj, err = env.CreateVar(scope, idOrKey, value, envVarSecret, astroCoreClient)
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

// envScope resolves the active scope, validating mutual exclusivity and falling
// back to the current workspace from context when neither flag is set.
func envScope() (env.Scope, error) {
	if envWorkspaceID != "" && envDeploymentID != "" {
		return env.Scope{}, env.ErrScopeAmbiguous
	}
	if envWorkspaceID == "" && envDeploymentID == "" {
		ws, err := coalesceWorkspace()
		if err != nil {
			return env.Scope{}, env.ErrScopeNotSpecified
		}
		return env.Scope{WorkspaceID: ws}, nil
	}
	return env.Scope{WorkspaceID: envWorkspaceID, DeploymentID: envDeploymentID}, nil
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
