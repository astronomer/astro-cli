//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/pkg/uv"
)

const fakePID = 4242

// fakeUV is a venvSyncer that fabricates the venv layout instead of running
// uv.
type fakeUV struct {
	err error
}

func (f fakeUV) EnsureSynced(_ context.Context, project, _ string, _ uv.Stdio) error {
	if f.err != nil {
		return f.err
	}
	binDir := filepath.Join(project, ".venv", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(binDir, "airflow"), []byte("#!/bin/sh\n"), 0o755)
}

// fakeProcs tracks fake process groups: which are alive and every signal
// sent. kill(-pgid, 0) is the engine's liveness probe.
type fakeProcs struct {
	alive map[int]bool
	sigs  []string
	// onTerm, when set, decides whether SIGTERM kills the group.
	onTerm func(pgid int) bool
}

func (f *fakeProcs) kill(pid int, sig syscall.Signal) error {
	if sig != 0 {
		f.sigs = append(f.sigs, fmt.Sprintf("%d:%v", pid, sig))
	}
	pgid := -pid
	if pid >= 0 || !f.alive[pgid] {
		if sig == 0 {
			return syscall.ESRCH
		}
		return nil
	}
	switch sig { //nolint:exhaustive // only the signals the engine sends matter
	case syscall.SIGTERM:
		if f.onTerm == nil || f.onTerm(pgid) {
			f.alive[pgid] = false
		}
	case syscall.SIGKILL:
		f.alive[pgid] = false
	}
	return nil
}

// testEngine builds an engine with every seam faked: no real uv, process,
// signal, or port probe.
func testEngine(t *testing.T) (*Engine, *fakeProcs, *[]string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	procs := &fakeProcs{alive: map[int]bool{}}
	var launches []string
	e := New(filepath.Join(t.TempDir(), "proxy"), nil)
	e.uv = func(context.Context) (venvSyncer, error) { return fakeUV{}, nil }
	e.launch = func(_ string, _ []string, name string, args ...string) (int, error) {
		launches = append(launches, name+" "+strings.Join(args, " "))
		procs.alive[fakePID] = true
		return fakePID, nil
	}
	e.prepAF2 = func(string) error { return nil }
	e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error { return nil }
	e.portFree = func(string) bool { return true }
	e.allocPort = func() (string, error) { return "", errors.New("allocation not expected") }
	e.kill = procs.kill
	e.selfExe = func() (string, error) { return "/usr/local/bin/astro", nil }
	e.goos = "linux"
	e.now = func() time.Time { return time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC) }
	e.stopTimeout = 50 * time.Millisecond
	e.stopPoll = time.Millisecond
	return e, procs, &launches
}

func testPlan(t *testing.T) rt.Plan {
	t.Helper()
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "dags"), 0o755))
	return rt.Plan{
		ProjectPath:   project,
		Mode:          rt.ModeStandalone,
		RequestedPort: 8081,
		Env:           map[string]string{"FOO": "bar"},
	}
}

func TestStartLaunchesSupervisedAirflow(t *testing.T) {
	e, procs, launches := testEngine(t)
	p := testPlan(t)

	var states []rt.State
	cb := rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }}

	af, err := e.Start(context.Background(), p, cb)
	require.NoError(t, err)
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateRunning}, states)

	// Every launch runs through the supervisor, which owns the capped log
	// file; a detached start does not arm the parent watch.
	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	require.Len(t, *launches, 1)
	assert.Equal(t, fmt.Sprintf("/usr/local/bin/astro __supervise --log-file %s -- %s standalone",
		filepath.Join(stateDir, logFileName),
		filepath.Join(p.ProjectPath, ".venv", "bin", "airflow")),
		(*launches)[0])
	assert.NotContains(t, (*launches)[0], "--parent-pid")

	// The state record captures what other tools need to reconnect.
	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, rt.ModeStandalone, rec.Mode)
	assert.Equal(t, fakePID, rec.PID)
	assert.Equal(t, fakePID, rec.Pgid)
	assert.Equal(t, 8081, rec.Port)
	assert.Equal(t, filepath.Base(p.ProjectPath)+".localhost", rec.Hostname)
	assert.False(t, rec.StopWithSession)

	// The proxy route is standalone-mode and carries the supervisor PID so
	// the daemon's stale-route pruning tracks the right process.
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, proxy.RouteModeStandalone, routes[0].Mode)
	assert.Equal(t, "8081", routes[0].Port)
	assert.Equal(t, fakePID, routes[0].PID)

	st, err := af.Status()
	require.NoError(t, err)
	assert.Equal(t, rt.StateRunning, st.State)
	assert.Equal(t, 8081, st.Port)

	// Once the group dies, the same record reads as stopped.
	procs.alive[fakePID] = false
	st, err = af.Status()
	require.NoError(t, err)
	assert.Equal(t, rt.StateStopped, st.State)
}

func TestStartSessionTiedArmsParentWatch(t *testing.T) {
	e, _, launches := testEngine(t)
	p := testPlan(t)
	p.StopWithSession = true

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, *launches, 1)
	assert.Contains(t, (*launches)[0], "--parent-pid "+strconv.Itoa(os.Getpid()))

	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.True(t, rec.StopWithSession)
}

func TestStartHealthFailureKillsGroupAndClearsState(t *testing.T) {
	// Desktop's PID-reuse bug: a failed-health start that leaves its PID
	// state behind makes a later start believe a recycled PID is Airflow.
	// The engine must kill the group and clear the record.
	e, procs, _ := testEngine(t)
	healthErr := errors.New("health check timed out")
	e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error { return healthErr }
	p := testPlan(t)

	var states []rt.State
	cb := rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }}

	_, err := e.Start(context.Background(), p, cb)
	require.ErrorIs(t, err, healthErr)
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateError}, states)

	assert.Contains(t, procs.sigs, fmt.Sprintf("%d:%v", -fakePID, syscall.SIGTERM))
	_, err = localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, err, localstate.ErrNotRunning)

	// No route was registered: routes only appear after health passes.
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)

	// With the state cleared, a second start goes through.
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorIs(t, err, healthErr)
}

func TestStartRefusesLiveRecordAndForeignMode(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// Still alive: a second start refuses.
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	require.ErrorContains(t, err, "already running")

	// Dead group: the stale record is overwritten.
	procs.alive[fakePID] = false
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// A docker-mode record is never this engine's to overwrite.
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: p.ProjectPath, Mode: rt.ModeDocker, ComposeProject: "x",
	}))
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorContains(t, err, "docker mode")

	// And the wrong plan mode never reaches this engine's lifecycle.
	p.Mode = rt.ModeDocker
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorContains(t, err, `localstandalone got a "docker" plan`)
}

func TestStartWithoutAirflowInVenv(t *testing.T) {
	e, _, launches := testEngine(t)
	// A syncer that produces no airflow binary (a pyproject without an
	// Airflow distribution) fails before anything launches.
	e.uv = func(context.Context) (venvSyncer, error) {
		return syncerFunc(func(_ context.Context, project, _ string, _ uv.Stdio) error {
			return os.MkdirAll(filepath.Join(project, ".venv", "bin"), 0o755)
		}), nil
	}
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.ErrorContains(t, err, "no airflow command")
	assert.Empty(t, *launches)
}

type syncerFunc func(ctx context.Context, project, python string, stdio uv.Stdio) error

func (f syncerFunc) EnsureSynced(ctx context.Context, project, python string, stdio uv.Stdio) error {
	return f(ctx, project, python, stdio)
}

func TestUVFailureSurfaces(t *testing.T) {
	e, _, _ := testEngine(t)
	uvErr := errors.New("no solution found")
	e.uv = func(context.Context) (venvSyncer, error) {
		return fakeUV{err: uvErr}, nil
	}
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorIs(t, err, uvErr)
}

func TestStopSignalsProcessGroup(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))

	// SIGTERM went to the process group (negative pgid), never the bare
	// master PID — the leak v1's master-PID stop still has.
	assert.Equal(t, []string{fmt.Sprintf("%d:%v", -fakePID, syscall.SIGTERM)}, procs.sigs)

	_, err = localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, err, localstate.ErrNotRunning)
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestStopEscalatesWhenGroupSurvivesSigterm(t *testing.T) {
	e, procs, _ := testEngine(t)
	procs.onTerm = func(int) bool { return false } // the group ignores SIGTERM
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))
	assert.Contains(t, procs.sigs, fmt.Sprintf("%d:%v", -fakePID, syscall.SIGTERM))
	assert.Contains(t, procs.sigs, fmt.Sprintf("%d:%v", -fakePID, syscall.SIGKILL))
	assert.False(t, procs.alive[fakePID])
}

func TestStopForceSkipsGracefulWindow(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{Force: true}))
	assert.Equal(t, []string{fmt.Sprintf("%d:%v", -fakePID, syscall.SIGKILL)}, procs.sigs)
}

func TestStopCleanRemovesDerivedState(t *testing.T) {
	e, _, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, logFileName), []byte("log"), 0o600))
	airflowHome := filepath.Join(p.ProjectPath, airflowrt.StandaloneDir)
	require.NoError(t, os.MkdirAll(airflowHome, 0o755))

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{Clean: true}))
	assert.NoDirExists(t, airflowHome)
	assert.NoDirExists(t, filepath.Join(p.ProjectPath, ".venv"))
	assert.NoFileExists(t, filepath.Join(stateDir, logFileName))
	assert.NoFileExists(t, filepath.Join(stateDir, jwtSecretFile))
	// The project's own files are untouched.
	assert.DirExists(t, filepath.Join(p.ProjectPath, "dags"))
}

func TestStopIsIdempotentWithoutProcess(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	procs.alive[fakePID] = false
	procs.sigs = nil
	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))
	assert.Empty(t, procs.sigs)
	_, err = localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, err, localstate.ErrNotRunning)
}

func TestAttachRefusesForeignRecords(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project, Mode: rt.ModeDocker, ComposeProject: "x",
	}))
	_, err := e.Attach(project)
	assert.ErrorIs(t, err, ErrNotStandaloneMode)
	_, err = e.ReadStatus(project)
	assert.ErrorIs(t, err, ErrNotStandaloneMode)
}

func TestReadStatusWithoutRecordIsStopped(t *testing.T) {
	e, _, _ := testEngine(t)
	project := t.TempDir()
	st, err := e.ReadStatus(project)
	require.NoError(t, err)
	assert.Equal(t, rt.StateStopped, st.State)
	assert.Equal(t, project, st.ProjectPath)
}

func TestRunExecsInProjectEnv(t *testing.T) {
	e, _, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	var gotDir, gotCall string
	var gotEnv []string
	e.cmd = commanderFunc(func(_ context.Context, dir string, env []string, _ rt.Stdio, name string, args ...string) error {
		gotDir, gotEnv = dir, env
		gotCall = name + " " + strings.Join(args, " ")
		return nil
	})

	require.NoError(t, af.Run(context.Background(), []string{"airflow", "dags", "list"}, rt.Stdio{}))
	assert.Equal(t, p.ProjectPath, gotDir)
	// The binary resolves through the venv PATH the env carries.
	assert.Equal(t, filepath.Join(p.ProjectPath, ".venv", "bin", "airflow")+" dags list", gotCall)
	assert.Contains(t, gotEnv, "VIRTUAL_ENV="+filepath.Join(p.ProjectPath, ".venv"))

	err = af.Run(context.Background(), nil, rt.Stdio{})
	assert.ErrorContains(t, err, "no command given")
}

type commanderFunc func(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error

func (f commanderFunc) Run(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error {
	return f(ctx, dir, env, s, name, args...)
}

func TestLaunchCommandPicksTheAF2DarwinShim(t *testing.T) {
	t.Parallel()
	project := "/proj"
	venvBin := filepath.Join(project, ".venv", "bin")

	// Desktop's AF2 macOS SIGSEGV: gunicorn's forked workers inherit
	// corrupted ObjC state, so AF2 on darwin runs through the Python shim
	// with the fork-safety prep.
	bin, args, prep := launchCommand("darwin", "2", project)
	assert.Equal(t, "/proj/.venv/bin/python", bin)
	assert.Equal(t, []string{filepath.Join(venvBin, af2ShimName)}, args)
	assert.True(t, prep)

	bin, args, prep = launchCommand("linux", "2", project)
	assert.Equal(t, "/proj/.venv/bin/airflow", bin)
	assert.Equal(t, []string{"standalone"}, args)
	assert.False(t, prep)

	bin, _, prep = launchCommand("darwin", "3", project)
	assert.Equal(t, "/proj/.venv/bin/airflow", bin)
	assert.False(t, prep)
}
