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
	"strconv"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localprune"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localshared"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/rt"
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

// Start brings the project's compose stack up and waits for Airflow to be
// healthy. The state record is written and the proxy route registered as
// soon as the containers are up, so status/stop/logs work even when the
// health wait fails or is interrupted.
func (e *Engine) Start(ctx context.Context, p rt.Plan, cb rt.Callbacks) (rt.Airflow, error) {
	if p.Mode != rt.ModeDocker {
		return nil, fmt.Errorf("localdocker got a %q plan", p.Mode)
	}
	projectPath, err := filepath.Abs(p.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", p.ProjectPath, err)
	}
	rt.OnState(cb, rt.StateStarting, nil)

	// Bring a stopped engine up first, so `start --docker` with the daemon
	// down recovers instead of failing on the first compose call.
	if err := e.ensureEngine(cb); err != nil {
		return nil, err
	}

	if e.images == nil {
		return nil, errors.New("docker mode needs an image builder: set Images on localrt.Config")
	}
	// The resolution takes a context because an Airflow 2 base image is a
	// lookup against the version service, not a tag built from the pin. It also
	// validates the generation, which everything below reads off the plan.
	image, err := e.images.RuntimeImage(ctx, p.AirflowVersion)
	if err != nil {
		return nil, err
	}
	major := airflowMajor(p.AirflowVersion)
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

	stateDir, err := e.stateDir(projectPath)
	if err != nil {
		return nil, err
	}
	// Install the project's dependencies and OS packages into a layer over the
	// runtime image so docker mode matches standalone. The builder drops
	// apache-airflow (the base image already provides it) and, with nothing
	// else to install, builds nothing and hands back the runtime image as-is.
	image, err = e.images.Build(ctx, rt.BuildRequest{
		WorkDir:      stateDir,
		BaseImage:    image,
		Tag:          builtImageTag(name),
		Dependencies: p.Dependencies,
		Packages:     p.Packages,
		Bin:          conn.bin,
		Env:          conn.env,
	}, cb)
	if err != nil {
		return nil, err
	}

	env := airflowEnv(name, webPort, major, p.Env)
	composePath, err := e.writeComposeFile(stateDir, composeInput{
		ProjectName:   name,
		Image:         image,
		PostgresImage: postgresImage,
		WebPort:       webPort,
		PostgresPort:  pgPort,
		Env:           env,
		PassEnv:       passEnv(p.PassthroughEnv, env),
		Mounts:        projectMounts(projectPath),
		DBCommand:     dbCommand(major),
		Services:      airflowServices(major),
	})
	if err != nil {
		return nil, err
	}

	up := composeLine{conn: conn, file: composePath, name: name, projectDir: projectPath}
	if err := e.runCompose(ctx, up, cb, "up", "--detach", "--quiet-pull"); err != nil {
		return nil, fmt.Errorf("starting project containers: %w", err)
	}

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
	if err := localstate.Save(rec); err != nil {
		return nil, err
	}
	e.addRoute(rec, pgPort, cb)

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
	_, name := e.findProject(context.Background(), rec.ProjectPath)
	return rec.Status(name == rec.ComposeProject)
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
	args := []string{"down", "--timeout", strconv.Itoa(timeout)}
	if opts.Clean {
		args = append(args, "--volumes", "--remove-orphans")
	}
	// No --file: down works from container labels alone, and the generated
	// compose file may already be gone (an interrupted --clean, another
	// tool's cleanup).
	line := composeLine{conn: conn, name: name}
	if err := a.eng.runCompose(ctx, line, rt.Callbacks{}, args...); err != nil {
		return fmt.Errorf("stopping project containers: %w", err)
	}
	errs := []error{a.eng.removeRoute(a.rec), localstate.Remove(a.rec.ProjectPath)}
	// The route is gone; drop the daemon too if it was the last one, so the
	// last project to stop leaves no orphan proxy behind.
	localshared.ReapDaemon(a.eng.daemon)
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
}

// runCompose runs one compose command, forwarding its output line by line
// to cb.OnLine (component "compose") so frontends can render engine
// progress; without a callback the output is dropped, never printed.
// --project-directory pins the working_dir label to the project path,
// which is what findProject discovers by.
func (e *Engine) runCompose(ctx context.Context, l composeLine, cb rt.Callbacks, args ...string) error {
	full := []string{"compose"}
	if l.file != "" {
		full = append(full, "--file", l.file, "--project-directory", l.projectDir)
	}
	full = append(append(full, "--project-name", l.name), args...)
	w := &rt.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(rt.LogLine{Component: "compose", Time: e.now(), Text: line})
		}
	}}
	err := e.cmd.Run(ctx, l.conn.env, rt.Stdio{Out: w, Err: w}, l.conn.bin, full...)
	w.Flush()
	return err
}

// addRoute registers the project with the local proxy. Failure is reported
// through the callback but does not fail the start: Airflow is reachable
// on localhost either way.
func (e *Engine) addRoute(rec localstate.Record, pgPort int, cb rt.Callbacks) {
	route := proxy.Route{
		Hostname:   rec.Hostname,
		Port:       strconv.Itoa(rec.Port),
		ProjectDir: rec.ProjectPath,
		PID:        rec.PID,
		Services:   map[string]string{"postgres": strconv.Itoa(pgPort)},
		Mode:       proxy.RouteModeDocker,
	}
	if err := e.routes.AddRoute(&route); err != nil && cb.OnLine != nil {
		cb.OnLine(rt.LogLine{
			Component: "system",
			Time:      e.now(),
			Text:      fmt.Sprintf("could not register the proxy route for %s: %s", rec.Hostname, err),
		})
	}
}

// removeRoute deregisters the project's proxy route.
func (e *Engine) removeRoute(rec localstate.Record) error {
	return localshared.RemoveRoute(e.routes, rec.Hostname)
}
