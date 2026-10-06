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
	cliout.AddOutputFlag(cmd, &envOutput, formatDotenv)
	return cmd
}

func newEnvVarExportCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export environment variables as a .env file",
		Long: "Write the scope's environment variables as KEY=VALUE lines. Secret values are\n" +
			"left blank unless --include-secrets is set.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvVarList(cmd, out, env.FormatDotenv)
		},
	}
	return cmd
}

func newEnvVarGetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show an environment variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvVarGet(cmd, out, args[0])
		},
	}
	cliout.AddOutputFlag(cmd, &envOutput, formatDotenv)
	return cmd
}

func newEnvVarSetCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set [<id-or-key>]",
		Short: "Set an environment variable",
		Long: "Set an environment variable, creating it if it does not exist. Pass --no-create\n" +
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
		var parsed cliout.Format
		parsed, err = cliout.ParseFormat(envOutput, formatDotenv)
		if err != nil {
			return err
		}
		f = env.Format(parsed)
	}
	cmd.SilenceUsage = true

	objs, err := env.ListVars(scope, envResolveLinked, envIncludeSecrets, astroV1Client)
	if err != nil {
		return err
	}
	if envIncludeSecrets {
		fmt.Fprintln(os.Stderr, includeSecretsWarning)
	}
	return env.WriteVarList(objs, f, envIncludeSecrets, out)
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
	return env.WriteVar(obj, env.Format(f), envIncludeSecrets, out)
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
			printPickupNoteIfItReachesDeployments(cmd.ErrOrStderr(), scope, obj)
			return nil
		}
		if errors.Is(err, env.ErrNotFound) && envVarNoCreate {
			return setNotFound("environment variable", idOrKey, err)
		}
		return err
	}
	fmt.Fprintf(out, "Updated %s\n", obj.ObjectKey)
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

func runEnvVarDelete(cmd *cobra.Command, out io.Writer, idOrKey string) error {
	scope, err := envScope()
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	if !envYes {
		ok, err := confirmTTY(fmt.Sprintf("Delete environment variable %q?", idOrKey))
		if err != nil {
			return err
		}
		if !ok {
			return errAbortedDelete
		}
	}
	if err := env.DeleteVar(idOrKey, scope, astroV1Client); err != nil {
		return err
	}
	fmt.Fprintf(out, "Deleted %s\n", idOrKey)
	return nil
}
