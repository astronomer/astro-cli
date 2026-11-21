package cliout

import (
	"errors"
	"fmt"
	"slices"
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
	return misinvoked(needsSubcommand(cmd))
}

// UnknownSubcommand is the usage error for arg, which names none of cmd's
// subcommands, with the suggestions cobra's own refusal would carry.
func UnknownSubcommand(cmd *cobra.Command, arg string) error {
	msg := fmt.Sprintf("unknown command %q for %q", arg, cmd.CommandPath())
	if suggestions := SubcommandSuggestions(cmd, arg); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += "\t" + s + "\n"
		}
	}
	return misinvoked(errors.New(msg))
}

// SubcommandSuggestions is what cobra's own refusal of an unknown command
// would suggest for arg among cmd's subcommands, by cobra's own matcher
// (plain Levenshtein or a prefix; unlike suggestFlag's, a transposition is
// two edits). Cobra refuses from the root, so it reads the root's settings
// (suggestionDistance): nothing when the root, or here any command on the
// way to cmd, disables suggestions. It is what a refusal shows and what
// telemetry records for it, so the two agree. SuggestionsFor reads the
// distance off the command it is called on, so cmd carries the root's for
// the call and gets its own back after.
func SubcommandSuggestions(cmd *cobra.Command, arg string) []string {
	distance, ok := suggestionDistance(cmd)
	if !ok {
		return nil
	}
	own := cmd.SuggestionsMinimumDistance
	defer func() { cmd.SuggestionsMinimumDistance = own }()
	cmd.SuggestionsMinimumDistance = distance
	return closestOne(cmd, arg, cmd.SuggestionsFor(arg))
}

// closestOne narrows cobra's candidates to the one to suggest. A command that
// names arg in its SuggestFor is the one its author meant for that word, and
// wins as it does in cobra. Otherwise the candidate nearest arg by EditDistance
// is the only one shown, and none when two are equally near: a refusal that
// offers two guesses is not helping, and a wrong one costs another error.
func closestOne(cmd *cobra.Command, arg string, candidates []string) []string {
	var explicit []string
	for _, sub := range cmd.Commands() {
		for _, word := range sub.SuggestFor {
			if strings.EqualFold(word, arg) && slices.Contains(candidates, sub.Name()) {
				explicit = append(explicit, sub.Name())
			}
		}
	}
	if len(explicit) > 0 {
		candidates = explicit
	}
	var best []string
	nearest := -1
	for _, c := range candidates {
		d := EditDistance(strings.ToLower(arg), strings.ToLower(c))
		switch {
		case nearest < 0 || d < nearest:
			nearest, best = d, []string{c}
		case d == nearest:
			best = append(best, c)
		}
	}
	if len(best) != 1 {
		return nil
	}
	return best
}

// suggestionDistance is how far a suggestion for something typed on cmd may
// be, by the settings cobra's own refusal reads: not at all (ok false) when
// cmd or a command on the way up disables suggestions, else the root's
// SuggestionsMinimumDistance, DefaultSuggestionDistance when that is unset.
// Subcommand and flag suggestions both read it, so they agree.
func suggestionDistance(cmd *cobra.Command) (distance int, ok bool) {
	for c := cmd; c != nil; c = c.Parent() {
		if c.DisableSuggestions {
			return 0, false
		}
	}
	distance = cmd.Root().SuggestionsMinimumDistance
	if distance <= 0 {
		distance = DefaultSuggestionDistance
	}
	return distance, true
}

// DefaultSuggestionDistance is cobra's distance for "Did you mean", which
// findSuggestions applies when the root sets none.
const DefaultSuggestionDistance = 2

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
			err = misinvoked(needsSubcommand(c))
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

// ArgvPassthroughAnnotation marks a command whose arguments after the first
// operand belong to a program it runs (`astro local run pytest --version`), so
// nothing after that operand is the CLI's to read: not a flag, and not the
// root's --version.
const ArgvPassthroughAnnotation = "argv-passthrough"

// UndeclaredArgs is the check a command gets when it declares no positionals
// (Args). Cobra's own default accepts any words, so a stray argument ran the
// command anyway (and past a pre-run login that failed for an unrelated
// reason). A group that runs refuses a word that names none of its
// subcommands. A leaf takes no arguments, and says so, rather than calling the
// word an unknown command it has none of.
func UndeclaredArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if cmd.HasSubCommands() {
		return UnknownSubcommand(cmd, args[0])
	}
	return misinvoked(&ArgsError{Command: cmd.CommandPath(), Unexpected: args[0]})
}
