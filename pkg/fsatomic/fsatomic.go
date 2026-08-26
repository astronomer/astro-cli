// Package fsatomic writes files atomically: temp file in the target
// directory, chmod, rename. Readers never see a half-written file and a
// crash leaves the old contents intact. Shared by the v2 state writers
// (pkg/localrt's record store and internal/userstate), which is why it is a
// sub-module rather than root-module internal/. pkg/proxy still keeps its own
// inline copy.
package fsatomic

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// WriteFile writes data to path atomically with the given permissions.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		// CreateTemp creates 0o600; make the published mode explicit.
		werr = os.Chmod(tmpPath, perm)
	}
	if werr == nil {
		werr = replace(tmpPath, path)
	}
	if werr != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup; the write error below is what we return
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}

// replaceRetries and replaceBackoff bound the wait for a destination another
// process is holding open. Short and few: the contended window is one rename,
// and a caller stuck behind a genuinely locked file is better told than left
// spinning.
const (
	replaceRetries = 20
	replaceBackoff = 5 * time.Millisecond
)

// replace renames tmp onto path, retrying while Windows reports the destination
// as busy.
//
// On Unix rename(2) is atomic and never fails because someone else has the
// destination open, so this is a single call there. Windows has no such
// guarantee: MoveFileEx fails with ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION
// when another handle is open on the target, which includes the perfectly
// ordinary case of a second writer replacing the same file at the same moment,
// or a reader that has it open to read.
//
// That is not a theoretical race for these files. This package exists for state
// two processes share — the astro config, the routes file, the vault's per-key
// values — where the CLI and the desktop write concurrently by design. Without
// the retry, one of them simply fails, and pkg/secrets' own concurrency test
// proved it the first time it ran on Windows.
func replace(tmp, path string) error {
	var err error
	for i := 0; i < replaceRetries; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		if !isBusy(err) {
			return err
		}
		time.Sleep(replaceBackoff)
	}
	return err
}
