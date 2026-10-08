//nolint:dupl // Mirror of env_var.go for AIRFLOW_VARIABLE; see comment there.
package astro

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

const envAirflowVarExamples = `
  # set a variable, creating it if it does not exist
  astro env airflow-variable set region --workspace <ws> --value us-east-1

  # set many from a dotenv file
  astro env airflow-variable set --workspace <ws> --from-file vars.env

  # list and delete
  astro env airflow-variable list --workspace <ws>
  astro env airflow-variable delete region --workspace <ws> --yes`

func newEnvAirflowVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "airflow-variable",
		Aliases:                    []string{"airflow-var", "airflow-vars", "airflow-variables"},
		Short:                      "Manage Airflow variables",
		Long:                       "Manage Airflow variables on Astro, scoped to a workspace or a deployment.",
		Example:                    envAirflowVarExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       cliout.GroupHelp,
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
		Example: `  # List the Airflow variables in the workspace
  astro env airflow-variable list

  # List one Deployment's, as JSON
  astro env airflow-variable list --deployment <DEPLOYMENT_ID> -o json`,
	}
	cliout.AddOutputFlag(cmd, &envOutput)
	return cmd
}

func newEnvAirflowVarGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <ID_OR_KEY>",
		Short: "Show an Airflow variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvAirflowVarGet(cmd, out, args[0])
		},
		Example: `  # Show an Airflow variable by its key
  astro env airflow-variable get region`,
	}
	cliout.AddOutputFlag(cmd, &envOutput)
	return cmd
}

func newEnvAirflowVarSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [ID_OR_KEY]",
		Short: "Set an Airflow variable",
		Long: "Set an Airflow variable, creating it if it does not exist. Pass --no-create to " +
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
		Example: `  # Set an Airflow variable, creating it if it does not exist
  astro env airflow-variable set region --value us-east-1

  # Set many from a dotenv file
  astro env airflow-variable set --from-file vars.env`,
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
		Use:     "delete <ID_OR_KEY>",
		Aliases: []string{"rm"},
		Short:   "Delete an Airflow variable",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvDelete(cmd, out, "Airflow variable", args[0], env.DeleteAirflowVar)
		},
		Example: `  # Delete an Airflow variable without the confirmation prompt
  astro env airflow-variable delete region --yes`,
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

func runEnvAirflowVarList(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput)
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
	return env.WriteAirflowVarList(objs, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

func runEnvAirflowVarGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetAirflowVar(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	return env.WriteAirflowVar(obj, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

func runEnvAirflowVarSetFromFile(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	r, err := envRenderer(out)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runFromFileSet(cmd, r, scope, autoLinkPtr(cmd), envVarSecret, envVarNoCreate, envVarFromFile, fromFileFns{env.CreateAirflowVar, env.UpdateAirflowVar, env.GetAirflowVar})
}

func runEnvAirflowVarSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	r, err := envRenderer(out)
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
			held := createdAsHeld(cmd.ErrOrStderr(), r, obj, scope, env.GetAirflowVar)
			return renderEnvSet(r, held, true)
		}
		if errors.Is(err, env.ErrNotFound) && envVarNoCreate {
			return setNotFound("Airflow variable", idOrKey, err)
		}
		return err
	}
	return renderEnvSet(r, obj, false)
}
