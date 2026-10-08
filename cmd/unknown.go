package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/internal/telemetry"
)

type unknownCommand struct {
	parent      *cobra.Command
	word        string
	suggestions []string
}

// trackUnknownCommand records a command the CLI does not have. It only
// listens: the refusal itself is cliout.Execute's, which reports an unknown
// command by the output contract (an ErrorObject under --output json, cobra's
// wording otherwise) and exits 2. Execute calls it after a run that failed as
// a usage error, so a run that worked sends nothing here, and by then cobra
// has added the help and completion commands, which would otherwise make
// `astro help` read as a command we do not have.
//
// It sees the groups that cannot run — the root, and every group without a
// RunE, which bareGroups refuses. A group with a RunE of its own (`astro
// local`, `astro af`) is runnable, and a word after a runnable command is an
// argument, so a mistyped subcommand there is not counted.
func trackUnknownCommand(root *cobra.Command, args []string) {
	if isShellCompletion(args) {
		return
	}
	unknown := findUnknownCommand(root, args)
	if unknown == nil {
		return
	}
	telemetry.TrackUnknownCommand(unknown.parent, unknown.word, unknown.suggestion())
}

// trackUnknownFlag records a flag the CLI does not have, then hands the error
// back unchanged for cliout to mark as a usage error and report. Cobra parses
// the flags before it runs the hook that tracks commands, so a wrong flag
// sends nothing without this. The root sets it once: a command with no error
// function of its own asks its parent for one, and cliout's (markUsageErrors,
// AddOutputFlag) consults it.
func trackUnknownFlag(cmd *cobra.Command, err error) error {
	if flag := unknownFlag(err); flag != "" {
		telemetry.TrackUnknownFlag(cmd, flag)
	}
	return err
}

// unknownFlag returns the flag as it was typed when pflag has no such flag, and
// "" for every other parse error: a missing value, or a value of the wrong
// type, is a mistake on a flag we do have. pflag reports the name on its own,
// so `--api-token=secret` arrives here as `--api-token`.
func unknownFlag(err error) string {
	var notExist *pflag.NotExistError
	if !errors.As(err, &notExist) {
		return ""
	}
	if notExist.GetSpecifiedShortnames() != "" {
		return "-" + notExist.GetSpecifiedName()
	}
	return "--" + notExist.GetSpecifiedName()
}

// isShellCompletion reports whether the shell is asking cobra for completions.
func isShellCompletion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd
}

// findUnknownCommand returns the first word that names no command, or nil when
// every word resolves. A command that runs takes its own arguments, so a word
// after one is an argument and not a guess at a command name.
func findUnknownCommand(root *cobra.Command, args []string) *unknownCommand {
	matched, rest, _ := root.Find(args) //nolint:errcheck // an unknown word is what Find's error reports, and what this looks for itself
	if matched.Runnable() || !matched.HasSubCommands() {
		return nil
	}

	operands := stripFlags(matched, rest)
	if len(operands) == 0 {
		return nil
	}

	return &unknownCommand{
		parent:      matched,
		word:        operands[0],
		suggestions: suggestionsFor(matched, operands[0]),
	}
}

func (u *unknownCommand) suggestion() string {
	if len(u.suggestions) == 0 {
		return ""
	}
	return u.suggestions[0]
}

// suggestionDistance is the edit distance cobra uses for "Did you mean this?".
// SuggestionsFor reads it from the command, which cobra fills in only on its
// own error path.
const suggestionDistance = 2

func suggestionsFor(parent *cobra.Command, word string) []string {
	if parent.DisableSuggestions {
		return nil
	}
	if parent.SuggestionsMinimumDistance <= 0 {
		parent.SuggestionsMinimumDistance = suggestionDistance
	}
	return parent.SuggestionsFor(word)
}

// stripFlags drops flags and their values, leaving the words cobra would read
// as commands. It follows cobra's own stripFlags, which is private, so that the
// word we report is the word cobra failed to match.
func stripFlags(cmd *cobra.Command, args []string) []string {
	flags := cmd.Flags()
	flags.AddFlagSet(cmd.InheritedFlags())

	operands := []string{}
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]

		switch {
		case arg == "--":
			return operands
		case consumesNextValue(flags, arg):
			if len(args) <= 1 {
				return operands
			}
			args = args[1:]
		case arg != "" && !strings.HasPrefix(arg, "-"):
			operands = append(operands, arg)
		}
	}
	return operands
}

// consumesNextValue reports whether arg is a flag that takes the word after it
// as its value. A flag we do not have is assumed to take one, which is what
// cobra assumes.
func consumesNextValue(flags *pflag.FlagSet, arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	if strings.HasPrefix(arg, "--") {
		return !takesNoValue(flags.Lookup(arg[2:]))
	}
	return len(arg) == 2 && strings.HasPrefix(arg, "-") && !takesNoValue(flags.ShorthandLookup(arg[1:]))
}

func takesNoValue(flag *pflag.Flag) bool {
	return flag != nil && flag.NoOptDefVal != ""
}
