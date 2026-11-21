package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/telemetry"
)

// newHelpCommand is `astro help [command]`. Cobra's own answers a word that
// names no command with "Unknown help topic", the root's help and success; a
// script that asked for a command's help and mistyped it would see a zero exit
// and the wrong page. This one fails like any unknown command does: a usage
// error naming the first word that matched nothing, with the nearest command
// it could be, and a pointer to the help of the command it was looked up under.
func newHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   helpName + " [command]",
		Short: "Help about any command",
		Long: `Help provides help for any command in the application.
Simply type ` + "`astro help [path to command]`" + ` for full details.`,
		Args: cobra.ArbitraryArgs,
		// Help needs no login, version check or tracking.
		Annotations: map[string]string{telemetry.SkipPreRunAnnotation: "true"},
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			cmd, _, e := c.Root().Find(args)
			if cmd == nil || e != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var completions []cobra.Completion
			for _, sub := range cmd.Commands() {
				if sub.IsAvailableCommand() || sub == c {
					if strings.HasPrefix(sub.Name(), toComplete) {
						completions = append(completions, cobra.CompletionWithDesc(sub.Name(), sub.Short))
					}
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			target := c.Root()
			for _, word := range args {
				next := childNamed(target, word)
				if next == nil {
					err := cliout.UnknownSubcommand(target, word)
					return cliout.UsageNoHelp(fmt.Errorf("%s\n\nRun %s --help for usage.", err.Error(), target.CommandPath()))
				}
				target = next
			}
			return target.Help()
		},
	}
}

// childNamed is the subcommand of parent that word names, by name or alias.
func childNamed(parent *cobra.Command, word string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == word || sub.HasAlias(word) {
			return sub
		}
	}
	return nil
}
