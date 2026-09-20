//go:build windows

package localstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// lockFile is the per-project start lock, next to the runtime record.
const lockFile = "start.lock"

// ErrLocked reports that another start for the same project holds the lock.
var ErrLocked = errors.New("another operation is already running for this project")

// Lock on Windows opens the lock file without flock, matching pkg/proxy's
// Windows lock: file locking is best-effort here because the MVP runs local
// Airflow on Windows in docker mode, where concurrent starts are rare.
func Lock(projectPath string) (release func(), err error) {
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, lockFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return func() { f.Close() }, nil
}
