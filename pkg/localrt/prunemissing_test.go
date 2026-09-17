//go:build !windows

package localrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// Constrained like the rest of this package's Runtime tests: judging liveness
// needs a real process group, and two cases here need a permission the
// platforms disagree about. The half of this fix that is only about files
// lives in removemissing_test.go, which runs everywhere.

// saveStale writes a stopped record for an existing project directory. No
// live process, so the record reads as stale.
func saveStale(t *testing.T, project string, port int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		Port:        port,
		Hostname:    filepath.Base(project) + ".localhost",
	}))
}

// orderedProjects returns two project directories under parent whose state
// directories sort in the order given, first before second.
//
// The prune loop walks records in os.ReadDir order, which is the sorted
// sha256 of each project's resolved path — so which of two records it reaches
// first is decided by a hash of a random temp path, and changes from run to
// run. A test about what happens AFTER a failure has to put the failing
// record first or it only catches the regression about half the time:
// measured against this fix's own mutation, it survived 5 runs in 10.
//
// Names are tried until the hashes fall the right way, which takes two
// attempts on average.
func orderedProjects(t *testing.T, parent, firstName, secondName string) (first, second string) {
	t.Helper()
	for i := 0; i < 64; i++ {
		first = filepath.Join(parent, fmt.Sprintf("%s-%d", firstName, i))
		second = filepath.Join(parent, fmt.Sprintf("%s-%d", secondName, i))
		require.NoError(t, os.MkdirAll(first, 0o755))
		require.NoError(t, os.MkdirAll(second, 0o755))
		firstDir, err := rt.StateDir(first)
		require.NoError(t, err)
		secondDir, err := rt.StateDir(second)
		require.NoError(t, err)
		if filepath.Base(firstDir) < filepath.Base(secondDir) {
			return first, second
		}
		require.NoError(t, os.RemoveAll(first))
		require.NoError(t, os.RemoveAll(second))
	}
	t.Fatal("no pair of names hashed into the order this test needs, in 64 tries")
	return "", ""
}

// `astro local list --clean` died on a record whose project directory was gone,
// with "resolving symlinks in <path>: no such file or directory" — which made
// that record permanent, because the one command that removes it could not run
// while it was there.
func TestPruneRemovesARecordWhoseProjectIsGone(t *testing.T) {
	rtime := realRuntime(t)
	gone := staleRecordFor(t, t.TempDir())

	removed, err := rtime.PruneStale()
	require.NoError(t, err, "a project directory that no longer exists must not stop the prune")

	require.Len(t, removed, 1)
	assert.Equal(t, gone, removed[0].ProjectPath)

	left, err := localstate.List()
	require.NoError(t, err)
	assert.Empty(t, left, "the record should be gone from disk")
}

// And it takes the state directory with it, not just the record inside.
//
// Nothing can name that directory again once the project is gone: the key is
// a hash of the resolved path, which cannot be derived without the project and
// cannot be inverted. Whatever is left in there — an airflow.log that grew for
// as long as the project ran — would be bytes nothing can reach and nothing
// will ever free.
func TestPruneReclaimsTheStateDirectoryOfAProjectThatIsGone(t *testing.T) {
	rtime := realRuntime(t)
	project := filepath.Join(t.TempDir(), "logger")
	saveStale(t, project, 8080)

	stateDir, err := rt.StateDir(project)
	require.NoError(t, err)
	log := filepath.Join(stateDir, "airflow.log")
	require.NoError(t, os.WriteFile(log, []byte("many megabytes, in real life\n"), 0o600))
	require.NoError(t, os.RemoveAll(project))

	_, err = rtime.PruneStale()
	require.NoError(t, err)

	_, err = os.Stat(stateDir)
	assert.True(t, errors.Is(err, os.ErrNotExist),
		"the whole state directory should be reclaimed, since nothing can name it again")
}

// Two stale records, one of each kind, both pruned.
func TestPruneRemovesBothAGoneAndAPresentProject(t *testing.T) {
	rtime := realRuntime(t)
	parent := t.TempDir()

	gone := staleRecordFor(t, parent)
	present := filepath.Join(parent, "present")
	saveStale(t, present, 8081)

	removed, err := rtime.PruneStale()
	require.NoError(t, err)

	var paths []string
	for _, st := range removed {
		paths = append(paths, st.ProjectPath)
	}
	assert.ElementsMatch(t, []string{gone, present}, paths,
		"both stale records should be pruned, in either order")

	left, err := localstate.List()
	require.NoError(t, err)
	assert.Empty(t, left)
}

// PruneStale returned on the first failure, so one record it could not remove
// kept every stale record behind it: the effect was that --clean stopped
// working at all rather than skipping one entry.
//
// The missing-directory case no longer fails, so the remaining way to get an
// unremovable record is a state directory that cannot be written, which is
// what this uses.
func TestPruneReportsWhatItCouldNotRemoveAndPrunesTheRest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the directory permission this relies on")
	}
	rtime := realRuntime(t)
	// The unremovable record is reached first, which is the only arrangement
	// that says anything about carrying on past it.
	stuck, ok := orderedProjects(t, t.TempDir(), "stuck", "ok")
	saveStale(t, stuck, 8082)
	saveStale(t, ok, 8083)

	// Make the record inside stuck's state directory impossible to unlink.
	stuckDir, err := rt.StateDir(stuck)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(stuckDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(stuckDir, 0o700) })

	removed, err := rtime.PruneStale()

	// What it could not remove is reported, and named: an error that does not
	// say which project leaves nothing to act on.
	require.Error(t, err, "a record it could not remove has to be reported")
	assert.Contains(t, err.Error(), stuck)

	// What it could is pruned, rather than hidden behind the failure — and
	// the failure is not counted as a removal.
	require.Len(t, removed, 1)
	assert.Equal(t, ok, removed[0].ProjectPath)

	// The record it could not remove is still there, rather than reported as
	// a problem and dropped anyway.
	left, err := localstate.List()
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, stuck, left[0].ProjectPath)
}

// A route that will not drop does not keep the record.
//
// The route already names a runtime that is not there, so holding the record
// cannot revive it — while a routes file that always fails to write would
// otherwise make every record unprunable for good, which is the same trap the
// fix above is about.
func TestPruneDropsTheRecordEvenWhenItsRouteWillNotGo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which ignores the directory permission this relies on")
	}
	routes := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	rtime := New(Config{RoutesDir: routes})

	project := filepath.Join(t.TempDir(), "routed")
	saveStale(t, project, 8084)
	// Nothing can be written under the routes dir, so removing the route
	// fails however it is attempted.
	require.NoError(t, os.Chmod(routes, 0o500))
	t.Cleanup(func() { _ = os.Chmod(routes, 0o700) })

	removed, err := rtime.PruneStale()

	require.Error(t, err, "a route it could not drop has to be reported")
	assert.Contains(t, err.Error(), "routed.localhost")

	require.Len(t, removed, 1, "the record should still be pruned")
	left, lerr := localstate.List()
	require.NoError(t, lerr)
	assert.Empty(t, left)
}

// An engine it could not reach is reported, not read as "nothing to do".
//
// ContainersGone errors only when no engine could be reached at all, which is
// the one case where a docker record must be left alone: its containers may
// well be running. Swallowed, --clean printed "No stale local Airflow records
// to remove" — which is a different claim, and the wrong one.
func TestPruneSaysItCouldNotReachAnEngineRatherThanNothingToDo(t *testing.T) {
	rtime := realRuntime(t)
	unreachable := errors.New("no container engine reachable")
	asked := 0
	rtime.containersGone = func(context.Context, string) (bool, error) {
		asked++
		return false, unreachable
	}

	removed, err := rtime.pruneAll([]Status{
		{ProjectPath: "/p/one", Mode: ModeDocker, State: StateStopped},
		{ProjectPath: "/p/two", Mode: ModeDocker, State: StateStopped},
	})

	require.ErrorIs(t, err, unreachable)
	assert.Empty(t, removed, "a record whose containers could not be checked must not be dropped")
	// Asked once. The engine will not come back inside one loop, and one
	// thing wrong with the machine should not be reported once per record,
	// each repeat costing another probe timeout.
	assert.Equal(t, 1, asked, "one machine-wide cause, asked and reported once")
}

// Only a project that is GONE gets the fallback.
//
// Deriving a record's location can fail for reasons that are not "the project
// was deleted" — a permission denial, a symlink loop. Falling back on any
// error would report those as a successful removal, since a scan that finds
// no record is not an error. Here the path resolves to nothing because it
// loops, and that has to stay loud.
//
// Needs a symlink, which is why it sits with the constrained tests.
func TestRemoveReportsAFailureToDeriveThatIsNotAMissingProject(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	loop := filepath.Join(t.TempDir(), "loop")
	require.NoError(t, os.Symlink(loop, loop))

	err := localstate.Remove(loop)
	require.Error(t, err, "a symlink loop is not a deleted project and must not read as one")
	assert.False(t, errors.Is(err, os.ErrNotExist))
}

// A docker record whose containers are still there is not stale, and skipping
// it is not a failure.
func TestPruneLeavesADockerRecordWhoseContainersAreStillThere(t *testing.T) {
	rtime := realRuntime(t)
	rtime.containersGone = func(context.Context, string) (bool, error) { return false, nil }

	removed, err := rtime.pruneAll([]Status{
		{ProjectPath: "/p/one", Mode: ModeDocker, State: StateStopped},
	})
	require.NoError(t, err)
	assert.Empty(t, removed)
}
