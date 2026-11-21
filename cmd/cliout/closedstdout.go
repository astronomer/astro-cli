package cliout

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// ErrClosedStdout is the cancellation cause of a run whose stdout reader went
// away (`astro deployment list | head -1`, SIGPIPE): main cancels the run's
// context with it, so the waits, polls, pages and uploads that watch the
// context stop, as they do for an interrupt. It is distinct from Signaled, so
// the exit status is not 128+n.
var ErrClosedStdout = errors.New("stdout was closed by its reader")

// ClosedStdout reports that err is a failed write to the process's stdout
// because its reader has gone: a *fs.PathError for a write to os.Stdout whose
// cause is a broken pipe (`astro config list | head -1` once head has its
// line). It is also what Windows reports, which has no SIGPIPE.
//
// Any other broken pipe is not stdout's: a child's stdin, a socket. Only a run
// already canceled with ErrClosedStdout, which main does once SIGPIPE has
// confirmed that fd 1 is the broken one (Go raises it for fd 1 and 2 alone),
// counts those as stdout's too (closedStdoutRun).
func ClosedStdout(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe) && pe.Op == "write" && pe.Path == os.Stdout.Name() && isBrokenPipe(pe.Err)
}

// closedStdoutRun reports that the run ended because its stdout reader went
// away, and that nothing could have been affected by it: the command only read
// or listed, or finished its work before the write failed. It is false for an
// interrupt (which keeps its own status, whatever else failed) and for an
// error that carries an unknown outcome: a write was in flight when the
// reader left, so the user must see that error and its exit status.
func closedStdoutRun(ctx context.Context, err error) bool {
	closed := ClosedStdout(err)
	if ctx.Err() != nil {
		closed = errors.Is(context.Cause(ctx), ErrClosedStdout)
	}
	return closed && !httputil.OutcomeUnknown(err)
}

// EndedByClosedStdout is closedstdout's verdict for callers outside the
// package that must not count such a run as a failure (the setup-failure
// event): see ExitCode.
func EndedByClosedStdout(ctx context.Context, err error) bool { return closedStdoutRun(ctx, err) }
