package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var globalFlag bool

func newConfigRootCmd(_ *houston.Client, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage project configuration",
		Long:  "Manage project configuration",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ensureGlobalFlag(cmd, args)
		},
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
		Use:   "get",
		Short: "Get project configuration",
		Long:  "Get project configuration",
		RunE:  configGet,
	}
	return cmd
}

func newConfigSetCmd(_ io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set project configuration",
		Long:  "Set project configuration",
		RunE:  configSet,
	}
	return cmd
}

func ensureGlobalFlag(cmd *cobra.Command, args []string) {
	isProjectDir, _ := config.IsProjectDir(config.WorkingPath)

	if !isProjectDir && !globalFlag {
		c := "astro config " + cmd.Use + " " + args[0] + " -g"
		fmt.Printf(messages.ConfigUseOutsideProjectDir, cmd.Use, cmd.Use, c)
		os.Exit(1)
	}
}

func configGet(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New(messages.ErrMissingConfigPathKey)
	}
	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]
	if !ok {
		return errors.New(messages.ErrInvalidConfigPathKey)
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
	if len(args) != 2 { // nolint:gomnd
		return errors.New(messages.ConfigInvalidSetArgs)
	}

	// get config struct
	cfg, ok := config.CFGStrMap[args[0]]

	if !ok {
		return errors.New(messages.ErrInvalidConfigPathKey)
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

	fmt.Printf(messages.ConfigSetSuccess+"\n", cfg.Path, args[1])
	return nil
}
