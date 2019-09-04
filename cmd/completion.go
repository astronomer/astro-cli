package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"

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
`

var (
	completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
	}
)

func runCompletion(out io.Writer, cmd *cobra.Command, args []string) error {
	return completionShells[args[0]](out, cmd)
}

func runCompletionBash(_ io.Writer, cmd *cobra.Command) error {
	var buf bytes.Buffer
	err := cmd.Root().GenBashCompletion(&buf)
	if err != nil {
		return fmt.Errorf("error while generating bash completion: %v", err)
	}
	code := buf.String()
	fmt.Print(code)
	return nil
}

func runCompletionZsh(_ io.Writer, cmd *cobra.Command) error {
	var buf bytes.Buffer
	err := cmd.Root().GenZshCompletion(&buf)
	if err != nil {
		return fmt.Errorf("error while generating bash completion: %v", err)
	}
	code := buf.String()
	fmt.Print(code)
	return nil
}

func init() {
	var shells []string
	for s := range completionShells {
		shells = append(shells, s)
	}

	cmd := &cobra.Command{
		Use:   "completion SHELL",
		Short: "Generate autocompletions script for the specified shell (bash or zsh)",
		Long:  completionLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(os.Stdout, cmd, args)
		},
		Args: cobra.ExactValidArgs(1),
		ValidArgs: shells,
	}

	RootCmd.AddCommand(cmd)
}
