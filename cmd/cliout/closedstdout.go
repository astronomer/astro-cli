package cliout

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// ErrClosedStdout is the cancellation cause of a run whose stdout reader went
// away (`astro deployment list | head -1`, SIGPIPE): main cancels the run's
// context with it, so the waits, polls, pages and uploads that watch the
// context stop, as they do for an interrupt. It is distinct from Signaled, so
// the exit status is not 128+n.
var ErrClosedStdout = errors.New("stdout was closed by its reader")

// ClosedStdout reports that err is a write to a pipe whose reader has gone: a
// failed write to the process's stdout (`astro config list | head -1` once
// head has its line), or any broken-pipe error that is not a network one.
//
// A network error is excluded: the same errno from a socket (a connection that
// broke while an upload was in flight) is a failure of the request, which
// ClosedStdout does not speak for.
func ClosedStdout(err error) bool {
	if err == nil || errors.As(err, new(*net.OpError)) {
		return false
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Op == "write" && pe.Path == os.Stdout.Name() && isBrokenPipe(pe.Err) {
		return true
	}
	// A wrapped stdout writer (a buffer, a tee) reports the errno without
	// the file's name.
	return isBrokenPipe(err)
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
