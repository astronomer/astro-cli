//go:build !windows

package cliout

import (
	"errors"
	"syscall"
)

func isBrokenPipe(err error) bool { return errors.Is(err, syscall.EPIPE) }
