package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/internal/telemetry"
)

// unknownCommand is a word the CLI has no command for, under the command it
// was typed against.
type unknownCommand struct {
	parent      *cobra.Command
	word        string
	suggestions []string
}

// HandleUnknownCommand reports and tracks a command the CLI does not have, and
// returns true when it found one. It runs before Execute, so it first registers
// the help and completion commands that Execute would otherwise add after this
// point, which would make `astro help` read as a command we do not have.
//
// Cobra handles this in two ways and neither is much use. At the root it
// returns an error from Execute, which is too late for the hook that tracks
// commands. Under a parent such as `dev` it prints the help and exits 0, so a
// wrong guess reads as a success.
//
// The fix cobra would want, an Args validator on each parent, does not work
// here: cobra returns the help for a command that does not run before it
// validates the arguments, so every parent would have to become runnable. That
// would run their pre-run hooks on a bare `astro dev`, which checks for a
// container runtime, and `astro dev` would report a missing Docker where today
// it prints the help.
func HandleUnknownCommand(root *cobra.Command, args []string, out io.Writer) bool {
	if isShellCompletion(args) {
		return false
	}

	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd(args...)

	unknown := findUnknownCommand(root, args)
	if unknown == nil {
		return false
	}

	telemetry.TrackUnknownCommand(unknown.parent, unknown.word, unknown.suggestion())
	unknown.report(out)
	return true
}

// trackUnknownFlag records a flag the CLI does not have, then hands the error
// back for cobra to report as it always has. Cobra parses the flags before it
// runs the hook that tracks commands, so a wrong flag sends nothing at all
// without this.
//
// The root sets it once. A command with no error function of its own asks its
// parent for one, so this covers every command in the tree.
func trackUnknownFlag(cmd *cobra.Command, err error) error {
	if isShellCompletion(os.Args[1:]) {
		return err
	}
	if flag := unknownFlag(err); flag != "" {
		telemetry.TrackUnknownFlag(cmd, flag)
	}
	return err
}

// unknownFlag returns the flag as it was typed when pflag has no such flag,
// and "" for every other parse error. A missing value, or a value of the wrong
// type, is a mistake on a flag we do have, which is not what we are counting.
//
// pflag reports the name on its own, so `--api-token=secret` arrives here as
// `--api-token` and the value never reaches us.
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
// Cobra registers __complete from a private call we cannot make, so the command
// does not exist yet when we resolve the tree. The shell is talking to itself
// here, and a word it has not finished typing is not a guess at a command.
func isShellCompletion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd
}

// findUnknownCommand returns the first word that names no command, or nil when
// every word resolves.
//
// A command that runs takes its own arguments, so a word after it is an
// argument and not a guess at a command name.
func findUnknownCommand(root *cobra.Command, args []string) *unknownCommand {
	matched, rest, _ := root.Find(args)
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

// suggestion returns the nearest command name, or "" when the word is nothing
// like a command we have. An empty suggestion is the interesting case: it marks
// a command someone expected to exist.
func (u *unknownCommand) suggestion() string {
	if len(u.suggestions) == 0 {
		return ""
	}
	return u.suggestions[0]
}

// message is worded as cobra words it, so that an unknown subcommand reads the
// same as an unknown command at the root.
func (u *unknownCommand) message() string {
	message := fmt.Sprintf("unknown command %q for %q", u.word, u.parent.CommandPath())
	if len(u.suggestions) == 0 {
		return message
	}

	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\nDid you mean this?\n")
	for _, suggestion := range u.suggestions {
		fmt.Fprintf(&sb, "\t%v\n", suggestion)
	}
	return sb.String()
}

func (u *unknownCommand) report(out io.Writer) {
	fmt.Fprintln(out, u.parent.ErrPrefix(), u.message())
	fmt.Fprintf(out, "Run '%v --help' for usage.\n", u.parent.CommandPath())
}

// suggestionDistance is the edit distance cobra uses for "Did you mean this?".
// SuggestionsFor reads it from the command, which cobra fills in only on the
// error path, so we set it ourselves to get the same answer.
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
		case strings.HasPrefix(arg, "--") && !strings.Contains(arg, "=") && !takesNoValue(flags.Lookup(arg[2:])),
			strings.HasPrefix(arg, "-") && !strings.Contains(arg, "=") && len(arg) == 2 && !takesNoValue(flags.ShorthandLookup(arg[1:])):
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

func takesNoValue(flag *pflag.Flag) bool {
	return flag != nil && flag.NoOptDefVal != ""
}
