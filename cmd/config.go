package cmd

import (
	"fmt"
	"os"

	"github.com/astronomerio/astro-cli/config"
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
	// Set up project root
	projectRoot, _ = config.ProjectRoot()

	// Config root
	RootCmd.AddCommand(configRootCmd)
	RootCmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "view or modify global config")

	// Config get
	configRootCmd.AddCommand(configGetCmd)

	// Config set
	configRootCmd.AddCommand(configSetCmd)
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) {
	if !(len(projectRoot) > 0) && !globalFlag {
		var c = "astro config " + cmd.Use + " " + args[0] + " -g"
		fmt.Println("You are attempting to " + cmd.Use + " a project config outside of a project directory\n" +
			"To " + cmd.Use + " a global config try\n" + c)
		os.Exit(1)
	}
}

func configGet(command *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("Must specify config key")
	}
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errors.New("Config does not exist, check your config key")
	}

	if globalFlag {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetHomeString())
	} else {
		fmt.Printf("%s: %s\n", cfg.Path, cfg.GetProjectString())
	}

	return nil
}

func configSet(command *cobra.Command, args []string) error {
	if len(args) != 2 {
		return errors.New("Must specify exactly two arguments (key value) when setting a config")
	}

	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errors.New("Config does not exist, check your config key")
	}

	if globalFlag {
		cfg.SetHomeString(args[1])
	} else {
		cfg.SetProjectString(args[1])
	}

	fmt.Printf("Setting %s to %s successfully\n", cfg.Path, args[1])
	return nil
}
