//go:build !windows

package localstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// lockFile is the per-project start lock, next to the runtime record in the
// project's state dir. Same flock mechanism as pkg/proxy's routes lock, but
// non-blocking: it is held across the whole start (venv sync, health wait), so
// a second concurrent start fails fast with ErrLocked rather than waiting.
const lockFile = "start.lock"

// ErrLocked reports that another start for the same project holds the lock.
// Its message is user-facing: a second `astro local start` returns it rather
// than racing the first and clobbering the record.
var ErrLocked = errors.New("another start is already in progress for this project")

// Lock takes the per-project start lock so two concurrent starts cannot both
// write the runtime record — where the loser's dying pid would orphan the
// winner's live Airflow. It returns a release function to call when the start
// sequence finishes; ErrLocked means another start holds the lock right now.
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
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck // release is best-effort; the close drops the lock anyway
		f.Close()
	}, nil
}
