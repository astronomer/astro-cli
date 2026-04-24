package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/agent"
)

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "agent [flags/args forwarded to Otto]",
		Short:              "Start the Otto AI agent",
		Long:               "Start the Otto AI agent for AI-assisted Airflow development and operations.\nAll flags and arguments are forwarded directly to Otto.",
		SilenceUsage:       true,
		DisableFlagParsing: true,
		RunE:               agentRun,
	}

	return cmd
}

func agentRun(cmd *cobra.Command, args []string) error {
	// With DisableFlagParsing, cobra won't route to subcommands,
	// so we dispatch "update" and "version" ourselves.
	if len(args) > 0 {
		switch args[0] {
		case "update":
			return agent.Update()
		case "version":
			installed, err := agent.InstalledVersion()
			if err != nil {
				return err
			}
			if installed == "" {
				fmt.Println("Otto is not installed. Run `astro agent` to install.")
				return nil
			}
			fmt.Printf("Otto %s\n", installed)

			available, latest, err := agent.IsUpdateAvailable()
			if err == nil && available {
				fmt.Printf("Update available: %s (run `astro agent update`)\n", latest)
			}
			return nil
		}
	}

	// Everything else (flags, prompt, etc.) is forwarded directly to Otto.
	// Propagate Otto's exit code when it exits non-zero — Otto has already
	// printed its own error, so we skip cobra's "Error: exit status N" noise.
	err := agent.Start(args)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
