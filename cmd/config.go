package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/sjmiller609/astro-cli/config"
	"github.com/sjmiller609/astro-cli/houston"
	"github.com/sjmiller609/astro-cli/messages"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newConfigRootCmd(client *houston.Client, out io.Writer) *cobra.Command {
	var globalFlag bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage astro project configurations",
		Long:  "Manage astro project configurations",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ensureGlobalFlag(cmd, args, globalFlag)
		},
	}
	cmd.PersistentFlags().BoolVarP(&globalFlag, "global", "g", false, "view or modify global config")
	cmd.AddCommand(
		newConfigGetCmd(client, out, globalFlag),
		newConfigSetCmd(client, out, globalFlag),
	)
	return cmd
}

func newConfigGetCmd(client *houston.Client, out io.Writer, globalFlag bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get astro project configuration",
		Long:  "Get astro project configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configGet(cmd, args, globalFlag)
		},
	}
	return cmd
}

func newConfigSetCmd(client *houston.Client, out io.Writer, globalFlag bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set astro project configuration",
		Long:  "Set astro project configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return configSet(cmd, args, globalFlag)
		},
	}
	return cmd
}

func ensureGlobalFlag(cmd *cobra.Command, args []string, globalFlag bool) {
	isProjectDir, _ := config.IsProjectDir(config.WorkingPath)

	if !isProjectDir && !globalFlag {
		var c = "astro config " + cmd.Use + " " + args[0] + " -g"
		fmt.Printf(messages.CONFIG_USE_OUTSIDE_PROJECT_DIR, cmd.Use, cmd.Use, c)
		os.Exit(1)
	}
}

func configGet(cmd *cobra.Command, args []string, globalFlag bool) error {
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

func configSet(cmd *cobra.Command, args []string, globalFlag bool) error {
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
