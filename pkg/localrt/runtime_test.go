//go:build !windows

// The runtime dispatch tests need a real process group to prove liveness
// (Setpgid, kill(-pgid)), which is Unix-only — the constraint came with them from
// cmd/local/deps_test.go. Without it `GOOS=windows go vet ./...` fails to compile
// this whole module, and nothing would say so: make test-submodules runs on
// ubuntu, and the root Windows CI job only covers the root module.

package localrt

import (
	"os/exec"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
)

// These tests came from cmd/local/deps_test.go along with the code they cover:
// refusing to start over a live runtime, and picking the right engine to read
// logs through. Both were CLI-private and are now the shared contract's, so the
// tests follow rather than being left behind testing nothing.

func TestRefuseLiveStartGuardsAgainstOrphaning(t *testing.T) {
	rt := isolatedRuntime(t)
	project := t.TempDir()
	pgid, reap := liveGroup(t)
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
	}))

	// A second start in the same mode is refused, not allowed to clobber.
	err := rt.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeStandalone})
	require.ErrorContains(t, err, "already running")

	// A different mode is refused and names the mode that is live.
	err = rt.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeDocker})
	require.ErrorContains(t, err, "standalone mode")

	// Once the runtime is gone, the stale record no longer blocks a start.
	reap()
	err = rt.refuseLiveStart(Plan{ProjectPath: project, Mode: ModeStandalone})
	require.NoError(t, err)
}

func TestRefuseLiveStartAllowsFirstStart(t *testing.T) {
	rt := isolatedRuntime(t)
	// No record at all: the first start proceeds.
	require.NoError(t, rt.refuseLiveStart(Plan{ProjectPath: t.TempDir(), Mode: ModeStandalone}))
}

func TestLogSourceDispatch(t *testing.T) {
	rt := isolatedRuntime(t)

	// No record: standalone's detached log handle serves a stopped project.
	stopped := t.TempDir()
	af, err := rt.LogSource(stopped)
	require.NoError(t, err)
	require.NotNil(t, af)

	// A docker record routes to the docker engine's attach.
	dockerProj := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath:    dockerProj,
		Mode:           ModeDocker,
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
		Mode:        ModeStandalone,
		PID:         pgid,
		Pgid:        pgid,
		Port:        8080,
	}))
	af, err = rt.LogSource(saProj)
	require.NoError(t, err)
	assert.NotNil(t, af)
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
