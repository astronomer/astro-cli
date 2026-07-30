package userstate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/project"
)

const windowsOS = "windows"

// setCache points the cache root at a fresh temp dir and returns it.
func setCache(t *testing.T) string {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	return cache
}

func TestCacheRootXDGOverride(t *testing.T) {
	cache := setCache(t)
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cache, "astro"), root)
}

func TestCacheRootIgnoresRelativeXDG(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("XDG fallback path is unix-shaped")
	}
	t.Setenv("XDG_CACHE_HOME", "relative/cache")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".cache", "astro"), root)
}

func TestCacheRootDefaultsToHome(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("XDG fallback path is unix-shaped")
	}
	t.Setenv("XDG_CACHE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := CacheRoot()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".cache", "astro"), root)
}

func TestDirKeyedByProjectID(t *testing.T) {
	cache := setCache(t)
	proj := t.TempDir()

	dir, err := Dir(proj)
	require.NoError(t, err)
	id, err := project.ID(proj)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cache, "astro", "projects", id), dir)
}

func TestLoadMissingFileIsEmptyState(t *testing.T) {
	setCache(t)
	proj := t.TempDir()

	s, err := Load(proj)
	require.NoError(t, err)
	assert.Equal(t, State{}, s)
}

func TestSaveThenLoadRoundtrip(t *testing.T) {
	setCache(t)
	proj := t.TempDir()
	want := State{Instance: "prod", Port: 8080, DevMode: true}

	require.NoError(t, Save(proj, want))
	got, err := Load(proj)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// The write is atomic: no temp files remain next to the state file.
	dir, err := Dir(proj)
	require.NoError(t, err)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state.json", entries[0].Name())
}

func TestSaveReplacesExistingState(t *testing.T) {
	setCache(t)
	proj := t.TempDir()

	require.NoError(t, Save(proj, State{Instance: "old"}))
	require.NoError(t, Save(proj, State{Instance: "new", Port: 9090}))

	got, err := Load(proj)
	require.NoError(t, err)
	assert.Equal(t, State{Instance: "new", Port: 9090}, got)
}

func TestLoadHealsNonCanonicalFile(t *testing.T) {
	setCache(t)
	proj := t.TempDir()
	dir, err := Dir(proj)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "state.json")

	// Older schema: the pin under its old name, an unknown field, no trailing
	// newline, odd formatting.
	stale := `{"deployment":"dep-1","legacyField":true,"port":-4}`
	require.NoError(t, os.WriteFile(path, []byte(stale), 0o600))

	s, err := Load(proj)
	require.NoError(t, err)
	assert.Equal(t, State{Instance: "dep-1"}, s) // negative port healed to zero

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "legacyField")
	assert.JSONEq(t, `{"instance":"dep-1"}`, string(raw))

	// A second load finds the file already canonical and leaves it alone.
	info1, err := os.Stat(path)
	require.NoError(t, err)
	_, err = Load(proj)
	require.NoError(t, err)
	info2, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime())
}

func TestLoadPrefersInstanceOverOldDeploymentKey(t *testing.T) {
	setCache(t)
	proj := t.TempDir()
	dir, err := Dir(proj)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"instance":"prod","deployment":"stale"}`), 0o600))

	s, err := Load(proj)
	require.NoError(t, err)
	assert.Equal(t, State{Instance: "prod"}, s)
}

func TestLoadCorruptFile(t *testing.T) {
	setCache(t)
	proj := t.TempDir()
	dir, err := Dir(proj)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte("{not json"), 0o600))

	_, err = Load(proj)
	var de *DecodeError
	require.ErrorAs(t, err, &de)
	assert.Contains(t, de.Path, "state.json")
}

func TestSymlinkedProjectSharesState(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlinks need extra privileges on windows")
	}
	setCache(t)
	root := t.TempDir()
	target := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(target, 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, Save(target, State{Instance: "shared"}))
	got, err := Load(link)
	require.NoError(t, err)
	assert.Equal(t, "shared", got.Instance)
}
