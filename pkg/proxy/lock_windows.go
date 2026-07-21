//go:build windows

package proxy

import (
	"fmt"
	"os"
)

// AcquireLock on Windows opens the lock file without flock.
// The proxy daemon does not run on Windows, so file locking is best-effort.
func (s *Store) AcquireLock() (*os.File, error) {
	if err := os.MkdirAll(s.dir, DirPermRWX); err != nil {
		return nil, fmt.Errorf("creating proxy directory: %w", err)
	}

	f, err := os.OpenFile(s.lockFilePath(), os.O_CREATE|os.O_RDWR, FilePermRW)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	return f, nil
}

// ReleaseLock closes the lock file.
func ReleaseLock(f *os.File) {
	if f != nil {
		f.Close()
	}
}
