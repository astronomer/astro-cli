// Package localdocker runs local Airflow in containers: the docker-mode
// implementation of the pkg/localrt Airflow contract. It ports
// the worthwhile ~25% of v1's airflow/docker.go — compose generation, port
// allocation, health waiting, log streaming — onto the v2 plan/state/proxy
// seams, shelling out to the engine's compose plugin instead of linking
// the compose library.
//
// It lives under pkg/localrt/internal/, so it is reachable only from inside that
// module: both consumers enter through localrt.Runtime instead. That is the point
// of the arrangement — the dispatch, the start lock, and the refuse-a-live-start
// rules are shared rather than reimplemented per consumer.
//
// Per the layer rules, nothing here prints: progress flows through
// rt.Callbacks and errors return typed.
package localdocker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localprune"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localshared"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

const (
	defaultWebPort      = 8080
	defaultPostgresPort = 5432
	// execService is the container Run and Shell exec into: the scheduler
	// has the full Airflow env and reaches the metadata DB (same choice as
	// desktop's DockerExecCommand).
	execService = "scheduler"
	// stateDirPerm is owner-only, matching the record store.
	stateDirPerm = 0o700
	// gracefulStopTimeout is how long compose waits on SIGTERM before
	// killing; Stop with Force uses 0.
	gracefulStopTimeout = 10
)

// ErrNotDockerMode reports a record this engine does not own.
var ErrNotDockerMode = errors.New("this project's local Airflow is not running in docker mode")

// Engine implements docker mode. The function fields are seams: production
// values come from New, tests replace them to keep every docker interaction
// fake.
type Engine struct {
	routes *proxy.Store
	// daemon is the reverse-proxy lifecycle: started after a route lands so
	// <name>.localhost resolves, reaped when the last route goes. Nil on
	// Windows and in tests, where localshared.EnsureDaemon/ReapDaemon no-op.
	daemon rt.ProxyDaemon

	// images builds the runtime image. Injected rather than constructed: see
	// rt.ImageBuilder for why localrt cannot import pkg/imagebuild directly.
	images rt.ImageBuilder

	cmd       Commander
	preferred func() (engineConn, error)
	connFor   func(bin string) engineConn
	portFree  func(port string) bool
	allocPort func() (string, error)
	health    func(ctx context.Context, urls []string, timeout time.Duration) error
	now       func() time.Time

	// ensureEngine brings a stopped engine daemon/machine up before the start
	// touches it (LOCAL engine auto-start). composeAvail checks the Compose v2
	// plugin is present. startSession spawns the detached session watcher for
	// --stop-with-session. All three are seams so tests never touch a daemon.
	ensureEngine func(cb rt.Callbacks) error
	composeAvail func(ctx context.Context, conn engineConn) error
	startSession func(projectPath string, parentPID int) error

	healthTimeout time.Duration
}

// SetHealthTimeout bounds how long a start waits for the api-server to answer.
//
// A setter rather than another parameter on New, which already takes three and
// would take a fourth that almost every caller leaves at its default. See
// localrt.Config.HealthTimeout for why this is configurable at all.
func (e *Engine) SetHealthTimeout(d time.Duration) {
	// Guarded here rather than only at the caller: a zero would make every
	// context expire on creation, so every start would fail instantly having
	// brought the containers all the way up. The rule that non-positive means
	// "keep the default" belongs with the field it protects.
	if d > 0 {
		e.healthTimeout = d
	}
}

// New builds the production engine. routesDir is where pkg/proxy keeps
// routes.json (~/.astro/proxy); the composition root supplies it because
// this package must not read config.
func New(routesDir string, daemon rt.ProxyDaemon, images rt.ImageBuilder) *Engine {
	s := proxy.NewStore(routesDir, proxy.WithRouteLiveness(localprune.RouteAlive))
	e := &Engine{
		routes:        s,
		daemon:        daemon,
		images:        images,
		cmd:           execCommander{},
		preferred:     resolvePreferredEngine,
		connFor:       connFor,
		portFree:      proxy.IsPortAvailable,
		allocPort:     s.AllocatePort,
		health:        waitHealthy,
		now:           time.Now,
		startSession:  spawnSessionWatcher,
		healthTimeout: defaultHealthTimeout,
	}
	e.ensureEngine = func(cb rt.Callbacks) error { return ensureEngineUp(cb, e.now) }
	e.composeAvail = e.probeCompose
	return e
}

// planMajor is the Airflow generation the compose file has to describe: read
// off the declared Dockerfile where there is one, and off the pin otherwise.
//
// The pin alone was wrong for exactly the projects the declared tier is for. A
// conversion writes the declaration itself and may have DEFAULTED the pin, so a
// Dockerfile on an Airflow 2 base can sit beside `airflow = "3.1"` — and major
// decides the compose service set (Airflow 2 has no api-server or
// dag-processor) and the db command. Building the AF2 file while emitting the
// AF3 service set is a stack that cannot come up.
//
// Astro Desktop fixed this in its own plan builder first; this is the same bug
// in the CLI's, and leaving it would have relocated the divergence the declared
// tier exists to end rather than closing it.
//
// A file that cannot be read falls back to the pin: imagebuild.Build reports
// that failure properly a moment later, and guessing here would put the wrong
// services in the compose file on the way to a better message.
func planMajor(airflowVersion, declared string) string {
	pinned := airflowMajor(airflowVersion)
	if declared == "" {
		return pinned
	}
	from, tag, err := airflowrt.ParseDockerfileAt(declared)
	if err != nil || !strings.Contains(from, "runtime") {
		return pinned
	}
	baseTag, _ := airflowrt.ParseRuntimeTagPython(tag)
	if airflowrt.IsRuntime3(baseTag) {
		return "3"
	}
	return "2"
}

// Start brings the project's compose stack up and waits for Airflow to be
// healthy. The state record is written and the proxy route registered as
// soon as the containers are up, so status/stop/logs work even when the
// health wait fails or is interrupted.

func (e *Engine) Start(ctx context.Context, p rt.Plan, cb rt.Callbacks) (af rt.Airflow, err error) {
	if p.Mode != rt.ModeDocker {
		return nil, fmt.Errorf("localdocker got a %q plan", p.Mode)
	}
	projectPath, err := filepath.Abs(p.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", p.ProjectPath, err)
	}
	rt.OnState(cb, rt.StateStarting, nil)
	// Every failure from here on reports StateError, once, with whatever error is
	// actually returned. Emitting it per-site was a fix at the wrong depth: it
	// covered the two exits that had been noticed while thirteen others — the
	// image build, the port allocation, the compose write, the record save —
	// ended the stream on "starting" and left a consumer to treat silence as
	// failure. A deferred emit on the named return cannot drift as exits are
	// added, and it reports the joined error rather than a prefix of it.
	defer func() {
		if err != nil {
			rt.OnState(cb, rt.StateError, err)
		}
	}()

	// Bring a stopped engine up first, so `start --docker` with the daemon
	// down recovers instead of failing on the first compose call.
	if err := e.ensureEngine(cb); err != nil {
		return nil, err
	}

	if e.images == nil {
		return nil, errors.New("docker mode needs an image builder: set Images on localrt.Config")
	}
	// Resolve the base image only for a generated build. A project that declared
	// its own Dockerfile has already said what it builds on, and asking the
	// version service for a base we would not use turns a working start into a
	// network dependency.
	var image string
	if p.Dockerfile == "" {
		// The resolution takes a context because an Airflow 2 base image is a
		// lookup against the version service, not a tag built from the pin. It
		// also validates the generation, which everything below reads off the
		// plan.
		var err error
		if image, err = e.images.RuntimeImage(ctx, p.AirflowVersion); err != nil {
			return nil, err
		}
	}
	// Resolved once, and used for both the build and the generation below. The
	// pairing of Dockerfile with Context is only correct together, and doing it
	// twice in one function is how they drift.
	declared := ""
	if p.Dockerfile != "" {
		// FromSlash because the manifest carries a slash-separated path (see the
		// field's doc) and this may be Windows.
		declared = filepath.Join(projectPath, filepath.FromSlash(p.Dockerfile))
	}

	major := planMajor(p.AirflowVersion, declared)
	hostname, err := localshared.PlanHostname(p, projectPath)
	if err != nil {
		return nil, err
	}
	name, err := composeProjectName(projectPath)
	if err != nil {
		return nil, err
	}
	webPort, err := e.choosePort(p.RequestedPort, defaultWebPort)
	if err != nil {
		return nil, err
	}
	pgPort, err := e.choosePort(0, defaultPostgresPort, webPort)
	if err != nil {
		return nil, err
	}

	conn, err := e.preferred()
	if err != nil {
		return nil, err
	}
	// A missing Compose v2 plugin otherwise surfaces as an opaque "exit status
	// 125"; catch it here with an actionable message before any compose call.
	if err := e.composeAvail(ctx, conn); err != nil {
		return nil, err
	}

	// Before anything is created: a project arriving from `astro dev` has its
	// metadata database in a volume this runtime's compose project cannot name,
	// and would otherwise come up empty. See legacydb.go. Deliberately ahead of
	// the image build, so a project that is going to be told it cannot be
	// migrated hears it now rather than after a long build.
	e.adoptLegacyMetadataDB(ctx, conn, projectPath, name, major, cb)

	stateDir, err := e.stateDir(projectPath)
	if err != nil {
		return nil, err
	}
	// Build the image the services run. With a project Dockerfile that file is
	// the build, run against the project as its context. Otherwise install the
	// project's dependencies and OS packages into a layer over the runtime image
	// so docker mode matches standalone: the builder drops apache-airflow (the
	// base image already provides it) and, with nothing else to install, builds
	// nothing and hands back the runtime image as-is.
	build := rt.BuildRequest{
		WorkDir:      stateDir,
		BaseImage:    image,
		Tag:          builtImageTag(name),
		Dependencies: p.Dependencies,
		Packages:     p.Packages,
		Bin:          conn.bin,
		Env:          conn.env,
	}
	if declared != "" {
		// Existence is NOT checked here: imagebuild.Build does it, and that is
		// where this path, the deploy path and the package path meet. Two copies
		// of that guard would be two messages for one mistake.
		build.Dockerfile = declared
		build.Context = projectPath
	}
	if image, err = e.images.Build(ctx, build, cb); err != nil {
		return nil, err
	}

	env := airflowEnv(name, webPort, major, p.Env, p.SecretEnv)
	composePath, err := e.writeComposeFile(stateDir, composeInput{
		ProjectName:   name,
		Image:         image,
		PostgresImage: postgresImage,
		WebPort:       webPort,
		PostgresPort:  pgPort,
		Env:           env,
		PassEnv:       passEnv(p.PassthroughEnv, p.SecretEnv, env),
		Mounts:        projectMounts(projectPath),
		DBCommand:     dbCommand(major),
		Services:      airflowServices(major),
	})
	if err != nil {
		return nil, err
	}

	// Whether THIS compose project already has containers, asked before the up so
	// a failure can tell what it created from what it merely found. Without it a
	// rollback is indiscriminate: `down` removes everything carrying the project
	// label, and a start can fail without touching a container (a registry
	// timeout, a daemon that went away), which would tear down a healthy Airflow
	// somebody is using. That is reachable, because "already running" is decided
	// from the state record and the record lives in a cache directory anything
	// may clear.
	//
	// Scoped to the project name the teardown itself uses, and with --all, which
	// is what makes the answer the right one. An earlier version asked the
	// working-dir label with plain `ps`: that missed stopped containers (a
	// `compose stop`, an earlier crash), so they read as "nothing here" and were
	// destroyed by the very guard meant to protect them — and it refused to clean
	// up when it found a FOREIGN project in the directory (v1's containers, the
	// desktop's), which `down --project-name` provably cannot touch.
	//
	// A probe that ERRORS counts as "something might be there". The cost of being
	// wrong that way is an orphan, which is the state this function improves on;
	// the cost the other way is deleting a running Airflow.
	existing, probeErr := e.projectContainers(ctx, conn, name)
	mayCleanUp := probeErr == nil && existing == 0

	// The values for the declarations passEnv wrote. Only on the up: it is the one
	// command that creates containers, and compose bakes the resolved value in at
	// creation. Stop and down are run with no --file, so they never read the
	// declarations at all — see passEnv.
	up := composeLine{
		conn:       conn,
		file:       composePath,
		name:       name,
		projectDir: projectPath,
		extraEnv:   secretEnviron(p.SecretEnv),
	}
	if err := e.bringUp(ctx, &up, mayCleanUp, cb); err != nil {
		return nil, err
	}

	rec, err := e.publish(projectPath, name, hostname, major, webPort, pgPort, p, cb)
	if err != nil {
		return nil, err
	}

	if err := e.health(ctx, healthURLs(webPort, major), e.healthTimeout); err != nil {
		rt.OnState(cb, rt.StateError, err)
		return nil, err
	}
	// The route landed before the health wait so status/stop worked during it;
	// start the daemon now that Airflow answers, so <name>.localhost resolves.
	localshared.EnsureDaemon(e.daemon, cb, e.now(), rec.Hostname)
	rt.OnState(cb, rt.StateRunning, nil)
	return &airflow{eng: e, rec: rec}, nil
}

// bringUp creates the containers, and cleans up after itself when it cannot.
//
// Its own function because Start is already at the complexity limit, and because
// the decision it carries — whether a failure is this start's to clean up — is
// worth reading in one place.
func (e *Engine) bringUp(ctx context.Context, up *composeLine, mayCleanUp bool, cb rt.Callbacks) error {
	if err := e.runCompose(ctx, *up, cb, "up", "--detach", "--quiet-pull"); err != nil {
		startErr := fmt.Errorf("starting project containers: %w", err)
		if !mayCleanUp {
			// Containers were here before this start, or we could not tell —
			// either way they are not ours to remove. Name the project so the
			// user can clean up by hand if they were in fact ours.
			return fmt.Errorf("%w (containers may remain under compose project %s)", startErr, up.name)
		}
		// Joined rather than logged: a cleanup report gated on cb.OnLine, which
		// the contract makes optional, would let a consumer passing
		// rt.Callbacks{} leak containers with no signal anywhere. Joining keeps
		// errors.Is on the start error intact, and Start's deferred StateError
		// reports the joined value.
		return errors.Join(startErr, e.rollback(ctx, up.conn, up.name, cb))
	}
	return nil
}

// publish records the running project and makes it reachable: the state record,
// the proxy route, and the session watcher when one was asked for.
//
// Its own function because Start is at the complexity limit, and because these
// three are one idea — everything that turns containers into a project other
// tools can find.
func (e *Engine) publish(projectPath, name, hostname, major string, webPort, pgPort int, p rt.Plan, cb rt.Callbacks) (localstate.Record, error) {
	rec := localstate.Record{
		ProjectPath:     projectPath,
		Mode:            rt.ModeDocker,
		ComposeProject:  name,
		Port:            webPort,
		Hostname:        hostname,
		AirflowMajor:    major,
		StartedAt:       e.now().UTC(),
		StopWithSession: p.StopWithSession,
	}
	if p.StopWithSession {
		// Containers outlive their starter, so a session-tied docker run
		// records the starter's PID: that is how any tool tells the owning
		// session has ended and the project should be killed.
		rec.PID = os.Getpid()
	}
	if err := e.saveRecord(rec); err != nil {
		// Deliberately no rollback. Airflow is up and working, and this write is
		// the kind of thing that loses a race rather than a disk: Save renames a
		// temp file into place, and on Windows a rename over a file another
		// process has open fails with a sharing violation — while the desktop
		// reads this record on every status poll, on the one platform where
		// docker is the only mode. Tearing down a healthy stack because
		// bookkeeping lost a race would be the worse failure.
		//
		// But without the record nothing can reach those containers: no stop, no
		// route, and no session watcher if one was asked for. So the error names
		// the compose project, which is the only thing that makes them
		// recoverable by hand.
		return localstate.Record{}, fmt.Errorf("project started but could not be recorded; its containers are running as compose project %s: %w", name, err)
	}
	e.recordRegisteredHostname(&rec, e.addRoute(rec, pgPort, cb), cb)

	if p.StopWithSession {
		// The record is written, so the watcher can Attach; spawn it now to
		// stop the project when the starter exits. A spawn failure leaves
		// Airflow up and reported — better than failing a healthy start — so
		// it is surfaced as a warning, not returned.
		if err := e.startSession(projectPath, rec.PID); err != nil && cb.OnLine != nil {
			cb.OnLine(rt.LogLine{
				Component: "system",
				Time:      e.now(),
				Text:      fmt.Sprintf("could not arm --stop-with-session cleanup: %s", err),
			})
		}
	}

	return rec, nil
}

// saveRecord writes the runtime record, retrying once.
//
// The failure it retries is a lost race, not a broken disk: fsatomic renames a
// temp file into place, and on Windows a rename over a file another process has
// open fails with a sharing violation — the desktop reads this record on every
// status poll. One retry turns the common instance of that into a non-event,
// and the caller still hears about it if it persists.
func (e *Engine) saveRecord(rec localstate.Record) error {
	err := localstate.Save(rec)
	if err == nil {
		return nil
	}
	return errors.Join(err, localstate.Save(rec))
}

// rollbackTimeout bounds the teardown of a failed start. Short on purpose: the
// start has already failed and the caller is waiting on an error, so a wedged
// daemon must not turn a failure into a hang.
// A var for the same reason as logCaptureTimeout: the property worth pinning is
// how the two budgets relate, which a test can only exercise at millisecond
// scale.
var rollbackTimeout = 2 * time.Minute

// logCaptureTimeout bounds the diagnostic read inside that budget. It gets its
// own deadline because it runs first and is the optional half: sharing one
// context let a wedged daemon block `compose logs` until the whole rollback
// budget expired, after which os/exec refuses to spawn the `down` at all — the
// same canceled-context defect this function was written to avoid, one level
// further in.
// A var, not a const, only so a test can shrink it: the behavior worth pinning
// is that the teardown still runs after this expires, and asserting that against
// the real 20 seconds would mean a 20-second test.
var logCaptureTimeout = 20 * time.Second

// rollback removes what a failed start created, in the one window where nothing
// else can.
//
// Every path that stops a project reaches it through its state record, and that
// record is written after `compose up` returns. So a start that fails in between
// leaves containers, and a network, that no `astro local stop` can see: it fails
// with "no local Airflow is recorded for this project", `astro local list
// --clean` prunes records rather than containers, and the only way out is docker
// directly. The containers also hold the ports the next attempt wants.
//
// Only called when the probe before the up found nothing: `down` removes
// everything carrying the project label, which it cannot tell apart from what
// this start created.
//
// Note what it does NOT remove: volumes. `down --volumes` here would delete the
// database volume a PREVIOUS successful run filled, because a second start
// against an existing project is exactly when this fires. Losing a developer's
// local Airflow database to a failed start is worse than leaving a volume that
// costs disk and nothing else, and a later `stop --clean` or `docker volume
// prune` collects it. The compose file stays for the same reason it does after a
// stop: the next start rewrites it.
func (e *Engine) rollback(ctx context.Context, conn engineConn, name string, cb rt.Callbacks) error {
	// Detached from the caller's context, which is the difference between this
	// running and not. The failure this exists for is a wedged daemon or a stalled
	// pull, and the desktop drives every docker action under a deadline — so the
	// up frequently fails BECAUSE ctx is already done, and os/exec refuses to
	// spawn a process on a canceled context. Reusing it would make the cleanup a
	// silent no-op in exactly the case it was written for.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()

	// Before the containers go: their logs are the only diagnosis for the most
	// common docker-start failure. Every service depends on the one-shot database
	// migration, so a failing `airflow db migrate` surfaces as nothing more than
	// "dependency failed to start ... exited (1)" while the traceback sits in that
	// container. Removing it first makes the error unrecoverable.
	tail := e.captureFailureLogs(ctx, conn, name, cb)

	// cb is threaded through so the teardown is not silent. It can take most of a
	// minute on a slow daemon, and a consumer rendering the event stream would
	// otherwise show the failure and then nothing, which reads as a hang.
	if err := e.downProject(ctx, conn, name, gracefulStopTimeout, cb); err != nil {
		return errors.Join(fmt.Errorf("cleaning up after the failed start: %w", err), tail)
	}
	return tail
}

// captureFailureLogs reports what the containers said before they are removed,
// and returns a bounded tail as an error when there is no line callback to send
// it to.
//
// The nil-callback case is the point. rt.Callbacks makes every field optional, so
// gating the diagnosis on OnLine — as an earlier version did — meant a consumer
// passing rt.Callbacks{} had the container holding the only explanation removed
// and its logs discarded, receiving nothing but "exit status 1". That is the same
// argument that put the cleanup error in the return value rather than a log line.
//
// Best effort throughout: a start is already failing, so logs that cannot be read
// are absent rather than a second failure.
func (e *Engine) captureFailureLogs(ctx context.Context, conn engineConn, name string, cb rt.Callbacks) error {
	ctx, cancel := context.WithTimeout(ctx, logCaptureTimeout)
	defer cancel()

	// --timestamps, and each line through parseLogLine, so a caller sees the
	// service that spoke and when — the same treatment Logs gives. Without it
	// every line arrived stamped "now" and attributed to compose, with the
	// "service-1  | " prefix still glued to the text.
	//
	// The tail is per container and there may be six services, so it is small.
	var held []string
	w := &rt.LineWriter{Emit: func(line string) {
		l := parseLogLine(line, e.now)
		if cb.OnLine != nil {
			cb.OnLine(l)
			return
		}
		if len(held) < heldLogLines {
			held = append(held, l.Component+": "+l.Text)
		}
	}}
	args := []string{"compose", "-p", name, "logs", "--no-color", "--timestamps", "--tail", "10"}
	cmdEnv := make([]string, 0, len(conn.env))
	cmdEnv = append(cmdEnv, conn.env...)
	//nolint:errcheck // best effort by design; see the doc comment
	_ = e.cmd.Run(ctx, cmdEnv, rt.Stdio{Out: w, Err: w}, conn.bin, args...)
	w.Flush()

	if cb.OnLine != nil || len(held) == 0 {
		return nil
	}
	return fmt.Errorf("container output before cleanup: %s", strings.Join(held, "; "))
}

// heldLogLines bounds what captureFailureLogs folds into an error when nothing is
// streaming. Enough to carry a Python traceback's tail, small enough that an
// error stays readable.
const heldLogLines = 20

// downProject takes a compose project down. Shared with Stop so the two agree on
// how that is spelled — no --file, since down works from container labels alone
// and the generated file may already be gone.
func (e *Engine) downProject(ctx context.Context, conn engineConn, name string, timeout int, cb rt.Callbacks, extra ...string) error {
	args := append([]string{"down", "--timeout", strconv.Itoa(timeout)}, extra...)
	line := composeLine{conn: conn, name: name}
	return e.runCompose(ctx, line, cb, args...)
}

// Clean takes down whatever a docker-mode run of this project left behind,
// without a state record to read, and reports whether it could tell.
//
// Volumes go too, which is the point: a stop leaves the metadata database
// behind by design, so a database someone wants to be rid of is precisely what
// survives into the state where there is no record to attach to.
//
// The compose project is discovered by path, and where discovery finds nothing
// the name is derived — composeProjectName is a pure function of the project
// path, so this is the same name the engine published under rather than a
// guess. Deriving it matters for the case that motivated this: containers
// already down, volume still there, nothing running to find.
//
// dockerReached distinguishes "no docker-mode leftovers" from "could not ask".
// Without it a machine with the engine stopped would report a complete reset
// while a volume full of the old database sat on disk.
func (e *Engine) Clean(ctx context.Context, projectPath string) (composeProject string, dockerReached bool, err error) {
	// The generated compose file is the evidence that this project has ever run
	// in docker mode: Start writes it and only a Clean removes it, so a plain
	// stop leaves it behind — which is exactly the state a stopped docker
	// project is in, containers gone and volume still there.
	//
	// Without this check a standalone-only project paid for a container-engine
	// probe it had no use for, and a no-op `down` under a derived name was
	// reported as "removed compose project X and its volumes" — a removal of
	// something that never existed.
	//
	// Every path out of here that could not read that evidence, or could not
	// act on it, reports dockerReached false. Saying "nothing to do" when the
	// truth is "could not tell" is the one answer that turns into a lie in the
	// caller's output.
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return "", false, err
	}
	composeFile := filepath.Join(dir, composeFileName)
	if _, serr := os.Stat(composeFile); serr != nil {
		if errors.Is(serr, os.ErrNotExist) {
			return "", true, nil
		}
		return "", false, serr
	}

	conn, name, reached := e.probeEngines(ctx, projectPath)
	if !reached {
		return "", false, nil
	}
	if name == "" {
		// The engine answered and knows nothing of this project, which is what
		// a stopped one looks like — its containers are gone. The volume is
		// not, so it is taken down under the name it was published as.
		// composeProjectName is a pure function of the path, so this is that
		// name rather than a guess.
		conn, err = e.preferred()
		if err != nil {
			return "", false, err
		}
		if name, err = composeProjectName(projectPath); err != nil {
			return "", false, err
		}
	}
	// The same graceful window Stop gives containers. A reset with no record
	// still finds a live project when the record was lost rather than removed,
	// and there is no reason for that one to be killed harder than a stop.
	if derr := e.downProject(ctx, conn, name, gracefulStopTimeout, rt.Callbacks{}, "--volumes", "--remove-orphans"); derr != nil {
		// The compose file stays. It is the only evidence this project runs in
		// docker mode, and removing it after a failed teardown would make every
		// later reset skip the volume entirely while still reporting a wipe —
		// the database the user is trying to be rid of, silently kept forever.
		return "", true, fmt.Errorf("taking down %s: %w", name, derr)
	}
	e.removeBuiltImage(ctx, conn, name)
	if rerr := os.Remove(composeFile); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
		return name, true, rerr
	}
	return name, true, nil
}

// Attach returns a handle to a running docker-mode Airflow from its state
// record alone.

func (e *Engine) Attach(projectPath string) (rt.Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return nil, err
	}
	if rec.Mode != rt.ModeDocker {
		return nil, ErrNotDockerMode
	}
	return &airflow{eng: e, rec: rec}, nil
}

// ReadStatus reports a project's status from its state record plus the
// container engine's view of whether the compose project is up.
func (e *Engine) ReadStatus(projectPath string) (rt.Status, error) {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return rt.Status{ProjectPath: projectPath, State: rt.StateStopped}, nil
	}
	if err != nil {
		return rt.Status{}, err
	}
	if rec.Mode != rt.ModeDocker {
		return rt.Status{}, ErrNotDockerMode
	}
	return e.StatusOf(rec), nil
}

// StatusOf reports the live status for an already loaded record, so
// callers holding one (list, the handle) skip the disk round trip.
func (e *Engine) StatusOf(rec localstate.Record) rt.Status {
	st, _ := e.StatusOfReached(rec)
	return st
}

// StatusOfReached is StatusOf plus whether an engine actually answered.
//
// The two are not the same question, and one caller must not confuse them.
// Bounding the probe means a busy engine can miss its deadline, and a missed
// deadline looks exactly like "no containers" — which for a listing is a
// cosmetic wrong answer, but for refuseLiveStart is permission to start a
// second Airflow over a live one. Docker mode has no other double-start guard,
// so that path asks this and refuses what it cannot confirm.
func (e *Engine) StatusOfReached(rec localstate.Record) (rt.Status, bool) {
	_, name, reached := e.probeEngines(context.Background(), rec.ProjectPath)
	return rec.Status(claims([]string{name}, rec)), reached
}

// StatusOfAll reports the live status of many records with one sweep of the
// engines rather than one probe per record. Order is preserved, so a caller
// can zip the answers back onto the records it asked about.
//
// The whole point is the call count: see runningProjects. A listing that asked
// per record spent 18 seconds on a machine that had 26 of them.
func (e *Engine) StatusOfAll(ctx context.Context, recs []localstate.Record) ([]rt.Status, error) {
	if len(recs) == 0 {
		return nil, nil
	}
	paths := make([]string, len(recs))
	for i := range recs {
		// Mirrors ReadStatus rather than judging a standalone record by
		// container liveness, which would report every one of them stopped
		// and say nothing about why. Today's only caller filters by mode, so
		// this guards the exported surface against the next one.
		if recs[i].Mode != rt.ModeDocker {
			return nil, fmt.Errorf("%w: %s", ErrNotDockerMode, recs[i].ProjectPath)
		}
		paths[i] = recs[i].ProjectPath
	}
	running, _ := e.runningProjects(ctx, paths)
	statuses := make([]rt.Status, len(recs))
	for i := range recs {
		statuses[i] = recs[i].Status(claims(running[recs[i].ProjectPath], recs[i]))
	}
	return statuses, nil
}

// claims reports whether any project running in the record's directory is the
// record's own, which is what makes its Airflow live.
//
// A list rather than one name because a directory can hold more than one
// compose project — a person's own `docker compose up` beside ours, an older
// astro stack still up under a previous name — and the record's own project
// must not lose its place to a neighbor.
//
// The emptiness guard is the other half. A probe that found nothing
// contributes "", and a record written without a compose project name carries
// "" too, so the bare equality this replaces read those two silences as
// agreement and reported such a record as a running Airflow — forever, since
// nothing about a stopped project ever makes the probe answer differently.
// Worse, it could not be cleaned up: a --clean sweep skips records that report
// running, so the only way out was deleting the state file by hand. Both
// halves have to name the same project, and "" is not a project.
func claims(probed []string, rec localstate.Record) bool {
	if rec.ComposeProject == "" {
		return false
	}
	return slices.Contains(probed, rec.ComposeProject)
}

// airflow is the handle to one docker-mode Airflow, valid because it was
// built from a state record (Start writes one, Attach reads one).
type airflow struct {
	eng *Engine
	rec localstate.Record

	// conn and composeName cache the first successful findProject, so a
	// follow-up call on the same handle (stop after status, each log
	// restart) does not re-probe the engines. Liveness checks (Status)
	// bypass the cache on purpose.
	conn        engineConn
	composeName string
}

// project resolves — once per handle — the engine connection and compose
// project name that own this project's running containers.
func (a *airflow) project(ctx context.Context) (conn engineConn, composeProject string) {
	if a.composeName != "" {
		return a.conn, a.composeName
	}
	conn, name := a.eng.findProject(ctx, a.rec.ProjectPath)
	if name != "" {
		a.conn, a.composeName = conn, name
	}
	return conn, name
}

func (a *airflow) Status() (rt.Status, error) {
	return a.eng.StatusOf(a.rec), nil
}

// Stop takes the compose project down. Volumes — the metadata DB — survive
// unless Clean, which also removes the generated compose file and the
// state record's directory contents this engine owns.
func (a *airflow) Stop(ctx context.Context, opts rt.StopOptions) error {
	conn, name := a.project(ctx)
	if name == "" {
		// Nothing running (created-but-stopped containers included):
		// still run down under the recorded name so those get removed.
		var err error
		if conn, err = a.eng.preferred(); err != nil {
			return err
		}
		name = a.rec.ComposeProject
	}
	timeout := gracefulStopTimeout
	if opts.Force {
		timeout = 0
	}
	var extra []string
	if opts.Clean {
		extra = []string{"--volumes", "--remove-orphans"}
	}
	if err := a.eng.downProject(ctx, conn, name, timeout, rt.Callbacks{}, extra...); err != nil {
		return fmt.Errorf("stopping project containers: %w", err)
	}
	// removeRoute drops the daemon too when this was the last route, so the
	// last project to stop leaves no orphan proxy behind.
	errs := []error{a.eng.removeRoute(a.rec), localstate.Remove(a.rec.ProjectPath)}
	if opts.Clean {
		if p := a.composeFilePath(); p != "" {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
		// Drop the per-project dependency image (a no-op when none was built).
		a.eng.removeBuiltImage(ctx, conn, a.rec.ComposeProject)
	}
	return errors.Join(errs...)
}

// Logs streams the compose project's logs into opts.OnLine or opts.Writer.
func (a *airflow) Logs(ctx context.Context, opts rt.LogOptions) error {
	if (opts.Writer == nil) == (opts.OnLine == nil) {
		return errors.New("exactly one of LogOptions.Writer and LogOptions.OnLine must be set")
	}
	conn, name := a.project(ctx)
	if name == "" {
		return errors.New("cannot read logs: this project's containers are not running")
	}
	args := []string{"compose", "-p", name, "logs", "--no-color", "--timestamps"}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(opts.Tail))
	}
	if !opts.Since.IsZero() {
		args = append(args, "--since", opts.Since.Format(time.RFC3339))
	}
	args = append(args, opts.Components...)

	w := &rt.LineWriter{Emit: func(line string) {
		l := parseLogLine(line, a.eng.now)
		if opts.OnLine != nil {
			opts.OnLine(l)
			return
		}
		fmt.Fprintln(opts.Writer, line)
	}}
	err := a.eng.cmd.Run(ctx, conn.env, rt.Stdio{Out: w, Err: w}, conn.bin, args...)
	w.Flush()
	// A canceled follow is the normal way a log stream ends.
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("streaming logs: %w", err)
	}
	return nil
}

// Run executes argv inside the scheduler container via compose exec (the
// approach desktop's DockerExecCommand seeds; compose allocates a TTY only
// when attached to one, so no -T juggling).
func (a *airflow) Run(ctx context.Context, argv []string, s rt.Stdio) error {
	if len(argv) == 0 {
		return errors.New("no command given")
	}
	return a.exec(ctx, argv, s)
}

// Shell opens an interactive shell inside the scheduler container.
func (a *airflow) Shell(ctx context.Context, s rt.Stdio) error {
	return a.exec(ctx, []string{"/bin/bash"}, s)
}

// Env has no answer in docker mode: the project's interpreter and its
// packages are inside the container, so there is no environment on this side
// that would make `airflow` resolve to the running one. A caller that wants a
// command run in the project's environment has Run, which execs into the
// container.
func (a *airflow) Env() ([]string, error) {
	return nil, fmt.Errorf("%w: a docker-mode project's environment lives in its container", rt.ErrNotImplemented)
}

func (a *airflow) exec(ctx context.Context, argv []string, s rt.Stdio) error {
	conn, name := a.project(ctx)
	if name == "" {
		return errors.New("this project's containers are not running; run `astro local start` first")
	}
	args := append([]string{"compose", "-p", name, "exec", execService}, argv...)
	return a.eng.cmd.Run(ctx, conn.env, s, conn.bin, args...)
}

// composeFilePath returns the generated compose file's location, or "" when
// the state dir cannot be derived.
func (a *airflow) composeFilePath() string {
	dir, err := rt.StateDir(a.rec.ProjectPath)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, composeFileName)
}

// choosePort resolves one published port through the shared policy. exclude
// carries ports already chosen this Start so the api and postgres draws can't
// collide before routes.json records either.
func (e *Engine) choosePort(requested, fallback int, exclude ...int) (int, error) {
	return localshared.ChoosePort(requested, fallback, e.portFree, e.allocPort, exclude...)
}

// stateDir resolves the project's runtime state home (the canonical location
// from rt.StateDir) and creates it. The compose file, the generated
// Dockerfile, and the requirements file all land here. Stop and
// composeFilePath derive the same path from the Record, so this must stay the
// one source of truth — a divergent location would leak the compose file.
func (e *Engine) stateDir(projectPath string) (string, error) {
	dir, err := rt.StateDir(projectPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, stateDirPerm); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	return dir, nil
}

// writeComposeFile renders and writes the compose file into the state dir.
func (e *Engine) writeComposeFile(dir string, in composeInput) (string, error) {
	yaml, err := generateCompose(in)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, composeFileName)
	if err := os.WriteFile(path, []byte(yaml), proxy.FilePermRW); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}

// composeLine is the invariant part of a compose invocation.
type composeLine struct {
	conn       engineConn
	file       string
	name       string
	projectDir string
	// extraEnv is added to the compose process environment on top of the engine
	// connection's own. It carries SecretEnv values, which the file declares
	// without recording — see passEnv.
	extraEnv []string
}

// argv is the compose command line for this invocation.
//
// --project-name and --project-directory are not optional decorations. The
// name is what every other command addresses the project by, so a command
// that omits it creates containers and networks under a name compose derives
// from the file's own directory — which no teardown ever sweeps. The directory
// pins the working_dir label findProject discovers by, and is where compose
// looks for the project's .env to resolve the valueless entries passEnv wrote.
func (l composeLine) argv(args ...string) []string {
	full := []string{"compose"}
	if l.file != "" {
		full = append(full, "--file", l.file, "--project-directory", l.projectDir)
	}
	return append(append(full, "--project-name", l.name), args...)
}

// env is the compose process's environment.
//
// A fresh slice rather than append onto conn.env: that slice is shared with
// every other compose line built from the same engine connection, and appending
// into spare capacity would leak one project's secrets into the next command
// that reused it.
//
// The connection goes LAST, and that ordering is load-bearing. os/exec keeps
// the last duplicate of a key, so anything in extraEnv that collides with
// DOCKER_HOST, CONTAINER_HOST or DOCKER_CONFIG would otherwise retarget this
// very compose invocation — a project's Airflow env is caller-supplied and can
// name anything, so a plan carrying DOCKER_HOST for a DockerOperator DAG would
// silently point the start at a different daemon while Stop, which uses
// conn.env untouched, still looked at the right one. A secret that loses to
// the engine connection simply does not reach the container; a compose command
// talking to the wrong daemon is unrecoverable from the UI.
func (l composeLine) env() []string {
	out := make([]string, 0, len(l.extraEnv)+len(l.conn.env))
	out = append(out, l.extraEnv...)
	out = append(out, l.conn.env...)
	return out
}

// runCompose runs one compose command, forwarding its output line by line
// to cb.OnLine (component "compose") so frontends can render engine
// progress; without a callback the output is dropped, never printed.
func (e *Engine) runCompose(ctx context.Context, l composeLine, cb rt.Callbacks, args ...string) error {
	full := l.argv(args...)
	w := &rt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(rt.LogLine{Component: "compose", Time: e.now(), Text: line})
		}
	}}
	err := e.cmd.Run(ctx, l.env(), rt.Stdio{Out: w, Err: w}, l.conn.bin, full...)
	w.Flush()
	return err
}

// addRoute registers the project with the local proxy. Failure is reported
// through the callback but does not fail the start: Airflow is reachable
// on localhost either way.
//
// Returns the hostname the route was registered under, which is not always
// the one asked for — AddRoute qualifies a name another project holds — and
// empty when registration failed.
func (e *Engine) addRoute(rec localstate.Record, pgPort int, cb rt.Callbacks) string {
	route := proxy.Route{
		Hostname:      rec.Hostname,
		Discriminator: localshared.HostnameDiscriminator(rec.ProjectPath),
		Port:          strconv.Itoa(rec.Port),
		ProjectDir:    rec.ProjectPath,
		PID:           rec.PID,
		Services:      map[string]string{"postgres": strconv.Itoa(pgPort)},
		Mode:          proxy.RouteModeDocker,
	}
	if err := e.routes.AddRoute(&route); err != nil {
		if cb.OnLine != nil {
			cb.OnLine(rt.LogLine{
				Component: "system",
				Time:      e.now(),
				Text:      fmt.Sprintf("could not register the proxy route for %s: %s", rec.Hostname, err),
			})
		}
		return ""
	}
	return route.Hostname
}

// recordRegisteredHostname keeps the state record agreeing with the route.
//
// AddRoute qualifies a name another project holds, and the record is what
// `astro local status` prints and what stop deregisters by — a record still
// naming the plain hostname would advertise a URL serving somebody else's
// Airflow and leave this project's route behind on the way out.
func (e *Engine) recordRegisteredHostname(rec *localstate.Record, registered string, cb rt.Callbacks) {
	if registered == "" || registered == rec.Hostname {
		return
	}
	asked := rec.Hostname
	rec.Hostname = registered
	if err := localstate.Save(*rec); err != nil && cb.OnLine != nil {
		cb.OnLine(rt.LogLine{
			Component: "system",
			Time:      e.now(),
			Text: fmt.Sprintf("registered the proxy route as %s because %s is taken, but could not record it: %s",
				registered, asked, err),
		})
	}
}

// removeRoute deregisters the project's proxy route, and the proxy with it
// when no route is left.
func (e *Engine) removeRoute(rec localstate.Record) error {
	return localshared.RemoveRoute(e.routes, rec.Hostname, e.daemon)
}
