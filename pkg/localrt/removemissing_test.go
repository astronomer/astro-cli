package localrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// No build constraint, unlike the rest of this package's tests: those need a
// real process group to judge liveness, and this needs only files. Resolving a
// path that is not there is the kind of thing that differs by platform, so it
// is worth checking where the paths differ.

// staleRecordFor writes a stopped record for a project directory under parent,
// then removes the directory — a record that outlived its project, which is
// what a deleted git worktree leaves behind.
func staleRecordFor(t *testing.T, parent string) string {
	t.Helper()
	const name = "goner"
	project := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		// No live process, so the record reads as stale.
		Port:     8080,
		Hostname: name + ".localhost",
	}))
	require.NoError(t, os.RemoveAll(project))
	return project
}

// The record that most needs removing is the one that cannot say where it
// lives.
//
// A record's state directory is keyed by the hash of the project's RESOLVED
// path, and resolving symlinks needs the directory to exist — so once the
// project is gone, deriving the record's own location fails. It is found by
// what it says about itself instead.
func TestRemoveFindsARecordByItsStoredPath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	gone := staleRecordFor(t, t.TempDir())

	before, err := localstate.List()
	require.NoError(t, err)
	require.Len(t, before, 1, "the record should exist before it is removed")

	require.NoError(t, localstate.Remove(gone))

	after, err := localstate.List()
	require.NoError(t, err)
	assert.Empty(t, after)

	// Twice is not an error, the same as for a record whose location can be
	// derived.
	require.NoError(t, localstate.Remove(gone))
}

// A path no record claims is not an error either, so a caller cleaning up
// after itself cannot be surprised by one.
func TestRemoveIsQuietAboutAPathNoRecordClaims(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	require.NoError(t, localstate.Remove(filepath.Join(t.TempDir(), "never-existed")))
}

// The record is found whichever way its path was spelled.
//
// A record stores the path its caller passed, so one project can be recorded
// as /p, as /p/, or with a . in the middle of it. A scan that compares those
// strings as they are misses the record, and finding no record is not an
// error — so the removal is reported as a success and leaves behind exactly
// the record --clean is for.
//
// Both directions: the stored path is the untidy one here, the asked-for path
// in the subtest.
func TestRemoveMatchesAnotherSpellingOfTheSamePath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	project := filepath.Join(t.TempDir(), "goner")
	require.NoError(t, os.MkdirAll(project, 0o755))
	// Recorded with a trailing separator, the way the caller happened to
	// spell it. The state directory is still keyed by the resolved path.
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project + string(filepath.Separator),
		Mode:        ModeStandalone,
		Port:        8080,
	}))
	require.NoError(t, os.RemoveAll(project))

	require.NoError(t, localstate.Remove(project))

	after, err := localstate.List()
	require.NoError(t, err)
	assert.Empty(t, after, "a trailing separator is the same project")

	t.Run("and the other way round", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		gone := staleRecordFor(t, t.TempDir())

		require.NoError(t, localstate.Remove(gone+string(filepath.Separator)))

		left, lerr := localstate.List()
		require.NoError(t, lerr)
		assert.Empty(t, left)
	})
}
