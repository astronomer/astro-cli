//go:build windows

package cliout

import (
	"errors"
	"syscall"
)

// What a write to a pipe whose reader has closed returns on Windows:
// ERROR_NO_DATA ("the pipe is being closed") for an anonymous pipe, and
// ERROR_BROKEN_PIPE once it is gone.
const (
	errorBrokenPipe syscall.Errno = 109
	errorNoData     syscall.Errno = 232
)

func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, errorBrokenPipe) || errors.Is(err, errorNoData)
}
