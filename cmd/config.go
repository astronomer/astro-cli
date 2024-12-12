package cmd

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/config"

	"github.com/spf13/cobra"
)

const (
	configSetSuccessMsg           = "Setting %s to %s successfully\n"
	configUseOutsideProjectDirMsg = "You are attempting to %s a project config outside of a project directory\n To %s a global config try\n%s\n"
)

var (
	globalFlag       bool
	configGetExample = `
		# Get your current project's name
		$ astro config get project.name
		`
	configSetExample = `
		# Set your current project's postgres user
		$ astro config set postgres.user postgres
		`
)

func newConfigRootCmd(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "config",
		Short:             "Manage a project's configurations",
		Long:              "Manage the project configurations stored at '.astro/config.yaml'. Please see https://www.astronomer.io/docs/astro/cli/configure-cli#available-cli-configurations for list of available cli configurations",
		PersistentPreRunE: ensureGlobalFlag,
	}
	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "view or modify global config")
	cmd.AddCommand(
		newConfigGetCmd(out),
		newConfigSetCmd(out),
	)
	return cmd
}

func newConfigGetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get [setting-name]",
		Short:   "Get project's configurations",
		Long:    "List the value for a particular setting in your config.yaml file",
		Args:    cobra.ExactArgs(1),
		Example: configGetExample,
		RunE:    configGet,
	}
	return cmd
}

func newConfigSetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set [setting-name]",
		Short:   "Set project's configurations",
		Long:    "Update or override a particular setting in your config.yaml file",
		Example: configSetExample,
		RunE:    configSet,
	}
	return cmd
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) error {
	isProjectDir, _ := config.IsProjectDir(config.WorkingPath)

	if !isProjectDir && !globalFlag {
		c := "astro config " + cmd.Use + " " + args[0] + " -g"
		return fmt.Errorf(configUseOutsideProjectDirMsg, cmd.Use, cmd.Use, c) //nolint
	}
	return nil
}

func configGet(cmd *cobra.Command, args []string) error {
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errInvalidConfigPath
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if globalFlag {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetHomeString())
	} else {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetProjectString())
	}

	return nil
}

func configSet(cmd *cobra.Command, args []string) error {
	if len(args) != 2 { //nolint:mnd
		return errInvalidSetArgs
	}

	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errInvalidConfigPath
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	var err error
	if globalFlag {
		err = cfg.SetHomeString(args[1])
	} else {
		err = cfg.SetProjectString(args[1])
	}
	if err != nil {
		return err
	}

	fmt.Printf(configSetSuccessMsg+"\n", cfg.Path, args[1])
	return nil
}
