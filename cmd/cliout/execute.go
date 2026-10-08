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

// ExitCode is the process exit status for the error Execute returned.
//
// Interrupted is keyed on the context alone, not on the error's identity. A
// canceled run rarely surfaces context.Canceled: Ctrl-C reaches the whole
// foreground process group, so what usually comes back is an exec error from
// `docker compose` dying. If the context is done, the error is a consequence
// of that.
func ExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}
	if ctx.Err() != nil {
		return ExitInterrupted
	}
	var exit *ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	if IsUsage(err) {
		return ExitUsage
	}
	return ExitFailure
}

// Execute runs root against args and reports a failure by the output contract,
// once, for every command in the tree:
//
//   - a command whose --output resolves to json publishes one ErrorObject on
//     stdout and nothing on stderr;
//   - any other command prints cobra's "Error: ..." on stderr, and its usage
//     unless it silenced that, exactly as cobra would;
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

	defer input.SetGuard(refuseUnderJSON(root, args))()

	refusedBareGroup, restoreHelp := bareGroups(root, findTarget(root, args))
	defer restoreHelp()

	ResetStream()
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
	if wantsJSON(cmd, args, err) {
		EmitError(stdout, err, ExitCode(ctx, err), kinds.Of(err))
		return err
	}

	// Text mode: what cobra itself would have printed, in its order. A root
	// that silenced errors silences them everywhere; otherwise the failing
	// command's own setting decides.
	if quietErrors || (cmd != root && cmd.SilenceErrors) {
		return err
	}
	if cmd.CalledAs() == "" {
		// Find failed (an unknown command at the root): cobra prints a pointer
		// to help instead of the usage block.
		cmd.PrintErrln(cmd.ErrPrefix(), err.Error())
		cmd.PrintErrf("Run '%v --help' for usage.\n", cmd.CommandPath())
		return err
	}
	root.PrintErrln(cmd.ErrPrefix(), err.Error())
	// The root's own SilenceUsage is forced on for the run, so for the root
	// only quietUsage, its setting from before, decides.
	if !quietUsage && (cmd == root || !cmd.SilenceUsage) {
		root.Println(cmd.UsageString())
	}
	return err
}

// refuseUnderJSON is the prompt guard for one run: a command whose --output is
// json asks nothing (pkg/input.SetGuard). A question on a terminal with the
// run blocked on stdin is not a result a program can parse, and an agent or a
// script driving the CLI has nobody at the keyboard to answer it — it hangs.
// The answer has to come with the invocation instead, and the refusal says
// which flag carries it.
//
// The command is found the way cobra will find it, before it runs, and its
// flag read only when a question comes up: by then cobra has parsed it, so
// the guard answers from the value the command itself sees, with no reading
// of raw arguments.
func refuseUnderJSON(root *cobra.Command, args []string) func() string {
	cmd := findTarget(root, args)
	if cmd == nil {
		return nil
	}
	return func() string {
		f := cmd.Flag("output")
		if f == nil || f.Value.String() != string(FormatJSON) {
			return ""
		}
		return "with --output json it cannot"
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
		if IsUsage(err) {
			return err
		}
		return Usage(err)
	})
	walk(root, func(c *cobra.Command) {
		validate := c.Args
		if validate == nil {
			return
		}
		c.Args = func(cmd *cobra.Command, args []string) error {
			return Usage(validate(cmd, args))
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
	return argsAskForJSON(args, f.Shorthand)
}

// argsAskForJSON finds --output json in raw arguments, in each spelling pflag
// accepts, stopping at the "--" that ends flags.
func argsAskForJSON(args []string, shorthand string) bool {
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
