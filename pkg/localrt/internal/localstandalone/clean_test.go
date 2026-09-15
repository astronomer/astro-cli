//go:build !windows

package localstandalone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// write creates a file and every directory above it.
func write(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
}

func TestCleanRemovesTheStateAStandaloneRunDerived(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	project := t.TempDir()
	airflowHome := filepath.Join(project, airflowrt.StandaloneDir)
	venv := filepath.Join(project, ".venv")
	write(t, filepath.Join(airflowHome, "airflow.db"))
	write(t, filepath.Join(venv, "bin", "python"))
	write(t, filepath.Join(project, "dags", "hello.py"))

	stateDir, err := rt.StateDir(project)
	require.NoError(t, err)
	write(t, filepath.Join(stateDir, logFileName))
	write(t, filepath.Join(stateDir, jwtSecretFile))

	require.NoError(t, (&Engine{}).Clean(project))

	assert.NoDirExists(t, airflowHome, "AIRFLOW_HOME holds the metadata database reset is for")
	assert.NoDirExists(t, venv, "the venv is the environment a standalone run derived")
	assert.NoFileExists(t, filepath.Join(stateDir, logFileName))
	assert.NoFileExists(t, filepath.Join(stateDir, jwtSecretFile))
	assert.FileExists(t, filepath.Join(project, "dags", "hello.py"), "the project's own files are untouched")
}

// The gate. AIRFLOW_HOME is what says standalone ever ran here, and without it
// the venv belongs to somebody else: `astro local check` parses DAGs with the
// project's own .venv interpreter whatever mode Airflow runs in, so wiping it
// for a docker-mode project breaks a command that has nothing to do with
// standalone — and the docker half of reset never touches the venv at all.
func TestCleanLeavesTheVenvOfAProjectThatNeverRanStandalone(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	project := t.TempDir()
	venv := filepath.Join(project, ".venv")
	write(t, filepath.Join(venv, "bin", "python"))

	require.NoError(t, (&Engine{}).Clean(project), "nothing to clean is not an error")
	assert.DirExists(t, venv, "no standalone evidence, so the venv is not standalone's to delete")
}

// Repeat runs are fine: the second finds no evidence and does nothing.
func TestCleanIsIdempotent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	project := t.TempDir()
	write(t, filepath.Join(project, airflowrt.StandaloneDir, "airflow.db"))

	require.NoError(t, (&Engine{}).Clean(project))
	require.NoError(t, (&Engine{}).Clean(project))
}
