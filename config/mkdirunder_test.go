package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Whoever creates the home config directory first, it is owner-only; a
// directory elsewhere is made as asked.
func TestMkdirAllUnderMakesTheHomeConfigDirOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX mode bits")
	}
	orig := HomeConfigPath
	t.Cleanup(func() { HomeConfigPath = orig })

	base := t.TempDir()
	HomeConfigPath = filepath.Join(base, "home", ".astro")
	require.NoError(t, MkdirAllUnder(filepath.Join(HomeConfigPath, "otto", "logs"), 0o755))
	for dir, want := range map[string]os.FileMode{HomeConfigPath: 0o700, filepath.Join(HomeConfigPath, "otto"): 0o755} {
		fi, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, want, fi.Mode().Perm(), dir)
	}

	HomeConfigPath = filepath.Join(base, "other", ".astro")
	require.NoError(t, MkdirAllUnder(HomeConfigPath, 0o755))
	fi, err := os.Stat(HomeConfigPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm(), "the home dir itself")

	elsewhere := filepath.Join(base, "elsewhere")
	require.NoError(t, MkdirAllUnder(elsewhere, 0o755))
	fi, err = os.Stat(elsewhere)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm())

	HomeConfigPath = ""
	require.NoError(t, MkdirAllUnder(filepath.Join(base, "nohome"), 0o755))
}
