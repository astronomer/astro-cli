package precompute

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestWriteArtifactIsReadableByRuntimeUser: artifacts are written through a
// temp file, which os.CreateTemp opens at 0o600. They ship inside the deploy
// bundle or image and the Airflow runtime user has to read them, so the final
// file must carry sidecarPerm - not the temp file's private mode.
func TestWriteArtifactIsReadableByRuntimeUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file mode bits do not apply on windows")
	}
	dir := t.TempDir()
	if err := writeArtifact(dir, slimManifestName, []byte(`{"x":1}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, sidecarDir, slimManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != sidecarPerm {
		t.Fatalf("artifact mode = %v, want %v", got, os.FileMode(sidecarPerm))
	}
}

// TestWriteArtifactReplacesAtomically: the rename must leave the destination
// holding the complete new contents and no temp file behind - a leftover
// .tmp-* would keep Cleanup from ever pruning .astro.
func TestWriteArtifactReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	if err := writeArtifact(dir, slimManifestName, []byte(`{"old":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifact(dir, slimManifestName, []byte(`{"new":true}`)); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, sidecarDir)
	data, err := os.ReadFile(filepath.Join(out, slimManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"new":true}` {
		t.Fatalf("artifact contents = %q, want the replacement", data)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind in %s: %s", sidecarDir, e.Name())
		}
	}
}
