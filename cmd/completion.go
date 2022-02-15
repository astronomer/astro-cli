package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var ErrGeneratingBash = "error while generating bash completion"

const completionLong = `
Generate autocompletions script for Astro for the specified shell (bash or zsh).

This command can generate shell autocompletions. e.g.

	$ astro completion bash

Can be sourced as such

	$ source <(astro completion bash)

Bash users can as well save it to the file and copy it to:
	/etc/bash_completion.d/

Correct arguments for SHELL are: "bash" and "zsh".
Notes:

1) zsh completions requires zsh 5.2 or newer.

2) macOS users have to install bash-completion framework to utilize
completion features. This can be done using homebrew:
	brew install bash-completion

Once installed, you must load bash_completion by adding following
line to your .profile or .bashrc/.zshrc:
	source $(brew --prefix)/etc/bash_completion

3) For oh-my-zsh users make sure you have added to your ~/.zshrc
	autoload -Uz compinit && compinit -C
`

var completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
	"bash": runCompletionBash,
	"zsh":  runCompletionZsh,
}

func runCompletion(out io.Writer, cmd *cobra.Command, args []string) error {
	cmd.Hidden = true
	return completionShells[args[0]](out, cmd)
}

func runCompletionBash(w io.Writer, cmd *cobra.Command) error {
	err := cmd.Root().GenBashCompletion(w)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratingBash, err)
	}
	return nil
}

func runCompletionZsh(w io.Writer, cmd *cobra.Command) error {
	err := cmd.Root().GenZshCompletion(w)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGeneratingBash, err)
	}
	return nil
}

func newCompletionCmd(out io.Writer) *cobra.Command {
	shells := make([]string, 0, len(completionShells))
	for s := range completionShells {
		shells = append(shells, s)
	}
	cmd := &cobra.Command{
		Use:   "completion SHELL",
		Short: "Generate autocompletions script for the specified shell (bash or zsh)",
		Long:  completionLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(out, cmd, args)
		},
		// ignore PersistentPreRunE of root command
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := SetUpLogs(out, verboseLevel)
			printDebugLogs()
			return err
		},
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: shells,
	}
	return cmd
}
