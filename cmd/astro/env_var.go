//nolint:dupl // Cobra wiring per env-object type is intentionally parallel; sharing across types via callbacks would obscure the per-type flag set.
package astro

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/env"
)

const envVarExamples = `
  # set a variable, creating it if it does not exist
  astro env variable set API_TOKEN --workspace <ws> --value "$TOKEN" --secret

  # fail instead of creating, to catch a mistyped key
  astro env variable set API_TOKEN --workspace <ws> --value "$TOKEN" --no-create

  # export to a .env file, or set many from one
  astro env variable export --workspace <ws> > .env
  astro env variable set --workspace <ws> --from-file .env

  # list, as a table or as JSON for a script, and delete
  astro env variable list --workspace <ws>
  astro env variable list --workspace <ws> -o json
  astro env variable delete API_TOKEN --workspace <ws> --yes`

func newEnvVarRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "variable",
		Aliases:                    []string{"var", "variables", "vars"},
		Short:                      "Manage environment variables",
		Long:                       "Manage environment variables on Astro, scoped to a workspace or a deployment.",
		Example:                    envVarExamples,
		Args:                       cobra.ArbitraryArgs,
		RunE:                       cliout.GroupHelp,
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
			return runEnvVarList(cmd, out, false)
		},
		Example: `  # List the environment variables in the workspace
  astro env variable list

  # List one Deployment's, as JSON
  astro env variable list --deployment <DEPLOYMENT_ID> -o json`,
	}
	cliout.AddOutputFlag(cmd, &envOutput, formatDotenv)
	return cmd
}

func newEnvVarExportCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export environment variables as a .env file",
		Long: "Write the scope's environment variables as KEY=VALUE lines. Secret values are " +
			"left blank unless --include-secrets is set. With -o json, the variables as " +
			"`list -o json` prints them.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarList(cmd, out, true)
		},
		Example: `  # Write the workspace's environment variables to a .env file
  astro env variable export > .env

  # Include secret values, which are otherwise left blank
  astro env variable export --include-secrets > .env`,
	}
	// dotenv is export's text, and is accepted by name as list and get
	// accept it.
	cliout.AddOutputFlag(cmd, &envOutput, formatDotenv)
	return cmd
}

func newEnvVarGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <ID_OR_KEY>",
		Short: "Show an environment variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarGet(cmd, out, args[0])
		},
		Example: `  # Show an environment variable by its key
  astro env variable get API_TOKEN

  # Show one on a Deployment, as a KEY=VALUE line
  astro env variable get API_TOKEN --deployment <DEPLOYMENT_ID> -o dotenv`,
	}
	cliout.AddOutputFlag(cmd, &envOutput, formatDotenv)
	return cmd
}

func newEnvVarSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [ID_OR_KEY]",
		Short: "Set an environment variable",
		Long: "Set an environment variable, creating it if it does not exist. Pass --no-create " +
			"to fail instead, or --from-file to set many from a dotenv file.",
		Args: cobra.MaximumNArgs(1),
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
		Example: `  # Set a secret variable, creating it if it does not exist
  astro env variable set API_TOKEN --value "$TOKEN" --secret

  # Set many from a dotenv file
  astro env variable set --from-file .env`,
	}
	cmd.Flags().StringVarP(&envVarValue, "value", "v", "", "The value; omit it to read from stdin or be prompted with echo off")
	cmd.Flags().BoolVarP(&envVarSecret, "secret", "s", false, "Mark the variable secret when it is created; to change it later, delete and re-create it")
	cmd.Flags().BoolVar(&envVarNoCreate, "no-create", false, "Fail if the variable does not exist, instead of creating it")
	cmd.Flags().StringVar(&envVarFromFile, "from-file", "", "Set many from a dotenv file ('-' reads stdin)")
	addAutoLinkFlag(cmd)
	cmd.MarkFlagsMutuallyExclusive("value", "from-file")
	return cmd
}

func newEnvVarDeleteCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <ID_OR_KEY>",
		Aliases: []string{"rm"},
		Short:   "Delete an environment variable",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvDelete(cmd, out, "environment variable", args[0], env.DeleteVar)
		},
		Example: `  # Delete an environment variable
  astro env variable delete API_TOKEN

  # Delete it without the confirmation prompt
  astro env variable delete API_TOKEN --yes`,
	}
	cmd.Flags().BoolVarP(&envYes, "yes", "y", false, "Skip confirmation prompt")
	return cmd
}

// runEnvVarList is `variable list`, and `variable export`, whose text is the
// dotenv file and whose json is the list's: the same variables, as data.
func runEnvVarList(cmd *cobra.Command, out io.Writer, export bool) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput, formatDotenv)
	if err != nil {
		return err
	}
	if export && f == cliout.FormatText {
		f = formatDotenv
	}
	cmd.SilenceUsage = true

	objs, err := env.ListVars(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, includeSecretsWarning)
	}
	if f == formatDotenv {
		return env.WriteVarDotenv(objs, envIncludeSecrets, out)
	}
	return env.WriteVarList(objs, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

func runEnvVarGet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	f, err := cliout.ParseFormat(envOutput, formatDotenv)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	obj, err := env.GetVar(idOrKey, scope, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if f == formatDotenv && obj != nil {
		return env.WriteVarDotenv([]astrov1.EnvironmentObject{*obj}, envIncludeSecrets, out)
	}
	return env.WriteVar(obj, envIncludeSecrets, cliout.Renderer{Format: f, Out: out})
}

func runEnvVarSetFromFile(cmd *cobra.Command, out io.Writer) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	r, err := envRenderer(out)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true
	return runFromFileSet(cmd, r, scope, autoLinkPtr(cmd), envVarSecret, envVarNoCreate, envVarFromFile, fromFileFns{env.CreateVar, env.UpdateVar, env.GetVar})
}

func runEnvVarSet(cmd *cobra.Command, out io.Writer, idOrKey string) error {
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
			held := createdAsHeld(cmd.ErrOrStderr(), r, obj, scope, env.GetVar)
			if err := renderEnvSet(r, held, true); err != nil {
				return err
			}
			printPickupNoteIfItReachesDeployments(cmd.ErrOrStderr(), scope, obj)
			return nil
		}
		if errors.Is(err, env.ErrNotFound) && envVarNoCreate {
			return setNotFound("environment variable", idOrKey, err)
		}
		return err
	}
	if err := renderEnvSet(r, obj, false); err != nil {
		return err
	}
	printPickupNoteIfItReachesDeployments(cmd.ErrOrStderr(), scope, obj)
	return nil
}

const deploymentPickupNote = "Deployments pick up the change within a few minutes. Tasks already running keep the old value."

func printPickupNoteIfItReachesDeployments(w io.Writer, scope env.Scope, obj *astrov1.EnvironmentObject) {
	autoLinked := obj.AutoLinkDeployments != nil && *obj.AutoLinkDeployments
	linked := obj.Links != nil && len(*obj.Links) > 0
	if scope.DeploymentID != "" || autoLinked || linked {
		fmt.Fprintln(w, deploymentPickupNote)
	}
}
