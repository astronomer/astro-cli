package cliout

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// A group is a command that holds others. Run with nothing it can run, it
// answers the same way in every tree, by one of two routes:
//
//   - A group with a RunE of its own (`astro af`, `astro local`, the env
//     families) returns GroupHelp from it, after any refusal of its own for
//     an argument it recognizes (a removed verb, a 1.x spelling).
//   - A group with none is not runnable, so cobra answers it by calling the
//     help func and succeeding. Execute intercepts that (bareGroups) and
//     answers as GroupHelp would.
//
// The answer: with an argument that names no subcommand, a usage error saying
// so, with cobra's "Did you mean this?" (exit 2, in either mode). With no
// argument, its help in text mode, as always; under --output json a usage
// error naming its subcommands, since help is prose and a json run's stdout
// holds only the result.

// GroupHelp is a group's RunE: see above.
func GroupHelp(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return UnknownSubcommand(cmd, args[0])
	}
	if !isJSON(cmd) {
		return cmd.Help()
	}
	return Usage(needsSubcommand(cmd))
}

// UnknownSubcommand is the usage error for arg, which names none of cmd's
// subcommands, with the suggestions cobra's own refusal would carry.
func UnknownSubcommand(cmd *cobra.Command, arg string) error {
	msg := fmt.Sprintf("unknown command %q for %q", arg, cmd.CommandPath())
	if suggestions := suggestionsFor(cmd, arg); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += "\t" + s + "\n"
		}
	}
	return Usage(errors.New(msg))
}

// suggestionsFor is what cobra's own refusal of an unknown command would
// suggest for arg among cmd's subcommands. Cobra refuses from the root, so it
// reads the root's settings: nothing when the root (or here, any command on
// the way to cmd) disables suggestions, and otherwise the subcommands within
// the root's SuggestionsMinimumDistance, 2 when that is unset. SuggestionsFor
// reads the distance off the command it is called on, so cmd carries the
// root's for the call and gets its own back after.
func suggestionsFor(cmd *cobra.Command, arg string) []string {
	for c := cmd; c != nil; c = c.Parent() {
		if c.DisableSuggestions {
			return nil
		}
	}
	distance := cmd.Root().SuggestionsMinimumDistance
	if distance <= 0 {
		distance = defaultSuggestionDistance
	}
	own := cmd.SuggestionsMinimumDistance
	defer func() { cmd.SuggestionsMinimumDistance = own }()
	cmd.SuggestionsMinimumDistance = distance
	return cmd.SuggestionsFor(arg)
}

// defaultSuggestionDistance is cobra's, which findSuggestions applies when the
// root sets none.
const defaultSuggestionDistance = 2

// isJSON reports whether cmd's parsed --output is json.
func isJSON(cmd *cobra.Command) bool {
	f := cmd.Flag("output")
	return f != nil && f.Value.String() == string(FormatJSON)
}

// needsSubcommand is why a group run bare under json produced nothing.
func needsSubcommand(cmd *cobra.Command) error {
	var names []string
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() {
			names = append(names, sub.Name())
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("%q has no command to run", cmd.CommandPath())
	}
	return fmt.Errorf("%q needs a subcommand: %s", cmd.CommandPath(), strings.Join(names, ", "))
}

// bareGroups answers, for one run, the group with no RunE that cobra is about
// to answer with its help. It returns the error that answer ends the run with,
// if it was one, and a func that puts the root's help func back.
//
// Only cobra's own path is intercepted: the command the arguments resolve to
// (target), when it is a group that cannot run and was not asked for --help.
// Help any command prints of its own accord, a parent's included, is help. The
// help func is wrapped rather than every group given a RunE: a RunE would make
// the group runnable, and its help would gain a usage line it does not have.
func bareGroups(root, target *cobra.Command) (refused func(ran *cobra.Command) error, restore func()) {
	help := root.HelpFunc()
	var (
		err error
		at  *cobra.Command
	)
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c != target || c.Runnable() || !c.HasSubCommands() || helpAsked(c) {
			help(c, args)
			return
		}
		switch extra := c.Flags().Args(); {
		case len(extra) > 0:
			err = UnknownSubcommand(c, extra[0])
		case isJSON(c):
			err = Usage(needsSubcommand(c))
		default:
			help(c, args)
			return
		}
		at = c
	})
	refused = func(ran *cobra.Command) error {
		if ran != at {
			return nil
		}
		return err
	}
	return refused, func() { root.SetHelpFunc(help) }
}

// helpAsked reports whether cmd was run with -h/--help.
func helpAsked(cmd *cobra.Command) bool {
	f := cmd.Flags().Lookup("help")
	return f != nil && f.Changed
}
