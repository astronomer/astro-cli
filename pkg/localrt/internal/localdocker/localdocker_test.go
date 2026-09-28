package localdocker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// fakeCmd is a Commander that never touches a real daemon. Each call is
// recorded as "name arg arg..."; responses come from the hooks.
type fakeCmd struct {
	calls  []string
	output func(call string) ([]byte, error)
	run    func(call string, s rt.Stdio) error
	// delayFor, when set, makes Run block for the returned duration or until its
	// context is done, whichever comes first — which is what exec.CommandContext
	// does to a real subprocess. A plain time.Sleep here would outlive its own
	// deadline and so could not model a command being killed by one, which is
	// exactly what a test about competing deadlines needs.
	delayFor func(call string) time.Duration
	// env records the environment each Run was given, keyed by the call string.
	// Without it the environment argument was dropped on the floor, which left
	// the one thing that makes SecretEnv work — the values reaching the up, and
	// not reaching anything else — with no coverage at all.
	env map[string][]string
	// outputBounded records, per Output call, whether its context carried a
	// deadline. Booleans rather than the contexts themselves: holding a
	// context in a struct is the thing containedctx exists to refuse, and the
	// only question a test asks of one here is whether it was bounded at all.
	outputBounded []bool
}

// envFor returns the environment recorded for the first call containing substr.
func (f *fakeCmd) envFor(substr string) ([]string, bool) {
	for call, env := range f.env {
		if strings.Contains(call, substr) {
			return env, true
		}
	}
	return nil, false
}

func (f *fakeCmd) Output(ctx context.Context, _ []string, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	_, bounded := ctx.Deadline()
	f.outputBounded = append(f.outputBounded, bounded)
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	// Same contract Run honors, and for a sharper reason here: an engine that
	// accepts the call and then never answers is the failure this package's
	// probe deadline exists for, and without a hook that blocks, every test
	// about that deadline can only assert a deadline was SET — which passes
	// for an implementation that sets one the plumbing cannot act on.
	if f.delayFor != nil {
		if d := f.delayFor(call); d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	if f.output == nil {
		return nil, nil
	}
	return f.output(call)
}

// Run honors the context, because os/exec does: Cmd.Start returns ctx.Err()
// before spawning anything on a context that is already done. A fake that
// ignored it recorded calls no real process would have made, which hid whether
// cleanup paths detach from the caller's canceled context — the difference
// between a teardown that runs and one that silently does not.
func (f *fakeCmd) Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.env == nil {
		f.env = map[string][]string{}
	}
	f.env[call] = env
	if f.delayFor != nil {
		if d := f.delayFor(call); d > 0 {
			select {
			case <-time.After(d):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
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
	images := newStubImages()
	e := New(filepath.Join(t.TempDir(), "proxy"), nil, images)
	e.images = images
	e.cmd = cmd
	e.preferred = func() (engineConn, error) { return engineConn{bin: "docker"}, nil }
	e.connFor = func(bin string) engineConn { return engineConn{bin: bin} }
	e.portFree = func(string) bool { return true }
	e.allocPort = func() (string, error) { return "", errors.New("allocation not expected") }
	e.health = func(context.Context, []string, time.Duration) error { return nil }
	e.now = func() time.Time { return time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC) }
	e.ensureEngine = func(rt.Callbacks) error { return nil }
	e.composeAvail = func(context.Context, engineConn) error { return nil }
	e.startSession = func(string, int) error { return nil }
	return e
}

func testPlan(t *testing.T) rt.Plan {
	t.Helper()
	project := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(project, "dags"), 0o755))
	return rt.Plan{
		ProjectPath:    project,
		Mode:           rt.ModeDocker,
		AirflowVersion: "3.1-2",
		RequestedPort:  8081,
		Env:            map[string]string{"FOO": "bar"},
	}
}

// The docker engine chooses its hostname through the same shared seam the
// standalone one does, and has to hand it the routes store for the same
// reason: two projects in directories called the same thing would otherwise
// both ask for one name, and the second would start with no route while the
// URL kept answering for the first.
//
// A separate case per engine because the wiring is per engine — the shared
// rule's tests stay green when either call site drops the store.
func TestStartQualifiesAHostnameAnotherProjectHolds(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)

	// Held by this test's own PID, because AddRoute prunes routes whose
	// process is gone and a pruned fixture leaves nothing to collide with.
	contested := filepath.Base(p.ProjectPath) + proxy.LocalhostSuffix
	require.NoError(t, e.routes.AddRoute(&proxy.Route{
		Hostname:   contested,
		ProjectDir: t.TempDir(),
		Port:       "8080",
		PID:        os.Getpid(),
	}))

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.NotEqual(t, contested, rec.Hostname, "that name belongs to another project")
	assert.True(t, strings.HasPrefix(rec.Hostname, filepath.Base(p.ProjectPath)+"-"),
		"the qualified name should still say which project it is: %q", rec.Hostname)

	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	var mine *proxy.Route
	for i := range routes {
		if routes[i].ProjectDir == p.ProjectPath {
			mine = &routes[i]
		}
	}
	require.NotNil(t, mine, "the second project must get a route of its own")
	assert.Equal(t, rec.Hostname, mine.Hostname)

	// The incumbent keeps what it had, the same check the standalone twin
	// makes: a regression in which the newcomer evicts the first project
	// rather than stepping around it would otherwise be caught by one
	// engine's test and not the other's.
	incumbent, err := e.routes.GetRoute(contested)
	require.NoError(t, err)
	require.NotNil(t, incumbent, "the first project must keep its name")
	assert.NotEqual(t, p.ProjectPath, incumbent.ProjectDir)
}

func TestStartBringsProjectUp(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)

	var states []rt.State
	cb := rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }}

	af, err := e.Start(context.Background(), p, cb)
	require.NoError(t, err)
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateRunning}, states)

	// The up call ran through compose with the generated file, the project
	// dir pinned (findProject discovers by that label), and detached.
	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	composeFile := filepath.Join(stateDir, composeFileName)
	// Two calls. First `compose -p <name> ps -aq`, asking whether THIS compose
	// project already has containers (so a failed start knows whether the
	// cleanup is its to do) — scoped to the name the teardown would use, and
	// --all so stopped containers count. Then the up. The `astro dev` note makes
	// no call here: nothing is listening for lines, so there is no one to tell
	// (see legacydb.go, and TestAStartTellsAProjectArrivingFromAstroDev).
	require.Len(t, cmd.calls, 2)
	assert.Equal(t, "docker compose -p "+name+" ps -aq", cmd.calls[0],
		"the pre-flight probe must ask about the project the teardown would remove")
	assert.Equal(t, fmt.Sprintf("docker compose --file %s --project-directory %s --project-name %s up --detach --quiet-pull",
		composeFile, p.ProjectPath, name), cmd.calls[1])
	assert.FileExists(t, composeFile)

	// The state record captures what other tools need to reconnect.
	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, rt.ModeDocker, rec.Mode)
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
	assert.Equal(t, rt.StateRunning, st.State)
	assert.Equal(t, 8081, st.Port)
}

// An Airflow 2 project runs the components that release has, off the image the
// builder resolves for it. Which image that is belongs to pkg/imagebuild; what
// the engine does with the generation is here.
func TestStartAirflow2(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.AirflowVersion = "2.11.2"

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// The record says Airflow 2, which is how every other tool knows the
	// credentials this Airflow wants.
	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, "2", rec.AirflowMajor)

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
	require.NoError(t, err)
	assert.Contains(t, string(compose), "image: "+e.images.(*stubImages).airflow2Base)
	assert.Contains(t, string(compose), "webserver:")
	assert.NotContains(t, string(compose), "api-server:")
}

func TestStartSessionTiedRecordsOwnerPID(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	p := testPlan(t)
	p.StopWithSession = true

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
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

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	name, err := composeProjectName(p.ProjectPath)
	require.NoError(t, err)
	tag := builtImageTag(name)

	// The engine asked the builder for exactly one build, carrying the project's
	// dependencies and the per-project tag. What the builder then does with them —
	// writing requirements.txt, invoking docker build — is pkg/imagebuild's own
	// concern and is tested there; injecting the builder is what moved that line.
	images := e.images.(*stubImages)
	require.Len(t, images.requests, 1)
	assert.Equal(t, p.Dependencies, images.requests[0].Dependencies)
	assert.Equal(t, tag, images.requests[0].Tag)
	// The compose up runs the built image, not the bare runtime image.
	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
	require.NoError(t, err)
	assert.Contains(t, string(compose), "image: "+tag)
	// Literal rather than imagebuild.RuntimeImageRepo: an in-package test
	// importing pkg/imagebuild would close the cycle rt.ImageBuilder exists to
	// break, since imagebuild imports the localrt contract.
	assert.NotContains(t, string(compose), "astrocrpublic.azurecr.io/runtime")
	// The build context's contents (requirements.txt, packages.txt) are the
	// builder's concern and are checked in internal/imagebuild.
}

func TestStartOSPackagesTriggerBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Packages = []string{"libpq-dev", "build-essential"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// OS packages alone reach the builder, even with no Python dependencies —
	// whether that produces a layer is the builder's call.
	images := e.images.(*stubImages)
	require.Len(t, images.requests, 1)
	assert.Equal(t, p.Packages, images.requests[0].Packages)
	assert.Empty(t, images.requests[0].Dependencies)
}

func TestStartNoDependenciesSkipsBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t) // no Dependencies

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// cmd.calls says nothing now that the builder is injected — no `docker build`
	// reaches the fake Commander either way, so the old assertion passed
	// unconditionally. What the engine is answerable for is asking with nothing to
	// install, and then running whichever image it gets back.
	images := e.images.(*stubImages)
	require.Len(t, images.requests, 1)
	assert.Empty(t, images.requests[0].Dependencies)
	assert.Empty(t, images.requests[0].Packages)

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
	require.NoError(t, err)
	assert.Contains(t, string(compose), "image: "+images.base,
		"with nothing to install the builder returns the base image, and that is what must run")
}

func TestStartAirflowOnlyDepsSkipsBuild(t *testing.T) {
	// The base image provides Airflow; a manifest listing only Airflow (any
	// extras/pin) needs no build layer.
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.Dependencies = []string{"apache-airflow==3.1.*", "apache-airflow[celery]"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	// Whether apache-airflow-only deps produce a layer is imagebuild's rule (it
	// drops them, since the base image already provides Airflow) and is tested
	// there. The engine's part is not to second-guess it: the plan's dependencies
	// go to the builder verbatim.
	images := e.images.(*stubImages)
	require.Len(t, images.requests, 1)
	assert.Equal(t, p.Dependencies, images.requests[0].Dependencies)
}

func TestStartFailedDepInstallReturnsNamedError(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, run: func(call string, _ rt.Stdio) error {
		return nil
	}}
	e := testEngine(t, cmd)
	// A sentinel, not a message: asserting on a substring this test planted would
	// only prove the engine returns the stub's error verbatim. Whether a failed
	// install produces a NAMED error rather than an opaque "exit status 1" is
	// imagebuild's contract, tested there.
	buildFailed := errors.New("build blew up")
	e.images.(*stubImages).err = buildFailed
	p := testPlan(t)
	p.Dependencies = []string{"nonexistent-package-xyz"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.ErrorIs(t, err, buildFailed)
	// The build failed, so no containers start.
	assert.False(t, hasCall(cmd.calls, "up --detach"), "no up after a failed build, got %v", cmd.calls)
}

func TestStartBringsEngineUpFirst(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	var ensured bool
	e.ensureEngine = func(rt.Callbacks) error { ensured = true; return nil }
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, ensured, "the engine must be brought up before the start")
}

func TestStartFailsWhenEngineCannotStart(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.ensureEngine = func(rt.Callbacks) error { return errors.New("engine down") }
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	assert.ErrorContains(t, err, "engine down")
	assert.Empty(t, cmd.calls, "nothing may run when the engine cannot start")
}

func TestStartReportsMissingCompose(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.composeAvail = func(context.Context, engineConn) error { return ErrComposeMissing }
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
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
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, p.ProjectPath, gotProject)
	assert.Equal(t, os.Getpid(), gotPID)
}

func TestStartHealthFailureKeepsRecordAndRoute(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	e.health = func(context.Context, []string, time.Duration) error { return ErrHealthTimeout }
	p := testPlan(t)

	var errState error
	cb := rt.Callbacks{OnState: func(s rt.State, err error) {
		if s == rt.StateError {
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
	p.Mode = rt.ModeStandalone
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorContains(t, err, "standalone")

	// A generation neither template covers is refused before any container
	// starts.
	p = testPlan(t)
	p.AirflowVersion = "1.10.15"
	_, err = e.Start(context.Background(), p, rt.Callbacks{})
	assert.ErrorContains(t, err, "Airflow 2 or Airflow 3")
	assert.Empty(t, e.cmd.(*fakeCmd).calls, "no containers may start for a rejected plan")
}

func TestChoosePort(t *testing.T) {
	e := testEngine(t, &fakeCmd{})

	// Requested and free: taken as-is.
	got, err := e.choosePort(8081, defaultWebPort)
	require.NoError(t, err)
	assert.Equal(t, 8081, got)

	// No request, default free: the default.
	got, err = e.choosePort(0, defaultWebPort)
	require.NoError(t, err)
	assert.Equal(t, defaultWebPort, got)

	// Busy ports fall back to pool allocation.
	e.portFree = func(string) bool { return false }
	e.allocPort = func() (string, error) { return "15001", nil }
	got, err = e.choosePort(8081, defaultWebPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)
	got, err = e.choosePort(0, defaultWebPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)

	// A busy requested port never silently lands on the default.
	e.portFree = func(p string) bool { return p == "8080" }
	got, err = e.choosePort(8081, defaultWebPort)
	require.NoError(t, err)
	assert.Equal(t, 15001, got)
}

// startProject boots a project through the fake engine and returns its
// handle plus the compose project name.
func startProject(t *testing.T, e *Engine, p rt.Plan) (af rt.Airflow, composeProject string) {
	t.Helper()
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
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

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))

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

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	composeFile := filepath.Join(stateDir, composeFileName)
	require.FileExists(t, composeFile)

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{Force: true, Clean: true}))

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
	assert.Equal(t, rt.StateStopped, st.State)
}

func TestAttachRefusesForeignRecords(t *testing.T) {
	e := testEngine(t, &fakeCmd{})
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{
		ProjectPath: project,
		Mode:        rt.ModeStandalone,
		PID:         123,
	}))
	_, err := e.Attach(project)
	assert.ErrorIs(t, err, ErrNotDockerMode)
}

func TestRunWithContainersDownSaysHowToStartThem(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	project := t.TempDir()
	require.NoError(t, localstate.Save(localstate.Record{ProjectPath: project, Mode: rt.ModeDocker}))
	af, err := e.Attach(project)
	require.NoError(t, err)
	err = af.Run(context.Background(), []string{"pytest"}, rt.Stdio{})
	assert.ErrorIs(t, err, localstate.ErrNotRunning)
	assert.ErrorContains(t, err, "astro local start --docker")
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
	cmd.run = func(call string, s rt.Stdio) error {
		if strings.Contains(call, "logs") {
			_, err := s.Out.Write([]byte(logOutput))
			return err
		}
		return nil
	}

	var lines []rt.LogLine
	err := af.Logs(context.Background(), rt.LogOptions{
		Follow: true,
		Tail:   10,
		OnLine: func(l rt.LogLine) { lines = append(lines, l) },
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

	err := af.Logs(context.Background(), rt.LogOptions{})
	assert.ErrorContains(t, err, "exactly one")
	err = af.Logs(context.Background(), rt.LogOptions{Writer: os.Stderr, OnLine: func(rt.LogLine) {}})
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

	require.NoError(t, af.Run(context.Background(), []string{"airflow", "version"}, rt.Stdio{}))
	assert.Equal(t, "docker compose -p "+name+" exec scheduler airflow version", cmd.calls[len(cmd.calls)-1])

	require.NoError(t, af.Shell(context.Background(), rt.Stdio{}))
	assert.Equal(t, "docker compose -p "+name+" exec scheduler /bin/bash", cmd.calls[len(cmd.calls)-1])
}

func TestRunWhenNotRunning(t *testing.T) {
	e := testEngine(t, &fakeCmd{output: noProjects})
	p := testPlan(t)
	af, _ := startProject(t, e, p)

	err := af.Run(context.Background(), []string{"true"}, rt.Stdio{})
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

// dockerfilePlan is a project that declared its own Dockerfile: the manifest's
// dependency fields are still populated, because a manifest carries them
// whichever tier it chose, and the engine has to hand them over untouched
// rather than act on them.
func dockerfilePlan(t *testing.T) rt.Plan {
	t.Helper()
	p := testPlan(t)
	// An Astro Runtime base, because Start refuses anything else: the compose
	// file runs Airflow as the `astro` user and reads its service set off the
	// runtime tag, so another base cannot come up. See refusebase_test.go.
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN echo hi\n"), 0o600))
	p.Dockerfile = "Dockerfile"
	p.Dependencies = []string{"pandas"}
	p.Packages = []string{"libaio"}
	return p
}

func TestStartWithProjectDockerfileBuildsThatFile(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := dockerfilePlan(t)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.Len(t, images.requests, 1)
	req := images.requests[0]
	assert.Equal(t, filepath.Join(p.ProjectPath, "Dockerfile"), req.Dockerfile,
		"the engine must resolve the plan's project-relative path")
	assert.Equal(t, p.ProjectPath, req.Context,
		"a multi-stage build COPYs from the repo, so the project is the context")
	assert.Empty(t, req.BaseImage, "the project's own FROM decides the base")
}

func TestStartHandsBuildSecretsToTheDockerfileBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := dockerfilePlan(t)
	p.BuildSecrets = []string{"id=netrc,env=NETRC_CONTENT", "id=pip,src=/tmp/pip.conf"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.Len(t, images.requests, 1)
	assert.Equal(t, p.BuildSecrets, images.requests[0].Secrets)
}

// The builder keeps the netrc secret for a generated build, which the runtime
// image's install step mounts, so the engine hands the secrets on.
func TestStartPassesBuildSecretsToAGeneratedBuild(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := testPlan(t)
	p.Dependencies = []string{"pandas"}
	p.BuildSecrets = []string{"id=netrc,env=NETRC_CONTENT"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.Len(t, images.requests, 1)
	assert.Equal(t, p.BuildSecrets, images.requests[0].Secrets)
}

// Resolving a base image we would discard turns a working start into a
// dependency on the version service, which is exactly what an air-gapped or
// offline Dockerfile project should not need.
func TestStartWithProjectDockerfileSkipsBaseImageResolution(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)

	_, err := e.Start(context.Background(), dockerfilePlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Zero(t, images.runtimeImageCalls, "Dockerfile mode must not resolve a runtime image")
}

// A plan's runtime build reaches the base-image resolution, which picks that
// build instead of the newest one of the series.
func TestStartPassesThePlansRuntimeToTheBaseResolution(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := testPlan(t)
	p.Runtime = "3.3-8"

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, []string{"3.3-8"}, images.runtimes)
}

// The generated path is unchanged: it still resolves a base and still gets the
// manifest's dependencies.
func TestStartWithoutProjectDockerfileStillResolvesBase(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)
	p := testPlan(t)
	p.Dependencies = []string{"pandas"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, 1, images.runtimeImageCalls)
	require.Len(t, images.requests, 1)
	req := images.requests[0]
	assert.Empty(t, req.Dockerfile)
	assert.Empty(t, req.Context)
	assert.Equal(t, images.base, req.BaseImage)
	assert.Equal(t, []string{"pandas"}, req.Dependencies)
}

// AirflowVersion is honored with a Dockerfile no base can be read from.
//
// One path reaches this now. A base that reads as something other than Astro
// Runtime is refused outright, so the pin decides the service set only when the
// parser could not answer at all — a file with no FROM here. That is left to
// imagebuild.Build on purpose, which reports it with the path and the reason;
// meanwhile the compose file still has to be written for some generation, and
// the pin is the only statement there is.
//
// An Airflow 2 project is the case that would break: it has no dag-processor,
// and declaring one fails the whole compose merge.
func TestStartWithProjectDockerfileStillUsesPlanGeneration(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := dockerfilePlan(t)
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"),
		[]byte("RUN echo no FROM here\n"), 0o600))
	p.AirflowVersion = "2.10.5"

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
	require.NoError(t, err)
	assert.NotContains(t, string(compose), "dag-processor",
		"Airflow 2 has no dag-processor service")
}

// --- SecretEnv reaches the containers, and nothing else ---

// The helpers are unit-tested in compose_test.go; this is the wiring that makes
// them matter. Delete the extraEnv line in Start and every one of those tests
// still passes while no secret ever reaches a container, which is why this asserts
// on the engine rather than the helpers.
func TestStartHandsSecretValuesToTheComposeUp(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.SecretEnv = map[string]string{"AIRFLOW_CONN_DB": "postgres://u:pw@h/db"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	env, ok := cmd.envFor("compose")
	require.True(t, ok, "no compose call was recorded")
	assert.Contains(t, env, "AIRFLOW_CONN_DB=postgres://u:pw@h/db",
		"the up must carry the values for the declarations the file makes, or the variable is declared and never resolved")
}

// The engine connection has to win a collision, because os/exec keeps the last
// duplicate and a compose command talking to the wrong daemon is unrecoverable
// from a UI — where a secret that loses simply does not reach the container.
func TestStartLetsTheEngineConnectionWinOverSecretEnv(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.preferred = func() (engineConn, error) {
		return engineConn{bin: "docker", env: []string{"DOCKER_HOST=unix:///real.sock"}}, nil
	}
	p := testPlan(t)
	p.SecretEnv = map[string]string{"DOCKER_HOST": "unix:///attacker.sock"}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	env, ok := cmd.envFor("compose")
	require.True(t, ok)
	var lastHost string
	for _, kv := range env {
		if strings.HasPrefix(kv, "DOCKER_HOST=") {
			lastHost = kv // os/exec resolves duplicates to the last one
		}
	}
	assert.Equal(t, "DOCKER_HOST=unix:///real.sock", lastHost,
		"a plan must not be able to retarget the compose invocation itself")
}

// Stop runs with no --file, so it never reads the declarations and has no reason
// to carry the values. Handing them over anyway would widen where they travel for
// nothing.
func TestStopDoesNotCarrySecretValues(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)
	p.SecretEnv = map[string]string{"AIRFLOW_CONN_DB": "postgres://u:pw@h/db"}

	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	cmd.env = nil // forget the start
	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))

	for call, env := range cmd.env {
		for _, kv := range env {
			assert.NotContains(t, kv, "pw@h",
				"a secret value reached %q, which does not read the file that declares it", call)
		}
	}
}

// A start that fails after compose created anything has to take it back down.
//
// Nothing else can: every stop path reaches a project through its state record,
// and that record is written after the up returns. So containers from a failed
// start are invisible to `astro local stop` (which fails with "no local Airflow
// is recorded for this project"), untouched by `astro local list --clean` (which
// prunes records, not containers), and holding the ports the next attempt wants.
func TestStartRemovesContainersWhenTheUpFails(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)
	p := testPlan(t)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.Error(t, err)

	name, nerr := composeProjectName(p.ProjectPath)
	require.NoError(t, nerr)
	var down string
	for _, c := range cmd.calls {
		if strings.Contains(c, " down") {
			down = c
		}
	}
	require.NotEmpty(t, down, "a failed up must be followed by a down; calls were %v", cmd.calls)
	assert.Contains(t, down, "--project-name "+name)
	// No --file, matching Stop: down works from container labels alone, and the
	// generated file may already be gone.
	assert.NotContains(t, down, "--file")

	// Volumes are deliberately left. A second start against an existing project
	// is exactly when this fires, so --volumes here would delete the database a
	// previous successful run filled — losing a developer's local Airflow data to
	// a failed start is worse than leaving a volume that costs only disk.
	assert.NotContains(t, down, "--volumes")
	// The other half of that argument: postgres needs a graceful SIGTERM to flush
	// before it exits, on a volume being preserved. "The start already failed,
	// kill it fast" is a plausible edit, and Stop's Force path right next door
	// spells exactly that.
	assert.Contains(t, down, "--timeout "+strconv.Itoa(gracefulStopTimeout))

	// The premise the rollback exists for: nothing else can reach these
	// containers, because there is no record and no route. If a later change
	// wrote the record before the up — a plausible refactor that would arguably
	// obsolete this function — the assertions above would all still pass.
	if _, err := localstate.Load(p.ProjectPath); !errors.Is(err, localstate.ErrNotRunning) {
		t.Errorf("a failed start must leave no state record, got %v", err)
	}
}

// A rollback must not remove containers this start did not create.
//
// `down` takes out everything carrying the project label, and it cannot tell
// what the failed up made from what was already running. That case is reachable:
// "already running" is decided from the state record, the record lives in a
// cache directory anything may clear, and a start can then get as far as the up
// and fail there without touching a container — a registry timeout, a daemon
// that went away. Removing a healthy Airflow somebody is using would be a worse
// outcome than the orphan this function exists to prevent.
func TestStartLeavesPreexistingContainersAloneWhenTheUpFails(t *testing.T) {
	cmd := &fakeCmd{}
	// The probe answers with a container id, i.e. this project already has one.
	cmd.output = func(string) ([]byte, error) { return []byte("abc123\n"), nil }
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)
	p := testPlan(t)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.Error(t, err)

	for _, c := range cmd.calls {
		assert.NotContains(t, c, " down", "a pre-existing project must not be torn down; calls were %v", cmd.calls)
	}
}

// The cleanup has to survive the cancellation that caused the failure.
//
// The failure this exists for is a wedged daemon or a stalled pull, and callers
// run starts under a deadline — the desktop wraps every docker action in one. So
// the up frequently fails BECAUSE the context is already done, and os/exec
// refuses to spawn a process on a canceled context: reusing the caller's ctx
// would make the teardown a silent no-op in exactly the case it was written for.
func TestRollbackRunsEvenWhenTheStartContextIsCancelled(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	p := testPlan(t)

	ctx, cancel := context.WithCancel(context.Background())
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			// What a killed compose looks like: the context is done and the
			// command failed because of it.
			cancel()
			return ctx.Err()
		}
		return nil
	}

	_, err := e.Start(ctx, p, rt.Callbacks{})
	require.Error(t, err)

	var down bool
	for _, c := range cmd.calls {
		if strings.Contains(c, " down") {
			down = true
		}
	}
	assert.True(t, down, "the teardown must not inherit the canceled context; calls were %v", cmd.calls)
}

// A failed start reports the error state, like the health-timeout path does.
// Without it a consumer driving a UI off the event stream sees "starting" and
// then silence, and has to treat a stream that stopped as a failure.
func TestStartEmitsErrorStateWhenTheUpFails(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)
	p := testPlan(t)

	var states []rt.State
	cb := rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }}
	if _, err := e.Start(context.Background(), p, cb); err == nil {
		t.Fatal("want the start to fail")
	}
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateError}, states)
}

// A cleanup failure has to reach the caller even with no callbacks. The report
// used to be gated on cb.OnLine, which the contract makes optional, so a
// consumer passing rt.Callbacks{} could leak containers with no signal anywhere.
func TestRollbackFailureIsReportedWithoutCallbacks(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		if strings.Contains(call, " down") {
			return errors.New("daemon gone")
		}
		return nil
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "starting project containers", "the start failure must stay in the message")
	assert.Contains(t, err.Error(), "cleaning up", "the leak must be visible without callbacks")
}

// The container logs are the only diagnosis for the most common docker-start
// failure — every service waits on the one-shot migration, so a failing
// `airflow db migrate` surfaces as "dependency failed to start" and nothing
// else. They have to be read before the containers are removed.
func TestRollbackReportsLogsBeforeRemovingContainers(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{OnLine: func(rt.LogLine) {}})
	require.Error(t, err)

	logsAt, downAt := -1, -1
	for i, c := range cmd.calls {
		if strings.Contains(c, " logs ") && logsAt < 0 {
			logsAt = i
		}
		if strings.Contains(c, " down") && downAt < 0 {
			downAt = i
		}
	}
	require.GreaterOrEqual(t, logsAt, 0, "logs were never read; calls were %v", cmd.calls)
	require.GreaterOrEqual(t, downAt, 0, "containers were never removed; calls were %v", cmd.calls)
	assert.Less(t, logsAt, downAt, "logs must be read before the containers are removed")
}

// A stopped container counts as pre-existing.
//
// The probe used to ask the working-dir label with plain `ps`, which lists only
// running containers — so a project someone had `compose stop`ped, or one an
// earlier crash left in `created`, read as "nothing here" and was removed by the
// guard meant to protect it. `compose -p <name> ps -aq` is what answers the
// question the guard actually asks.
func TestStartLeavesStoppedPreexistingContainersAlone(t *testing.T) {
	cmd := &fakeCmd{}
	// A stopped container still has an id, which is the whole point.
	cmd.output = func(string) ([]byte, error) { return []byte("stopped-id\n"), nil }
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	for _, c := range cmd.calls {
		assert.NotContains(t, c, " down", "a stopped pre-existing project must not be torn down; calls were %v", cmd.calls)
	}
}

// A probe that cannot answer must not license a teardown.
//
// This is the branch the reasoning leans on hardest and it had no test: with the
// engine unreachable we do not know what is there, and the cost of guessing
// "nothing" is deleting a running Airflow, while the cost of guessing "something"
// is an orphan — the state this whole path improves on.
func TestStartSkipsCleanupWhenTheProbeFails(t *testing.T) {
	cmd := &fakeCmd{}
	cmd.output = func(string) ([]byte, error) { return nil, errors.New("daemon gone") }
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	for _, c := range cmd.calls {
		assert.NotContains(t, c, " down", "an unanswerable probe must not license a teardown; calls were %v", cmd.calls)
	}
	// And the user is told where to look, since we are leaving containers behind.
	assert.Contains(t, err.Error(), "compose project", "the error should name the project so it can be cleaned up by hand")
}

// Every failure exit reports StateError, not just the two that were noticed.
// The image build is the slowest and most failure-prone step in a start, and it
// used to end the event stream on "starting".
func TestStartEmitsErrorStateFromAnEarlyFailure(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	// The image build is the step this is about: slow, failure-prone, and it used
	// to end the stream on "starting".
	images := newStubImages()
	images.err = errors.New("build failed")
	e.images = images

	var states []rt.State
	cb := rt.Callbacks{OnState: func(s rt.State, _ error) { states = append(states, s) }}
	if _, err := e.Start(context.Background(), testPlan(t), cb); err == nil {
		t.Fatal("want the start to fail")
	}
	assert.Equal(t, []rt.State{rt.StateStarting, rt.StateError}, states)
}

// With no line callback the diagnosis has nowhere to stream, so it rides the
// error instead. Otherwise the container holding the only explanation is removed
// and its output is gone, leaving "exit status 1" and nothing else.
func TestFailedStartCarriesContainerOutputWhenNothingIsStreaming(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, s rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		if strings.Contains(call, " logs ") && s.Out != nil {
			_, _ = s.Out.Write([]byte("db-migration-1  | 2026-08-25T00:00:00.000000000Z Traceback: boom\n"))
		}
		return nil
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Traceback: boom", "the container output must survive in the error")
	assert.Contains(t, err.Error(), "db-migration", "and name the service that said it")
}

// The diagnostic read must not be able to eat the teardown's budget.
//
// This is the same defect as reusing a canceled context, one level in. The log
// capture runs first; when it shared a single rollback deadline, a wedged daemon
// blocked it until that deadline expired, and os/exec then refuses to spawn the
// `down` at all — so the containers leaked and the caller was told only that
// cleanup had timed out.
//
// Both budgets are shrunk so the arrangement is what decides the outcome: the
// capture is given less time than it needs, and the teardown needs what is left.
func TestLogCaptureCannotStarveTheTeardown(t *testing.T) {
	prevRollback, prevCapture := rollbackTimeout, logCaptureTimeout
	rollbackTimeout, logCaptureTimeout = 300*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { rollbackTimeout, logCaptureTimeout = prevRollback, prevCapture })

	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up ") {
			return errors.New("boom")
		}
		return nil
	}
	// A wedged daemon: the log read would block far longer than either budget.
	cmd.delayFor = func(call string) time.Duration {
		if strings.Contains(call, " logs ") {
			return 5 * time.Second
		}
		return 0
	}
	e := testEngine(t, cmd)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{OnLine: func(rt.LogLine) {}})
	require.Error(t, err)

	var down bool
	for _, c := range cmd.calls {
		if strings.Contains(c, " down") {
			down = true
		}
	}
	assert.True(t, down, "the teardown must still run after the log capture times out; calls were %v", cmd.calls)
}

// A declared Dockerfile in a subdirectory resolves and builds.
//
// The refusal tests above pass whether or not a subdirectory path resolves,
// because Stat failing is what they assert — so without this the positive case
// was untested, and the interesting platform is Windows. The manifest carries a
// slash-separated path (see the field's doc), filepath.Join turns it into
// C:\proj\docker/Dockerfile there, and mixed separators are only fine because
// the Windows API accepts both. CI runs this sub-module on Windows via
// scripts/test-submodules.sh, so that is asserted rather than assumed.
func TestStartResolvesADeclaredDockerfileInASubdirectory(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	images, ok := e.images.(*stubImages)
	require.True(t, ok)

	p := testPlan(t)
	require.NoError(t, os.MkdirAll(filepath.Join(p.ProjectPath, "docker"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "docker", "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN echo hi\n"), 0o600))
	p.Dockerfile = "docker/Dockerfile"

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	require.Len(t, images.requests, 1)
	assert.Equal(t, filepath.Join(p.ProjectPath, "docker", "Dockerfile"), images.requests[0].Dockerfile)
	assert.Equal(t, p.ProjectPath, images.requests[0].Context,
		"the context stays the project even when the file is deeper, or a COPY of anything above it breaks")
}

// The compose service set follows the declared Dockerfile's base, not the pin.
//
// A conversion writes the declaration itself and may have DEFAULTED the pin, so
// an Airflow 2 Dockerfile beside `airflow = "3.1"` is a real shape. Taking the
// pin built the AF2 file while emitting the AF3 service set (api-server,
// dag-processor) and the AF3 db command — a stack that cannot come up.
//
// Astro Desktop fixed this in its own plan builder; this asserts the CLI's, so
// the two tools agree rather than the divergence moving.
func TestStartReadsTheGenerationFromTheDeclaredDockerfile(t *testing.T) {
	for _, tc := range []struct {
		name, pin, body, wantMajor string
	}{
		{
			name: "pin says 3, file is airflow 2",
			pin:  "3.1",
			body: "FROM astrocrpublic.azurecr.io/runtime:12.1.0\nRUN echo hi\n",
			// runtime 12.x is an Airflow 2 image.
			wantMajor: "2",
		},
		{
			name:      "pin says 2, file is airflow 3",
			pin:       "2",
			body:      "FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN echo hi\n",
			wantMajor: "3",
		},
		{
			name:      "multi-stage takes the final stage",
			pin:       "2",
			body:      "FROM python:3.12-slim AS builder\nFROM astrocrpublic.azurecr.io/runtime:3.1-2\n",
			wantMajor: "3",
		},
		{
			// Astronomer publishes more than one image name. This one is
			// accepted because of where it comes from, not what it is called,
			// and the generation is then read from its tag like any other —
			// matching on "runtime" appearing in the name instead would accept
			// the image and then ignore what it says.
			name:      "an astronomer image whose name is not runtime",
			pin:       "3.1",
			body:      "FROM quay.io/astronomer/ap-airflow:2.5.1\nRUN echo hi\n",
			wantMajor: "2",
		},
		// A non-runtime base used to appear here, falling back to the pin.
		// Start refuses that file now rather than writing a compose file for an
		// image it cannot run, so the case lives in refusebase_test.go.
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &fakeCmd{output: noProjects}
			e := testEngine(t, cmd)

			p := testPlan(t)
			p.AirflowVersion = tc.pin
			p.Dockerfile = "Dockerfile"
			require.NoError(t, os.WriteFile(filepath.Join(p.ProjectPath, "Dockerfile"), []byte(tc.body), 0o600))

			_, err := e.Start(context.Background(), p, rt.Callbacks{})
			require.NoError(t, err)

			// Asserted through the compose file rather than an internal, because
			// the service set IS the consequence: Airflow 2 has no dag-processor,
			// and declaring one fails the whole compose merge.
			stateDir, err := rt.StateDir(p.ProjectPath)
			require.NoError(t, err)
			compose, err := os.ReadFile(filepath.Join(stateDir, composeFileName))
			require.NoError(t, err)
			if tc.wantMajor == "3" {
				assert.Contains(t, string(compose), "dag-processor",
					"an Airflow 3 stack has a dag-processor")
			} else {
				assert.NotContains(t, string(compose), "dag-processor",
					"an Airflow 2 stack has none, and declaring one fails the merge")
			}
		})
	}
}
