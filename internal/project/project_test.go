package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const windowsOS = "windows"

func writeMarker(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, Marker), []byte("[project]\nname = \"demo\"\n"), 0o600))
}

func TestDiscoverWalksUpFromNestedDir(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "my-project")
	nested := filepath.Join(proj, "dags", "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeMarker(t, proj)

	p, err := Discover(nested)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
	assert.Equal(t, "my-project.localhost", p.Hostname)
	assert.False(t, p.IsWorktree)
	assert.Len(t, p.ID, 64)
}

func TestDiscoverFindsMarkerInStartDir(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	writeMarker(t, proj)

	p, err := Discover(proj)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
}

func TestDiscoverPrefersNearestMarker(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "outer")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	writeMarker(t, outer)
	writeMarker(t, inner)

	p, err := Discover(inner)
	require.NoError(t, err)
	assert.Equal(t, inner, p.Dir)
}

func TestDiscoverNotFound(t *testing.T) {
	dir := t.TempDir()

	p, err := Discover(dir)
	assert.Nil(t, p)
	var nf *NotFoundError
	require.ErrorAs(t, err, &nf)
	assert.Contains(t, nf.Error(), Marker)
}

func TestDiscoverIgnoresMarkerDirectory(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	nested := filepath.Join(proj, "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	writeMarker(t, proj)
	// A directory named pyproject.toml is not a manifest.
	require.NoError(t, os.MkdirAll(filepath.Join(nested, Marker), 0o755))

	p, err := Discover(nested)
	require.NoError(t, err)
	assert.Equal(t, proj, p.Dir)
}

func TestIDSymlinkedPathsHashIdentically(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlinks need extra privileges on windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "real-project")
	require.NoError(t, os.MkdirAll(target, 0o755))
	writeMarker(t, target)
	link := filepath.Join(root, "link-project")
	require.NoError(t, os.Symlink(target, link))

	realID, err := ID(target)
	require.NoError(t, err)
	linkID, err := ID(link)
	require.NoError(t, err)
	assert.Equal(t, realID, linkID)

	p, err := Discover(link)
	require.NoError(t, err)
	assert.Equal(t, link, p.Dir)
	assert.Equal(t, realID, p.ID)
}

func TestIDDiffersPerDirectory(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	require.NoError(t, os.MkdirAll(a, 0o755))
	require.NoError(t, os.MkdirAll(b, 0o755))

	aID, err := ID(a)
	require.NoError(t, err)
	bID, err := ID(b)
	require.NoError(t, err)
	assert.NotEqual(t, aID, bID)
}

func TestDiscoverDetectsWorktree(t *testing.T) {
	root := t.TempDir()
	// Layout of a linked worktree: the main repo holds
	// .git/worktrees/<name>, and the worktree's .git is a file pointing
	// there.
	repo := filepath.Join(root, "main-repo")
	gitdir := filepath.Join(repo, ".git", "worktrees", "feature-x")
	require.NoError(t, os.MkdirAll(gitdir, 0o755))
	wt := filepath.Join(root, "feature-x")
	require.NoError(t, os.MkdirAll(wt, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600))
	writeMarker(t, wt)

	p, err := Discover(wt)
	require.NoError(t, err)
	assert.True(t, p.IsWorktree)
	assert.Equal(t, "feature-x.main-repo.localhost", p.Hostname)

	wtID, err := ID(wt)
	require.NoError(t, err)
	assert.Equal(t, wtID, p.ID)
	repoID, err := ID(repo)
	require.NoError(t, err)
	assert.NotEqual(t, repoID, p.ID)
}

func TestDiscoverRegularRepoIsNotWorktree(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(filepath.Join(proj, ".git"), 0o755))
	writeMarker(t, proj)

	p, err := Discover(proj)
	require.NoError(t, err)
	assert.False(t, p.IsWorktree)
	assert.Equal(t, "proj.localhost", p.Hostname)
}

func TestIDMissingDirectory(t *testing.T) {
	_, err := ID(filepath.Join(t.TempDir(), "gone"))
	assert.Error(t, err)
}

func TestNotFoundErrorMentionsStart(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover(dir)
	var nf *NotFoundError
	require.True(t, errors.As(err, &nf))
	assert.Contains(t, nf.Start, filepath.Base(dir))
}
