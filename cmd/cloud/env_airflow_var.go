//nolint:dupl // Mirror of env_var.go for AIRFLOW_VARIABLE; see comment there.
package cloud

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cloud/env"
)

const envAirflowVarExamples = `
  # List Airflow variables in a workspace
  astro env airflow-var list --workspace-id <ws-id>

  # Create
  astro env airflow-var create --workspace-id <ws-id> --key MY_VAR --value some-value

  # Update
  astro env airflow-var update MY_VAR --workspace-id <ws-id> --value new-value

  # Delete
  astro env airflow-var delete MY_VAR --workspace-id <ws-id> --yes
`

func newEnvAirflowVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "airflow-var",
		Aliases: []string{"airflow-variable", "airflow-vars", "airflow-variables"},
		Short:   "Manage environment-manager Airflow variables",
		Long:    "List, create, update, or delete Airflow variables managed through the platform's environment manager. Variables can be scoped to a workspace or a deployment.",
		Example: envAirflowVarExamples,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvAirflowVarListCmd(out),
		newEnvAirflowVarGetCmd(out),
		newEnvAirflowVarCreateCmd(out),
		newEnvAirflowVarUpdateCmd(out),
		newEnvAirflowVarDeleteCmd(out),
	)
	return cmd
}

func newEnvAirflowVarListCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Airflow variables",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvAirflowVarList(cmd, out)
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	cmd.Flags().StringVar(&envOutputPath, "output", "-", "Write output to FILE (use '-' for stdout)")
	return cmd
}

func newEnvAirflowVarGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Get a single Airflow variable by ID or key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvAirflowVarGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func newEnvAirflowVarCreateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "create",
		Aliases: []string{"cr"},
		Short:   "Create an Airflow variable",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvAirflowVarCreate(cmd, out)
		},
	}
	cmd.Flags().StringVarP(&envVarKey, "key", "k", "", "Variable key (required)")
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "Variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "Mark this variable as secret")
	_ = cmd.MarkFlagRequired("key")
	return cmd
}

func newEnvAirflowVarUpdateCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update <id-or-key>",
		Aliases: []string{"up"},
		Short:   "Update an Airflow variable's value",
		Long:    "Update the value of an existing Airflow variable. The platform API does not allow toggling the secret flag; delete and recreate to change it.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvAirflowVarUpdate(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "New variable value. If omitted, read from stdin (piped) or prompted (TTY) with echo disabled.")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "If the variable does not exist (upsert path), mark it as secret on create. Has no effect when updating an existing variable.")
	cmd.Flags().BoolVar(&envVarStrict, "strict", false, "Fail if the variable does not exist (default: upsert)")
	return cmd
}

func newEnvAirflowVarDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <id-or-key>",
		Aliases: []string{"rm"},
		Short:   "Delete an Airflow variable",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvAirflowVarDelete(cmd, out, args[0])
		},
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runEnvAirflowVarList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	objs, err := env.ListAirflowVars(scope, envResolveLinked, envIncludeSecrets, astroCoreClient)
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
	return env.WriteAirflowVarList(objs, f, envIncludeSecrets, w)
}

func runEnvAirflowVarGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := env.ParseFormat(envFormat)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetAirflowVar(idOrKey, scope, envIncludeSecrets, astroCoreClient)
	if err != nil {
		return err
	}
	return env.WriteAirflowVar(obj, f, envIncludeSecrets, out)
}

func runEnvAirflowVarCreate(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	value, err := readSecretValue(envVarValue, fmt.Sprintf("Value for %s", envVarKey))
	if err != nil {
		return err
	}
	obj, err := env.CreateAirflowVar(scope, envVarKey, value, envVarSecret, astroCoreClient)
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

func runEnvAirflowVarUpdate(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	value, err := readSecretValue(envVarValue, fmt.Sprintf("New value for %s", idOrKey))
	if err != nil {
		return err
	}
	obj, err := env.UpdateAirflowVar(idOrKey, scope, value, astroCoreClient)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) && !envVarStrict {
			obj, err = env.CreateAirflowVar(scope, idOrKey, value, envVarSecret, astroCoreClient)
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

func runEnvAirflowVarDelete(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if !envYes && !confirmTTY(fmt.Sprintf("Delete Airflow variable %q?", idOrKey)) {
		return errors.New("aborted: pass --yes (or confirm interactively) to delete")
	}
	if err := env.DeleteAirflowVar(idOrKey, scope, astroCoreClient); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}
