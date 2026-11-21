package cliout

import (
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/astronomer/astro-cli/internal/telemetry"
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
// "use --deployment".
func WarnDeprecated(w io.Writer, spelling, use string) {
	WarnOnce(w, spelling, spelling+" is deprecated: "+use)
}

var (
	onceMu     sync.Mutex
	onceWarned = map[string]bool{}
)

// WarnOnce prints msg as a warning to w, once per run for each key. It is what
// WarnDeprecated is made of, for a warning that is no deprecated spelling but
// is as much the run's, not the process's, such as an environment variable
// that is set and no longer used. Execute starts each run with nothing said.
func WarnOnce(w io.Writer, key, msg string) {
	onceMu.Lock()
	seen := onceWarned[key]
	onceWarned[key] = true
	onceMu.Unlock()
	if !seen {
		Warn(w, msg)
	}
}

// ResetWarnOnce forgets what the run has warned of. Execute does it as a run
// starts; a test that does not run through Execute does it for itself.
func ResetWarnOnce() {
	onceMu.Lock()
	clear(onceWarned)
	onceMu.Unlock()
}

// WarnDeprecatedFlags warns, through WarnDeprecated, of each deprecated flag
// (DeprecateFlag) this run gave cmd, in the order of their names.
func WarnDeprecatedFlags(w io.Writer, cmd *cobra.Command) {
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if use, ok := DeprecatedUse(f); ok {
			WarnDeprecated(w, "--"+f.Name, use)
		}
	})
}

// RecordRenamed queues the telemetry event of a deprecated spelling a run used
// (telemetry.RecordRenamedFlags, as the hidden flag aliases do); a test swaps it
// to see what was sent.
var RecordRenamed = telemetry.RecordRenamedFlags

// DeprecatedCommandAnnotation marks, with "true", a command that is a
// deprecated spelling (DeprecateCommand), for the tests that hold each one to
// its table.
const DeprecatedCommandAnnotation = "astro.deprecated-command"

// DeprecateCommand marks cmd as a command v2 renamed or folded into another,
// kept for the scripts that type it: it is hidden from help, still runs as
// it did, and says once per run (WarnDeprecated) that the spelling is
// deprecated, naming what to type instead. replacement reads the command
// path to use from cmd as it was typed (cmd.Parent().CommandPath()+" remove"),
// since the tree around it is known only once it is built. suffix, when the
// new spelling is not quite the old one (it asks where the old one did not,
// it shows more), follows the path in the warning: "use astro x remove --yes".
// The run is noted for telemetry as the hidden flag aliases are
// (telemetry.RecordRenamedFlags), the old path beside the new one, so we know
// when the old verb can go. It returns cmd.
//
// The warning is given with the arguments check, which runs for a command
// that is typed whatever happens next, as a hidden flag alias's does, and
// not in the run, so a test that replaces every run still sees it.
func DeprecateCommand(cmd *cobra.Command, replacement func(cmd *cobra.Command) string, suffix string) *cobra.Command {
	cmd.Hidden = true
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[DeprecatedCommandAnnotation] = "true"
	check := cmd.Args
	if check == nil {
		check = UndeclaredArgs
	}
	cmd.Args = func(c *cobra.Command, args []string) error {
		instead := replacement(c)
		WarnDeprecated(c.ErrOrStderr(), c.CommandPath(), "use "+instead+suffix)
		RecordRenamed(c, []string{strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")}, []string{strings.TrimPrefix(instead, c.Root().Name()+" ")})
		return check(c, args)
	}
	return cmd
}
