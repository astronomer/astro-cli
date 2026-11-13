package cliout

import (
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// A spelling that still works but that v2 means to drop (a hidden 1.x flag
// alias, a kept 1.x subcommand, a flag deprecated in v2) says so once per run,
// through WarnDeprecated, the one helper every such spelling goes through:
//
//   - one line on stderr, led by the CLI's yellow Warning: label, naming what
//     was typed and what to type instead: --deployment-id is deprecated: use
//     --deployment;
//   - once per run per distinct spelling: two different old flags give two
//     lines, the same one twice gives one;
//   - stderr only, never stdout. Under --output json stdout is still the one
//     document, the result or the error object, as with every other warning;
//   - nothing turns it off, and it changes no exit code.
//
// A removed spelling does not come here: it fails with its removal message
// (removedFlagError) and the run never gets as far as using it. A spelling
// that is simply supported (the ls alias of list) is not deprecated and says
// nothing.

// deprecatedAnnotation marks, on a flag, that its spelling is deprecated, with
// what to type instead as its value.
const deprecatedAnnotation = "astro.deprecated-use"

// warnedAnnotation holds, on a run's root, the spellings it has warned of, one
// to a line. Execute clears it as a run starts.
const warnedAnnotation = "astro.deprecation-warned"

// DeprecateFlag marks f as a deprecated spelling, with use as what to type
// instead ("use --deployment"), and hides it from help. WarnDeprecatedFlags
// warns of it when a run gives it.
func DeprecateFlag(f *pflag.Flag, use string) {
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[deprecatedAnnotation] = []string{use}
	f.Hidden = true
}

// DeprecatedUse is what to type instead of f, when f is a deprecated
// spelling (DeprecateFlag), and whether it is one.
func DeprecatedUse(f *pflag.Flag) (string, bool) {
	if use := f.Annotations[deprecatedAnnotation]; len(use) == 1 {
		return use[0], true
	}
	return "", false
}

// WarnDeprecated tells the run, on stderr (w, which is the command's own error
// stream, os.Stderr in the binary), that spelling (as typed: "--secret",
// "astro deployment logs scheduler") is deprecated and that use is what to
// type instead, once for each spelling in a run. use reads after the colon:
// "use --deployment". cmd is any command of the run's tree: the run's root
// remembers what it has said.
func WarnDeprecated(w io.Writer, cmd *cobra.Command, spelling, use string) {
	root := cmd.Root()
	warned := root.Annotations[warnedAnnotation]
	if slices.Contains(strings.Split(warned, "\n"), spelling) {
		return
	}
	if root.Annotations == nil {
		root.Annotations = map[string]string{}
	}
	root.Annotations[warnedAnnotation] = warned + spelling + "\n"
	Warn(w, spelling+" is deprecated: "+use)
}

// WarnDeprecatedFlags warns, through WarnDeprecated, of each deprecated flag
// (DeprecateFlag) this run gave cmd, in the order of their names.
func WarnDeprecatedFlags(w io.Writer, cmd *cobra.Command) {
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if use, ok := DeprecatedUse(f); ok {
			WarnDeprecated(w, cmd, "--"+f.Name, use)
		}
	})
}

// resetDeprecations forgets what root has warned of, as a new run starts.
func resetDeprecations(root *cobra.Command) {
	delete(root.Annotations, warnedAnnotation)
}
