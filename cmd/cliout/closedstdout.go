package cliout

import (
	"errors"
	"io/fs"
	"os"
)

// ClosedStdout reports that err is a failed write to the process's stdout
// because the reader on the other end has gone: `astro config list | head -1`
// once head has its line. That is the reader's choice, not a failure of the
// command, so Execute reports nothing for it and ExitCode makes it 0.
//
// It is stdout only. The same errno from a socket (a connection that broke
// while an upload was in flight) is a real failure whose outcome the
// caller must see, and a *fs.PathError naming /dev/stdout is what a write to
// the os.Stdout file returns, where a network error is a *net.OpError.
func ClosedStdout(err error) bool {
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Op != "write" || pe.Path != os.Stdout.Name() {
		return false
	}
	return isBrokenPipe(pe.Err)
}
