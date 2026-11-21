package cliout

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"sync/atomic"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// sigpipeSeen records that the process was sent SIGPIPE. Go raises it for a
// failed write to fd 1 or 2 alone, so once it is set a bare broken pipe came
// from stdout (or stderr), and not from a socket or a child's pipe.
var sigpipeSeen atomic.Bool

// NoteSIGPIPE records that the process was sent SIGPIPE (main's handler). It
// cancels nothing: the run goes on, and what ends it is the command's own
// error from the write that failed.
func NoteSIGPIPE() { sigpipeSeen.Store(true) }

// ClosedStdout reports that err, the error a command returned, is a failed
// write to a stdout whose reader has gone (`astro config list | head -1` once
// head has its line): a *fs.PathError for a write to os.Stdout whose cause is
// a broken pipe, or, once SIGPIPE has been seen, a bare broken pipe (a writer
// wrapped around stdout reports the errno without the file's name). A network
// error is never one.
//
// Only an error that is itself the failed write counts. A canceled context, an
// API error, an unknown outcome and a setup failure are the command's own
// results, whatever happened to stdout, and are reported as usual.
//
// On Windows, which has no SIGPIPE, only a direct write to os.Stdout is
// recognized: the PathError names the file, and a wrapped writer's bare error
// is not told from any other pipe's.
func ClosedStdout(err error) bool {
	if err == nil || httputil.OutcomeUnknown(err) {
		return false
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && pe.Op == "write" && pe.Path == os.Stdout.Name() && isBrokenPipe(pe.Err) {
		return true
	}
	return sigpipeSeen.Load() && !errors.As(err, new(*net.OpError)) && isBrokenPipe(err)
}
