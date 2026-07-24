package localdocker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/imagebuild"
	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// fakeCmd is a Commander that never touches a real daemon. Each call is
// recorded as "name arg arg..."; responses come from the hooks.
type fakeCmd struct {
	calls  []string
	output func(call string) ([]byte, error)
	run    func(call string, s localrt.Stdio) error
}

func (f *fakeCmd) Output(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.output == nil {
		return nil, nil
	}
	return f.output(call)
}

func (f *fakeCmd) Run(_ context.Context, _ []string, s localrt.Stdio, name string, args ...string) error {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.run == nil {
		return nil
	}
	return f.run(call, s)
}

// noProjects is an output hook for "nothing is running anywhere".
func noProjects(string) ([]byte, error) { return nil, nil }

// testEngine builds an engine with every seam faked: docker preferred, all
// ports free, health immediately OK, deterministic clock.
func testEngine(t *testing.T, cmd *fakeCmd) *Engine {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	e := New(filepath.Join(t.TempDir(), "proxy"), nil)
	e.cmd = cmd
	e.preferred = func() (engineConn, error) { return engineConn{bin: "docker"}, nil }
	e.connFor = func(bin string) engineConn { return engineConn{bin: bin} }
	e.portFree = func(string) bool { return true }
	e.allocPort = func() (string, error) { return "", errors.New("allocation not expected") }
	e.health = func(context.Context, string, time.Duration) error { return nil }
	e.now = func() time.Time { return time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC) }
	e.ensureEngine = func(localrt.Callbacks) error { return nil }
	e.composeAvail = func(context.Context, engineConn) error { return nil }
	e.startSession = func(string, int) error { return nil }
	return e
}

func testPlan(t *testing.T) localrt.Plan {
	t.Helper()
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "dags"), 0o755))
	return localrt.Plan{
		ProjectPath:    project,
		Mode:           localrt.ModeDocker,
		AirflowVersion: "3.1-2",
		RequestedPort:  8081,
		Env:            map[string]string{"FOO": "bar"},
	}
}

func TestStartBringsProjectUp(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)

	var states []localrt.State
	cb := localrt.Callbacks{OnState: func(s localrt.State, _ error) { states = append(states, s) }}

	af, err := e.Start(context.Background(), p, cb)
	require.NoError(t, err)
	assert.Equal(t, []localrt.State{localrt.StateStarting, localrt.StateRunning}, states)

	// The up call ran through compose with the generated file, the project
	// dir pinned (findProject discovers by that label), and detached.
	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	stateDir, err := localrt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	composeFile := filepath.Join(stateDir, composeFileName)
	require.Len(t, cmd.calls, 1)
	assert.Equal(t, fmt.Sprintf("docker compose --file %s --project-directory %s --project-name %s up --detach --quiet-pull",
		composeFile, p.ProjectPath, name), cmd.calls[0])
	assert.FileExists(t, composeFile)

	// The state record captures what other tools need to reconnect.
	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, localrt.ModeDocker, rec.Mode)
	assert.Equal(t, name, rec.ComposeProject)
	assert.Equal(t, 8081, rec.Port)
	assert.Equal(t, filepath.Base(p.ProjectPath)+".localhost", rec.Hostname)
	assert.False(t, rec.StopWithSession)
	assert.Zero(t, rec.PID)

	// The proxy route is registered as a docker-mode route.
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	require.Len(t, routes, 1)
	assert.Equal(t, proxy.RouteModeDocker, routes[0].Mode)
	assert.Equal(t, "8081", routes[0].Port)
	assert.Equal(t, p.ProjectPath, routes[0].ProjectDir)
	assert.Contains(t, routes[0].Services, "postgres")

	// The handle reports running while the engine sees the project.
	cmd.output = func(call string) ([]byte, error) {
		if strings.Contains(call, workingDirLabel+"="+p.ProjectPath) {
			return []byte(name + "\n"), nil
		}
		return nil, nil
	}
	st, err := af.Status()
	require.NoError(t, err)
	assert.Equal(t, localrt.StateRunning, st.State)
	assert.Equal(t, 8081, st.Port)
}

func TestStartSessionTiedRecordsOwnerPID(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	p := testPlan(t)
	p.StopWithSession = true

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)

	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.True(t, rec.StopWithSession)
	assert.Equal(t, os.Getpid(), rec.PID)
}

func TestStartInstallsDependenciesIntoImage(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Dependencies = []string{"pandas==2.2.0", "requests"}

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)

	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	tag := builtImageTag(name)

	// A build ran, tagging the per-project image over the runtime base.
	assert.True(t, hasCall(cmd.calls, "docker build --tag "+tag), "expected a build, got %v", cmd.calls)
	// The compose up runs the built image, not the bare runtime image.
	stateDir, err := localrt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
	require.NoError(t, err)
	assert.Contains(t, string(compose), "image: "+tag)
	assert.NotContains(t, string(compose), imagebuild.RuntimeImageRepo)
	// The build context's contents (requirements.txt, packages.txt) are the
	// builder's concern and are checked in internal/imagebuild.
}

func TestStartOSPackagesTriggerBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Packages = []string{"libpq-dev", "build-essential"}

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)

	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	tag := builtImageTag(name)
	// OS packages alone trigger the build layer, even with no extra deps.
	assert.True(t, hasCall(cmd.calls, "docker build --tag "+tag), "expected a build, got %v", cmd.calls)
}

func TestStartNoDependenciesSkipsBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t) // no Dependencies

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasCall(cmd.calls, "build"), "no build may run without extra deps, got %v", cmd.calls)
}

func TestStartAirflowOnlyDepsSkipsBuild(t *testing.T) {
	// The base image provides Airflow; a manifest listing only Airflow (any
	// extras/pin) needs no build layer.
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Dependencies = []string{"apache-airflow==3.1.*", "apache-airflow[celery]"}

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasCall(cmd.calls, "build"), "airflow-only deps must not trigger a build, got %v", cmd.calls)
}

func TestStartFailedDepInstallReturnsNamedError(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, run: func(call string, _ localrt.Stdio) error {
		if strings.Contains(call, "build") {
			return errors.New("exit status 1")
		}
		return nil
	}}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Dependencies = []string{"nonexistent-package-xyz"}

	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies")
	// The build failed, so no containers start.
	assert.False(t, hasCall(cmd.calls, "up --detach"), "no up after a failed build, got %v", cmd.calls)
}

func TestStartBringsEngineUpFirst(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	var ensured bool
	e.ensureEngine = func(localrt.Callbacks) error { ensured = true; return nil }
	_, err := e.Start(context.Background(), testPlan(t), localrt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, ensured, "the engine must be brought up before the start")
}

func TestStartFailsWhenEngineCannotStart(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.ensureEngine = func(localrt.Callbacks) error { return errors.New("engine down") }
	_, err := e.Start(context.Background(), testPlan(t), localrt.Callbacks{})
	assert.ErrorContains(t, err, "engine down")
	assert.Empty(t, cmd.calls, "nothing may run when the engine cannot start")
}

func TestStartReportsMissingCompose(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.composeAvail = func(context.Context, engineConn) error { return ErrComposeMissing }
	_, err := e.Start(context.Background(), testPlan(t), localrt.Callbacks{})
	assert.ErrorIs(t, err, ErrComposeMissing)
	assert.False(t, hasCall(cmd.calls, "up"), "no compose up when the plugin is missing")
}

func TestStartSessionTiedSpawnsWatcher(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	var gotProject string
	var gotPID int
	e.startSession = func(project string, pid int) error {
		gotProject, gotPID = project, pid
		return nil
	}
	p := testPlan(t)
	p.StopWithSession = true
	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, p.ProjectPath, gotProject)
	assert.Equal(t, os.Getpid(), gotPID)
}

func TestStartHealthFailureKeepsRecordAndRoute(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	e.health = func(context.Context, string, time.Duration) error { return ErrHealthTimeout }
	p := testPlan(t)

	var errState error
	cb := localrt.Callbacks{OnState: func(s localrt.State, err error) {
		if s == localrt.StateError {
			errState = err
		}
	}}
	_, err := e.Start(context.Background(), p, cb)
	assert.ErrorIs(t, err, ErrHealthTimeout)
	assert.ErrorIs(t, errState, ErrHealthTimeout)

	// The containers keep starting, so stop/status must still work.
	_, err = localstate.Load(p.ProjectPath)
	assert.NoError(t, err)
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	assert.Len(t, routes, 1)
}

func TestStartRejectsNonDockerPlanAndBadVersions(t *testing.T) {
	e := testEngine(t, &fakeCmd{})

	p := testPlan(t)
	p.Mode = localrt.ModeStandalone
	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	assert.ErrorContains(t, err, "standalone")

	p = testPlan(t)
	p.AirflowVersion = "2.9.3"
	_, err = e.Start(context.Background(), p, localrt.Callbacks{})
	assert.ErrorContains(t, err, "Airflow 3")
	assert.Empty(t, e.cmd.(*fakeCmd).calls, "no containers may start for a rejected plan")
}

func TestChoosePort(t *testing.T) {
	e := testEngine(t, &fakeCmd{})

	// Requested and free: taken as-is.
	got, err := e.choosePort(8081, defaultAPIServerPort)
	require.NoError(t, err)
	assert.Equal(t, 8081, got)

	// No request, default free: the default.
	got, err = e.choosePort(0, defaultAPIServerPort)
	require.NoError(t, err)
	assert.Equal(t, defaultAPIServerPort, got)

	// Busy ports fall back to pool allocation.
	e.portFree = func(string) bool { return false }
	e.allocPort = func() (string, error) { return "15001", nil }
	got, err = e.choosePort(8081, defaultAPIServerPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)
	got, err = e.choosePort(0, defaultAPIServerPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)

	// A busy requested port never silently lands on the default.
	e.portFree = func(p string) bool { return p == "8080" }
	got, err = e.choosePort(8081, defaultAPIServerPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)
}

// startProject boots a project through the fake engine and returns its
// handle plus the compose project name.
func startProject(t *testing.T, e *Engine, p localrt.Plan) (af localrt.Airflow, composeProject string) {
	t.Helper()
	af, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	return af, name
}

func TestStopRemovesRouteAndRecord(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	af, name := startProject(t, e, p)

	require.NoError(t, af.Stop(context.Background(), localrt.StopOptions{}))

	down := cmd.calls[len(cmd.calls)-1]
	assert.Contains(t, down, "--project-name "+name+" down --timeout 10")
	assert.NotContains(t, down, "--volumes")

	_, err := localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, err, localstate.ErrNotRunning)
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	assert.Empty(t, routes)
}

func TestStopForceAndClean(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	af, _ := startProject(t, e, p)

	stateDir, err := localrt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	composeFile := filepath.Join(stateDir, composeFileName)
	require.FileExists(t, composeFile)

	require.NoError(t, af.Stop(context.Background(), localrt.StopOptions{Force: true, Clean: true}))

	assert.True(t, hasCall(cmd.calls, "down --timeout 0 --volumes --remove-orphans"), "expected a forced clean down, got %v", cmd.calls)
	// A clean stop also drops the per-project dependency image (best-effort).
	assert.True(t, hasCall(cmd.calls, "image rm --force "+builtImagePrefix), "expected an image rm, got %v", cmd.calls)
	assert.NoFileExists(t, composeFile)
}

// hasCall reports whether any recorded call contains substr.
func hasCall(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestFindProjectProbesBothEngines(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasPrefix(call, "docker ps") {
			return nil, errors.New("docker daemon not running")
		}
		if strings.HasPrefix(call, "podman ps") {
			return []byte("astro-demo-abc123\n"), nil
		}
		return nil, nil
	}}
	e := testEngine(t, cmd)

	conn, name := e.findProject(context.Background(), "/home/me/demo")
	assert.Equal(t, "podman", conn.bin)
	assert.Equal(t, "astro-demo-abc123", name)
	// Preferred engine (docker) was probed first.
	require.GreaterOrEqual(t, len(cmd.calls), 2)
	assert.True(t, strings.HasPrefix(cmd.calls[0], "docker ps"))
	assert.Contains(t, cmd.calls[0], workingDirLabel+"=/home/me/demo")
}

func TestReadStatusWithoutRecordIsStopped(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	st, err := e.ReadStatus(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, localrt.StateStopped, st.State)
}

func TestAttachRefusesForeignRecords(t *testing.T) {
	e := testEngine(t, &fakeCmd{})
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        localrt.ModeStandalone,
		PID:         123,
	}))
	_, err := e.Attach(project)
	assert.ErrorIs(t, err, ErrNotDockerMode)
}

func TestLogsStreamsParsedLines(t *testing.T) {
	logOutput := strings.Join([]string{
		`scheduler-1  | 2026-07-21T10:00:00.500Z scheduler heartbeat`,
		`api-server-1  | 2026-07-21T10:00:01.500Z listening on 8080`,
		``,
		`some engine banner`,
	}, "\n") + "\n"

	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	p := testPlan(t)
	af, name := startProject(t, e, p)

	cmd.output = func(call string) ([]byte, error) {
		if strings.Contains(call, workingDirLabel) {
			return []byte(name + "\n"), nil
		}
		return nil, nil
	}
	cmd.run = func(call string, s localrt.Stdio) error {
		if strings.Contains(call, "logs") {
			_, err := s.Out.Write([]byte(logOutput))
			return err
		}
		return nil
	}

	var lines []localrt.LogLine
	err := af.Logs(context.Background(), localrt.LogOptions{
		Follow: true,
		Tail:   10,
		OnLine: func(l localrt.LogLine) { lines = append(lines, l) },
	})
	require.NoError(t, err)

	logsCall := cmd.calls[len(cmd.calls)-1]
	assert.Contains(t, logsCall, "compose -p "+name+" logs --no-color --timestamps --follow --tail 10")

	require.Len(t, lines, 3)
	assert.Equal(t, "scheduler", lines[0].Component)
	assert.Equal(t, "scheduler heartbeat", lines[0].Text)
	assert.Equal(t, time.Date(2026, 7, 21, 10, 0, 0, 500e6, time.UTC), lines[0].Time.UTC())
	assert.Equal(t, "api-server", lines[1].Component)
	assert.Equal(t, "system", lines[2].Component)
	assert.Equal(t, "some engine banner", lines[2].Text)
}

func TestLogsRequiresExactlyOneSink(t *testing.T) {
	e := testEngine(t, &fakeCmd{})
	p := testPlan(t)
	af, _ := startProject(t, e, p)

	err := af.Logs(context.Background(), localrt.LogOptions{})
	assert.ErrorContains(t, err, "exactly one")
	err = af.Logs(context.Background(), localrt.LogOptions{Writer: os.Stderr, OnLine: func(localrt.LogLine) {}})
	assert.ErrorContains(t, err, "exactly one")
}

func TestRunExecsInScheduler(t *testing.T) {
	cmd := &fakeCmd{}
	e := testEngine(t, cmd)
	p := testPlan(t)
	af, name := startProject(t, e, p)
	cmd.output = func(call string) ([]byte, error) {
		if strings.Contains(call, workingDirLabel) {
			return []byte(name + "\n"), nil
		}
		return nil, nil
	}

	require.NoError(t, af.Run(context.Background(), []string{"airflow", "version"}, localrt.Stdio{}))
	assert.Equal(t, "docker compose -p "+name+" exec scheduler airflow version", cmd.calls[len(cmd.calls)-1])

	require.NoError(t, af.Shell(context.Background(), localrt.Stdio{}))
	assert.Equal(t, "docker compose -p "+name+" exec scheduler /bin/bash", cmd.calls[len(cmd.calls)-1])
}

func TestRunWhenNotRunning(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	p := testPlan(t)
	af, _ := startProject(t, e, p)

	err := af.Run(context.Background(), []string{"true"}, localrt.Stdio{})
	assert.ErrorContains(t, err, "not running")
}

func TestContainersGone(t *testing.T) {
	// A daemon that cannot be reached is not a confirmation that the project
	// is gone: `list --clean` must not delete a project on a blip.
	t.Run("both engines unreachable is not confirmed gone", func(t *testing.T) {
		cmd := &fakeCmd{output: func(string) ([]byte, error) {
			return nil, errors.New("cannot connect to the docker daemon")
		}}
		e := testEngine(t, cmd)
		gone, err := e.ContainersGone(context.Background(), "/home/me/demo")
		require.Error(t, err)
		assert.False(t, gone)
	})

	// One engine answering cleanly with no match confirms gone; the other
	// engine being absent (podman not installed) is normal, not a blocker.
	t.Run("a reachable engine with no match confirms gone", func(t *testing.T) {
		cmd := &fakeCmd{output: func(call string) ([]byte, error) {
			if strings.HasPrefix(call, "docker ps") {
				return nil, nil
			}
			return nil, errors.New("podman machine not running")
		}}
		e := testEngine(t, cmd)
		gone, err := e.ContainersGone(context.Background(), "/home/me/demo")
		require.NoError(t, err)
		assert.True(t, gone)
	})

	t.Run("a running container is not gone", func(t *testing.T) {
		cmd := &fakeCmd{output: func(call string) ([]byte, error) {
			if strings.HasPrefix(call, "docker ps") {
				return []byte("astro-demo\n"), nil
			}
			return nil, nil
		}}
		e := testEngine(t, cmd)
		gone, err := e.ContainersGone(context.Background(), "/home/me/demo")
		require.NoError(t, err)
		assert.False(t, gone)
	})
}
