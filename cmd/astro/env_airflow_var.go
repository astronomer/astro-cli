//nolint:dupl // Mirror of env_var.go for AIRFLOW_VARIABLE; see comment there.
package astro

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

const envAirflowVarExamples = `
  # set a variable, creating it if it does not exist
  astro env airflow-variable set region --workspace-id <ws> --value us-east-1

  # set many from a dotenv file
  astro env airflow-variable set --workspace-id <ws> --from-file vars.env

  # list and delete
  astro env airflow-variable list --workspace-id <ws>
  astro env airflow-variable delete region --workspace-id <ws> --yes`

func newEnvAirflowVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "airflow-variable",
		Aliases:                    []string{"airflow-var", "airflow-vars", "airflow-variables"},
		Short:                      "Manage Airflow variables",
		Long:                       "Manage Airflow variables on Astro, scoped to a workspace or a deployment.",
		Example:                    envAirflowVarExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       helpOrUnknownSubcommand,
		SuggestionsMinimumDistance: 2,
	}
	cmd.SetOut(out)
	addScopePersistentFlags(cmd)
	cmd.AddCommand(
		newEnvAirflowVarListCmd(out),
		newEnvAirflowVarGetCmd(out),
		newEnvAirflowVarSetCmd(out),
		newRemovedVerbCmd("create", "airflow-variable"),
		newRemovedVerbCmd("update", "airflow-variable"),
		newEnvAirflowVarDeleteCmd(out),
		newEnvLinkRootCmd(out, &airflowVarLinkNoun),
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
		Short: "Show an Airflow variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvAirflowVarGet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVar(&envFormat, "format", string(env.FormatTable), "Output format: table|json|yaml")
	return cmd
}

func newEnvAirflowVarSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [<id-or-key>]",
		Short: "Set an Airflow variable",
		Long: "Set an Airflow variable, creating it if it does not exist. Pass --no-create to\n" +
			"fail instead, or --from-file to set many from a dotenv file.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if envVarFromFile != "" {
				if len(args) > 0 {
					return errors.New("cannot pass an <id-or-key> together with --from-file; --from-file sets every entry in the file")
				}
				return runEnvAirflowVarSetFromFile(cmd, out)
			}
			if len(args) == 0 {
				return errors.New("set requires <id-or-key> or --from-file")
			}
			return runEnvAirflowVarSet(cmd, out, args[0])
		},
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "The value; omit it to read from stdin or be prompted with echo off")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "Mark the variable secret when it is created")
	cmd.Flags().BoolVar(&envVarNoCreate, "no-create", false, "Fail if the variable does not exist, instead of creating it")
	cmd.Flags().StringVar(&envVarFromFile, "from-file", "", "Set many from a dotenv file ('-' reads stdin)")
	addAutoLinkFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("value", "from-file")
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

	objs, err := env.ListAirflowVars(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
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

	obj, err := env.GetAirflowVar(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteAirflowVar(obj, f, envIncludeSecrets, out)
}

func runEnvAirflowVarSetFromFile(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runFromFileSet(out, scope, autoLinkPtr(cmd), envVarSecret, envVarNoCreate, envVarFromFile, env.CreateAirflowVar, env.UpdateAirflowVar)
}

func runEnvAirflowVarSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
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
	obj, err := env.UpdateAirflowVar(idOrKey, scope, value, autoLink, astroV1Client)
	if err != nil {
		if errors.Is(err, env.ErrNotFound) && !envVarNoCreate {
			if cerr := refuseCreateByID("Airflow variable", idOrKey); cerr != nil {
				return cerr
			}
			obj, err = env.CreateAirflowVar(scope, idOrKey, value, envVarSecret, autoLink, astroV1Client)
			if err != nil {
				return err
			}
			printCreated(out, obj)
			return nil
		}
		if errors.Is(err, env.ErrNotFound) && envVarNoCreate {
			return setNotFound("Airflow variable", idOrKey, err)
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
	if err := env.DeleteAirflowVar(idOrKey, scope, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}
