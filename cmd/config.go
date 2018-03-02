package cmd

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var (
	configRootCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage astro project configurations",
		Long:  "Manage astro project configurations",
	}

	configGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get astro project configuration",
		Long:  "Get astro project configuration",
		RunE:  configGet,
		// TODO add arg validator
		// TODO check project specific config
	}

	configSetCmd = &cobra.Command{
		Use:   "set",
		Short: "Set astro project configuration",
		Long:  "Set astro project configuration",
		RunE:  configSet,
		// TODO add arg validator
		// TODO check project specific config
	}
)

func init() {
	// Set up project root
	projectRoot, _ = config.ProjectRoot()

	// Config root
	RootCmd.AddCommand(configRootCmd)

	// Config get
	configRootCmd.AddCommand(configGetCmd)
	// Config set
	configRootCmd.AddCommand(configSetCmd)
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

	if !cfg.Gettable {
		errMsg := fmt.Sprintf("Config %s is not gettable", cfg.Path)
		return errors.New(errMsg)
	}

	fmt.Printf("%s: %s\n", cfg.Path, cfg.GetString())

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

	if !cfg.Settable {
		errMsg := fmt.Sprintf("Config %s is not settable", cfg.Path)
		return errors.New(errMsg)
	}

	cfg.SetProjectString(args[1])

	fmt.Printf("Setting %s to %s successfully\n", cfg.Path, args[1])
	return nil
}
