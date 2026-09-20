package localrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// saveRecord writes a record for an existing project directory. No live process
// backs it, so liveness reads as stopped — which is what most of these want.
func saveRecord(t *testing.T, project string, mode Mode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(project, 0o750))
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        mode,
		Hostname:    filepath.Base(project) + ".localhost",
	}))
}

// A docker project's dependencies live in an image, so making them live is a
// rebuild — a restart under another name. Refusing says so; dispatching to the
// standalone engine would install into a venv that does not exist.
func TestHotInstallRefusesDockerMode(t *testing.T) {
	// Built first, because it is what moves the cache lever: saveRecord writes
	// through whatever the environment says at the moment it runs, and running
	// it first put the record in the developer's real cache.
	rtime := isolatedRuntime(t)
	project := filepath.Join(t.TempDir(), "proj")
	saveRecord(t, project, ModeDocker)

	err := rtime.HotInstall(t.Context(), project, []string{"pandas"}, Callbacks{})

	require.ErrorIs(t, err, ErrNotImplemented)
	require.Contains(t, err.Error(), "docker-mode")
}

// Recorded is not running. A crash leaves the record behind, and installing
// into a stopped project then reporting success tells the caller a package is
// live in a scheduler that does not exist — when the whole reason to call this
// rather than restart is that something IS running.
func TestHotInstallRefusesAProjectThatIsNotRunning(t *testing.T) {
	rtime := isolatedRuntime(t)
	project := filepath.Join(t.TempDir(), "proj")
	saveRecord(t, project, ModeStandalone)

	err := rtime.HotInstall(t.Context(), project, []string{"pandas"}, Callbacks{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

// Answered before the record is even loaded: a caller watching a manifest
// cannot know a project declares no dependencies until it asks, and "nothing to
// do" is truer than "no Airflow is recorded" — which is what it used to say,
// because the check sat below the load.
func TestHotInstallWithNoDependenciesAnswersBeforeLookingForARuntime(t *testing.T) {
	rtime := isolatedRuntime(t)

	err := rtime.HotInstall(t.Context(), filepath.Join(t.TempDir(), "never-started"), nil, Callbacks{})

	require.NoError(t, err)
}
