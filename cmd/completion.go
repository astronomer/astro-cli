package cmd

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/spf13/cobra"
)

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

var (
	completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
	}
)

func runCompletion(out io.Writer, cmd *cobra.Command, args []string) error {
	cmd.Hidden = true
	return completionShells[args[0]](out, cmd)
}

func runCompletionBash(w io.Writer, cmd *cobra.Command) error {
	err := cmd.Root().GenBashCompletion(w)
	if err != nil {
		return fmt.Errorf("error while generating bash completion: %v", err)
	}
	return nil
}

func runCompletionZsh(w io.Writer, cmd *cobra.Command) error {
	err := cmd.Root().GenZshCompletion(w)
	if err != nil {
		return fmt.Errorf("error while generating bash completion: %v", err)
	}
	return nil
}

func newCompletionCmd(client *houston.Client, out io.Writer) *cobra.Command {
	var shells []string
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
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: shells,
	}
	return cmd
}
