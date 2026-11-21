package cmd

import (
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/version"
)

// versionOutput is what `astro version -o json` prints. It is a contract, so
// it starts minimal: a field is easy to add and impossible to take back.
type versionOutput struct {
	Version string `json:"version"`
}

func newVersionCommand() *cobra.Command {
	var output cliout.Format
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Astro CLI version",
		Long:  `Print the Astro CLI version.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cliout.Renderer{Format: output, Out: cmd.OutOrStdout()}.Emit(
				versionOutput{Version: version.Current()},
				version.PrintVersion,
			)
		},
		Example: `  # Print the CLI's version
  astro version`,
	}
	cliout.AddOutputFlag(cmd, &output)
	return cmd
}

// versionFlag is the root's --version. Like --verbosity it is listed on the
// root page only. There is no -v: it is already the short form of --verbose on
// several commands.
const versionFlag = "version"

// addVersionFlag declares --version, so that every command parses it. The flag
// itself does nothing: a run that carries it is answered before cobra runs
// (printVersionIfAsked), ahead of any login, config read or network call.
func addVersionFlag(fs *pflag.FlagSet) {
	fs.Bool(versionFlag, false, "Print the Astro CLI version")
}

// printVersionIfAsked prints what `astro version` prints and reports true when
// args ask for --version on the command they run. Help wins: a line that also
// asks for help is left to cobra. A command that reads its own arguments
// (flag parsing off), a flag that takes the word --version as its value, and
// anything after "--" are not asks.
func printVersionIfAsked(root *cobra.Command, args []string, out io.Writer) (bool, error) {
	if isShellCompletion(args) || hasHelpFlag(args) {
		return false, nil
	}
	// rest is the line without the words that named the command, so a
	// passthrough command's own operand is the first word left.
	target, rest, err := root.Find(args)
	if err != nil || target.DisableFlagParsing || !hasVersionFlag(target, rest) {
		return false, nil
	}
	// The same shape `astro version -o json` prints.
	format := cliout.FormatText
	if cliout.ArgsAskForJSON(args, "o") {
		format = cliout.FormatJSON
	}
	return true, cliout.Renderer{Format: format, Out: out}.Emit(versionOutput{Version: version.Current()}, version.PrintVersion)
}

func hasVersionFlag(cmd *cobra.Command, args []string) bool {
	flags := pflag.NewFlagSet("version", pflag.ContinueOnError)
	flags.AddFlagSet(cmd.Flags())
	flags.AddFlagSet(cmd.InheritedFlags())
	if f := flags.Lookup(versionFlag); f == nil {
		return false
	}
	passthrough := cmd.Annotations[cliout.ArgvPassthroughAnnotation] == "true"
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			return false
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if name == "--"+versionFlag {
			if on, err := strconv.ParseBool(value); !hasValue || (err == nil && on) {
				return true
			}
			continue
		}
		if consumesNextValue(flags, arg) && len(args) > 0 {
			args = args[1:]
		} else if passthrough && !strings.HasPrefix(arg, "-") {
			return false
		}
	}
	return false
}
