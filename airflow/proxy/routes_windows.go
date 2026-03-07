//go:build windows

package proxy

import (
	"fmt"
	"os"
)

// acquireLock on Windows opens the lock file without flock.
// The proxy daemon does not run on Windows, so file locking is best-effort.
func acquireLock() (*os.File, error) {
	if err := os.MkdirAll(proxyDirPath(), dirPermRWX); err != nil {
		return nil, fmt.Errorf("error creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(lockFilePath(), os.O_CREATE|os.O_RDWR, filePermRW)
	if err != nil {
		return nil, fmt.Errorf("error opening lock file: %w", err)
	}
	return f, nil
}

// releaseLock closes the lock file.
func releaseLock(f *os.File) {
	if f != nil {
		f.Close()
	}
}

// isPIDAlive on Windows always returns false.
// The proxy daemon is not supported on Windows.
var isPIDAlive = func(pid int) bool {
	return false
}
