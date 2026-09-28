//go:build !windows

package fsatomic

import (
	"errors"
	"os"
	"syscall"
)

// tryLock makes one non-blocking attempt at an exclusive flock. held is false,
// with no error, when another open file holds it.
func tryLock(f *os.File) (held bool, err error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		case errors.Is(err, syscall.EINTR):
			continue
		default:
			return false, err
		}
	}
}

// unlockFile releases the flock. The close that follows releases it anyway, so
// a failure here changes nothing a caller could act on.
func unlockFile(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck // see above
}
