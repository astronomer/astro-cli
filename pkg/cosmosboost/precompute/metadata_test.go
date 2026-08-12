package precompute

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteSidecarError covers the best-effort failure branch: when a plain file
// already occupies the .astro path, MkdirAll can't create the sidecar directory, so
// writeSidecar returns an error rather than panicking.
func TestWriteSidecarError(t *testing.T) {
	dir := t.TempDir()
	// Block the ".astro" directory with a regular file of the same name.
	if err := os.WriteFile(filepath.Join(dir, sidecarDir), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSidecar(dir, algoProjectTree, "deadbeef", "test"); err == nil {
		t.Fatal("writeSidecar should fail when .astro is a file, not a directory")
	}
}
