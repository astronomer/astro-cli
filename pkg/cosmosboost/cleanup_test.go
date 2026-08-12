package cosmosboost

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanupDefaultRootRemovesArtifact(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir, true))
	t.Chdir(dir)

	require.NoError(t, Cleanup())

	_, err := os.Stat(filepath.Join(dir, artifactRelPath))
	require.True(t, os.IsNotExist(err), "artifact under the default root must be removed")
}

func TestCleanupNothingToRemove(t *testing.T) {
	require.NoError(t, Cleanup(t.TempDir()), "an already-clean tree is not an error")
}

func TestCleanupNonexistentRoot(t *testing.T) {
	require.Error(t, Cleanup(filepath.Join(t.TempDir(), "does-not-exist")))
}

// TestCleanupTouchesOnlyTheRequestedPaths pins the command's scope: cleanup
// acts on the paths it was given and nothing else on the machine.
func TestCleanupTouchesOnlyTheRequestedPaths(t *testing.T) {
	requested := t.TempDir()
	writeDbtProject(t, requested)
	require.NoError(t, PreDeploy(requested, true))

	elsewhere := t.TempDir()
	writeDbtProject(t, elsewhere)
	require.NoError(t, PreDeploy(elsewhere, true))

	require.NoError(t, Cleanup(requested))

	_, err := os.Stat(filepath.Join(requested, artifactRelPath))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(elsewhere, artifactRelPath),
		"a path that was not requested must not be touched")
}
