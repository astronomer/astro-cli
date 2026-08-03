package cosmosboost

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// isolateHome points config.HomeConfigPath at a temp dir so retired-helper
// removal is exercised against a throwaway ~/.astro.
func isolateHome(t *testing.T) string {
	t.Helper()
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	orig := config.HomeConfigPath
	home := t.TempDir()
	config.HomeConfigPath = home
	t.Cleanup(func() { config.HomeConfigPath = orig })
	return home
}

func TestCleanupDefaultRootRemovesArtifact(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))
	t.Chdir(dir)

	require.NoError(t, Cleanup())

	_, err := os.Stat(filepath.Join(dir, artifactRelPath))
	require.True(t, os.IsNotExist(err), "artifact under the default root must be removed")
}

func TestCleanupRemovesRetiredHelper(t *testing.T) {
	home := isolateHome(t)
	binDir := filepath.Join(home, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	name := "astro-cosmos-boost"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helper := filepath.Join(binDir, name)
	require.NoError(t, os.WriteFile(helper, []byte("retired"), 0o755))
	leftover := filepath.Join(binDir, ".cosmosboost-12345")
	require.NoError(t, os.WriteFile(leftover, []byte("partial"), 0o644))

	require.NoError(t, Cleanup(t.TempDir()))

	_, err := os.Stat(helper)
	require.True(t, os.IsNotExist(err), "the retired standalone helper must be removed")
	_, err = os.Stat(leftover)
	require.True(t, os.IsNotExist(err), "partial extracts from interrupted installs must be removed")
}

func TestCleanupNothingToRemove(t *testing.T) {
	isolateHome(t)
	require.NoError(t, Cleanup(t.TempDir()), "an already-clean machine is not an error")
}

func TestCleanupNonexistentRoot(t *testing.T) {
	isolateHome(t)
	require.Error(t, Cleanup(filepath.Join(t.TempDir(), "does-not-exist")))
}
