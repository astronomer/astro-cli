package proxydaemon

import (
	"path/filepath"
	"testing"

	"github.com/mitchellh/go-homedir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uncachedHome makes go-homedir look at the environment again on every call,
// so a test's HOME is seen, and puts its cache back afterwards.
func uncachedHome(t *testing.T) {
	t.Helper()
	orig := homedir.DisableCache
	homedir.DisableCache = true
	t.Cleanup(func() { homedir.DisableCache = orig })
}

// Dir is <ASTRO_HOME or home>/.astro/proxy, where config.HomeConfigPath puts
// the proxy directory too.
func TestDirIsUnderAstroHomeOrHome(t *testing.T) {
	uncachedHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	astroHome := t.TempDir()
	t.Setenv("ASTRO_HOME", astroHome)
	dir, err := Dir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(astroHome, ".astro", "proxy"), dir)

	t.Setenv("ASTRO_HOME", "")
	dir, err = Dir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".astro", "proxy"), dir)
}

// With $HOME empty, go-homedir asks the system (getent, dscl, the shell) for
// the user's home. Dir returns an absolute path or an error, never the
// relative .astro/proxy that would put the routes in whatever directory the
// command ran from.
func TestDirWithoutHomeIsNeverRelative(t *testing.T) {
	uncachedHome(t)
	t.Setenv("ASTRO_HOME", "")
	t.Setenv("HOME", "")

	dir, err := Dir()
	if err != nil {
		assert.Empty(t, dir)
		return
	}
	assert.True(t, filepath.IsAbs(dir), "Dir() = %q", dir)
}
