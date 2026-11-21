package cliout

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/pkg/input"
)

// The process exit statuses Execute's caller ends with. A command may carry
// its own through ExitError; these are the ones the contract itself assigns.
const (
	// ExitFailure is any failure that is neither of the two below.
	ExitFailure = 1
	// ExitUsage is a usage error: nothing ran. 2 is what getopt-style tools
	// and shells use for "you invoked this wrongly".
	ExitUsage = 2
	// ExitInterrupted is the conventional shell code for a process ended by
	// SIGINT.
	ExitInterrupted = 130
)

// Signaled is the cancellation cause of a context canceled by a signal. It
// records which one, so the process can exit 128+n as a shell reports a
// process ended by that signal: 130 for SIGINT, 143 for SIGTERM, 129 for
// SIGHUP.
type Signaled struct {
	// Number is the signal's number (2 for SIGINT, 15 for SIGTERM).
	Number int
	// Name is the signal's name, for the cause's text.
	Name string
}

func (s *Signaled) Error() string { return "interrupted by " + s.Name }

// SignalExitCode is the shell's status for a process ended by signal n.
func SignalExitCode(n int) int { return signalExitBase + n }

// signalExitBase is what a shell adds to a signal's number for the status of a
// process that signal ended.
const signalExitBase = 128

// ExitCode is the process exit status for the error Execute returned.
//
// Interrupted is keyed on the context alone, not on the error's identity. A
// canceled run rarely surfaces context.Canceled: Ctrl-C reaches the whole
// foreground process group, so what usually comes back is an exec error from
// `docker compose` dying. If the context is done, the error is a consequence
// of that.
//
// A run ends 0, and reports nothing, when the error the command returned is
// itself a failed write to a stdout whose reader has gone (`| head -1`:
// ClosedStdout), and the context was not canceled: the reader has what it
// wanted. Every other error is reported and exits as it normally would,
// whether or not stdout is closed: a canceled context, an API error, an
// unknown outcome, a setup failure. The command's own code (ExitError) and an
// interrupt (128+n) are never replaced.
func ExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}
	// A prompt reads Ctrl-C from a raw terminal as a keypress, not a signal,
	// so it cancels nothing; it says so with input.ErrInterrupted instead.
	if ctx.Err() != nil {
		var sig *Signaled
		if errors.As(context.Cause(ctx), &sig) && sig.Number > 0 {
			return SignalExitCode(sig.Number)
		}
		return ExitInterrupted
	}
	if errors.Is(err, input.ErrInterrupted) {
		return ExitInterrupted
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	if IsUsage(err) {
		return ExitUsage
	}
	// The reader of stdout went away (`| head -1`): the command's work was
	// done or was only a read, and it is the write that failed.
	if ClosedStdout(err) {
		return 0
	}
	return ExitFailure
}

// Execute runs root against args and reports a failure by the output contract,
// once, for every command in the tree:
//
//   - a command whose --output resolves to json publishes one ErrorObject on
//     stdout and nothing on stderr;
//   - any other command prints "Error: ..." on stderr, and for a usage error
//     a pointer to the failing command's help, with the flag it likely meant
//     when one is clearly close (UsageNoHelp drops the pointer: see the
//     text-mode branch below). Never the usage block
//     itself: cobra's automatic usage is off for every command;
//   - a write to a stdout whose reader has gone (`| head -1`) reports nothing
//     and exits 0 (ExitCode says when it does not);
//   - a command that already reported its failure — an ExitError, or an error
//     marked JSONShown — gets nothing added in either mode.
//
// It returns the command's error; ExitCode turns that into the exit status.
//
// The reporting is taken over from cobra rather than layered on each command's
// RunE because a RunE wrapper sees only RunE's errors. A flag that does not
// parse, a wrong argument count, a pre-run that finds no login and a missing
// required flag all fail outside it, and under `-o json` each of those used to
// reach stderr as prose.
//
// It also answers a group that cannot run, which cobra would answer with its
// help and success: bare under json, and given an argument that names no
// subcommand in either mode, it is a usage error (bareGroups). A group with a
// RunE of its own gets the same answer by returning GroupHelp from it. See
// group.go.
func Execute(ctx context.Context, root *cobra.Command, args []string, stdout io.Writer, kinds Kinds) error {
	markUsageErrors(root)
	quietErrors, quietUsage := root.SilenceErrors, root.SilenceUsage
	root.SilenceErrors, root.SilenceUsage = true, true
	defer func() { root.SilenceErrors, root.SilenceUsage = quietErrors, quietUsage }()

	defer input.SetGuard(refuseToAsk(root, args))()
	// What answers a shared picker is per command and per run.
	input.ClearHints()
	defer input.ClearHints()
	// The run's first Ctrl-C cancels ctx; a question waiting for a line ends
	// at once on it, rather than after a second Ctrl-C or an Enter.
	defer input.SetInterrupt(ctx)()

	refusedBareGroup, restoreHelp := bareGroups(root, findTarget(root, args))
	defer restoreHelp()

	ResetStream()
	ResetWarnOnce()
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		err = refusedBareGroup(cmd)
	}
	if err == nil {
		return nil
	}
	if cmd == nil {
		cmd = root
	}

	var exit *ExitError
	if errors.As(err, &exit) || errors.As(err, new(jsonShown)) {
		return err
	}
	// Nowhere to report to: stdout is closed, and a message on stderr would
	// be noise for a reader that stopped listening on purpose. ExitCode
	// makes it 0.
	if ctx.Err() == nil && ClosedStdout(err) {
		return err
	}
	if wantsJSON(cmd, args, err) || unknownCommandAsksJSON(cmd, args, err) {
		EmitError(stdout, err, ExitCode(ctx, err), kinds.Of(err))
		return err
	}

	// Text mode: "Error: ..." on stderr, in red, as Warning: is yellow
	// (ErrorLabel); a command that set its own ErrPrefix keeps it. A root
	// that silenced errors silences them everywhere; otherwise the failing
	// command's own setting decides.
	if quietErrors || (cmd != root && cmd.SilenceErrors) {
		return err
	}
	root.PrintErrln(errorPrefixFor(cmd.ErrPrefix(), root.ErrOrStderr()), errorText(root.ErrOrStderr(), err))
	// A failure that is not a usage error is the whole message: the command
	// was invoked rightly, so its usage would only bury the error. A usage
	// error points at the failing command's help rather than printing it, with
	// the flag that was likely meant when one is clearly close. Only a run
	// that exits as a usage error gets it: an interrupt ends the run with 130
	// whatever the error says, and nothing about how it was invoked needs
	// fixing.
	//
	// A root that silenced its usage silences the pointer everywhere (its
	// setting from before the run, quietUsage, since the run forces it on).
	// Below the root, a command's own SilenceUsage decides nothing here: it
	// only keeps cobra's block out, which the root's forced setting already
	// does. One found before the command ran always gets the pointer, since
	// its help is where the right invocation is. One the command returned
	// itself does too, unless it says its help would not settle it with
	// UsageNoHelp (a removed command's stub, a Deployment name several
	// Deployments share).
	if quietUsage || (omitsHelpPointer(err) && !isMisinvoked(err)) || ExitCode(ctx, err) != ExitUsage {
		return err
	}
	if flag := suggestFlag(cmd, err); flag != "" {
		root.PrintErrf("Did you mean %s?\n", flag)
	}
	pointAt := cmd
	var u *usageError
	if errors.As(err, &u) && u.pointAt != nil {
		pointAt = u.pointAt
	}
	root.PrintErrf("Run %s --help for usage.\n", pointAt.CommandPath())
	return err
}

// refuseToAsk is the prompt guard for one run, and the one place its
// interaction mode is resolved (input.Refusal): a run asks nothing under
// --output json, with stdin or stderr not a terminal, with CI set (or Jenkins
// or Azure Pipelines saying so), or with --non-interactive (pkg/input.SetGuard).
// A question with the run blocked on stdin is not a result a program can
// parse, and an agent, a script or a CI job driving the CLI has nobody to
// answer it — it hangs. The answer has to come with the invocation instead, and
// the refusal says which flag carries it.
//
// The command is found the way cobra will find it, before it runs, and its
// flags read only when a question comes up: by then cobra has parsed them, so
// the guard answers from the values the command itself sees, with no reading
// of raw arguments. A line that resolves to no command still gets the guard,
// for the terminals' and the environment's part of the rule.
func refuseToAsk(root *cobra.Command, args []string) func() string {
	cmd := findTarget(root, args)
	return func() string {
		if cmd == nil {
			return input.Refusal(false, nil)
		}
		f := cmd.Flag("output")
		return input.Refusal(f != nil && f.Value.String() == string(FormatJSON), cmd.Flag(input.NonInteractiveFlag))
	}
}

// findTarget is the command args resolve to, found the way cobra will find
// it, or nil when they resolve to none.
func findTarget(root *cobra.Command, args []string) *cobra.Command {
	find := root.Find
	if root.TraverseChildren {
		find = root.Traverse
	}
	cmd, _, err := find(args)
	if err != nil {
		return nil
	}
	return cmd
}

// markUsageErrors makes the usage errors cobra produces through a hook
// recognizable as such: a flag that does not parse (the flag error func, which
// every command inherits from the root) and an argument-count validator.
//
// A flag error func the root already has still runs, first: the CLI's root
// records a flag it does not have there (cmd.trackUnknownFlag). As in
// AddOutputFlag, a func that returns nil does not turn the error into success.
func markUsageErrors(root *cobra.Command) {
	own := root.FlagErrorFunc()
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if e := own(c, err); e != nil {
			err = e
		}
		return misinvoked(err)
	})
	walk(root, func(c *cobra.Command) {
		// Not a usage error, but this is the one walk over the built tree.
		nameRemovedAlias(c)
		validate := c.Args
		if validate == nil {
			if !c.Runnable() {
				return
			}
			// Cobra's own default accepts any words after a command that
			// runs: a leaf takes none, and a group refuses a word that is
			// none of its subcommands (UndeclaredArgs).
			validate = UndeclaredArgs
		}
		c.Args = func(cmd *cobra.Command, args []string) error {
			return misinvoked(validate(cmd, args))
		}
	})
}

func walk(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		walk(sub, fn)
	}
}

// wantsJSON reports whether the failed command was asked for json.
//
// The parsed flag decides when there is one. A usage error can stop flag
// parsing before --output is reached (`--bogus -o json`), so then the
// arguments are read for it directly — only when the command really has the
// flag, so an argument that merely looks like one cannot switch a command
// with no json mode into it.
func wantsJSON(cmd *cobra.Command, args []string, err error) bool {
	f := cmd.Flag("output")
	if f == nil {
		return false
	}
	if f.Changed || !IsUsage(err) {
		return f.Value.String() == string(FormatJSON)
	}
	return ArgsAskForJSON(args, f.Shorthand)
}

// unknownCommandAsksJSON is wantsJSON for a word that names no command at the
// root, where cobra stops before any command with an --output flag is found
// (the root has none), so there is no parsed flag to read: `astro bogus -o
// json` reads the arguments for it, as a usage error that stops flag parsing
// does.
func unknownCommandAsksJSON(cmd *cobra.Command, args []string, err error) bool {
	if cmd.Flag("output") != nil || !IsUsage(err) || !strings.HasPrefix(err.Error(), "unknown command ") {
		return false
	}
	return ArgsAskForJSON(args, "o")
}

// ArgsAskForJSON finds --output json in raw arguments, in each spelling pflag
// accepts, stopping at the "--" that ends flags. A command with flag parsing
// off (a removed command's stub) reads its --output with it, so it agrees
// with Execute on every spelling.
func ArgsAskForJSON(args []string, shorthand string) bool {
	long := "--output"
	short := ""
	if shorthand != "" {
		short = "-" + shorthand
	}
	for i, a := range args {
		if a == "--" {
			return false
		}
		value, ok := "", false
		switch {
		case a == long || (short != "" && a == short):
			if i+1 < len(args) {
				value, ok = args[i+1], true
			}
		case strings.HasPrefix(a, long+"="):
			value, ok = strings.TrimPrefix(a, long+"="), true
		case short != "" && strings.HasPrefix(a, short+"="):
			value, ok = strings.TrimPrefix(a, short+"="), true
		case short != "" && strings.HasPrefix(a, short) && !strings.HasPrefix(a, "--"):
			value, ok = strings.TrimPrefix(a, short), true
		}
		if ok && value == string(FormatJSON) {
			return true
		}
	}
	return false
}

// stderrIsTerminal reports whether w is a terminal. A variable so a test can
// stand one in for a buffer.
var stderrIsTerminal = IsTerminal

// errorText is err's message as the text-mode error line prints it to w. An
// error can carry what a server sent (an API's message, a proxy's page), and
// to a terminal it goes without the escapes and other control characters the
// terminal would act on (input.Lines), its line breaks kept. To a file or a
// pipe it goes as it is.
func errorText(w io.Writer, err error) string {
	if stderrIsTerminal(w) {
		return input.Lines(err.Error())
	}
	return err.Error()
}
