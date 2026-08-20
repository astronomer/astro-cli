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
		werr = os.Rename(tmpPath, path)
	}
	if werr != nil {
		_ = os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup; the write error below is what we return
		return fmt.Errorf("writing %s: %w", path, werr)
	}
	return nil
}
