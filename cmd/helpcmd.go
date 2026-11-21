package cmd

import (
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
	var (
		format cliout.Format
		all    bool
	)
	helpCmd := &cobra.Command{
		Use:   helpName + " [command]",
		Short: "Help about any command",
		Long: `Help provides help for any command in the application.
Simply type astro help [path to command] for full details.

With --output json, a command's help is one JSON document, for scripts and coding agents. astro help search <QUERY> lists the commands that match every word of the query, up to 10. astro help --all lists every command, one per line with its summary, or with --output json every command's help in one document.`,
		Example: `  astro help deployment list
  astro help deployment list --output json
  astro help search "create deployment"
  astro help --all --output json`,
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
			return runHelp(c, args, all, format)
		},
	}
	cliout.AddOutputFlag(helpCmd, &format)
	helpCmd.Flags().BoolVar(&all, "all", false, "List every command")
	return helpCmd
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
