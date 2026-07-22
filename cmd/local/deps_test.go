//go:build !windows

package local

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// realModeRuntime builds the production runtime against temp state and proxy
// dirs, so its record-based dispatch runs for real without touching the user's
// machine.
func realModeRuntime(t *testing.T) modeRuntime {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ASTRO_HOME", t.TempDir())
	return newModeRuntime()
}

// liveGroup starts a real detached process in its own group and returns its
// pgid; the engine's liveness probe (kill(-pgid, 0)) sees it as alive until
// reap closes it.
func liveGroup(t *testing.T) (pgid int, reap func()) {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pgid = cmd.Process.Pid
	reaped := false
	reap = func() {
		if reaped {
			return
		}
		reaped = true
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		_ = cmd.Wait()
	}
	t.Cleanup(reap)
	return pgid, reap
}

func TestRefuseLiveStartGuardsAgainstOrphaning(t *testing.T) {
	rt := realModeRuntime(t)
	project := t.TempDir()
	pgid, reap := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        localrt.ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
	}))

	// A second start in the same mode is refused, not allowed to clobber.
	err := rt.refuseLiveStart(localrt.Plan{ProjectPath: project, Mode: localrt.ModeStandalone})
	require.ErrorContains(t, err, "already running")

	// A different mode is refused and names the mode that is live.
	err = rt.refuseLiveStart(localrt.Plan{ProjectPath: project, Mode: localrt.ModeDocker})
	require.ErrorContains(t, err, "standalone mode")

	// Once the runtime is gone, the stale record no longer blocks a start.
	reap()
	err = rt.refuseLiveStart(localrt.Plan{ProjectPath: project, Mode: localrt.ModeStandalone})
	require.NoError(t, err)
}

func TestRefuseLiveStartAllowsFirstStart(t *testing.T) {
	rt := realModeRuntime(t)
	// No record at all: the first start proceeds.
	require.NoError(t, rt.refuseLiveStart(localrt.Plan{ProjectPath: t.TempDir(), Mode: localrt.ModeStandalone}))
}

func TestLogSourceDispatch(t *testing.T) {
	rt := realModeRuntime(t)

	// No record: standalone's detached log handle serves a stopped project.
	stopped := t.TempDir()
	af, err := rt.LogSource(stopped)
	require.NoError(t, err)
	require.NotNil(t, af)

	// A docker record routes to the docker engine's attach.
	dockerProj := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath:    dockerProj,
		Mode:           localrt.ModeDocker,
		ComposeProject: "astro-x",
		Port:           8080,
	}))
	af, err = rt.LogSource(dockerProj)
	require.NoError(t, err)
	assert.NotNil(t, af)

	// A standalone record routes to standalone attach.
	saProj := t.TempDir()
	pgid, _ := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: saProj,
		Mode:        localrt.ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
	}))
	af, err = rt.LogSource(saProj)
	require.NoError(t, err)
	assert.NotNil(t, af)
}
