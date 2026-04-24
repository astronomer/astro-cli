package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/otto"
)

func newOttoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "otto [flags/args forwarded to Otto]",
		Short:              "Start the Otto AI agent",
		Long:               "Start the Otto AI agent for AI-assisted Airflow development and operations.\nAll flags and arguments are forwarded directly to Otto.",
		SilenceUsage:       true,
		DisableFlagParsing: true,
		RunE:               ottoRun,
	}

	return cmd
}

func ottoRun(cmd *cobra.Command, args []string) error {
	// With DisableFlagParsing, cobra won't route to subcommands,
	// so we dispatch "update" and "version" ourselves.
	if len(args) > 0 {
		switch args[0] {
		case "update":
			return otto.Update()
		case "version":
			installed, err := otto.InstalledVersion()
			if err != nil {
				return err
			}
			if installed == "" {
				fmt.Println("Otto is not installed. Run `astro otto` to install.")
				return nil
			}
			fmt.Printf("Otto %s\n", installed)

			available, latest, err := otto.IsUpdateAvailable()
			if err == nil && available {
				fmt.Printf("Update available: %s (run `astro otto update`)\n", latest)
			}
			return nil
		}
	}

	// Everything else (flags, prompt, etc.) is forwarded directly to Otto.
	// Propagate Otto's exit code when it exits non-zero — Otto has already
	// printed its own error, so we skip cobra's "Error: exit status N" noise.
	// Same treatment for ErrNotLoggedIn: Start has already printed the sign-up
	// guidance to stderr, so returning the error to cobra would just tack on a
	// redundant "Error: not logged in" line.
	err := otto.Start(args)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	if errors.Is(err, otto.ErrNotLoggedIn) {
		os.Exit(1)
	}
	return err
}
