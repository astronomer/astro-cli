package cmd

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	// Flags
	globalFlag bool

	//Commands
	configRootCmd = &cobra.Command{
		Use:              "config",
		Short:            "Manage astro project configurations",
		Long:             "Manage astro project configurations",
		PersistentPreRun: ensureGlobalFlag,
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get astro project configuration",
		Long:  "Get astro project configuration",
		RunE:  configGet,
	}

	configSetCmd = &cobra.Command{
		Use:   "set",
		Short: "Set astro project configuration",
		Long:  "Set astro project configuration",
		RunE:  configSet,
	}
)

func init() {
	// Config root
	RootCmd.AddCommand(configRootCmd)
	configRootCmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "view or modify global config")

	// Config get
	configRootCmd.AddCommand(configGetCmd)

	// Config set
	configRootCmd.AddCommand(configSetCmd)
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) {
	isProjectDir, _ := config.IsProjectDir(config.WorkingPath)

	if !isProjectDir && !globalFlag {
		var c = "astro config " + cmd.Use + " " + args[0] + " -g"
		fmt.Printf(messages.CONFIG_USE_OUTSIDE_PROJECT_DIR, cmd.Use, cmd.Use, c)
		os.Exit(1)
	}
}

func configGet(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New(messages.CONFIG_PATH_KEY_MISSING_ERROR)
	}
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errors.New(messages.CONFIG_PATH_KEY_INVALID_ERROR)
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
	if len(args) != 2 {
		return errors.New(messages.CONFIG_INVALID_SET_ARGS)
	}

	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errors.New(messages.CONFIG_PATH_KEY_INVALID_ERROR)
	}

	// Silence Usage as we have now validated command input
	cmd.SilenceUsage = true

	if globalFlag {
		cfg.SetHomeString(args[1])
	} else {
		cfg.SetProjectString(args[1])
	}

	fmt.Printf(messages.CONFIG_SET_SUCCESS+"\n", cfg.Path, args[1])
	return nil
}
