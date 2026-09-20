//go:build !windows

package localrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// newSyncProject makes the directory on disk. Without it every call below
// fails inside localstate.Lock, which resolves symlinks on the path — and a
// test that fails there satisfies every assertion about what Sync refuses
// while never reaching Sync at all.
func newSyncProject(t *testing.T) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.MkdirAll(project, 0o750))
	return project
}

// reachedEngine reports whether the engine was entered, by the StateStarting
// it emits before doing anything. Proving the refusals is not enough on its
// own: a Sync that refused everything would pass all of them.
func reachedEngine(t *testing.T) (cb Callbacks, reached *bool) {
	t.Helper()
	var seen bool
	return Callbacks{OnState: func(s State, _ error) {
		if s == StateStarting {
			seen = true
		}
	}}, &seen
}

// A docker project's packages live in an image, so the equivalent of this is a
// build — a different operation with a different cost. Dispatching to the
// standalone engine instead would provision a venv the project never uses.
func TestSyncRefusesDockerMode(t *testing.T) {
	rtime := isolatedRuntime(t)
	project := newSyncProject(t)
	cb, reached := reachedEngine(t)

	err := rtime.Sync(t.Context(), Plan{ProjectPath: project, Mode: ModeDocker}, cb)

	require.ErrorIs(t, err, ErrNotImplemented)
	require.Contains(t, err.Error(), "docker-mode")
	require.False(t, *reached)
}

// uv sync resolves the whole set, removes what the manifest no longer names,
// and deletes the venv outright when a previous run left the completion
// marker off. Under a live scheduler executing from that venv that is not a
// risk to weigh — and the caller is an editor opening a file, not anyone who
// asked for it.
func TestSyncRefusesAProjectThatIsRunning(t *testing.T) {
	rtime := isolatedRuntime(t)
	project := newSyncProject(t)
	pgid, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
		Hostname:    "synctest.localhost",
	}))
	cb, reached := reachedEngine(t)

	err := rtime.Sync(t.Context(), Plan{ProjectPath: project, Mode: ModeStandalone}, cb)

	require.ErrorIs(t, err, ErrNotImplemented)
	require.Contains(t, err.Error(), "running")
	require.False(t, *reached, "the venv would have been rebuilt underneath a live scheduler")
}

// The invariant that makes this entry point worth having: it is the only
// method on Runtime that works with no record, because a project nobody has
// started is exactly what the editor wants a venv for. Anything demanding one
// would answer "not running" and leave the editor with no imports.
//
// Asserted by reaching the engine, not by the error — inserting a record check
// would keep the error non-nil and fail this.
func TestSyncWithNoRecordReachesTheEngine(t *testing.T) {
	rtime := isolatedRuntime(t)
	project := newSyncProject(t)
	cb, reached := reachedEngine(t)

	// Provisioning itself fails here: there is no project to sync and no
	// network wanted in a unit test. Where it fails is the point.
	_ = rtime.Sync(t.Context(), Plan{ProjectPath: project, Mode: ModeStandalone}, cb)

	require.True(t, *reached, "Sync stopped before the engine; a record check does exactly this")
}

// A plan built for a command that asked for no particular runtime leaves Mode
// empty, and Start treats that as standalone. Sync refusing it would make the
// two disagree about the same plan.
func TestSyncAcceptsThePlanStartAccepts(t *testing.T) {
	rtime := isolatedRuntime(t)
	project := newSyncProject(t)
	cb, reached := reachedEngine(t)

	_ = rtime.Sync(t.Context(), Plan{ProjectPath: project}, cb)

	require.True(t, *reached, "an empty Mode was refused; Start defaults it to standalone")
}

// An empty path resolves to the process's working directory, which would
// provision a venv wherever the embedder happens to be standing.
func TestSyncRefusesAnEmptyProjectPath(t *testing.T) {
	rtime := isolatedRuntime(t)
	cb, reached := reachedEngine(t)

	require.Error(t, rtime.Sync(t.Context(), Plan{Mode: ModeStandalone}, cb))
	require.False(t, *reached)
}
