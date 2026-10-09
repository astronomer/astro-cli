package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))

	assert.True(t, SamePath(dir, dir))
	assert.True(t, SamePath(dir, dir+string(filepath.Separator)+"."), "the same directory spelled differently")
	assert.True(t, SamePath(link, dir), "a symlink names what it points at")
	assert.False(t, SamePath(dir, other))
	assert.False(t, SamePath("", dir), "an empty path names nothing, not the working directory")
	assert.False(t, SamePath(dir, ""))

	// Neither exists: the paths themselves are compared.
	missing := filepath.Join(dir, "missing")
	assert.True(t, SamePath(missing, filepath.Join(dir, ".", "missing")))
	assert.False(t, SamePath(missing, filepath.Join(other, "missing")))
}

func TestNearestDir(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "outer")
	mid := filepath.Join(outer, "a")
	inner := filepath.Join(mid, "b")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	is := func(dirs ...string) func(string) (bool, error) {
		return func(d string) (bool, error) {
			for _, want := range dirs {
				if d == want {
					return true, nil
				}
			}
			return false, nil
		}
	}

	got, err := NearestDir(inner, nil, is(outer))
	require.NoError(t, err)
	assert.Equal(t, outer, got, "the nearest match, however deep")

	got, err = NearestDir(outer, nil, is(outer))
	require.NoError(t, err)
	assert.Equal(t, outer, got, "the start directory itself counts")

	got, err = NearestDir(inner, nil, is(outer, mid))
	require.NoError(t, err)
	assert.Equal(t, mid, got, "the nearer of two")

	got, err = NearestDir(inner, func(d string) bool { return d == mid }, is(outer, mid))
	require.NoError(t, err)
	assert.Equal(t, outer, got, "a skipped directory is passed over, and the walk goes on above it")

	got, err = NearestDir(inner, nil, is())
	require.NoError(t, err)
	assert.Empty(t, got)

	// The filesystem root is never asked.
	var asked []string
	_, err = NearestDir(inner, nil, func(d string) (bool, error) {
		asked = append(asked, d)
		return false, nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, asked)
	last := asked[len(asked)-1]
	assert.NotEqual(t, filepath.Dir(last), last, "the root was asked: %v", asked)

	boom := errors.New("boom")
	_, err = NearestDir(inner, nil, func(string) (bool, error) { return false, boom })
	assert.ErrorIs(t, err, boom)
}

// A directory match cannot read is no match, and the walk goes on above it.
func TestNearestReadableDir(t *testing.T) {
	root := t.TempDir()
	outer := filepath.Join(root, "outer")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	boom := errors.New("boom")
	match := func(d string) (bool, error) {
		switch d {
		case inner:
			return true, boom // unreadable, whatever it says
		case outer:
			return true, nil
		}
		return false, nil
	}
	assert.Equal(t, outer, NearestReadableDir(inner, nil, match))
	assert.Empty(t, NearestReadableDir(inner, func(d string) bool { return d == outer }, match))
}

// SamePathAs answers as SamePath does, for a b read once.
func TestSamePathAs(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))
	isDir := SamePathAs(dir)
	assert.True(t, isDir(link))
	assert.True(t, isDir(dir))
	assert.False(t, isDir(t.TempDir()))
	assert.False(t, isDir(""))
	assert.False(t, SamePathAs("")(dir))

	missing := filepath.Join(dir, "missing")
	assert.True(t, SamePathAs(missing)(filepath.Join(dir, ".", "missing")), "a b that cannot be read is compared by path")
}
