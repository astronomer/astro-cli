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
