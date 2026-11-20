package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveDarwinForkSafetyPatchUndoesTheWrite(t *testing.T) {
	t.Parallel()
	venv := t.TempDir()
	sitePackages := filepath.Join(venv, "lib", "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))
	other := filepath.Join(sitePackages, "airflow.pth")
	require.NoError(t, os.WriteFile(other, []byte("x"), 0o644))

	require.NoError(t, WriteDarwinForkSafetyPatch(venv))
	require.FileExists(t, filepath.Join(sitePackages, "_fix_setproctitle.pth"))
	require.FileExists(t, filepath.Join(sitePackages, "_fix_setproctitle.py"))

	require.NoError(t, RemoveDarwinForkSafetyPatch(venv))
	assert.NoFileExists(t, filepath.Join(sitePackages, "_fix_setproctitle.pth"))
	assert.NoFileExists(t, filepath.Join(sitePackages, "_fix_setproctitle.py"))
	assert.FileExists(t, other, "only the patch's own files are removed")
}

func TestRemoveDarwinForkSafetyPatchWithNothingToRemove(t *testing.T) {
	t.Parallel()
	venv := t.TempDir()
	assert.NoError(t, RemoveDarwinForkSafetyPatch(venv), "no lib dir")

	require.NoError(t, os.MkdirAll(filepath.Join(venv, "lib", "python3.12", "site-packages"), 0o755))
	assert.NoError(t, RemoveDarwinForkSafetyPatch(venv), "unpatched venv")
}

func TestRemoveDarwinForkSafetyPatchKeepsFilesThatAreNotOurs(t *testing.T) {
	t.Parallel()
	venv := t.TempDir()
	sitePackages := filepath.Join(venv, "lib", "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))
	pth := filepath.Join(sitePackages, "_fix_setproctitle.pth")
	py := filepath.Join(sitePackages, "_fix_setproctitle.py")
	require.NoError(t, os.WriteFile(pth, []byte("import somebody_elses\n"), 0o644))
	require.NoError(t, os.WriteFile(py, []byte("# somebody else's\n"), 0o644))

	require.NoError(t, RemoveDarwinForkSafetyPatch(venv))
	assert.FileExists(t, pth)
	assert.FileExists(t, py)
}

// The patch's files are written as files of their own: a symlink already at
// either name (a dangling one included, which the .pth sentinel check does not
// see) is replaced, and what it pointed at is left alone.
func TestWriteDarwinForkSafetyPatchReplacesSymlinks(t *testing.T) {
	t.Parallel()
	venv := t.TempDir()
	sitePackages := filepath.Join(venv, "lib", "python3.12", "site-packages")
	require.NoError(t, os.MkdirAll(sitePackages, 0o755))
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep.py")
	require.NoError(t, os.WriteFile(keep, []byte("keep"), 0o600))
	py := filepath.Join(sitePackages, "_fix_setproctitle.py")
	pth := filepath.Join(sitePackages, "_fix_setproctitle.pth")
	if err := os.Symlink(keep, py); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	require.NoError(t, os.Symlink(filepath.Join(outside, "dangling.pth"), pth))

	require.NoError(t, WriteDarwinForkSafetyPatch(venv))

	got, err := os.ReadFile(keep)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(got), "the symlink's target was written through")
	assert.NoFileExists(t, filepath.Join(outside, "dangling.pth"), "the dangling link's target was created")
	for _, name := range []string{py, pth} {
		info, err := os.Lstat(name)
		require.NoError(t, err)
		assert.True(t, info.Mode().IsRegular(), "%s is a file of its own", filepath.Base(name))
	}
}
