//go:build !windows

// Reset's own tests run the real engines against throwaway directories, which
// needs the same real process group runtime_test.go builds for liveness — so
// they share its Unix-only constraint. No compose marker is ever written here,
// which is what keeps them hermetic: the docker engine's gate short-circuits
// before it would probe for a container engine.

package localrt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// standaloneProject is a project directory carrying what a standalone run
// leaves behind: AIRFLOW_HOME with the metadata database, the venv, and a dag
// of the user's own to prove reset does not reach past what it derived.
func standaloneProject(t *testing.T) (dir, airflowHome, venv string) {
	t.Helper()
	dir = t.TempDir()
	airflowHome = filepath.Join(dir, airflowrt.StandaloneDir)
	venv = filepath.Join(dir, ".venv")
	for _, f := range []string{
		filepath.Join(airflowHome, "airflow.db"),
		filepath.Join(venv, "bin", "python"),
		filepath.Join(dir, "dags", "hello.py"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(f), 0o755))
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	}
	return dir, airflowHome, venv
}

// Whether Airflow was live has to be read before the stop, not after.
//
// statusOf probes for real — a signal to the process group — and the stop is
// precisely what makes that stop answering. Asked afterwards it always says
// "stopped", so `airflow: stopped` never printed and the json said false on
// every reset there has ever been.
func TestResetReportsTheStopItActuallyPerformed(t *testing.T) {
	rt := realRuntime(t)
	project, airflowHome, venv := standaloneProject(t)
	pgid, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
		Hostname:    "resettest.localhost",
	}))

	report, err := rt.Reset(context.Background(), project)
	require.NoError(t, err)

	assert.True(t, report.Stopped, "a live Airflow was stopped, and the report must say so")
	assert.NoDirExists(t, airflowHome)
	assert.NoDirExists(t, venv)
	assert.FileExists(t, filepath.Join(project, "dags", "hello.py"))
	_, lerr := localstate.Load(project)
	assert.ErrorIs(t, lerr, localstate.ErrNotRunning, "the stop retires the record")
}

// A record whose runtime is already gone must not be reported as a stop. This
// is the other half of reading liveness before the stop: read after, every
// reset looks the same, and a report that is always false is as useless as one
// that is always true.
func TestResetDoesNotClaimAStopItDidNotPerform(t *testing.T) {
	rt := realRuntime(t)
	project, airflowHome, venv := standaloneProject(t)
	pgid, reap := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
		Hostname:    "resettest.localhost",
	}))
	reap() // the record outlives the process

	report, err := rt.Reset(context.Background(), project)
	require.NoError(t, err)

	assert.False(t, report.Stopped, "nothing was running, so nothing was stopped")
	assert.NoDirExists(t, airflowHome, "a stale record still gets its state wiped")
	assert.NoDirExists(t, venv)
}

// Finding 6 from the v2 test plan: the case reset exists for.
//
// A stop removes the state record and every other entry point reaches the
// engines through Attach, which needs one — so "I stopped it, the database is
// broken, wipe it" got `no local Airflow is recorded for this project` and
// nothing was wiped.
func TestResetWipesAStoppedProjectWithNoRecord(t *testing.T) {
	rt := realRuntime(t)
	project, airflowHome, venv := standaloneProject(t)

	report, err := rt.Reset(context.Background(), project)
	require.NoError(t, err)

	assert.False(t, report.Stopped)
	assert.Empty(t, report.ComposeProject, "this project never ran in docker mode")
	assert.False(t, report.DockerUnreachable, "nothing to ask docker about is not an unreachable docker")
	assert.NoDirExists(t, airflowHome)
	assert.NoDirExists(t, venv)
	assert.FileExists(t, filepath.Join(project, "dags", "hello.py"))
}

// Reset must not wipe the venv of a project standalone never ran, with or
// without a record: `astro local check` parses DAGs with the project's own
// .venv interpreter in docker mode too.
func TestResetLeavesTheVenvOfAProjectStandaloneNeverRan(t *testing.T) {
	rt := realRuntime(t)
	project := t.TempDir()
	venv := filepath.Join(project, ".venv")
	require.NoError(t, os.MkdirAll(filepath.Join(venv, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(venv, "bin", "python"), []byte("x"), 0o600))

	report, err := rt.Reset(context.Background(), project)
	require.NoError(t, err)

	assert.False(t, report.Stopped)
	assert.DirExists(t, venv, "no standalone evidence, so the venv is not reset's to delete")
}

// Reset takes the same per-project lock Start does, so the two cannot
// interleave: a reset landing mid-start would delete the venv out from under
// its uv sync, or tear down containers the other half is still writing a record
// for. The lock is non-blocking by design, so what that buys reset is the same
// thing it buys a second start — a refusal, not a wait.
func TestResetRefusesToRaceAStart(t *testing.T) {
	rt := realRuntime(t)
	project, airflowHome, _ := standaloneProject(t)

	unlock, err := localstate.Lock(project)
	require.NoError(t, err)

	_, err = rt.Reset(context.Background(), project)
	require.ErrorIs(t, err, localstate.ErrLocked)
	assert.DirExists(t, airflowHome, "a refused reset must not have deleted anything first")

	// Once the start is done, the same reset goes through.
	unlock()
	_, err = rt.Reset(context.Background(), project)
	require.NoError(t, err)
	assert.NoDirExists(t, airflowHome)
}
