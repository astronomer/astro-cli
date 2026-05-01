package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/otto"
)

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func printAstroSubcommands(w io.Writer) {
	fmt.Fprintln(w, "Subcommands handled by astro CLI (everything else is forwarded to Otto):")
	fmt.Fprintln(w, "  astro otto update    Download and install the latest Otto binary")
	fmt.Fprintln(w, "  astro otto version   Print the installed Otto version")
	fmt.Fprintln(w)
}

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
	// so we dispatch "update" and "version" ourselves. "upgrade" is
	// aliased to "update" — otherwise it falls through to Otto and gets
	// interpreted as a prompt asking it to upgrade the Astro project.
	if len(args) > 0 {
		switch args[0] {
		case "update", "upgrade":
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

	// `astro otto --help` forwards to Otto's own help, which doesn't know
	// about the astro CLI's `update` / `version` subcommands. Prepend a short
	// banner so they're discoverable.
	if hasHelpFlag(args) {
		printAstroSubcommands(os.Stdout)
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
