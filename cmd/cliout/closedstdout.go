package cliout

import (
	"errors"
	"io/fs"
	"os"

	"github.com/astronomer/astro-cli/pkg/httputil"
)

// ClosedStdout reports that err, the error a command returned, is a failed
// write to a stdout whose reader has gone (`astro config list | head -1` once
// head has its line): errors.As finds a *fs.PathError for a write to os.Stdout
// whose cause is a broken pipe. Nothing else counts, so a broken pipe of a
// socket, or of a child's stdin, keeps its error and its exit status.
//
// Only an error that is itself the failed write counts. A canceled context, an
// API error, an unknown outcome and a setup failure are the command's own
// results, whatever happened to stdout, and are reported as usual. A code path
// that wraps a stdout write error must wrap it with %w, so the PathError stays
// findable; the renderer's own writers (bufio, input.TerminalWriter) return it
// as it came.
//
// On Windows, which has no SIGPIPE, the same holds: only a direct write to
// os.Stdout is recognized, as its PathError names the file.
func ClosedStdout(err error) bool {
	if err == nil || httputil.OutcomeUnknown(err) {
		return false
	}
	var pe *fs.PathError
	return errors.As(err, &pe) && pe.Op == "write" && pe.Path == os.Stdout.Name() && isBrokenPipe(pe.Err)
}
