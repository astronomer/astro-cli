//go:build !windows

// Package localstandalone runs local Airflow as host processes from the
// project's uv-managed venv: the standalone-mode implementation of the
// pkg/localrt Airflow contract. The lifecycle is lifted from
// Astro Desktop's runtime/standalone.go — callback-based, context-aware,
// with state cleared on failed health so a reused PID cannot masquerade as
// a running Airflow — adapted onto the v2 plan/state/proxy seams, with the
// environment provisioned by pkg/uv from the project's pyproject.toml
// instead of desktop's Dockerfile/constraints pipeline.
//
// Like internal/localdocker it lives in the root module, not under pkg/:
// it needs internal/localstate and internal/localshared, and everything a
// second consumer needs (the contract, state locations) already lives in
// pkg/localrt.
//
// Per the layer rules, nothing here prints: progress flows through
// localrt.Callbacks and errors return typed.
package localstandalone

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/astronomer/astro-cli/internal/localprune"
	"github.com/astronomer/astro-cli/internal/localshared"
	"github.com/astronomer/astro-cli/internal/localstandalone/supervise"
	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/pkg/uv"
)

const (
	defaultAPIServerPort = 8080
	// defaultHealthTimeout bounds the wait for `airflow standalone` to come
	// up. First runs initialize the metadata database, so this is generous.
	defaultHealthTimeout = 5 * time.Minute
	// stateDirPerm is owner-only, matching internal/localstate.
	stateDirPerm = 0o700
	logFileName  = "airflow.log"
)

// ErrNotStandaloneMode reports a record this engine does not own.
var ErrNotStandaloneMode = errors.New("this project's local Airflow is not running in standalone mode")

// venvSyncer is the slice of pkg/uv the engine needs; tests fake it so no
// real uv or Python ever runs.
type venvSyncer interface {
	EnsureSynced(ctx context.Context, project, python string, stdio uv.Stdio) error
}

// launchFunc starts the supervisor process detached: its own process
// group, no tie to the caller's lifetime. It returns the child's PID,
// which Setpgid makes the process group ID too.
type launchFunc func(dir string, env []string, name string, args ...string) (int, error)

// Engine implements standalone mode. The function fields are seams:
// production values come from New, tests replace them so no real uv,
// Airflow, or signal ever runs in unit tests (the localdocker Commander
// pattern).
type Engine struct {
	routes *proxy.Store
	// daemon is the reverse-proxy lifecycle: started after a route lands so
	// <name>.localhost resolves, reaped when the last route goes. Nil on
	// Windows and in tests, where localshared.EnsureDaemon/ReapDaemon no-op.
	daemon localshared.ProxyDaemon

	cmd       Commander
	uv        func(ctx context.Context) (venvSyncer, error)
	launch    launchFunc
	prepAF2   func(projectPath string) error
	health    func(ctx context.Context, port string, timeout time.Duration, cfg airflowrt.HealthCheckConfig) error
	portFree  func(port string) bool
	allocPort func() (string, error)
	kill      func(pid int, sig syscall.Signal) error
	selfExe   func() (string, error)
	goos      string
	now       func() time.Time

	healthTimeout time.Duration
	stopTimeout   time.Duration
	stopPoll      time.Duration
}

// New builds the production engine. routesDir is where pkg/proxy keeps
// routes.json (~/.astro/proxy); the composition root supplies it because
// this package must not read config.
func New(routesDir string, daemon localshared.ProxyDaemon) *Engine {
	s := proxy.NewStore(routesDir, proxy.WithRouteLiveness(localprune.RouteAlive))
	return &Engine{
		routes:        s,
		daemon:        daemon,
		cmd:           execCommander{},
		uv:            newUVClient,
		launch:        launchDetached,
		prepAF2:       prepDarwinAF2,
		health:        checkHealth,
		portFree:      proxy.IsPortAvailable,
		allocPort:     s.AllocatePort,
		kill:          syscall.Kill,
		selfExe:       os.Executable,
		goos:          goruntime.GOOS,
		now:           time.Now,
		healthTimeout: defaultHealthTimeout,
		stopTimeout:   airflowrt.StopTimeout,
		stopPoll:      airflowrt.StopPollInterval,
	}
}

// newUVClient discovers uv lazily — at Start, not engine construction — so
// every other operation (status, stop, logs) works on a machine whose uv
// disappeared. The cache dir is shared across projects under the astro
// cache root, so Python toolchains and wheels download once.
func newUVClient(ctx context.Context) (venvSyncer, error) {
	root, err := localrt.CacheRoot()
	if err != nil {
		return nil, err
	}
	return uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
}

// checkHealth adapts pkg/airflowrt's poller to the engine's seam shape.
func checkHealth(ctx context.Context, port string, timeout time.Duration, cfg airflowrt.HealthCheckConfig) error {
	return airflowrt.CheckHealth(ctx, port, timeout, cfg)
}

// Start provisions the project venv with uv, launches `airflow standalone`
// detached, and waits for it to become healthy. The state record is written
// as soon as the process starts, so stop and status work during the health
// wait; the proxy route is registered only after health passes. On failed
// health the process group is killed and the record cleared — leaving them
// would make the next Start mistake a reused PID for a running Airflow
// (desktop's PID-reuse-after-failed-health bug).
func (e *Engine) Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error) {
	if p.Mode != localrt.ModeStandalone {
		return nil, fmt.Errorf("localstandalone got a %q plan", p.Mode)
	}
	projectPath, err := filepath.Abs(p.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", p.ProjectPath, err)
	}
	if err := e.checkNotRunning(projectPath); err != nil {
		return nil, err
	}
	localshared.OnState(cb, localrt.StateStarting, nil)

	hostname, err := localshared.PlanHostname(p, projectPath)
	if err != nil {
		return nil, err
	}
	stateDir, err := planStateDir(p, projectPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, stateDirPerm); err != nil {
		return nil, fmt.Errorf("creating %s: %w", stateDir, err)
	}
	port, err := localshared.ChoosePort(p.RequestedPort, defaultAPIServerPort, e.portFree, e.allocPort)
	if err != nil {
		return nil, err
	}

	if err := e.syncVenv(ctx, projectPath, p.PythonVersion, cb); err != nil {
		return nil, err
	}

	airflowHome := p.AirflowHome
	if airflowHome == "" {
		airflowHome = filepath.Join(projectPath, airflowrt.StandaloneDir)
	}
	if err := os.MkdirAll(airflowHome, airflowrt.DirPermissions); err != nil {
		return nil, fmt.Errorf("creating %s: %w", airflowHome, err)
	}

	major := airflowMajor(p.AirflowVersion)
	bin, args, needsAF2Prep := launchCommand(e.goos, major, projectPath)
	if needsAF2Prep {
		if err := e.prepAF2(projectPath); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".venv", "bin", "airflow")); err != nil {
		return nil, errors.New("the project environment has no airflow command; add an Airflow distribution (e.g. apache-airflow) to pyproject.toml and retry")
	}
	bin, args, err = e.superviseArgs(bin, args, filepath.Join(stateDir, logFileName), p.StopWithSession)
	if err != nil {
		return nil, err
	}

	env := e.buildEnv(p, projectPath, stateDir, airflowHome, port)
	pid, err := e.launch(projectPath, env, bin, args...)
	if err != nil {
		return nil, fmt.Errorf("starting airflow: %w", err)
	}

	rec := localstate.Record{
		ProjectPath: projectPath,
		Mode:        localrt.ModeStandalone,
		PID:         pid,
		// launch sets Setpgid, so the child leads its own group and the
		// pgid equals its PID.
		Pgid:     pid,
		Port:     port,
		Hostname: hostname,
		// The generation this process was launched for, so whatever talks to it
		// later follows the process rather than a manifest that may since have
		// been edited.
		AirflowMajor:    major,
		StartedAt:       e.now().UTC(),
		StopWithSession: p.StopWithSession,
	}
	if err := localstate.Save(rec); err != nil {
		e.killGroup(pid, syscall.SIGTERM)
		return nil, err
	}

	cfg := airflowrt.HealthCheckConfig{}
	if major == "2" {
		cfg.AirflowMajorVersion = "2"
	}
	if err := e.health(ctx, strconv.Itoa(port), e.healthTimeout, cfg); err != nil {
		if ctx.Err() != nil {
			// Canceled, not unhealthy: leave Airflow starting in the
			// background with its record, the same shape as docker mode —
			// `astro local stop` can reap it.
			return nil, ctx.Err()
		}
		e.killGroup(pid, syscall.SIGTERM)
		if rmErr := localstate.Remove(projectPath); rmErr != nil {
			err = errors.Join(err, rmErr)
		}
		localshared.OnState(cb, localrt.StateError, err)
		return nil, err
	}

	e.addRoute(rec, cb)
	localshared.EnsureDaemon(e.daemon, cb, e.now(), rec.Hostname)
	localshared.OnState(cb, localrt.StateRunning, nil)
	return &airflow{eng: e, rec: rec}, nil
}

// checkNotRunning refuses to start over a live record. A dead record is
// stale state from an unclean shutdown and gets overwritten.
func (e *Engine) checkNotRunning(projectPath string) error {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if rec.Mode != localrt.ModeStandalone {
		return fmt.Errorf("this project's local Airflow is already recorded in %s mode; stop it first with `astro local stop`", rec.Mode)
	}
	if e.groupAlive(rec) {
		return fmt.Errorf("local Airflow is already running for this project (PID %d); `astro local stop` stops it", rec.PID)
	}
	return nil
}

// syncVenv provisions <project>/.venv via uv. EnsureSynced carries the
// completion marker and wipe-and-retry that recover poisoned venvs (uv
// tracks installs inside the venv, so an interrupted run fails later
// installs with metadata errors — desktop's uv-metadata-poisoning bug,
// handled in pkg/uv).
func (e *Engine) syncVenv(ctx context.Context, projectPath, python string, cb localrt.Callbacks) error {
	client, err := e.uv(ctx)
	if err != nil {
		return err
	}
	w := &localshared.LineWriter{Emit: func(line string) {
		if cb.OnLine != nil {
			cb.OnLine(localrt.LogLine{Component: "uv", Time: e.now(), Text: line})
		}
	}}
	err = client.EnsureSynced(ctx, projectPath, python, uv.Stdio{Out: w, Err: w})
	w.Flush()
	if err != nil {
		return fmt.Errorf("preparing the project environment: %w", err)
	}
	return nil
}

// launchCommand picks the binary and arguments that run Airflow. AF2 on
// macOS launches through a Python shim instead of `airflow standalone`:
// gunicorn forks workers that inherit corrupted ObjC runtime state and
// SIGSEGV (desktop's AF2 macOS fork-safety bug); the shim plus the patches
// prepDarwinAF2 writes avoid the fork paths entirely.
func launchCommand(goos, airflowMajor, projectPath string) (bin string, args []string, needsAF2Prep bool) {
	venvBin := filepath.Join(projectPath, ".venv", "bin")
	if airflowMajor == "2" && goos == "darwin" {
		return filepath.Join(venvBin, "python"), []string{filepath.Join(venvBin, af2ShimName)}, true
	}
	return filepath.Join(venvBin, "airflow"), []string{"standalone"}, false
}

const af2ShimName = "_standalone_macos.py"

// prepDarwinAF2 writes the three macOS AF2 fork-safety pieces into place:
// the standalone shim, the site-packages fork/setproctitle patch, and the
// LocalExecutor pickle-fix plugin.
func prepDarwinAF2(projectPath string) error {
	venvBin := filepath.Join(projectPath, ".venv", "bin")
	if err := os.WriteFile(filepath.Join(venvBin, af2ShimName), airflowrt.AF2DarwinShim, airflowrt.FilePermissions); err != nil {
		return fmt.Errorf("writing the macOS standalone shim: %w", err)
	}
	if err := airflowrt.WriteDarwinForkSafetyPatch(filepath.Join(projectPath, ".venv")); err != nil {
		return fmt.Errorf("writing the macOS fork-safety patch: %w", err)
	}
	pluginsDir := filepath.Join(projectPath, "plugins")
	if err := os.MkdirAll(pluginsDir, airflowrt.DirPermissions); err != nil {
		return fmt.Errorf("creating %s: %w", pluginsDir, err)
	}
	pickleFix := filepath.Join(pluginsDir, "fix_local_executor_pickle.py")
	if _, err := os.Stat(pickleFix); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(pickleFix, airflowrt.AF2PickleFixPlugin, airflowrt.FilePermissions); err != nil {
			return fmt.Errorf("writing the LocalExecutor pickle fix plugin: %w", err)
		}
	}
	return nil
}

// superviseArgs wraps the launch in `astro __supervise`. Every launch goes
// through the supervisor because it owns the capped log file — a pipe from
// this process would close when the CLI exits, and detached Airflow must
// keep logging after that. stopWithSession additionally arms the parent
// watch, which kills Airflow when this process exits — how StopWithSession
// holds even when the starter dies without running its stop path.
func (e *Engine) superviseArgs(bin string, args []string, logPath string, stopWithSession bool) (wrappedBin string, wrappedArgs []string, err error) {
	exe, err := e.selfExe()
	if err != nil {
		return "", nil, fmt.Errorf("resolving the astro binary for supervision: %w", err)
	}
	wrapped := []string{supervise.Subcommand, supervise.LogFileFlag, logPath}
	if stopWithSession {
		wrapped = append(wrapped, supervise.ParentPIDFlag, strconv.Itoa(os.Getpid()))
	}
	wrapped = append(wrapped, "--", bin)
	wrapped = append(wrapped, args...)
	return exe, wrapped, nil
}

// Attach returns a handle to a standalone Airflow from its state record
// alone.
func (e *Engine) Attach(projectPath string) (localrt.Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return nil, err
	}
	if rec.Mode != localrt.ModeStandalone {
		return nil, ErrNotStandaloneMode
	}
	return &airflow{eng: e, rec: rec}, nil
}

// LogHandle returns a handle for reading a stopped project's persisted log
// file, without a live record: the standalone log file outlives the record
// (Stop removes the record but not the log), and its path derives from the
// project's state dir. Logs on this handle reads the backlog and, since a
// zero pgid reads as not alive, a follow ends once the file is drained.
func (e *Engine) LogHandle(projectPath string) (localrt.Airflow, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", projectPath, err)
	}
	return &airflow{eng: e, rec: localstate.Record{ProjectPath: abs, Mode: localrt.ModeStandalone}}, nil
}

// ReadStatus reports a project's status from its state record plus a
// process-group liveness check.
func (e *Engine) ReadStatus(projectPath string) (localrt.Status, error) {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return localrt.Status{ProjectPath: projectPath, State: localrt.StateStopped}, nil
	}
	if err != nil {
		return localrt.Status{}, err
	}
	if rec.Mode != localrt.ModeStandalone {
		return localrt.Status{}, ErrNotStandaloneMode
	}
	return e.StatusOf(rec), nil
}

// StatusOf reports the live status for an already loaded record, so
// callers holding one (list, the handle) skip the disk round trip.
func (e *Engine) StatusOf(rec localstate.Record) localrt.Status {
	return rec.Status(e.groupAlive(rec))
}

// groupAlive reports whether any process in the record's group is still
// reachable. The group, not the master PID: components outlive the master
// during shutdown, and a supervisor leads the group when session-tied.
func (e *Engine) groupAlive(rec localstate.Record) bool {
	pgid := rec.Pgid
	if pgid == 0 {
		pgid = rec.PID
	}
	if pgid <= 0 {
		return false
	}
	return e.kill(-pgid, 0) == nil
}

// killGroup signals the whole process group.
func (e *Engine) killGroup(pgid int, sig syscall.Signal) {
	if pgid > 0 {
		_ = e.kill(-pgid, sig) //nolint:errcheck // best-effort signal to the process group
	}
}

// addRoute registers the project with the local proxy. Failure is reported
// through the callback but does not fail the start: Airflow is reachable
// on localhost either way.
func (e *Engine) addRoute(rec localstate.Record, cb localrt.Callbacks) {
	route := proxy.Route{
		Hostname:   rec.Hostname,
		Port:       strconv.Itoa(rec.Port),
		ProjectDir: rec.ProjectPath,
		PID:        rec.PID,
		Mode:       proxy.RouteModeStandalone,
	}
	if err := e.routes.AddRoute(&route); err != nil && cb.OnLine != nil {
		cb.OnLine(localrt.LogLine{
			Component: "system",
			Time:      e.now(),
			Text:      fmt.Sprintf("could not register the proxy route for %s: %s", rec.Hostname, err),
		})
	}
}

// airflow is the handle to one standalone Airflow, valid because it was
// built from a state record (Start writes one, Attach reads one).
type airflow struct {
	eng *Engine
	rec localstate.Record
}

func (a *airflow) Status() (localrt.Status, error) {
	return a.eng.StatusOf(a.rec), nil
}

// Stop terminates the process group and removes the route and record.
// It signals and polls the pgid, never just the master PID: `airflow
// standalone` spawns scheduler/api-server/triggerer into the group and the
// master often exits on SIGTERM well before its children finish, so
// watching only the master leaks them (the bug v1's stop and airflowrt's
// StopProcess still have; an earlier fix fixes it there).
func (a *airflow) Stop(ctx context.Context, opts localrt.StopOptions) error {
	e := a.eng
	pgid := a.rec.Pgid
	if pgid == 0 {
		pgid = a.rec.PID
	}
	if pgid > 0 && e.groupAlive(a.rec) {
		if opts.Force {
			// Airflow's sqlite runs in WAL mode, so SIGKILL cannot corrupt
			// the database.
			e.killGroup(pgid, syscall.SIGKILL)
		} else {
			e.killGroup(pgid, syscall.SIGTERM)
			// kill(-pgid, 0) succeeds while any group member remains.
			deadline := time.Now().Add(e.stopTimeout)
			for time.Now().Before(deadline) && ctx.Err() == nil {
				time.Sleep(e.stopPoll)
				if !e.groupAlive(a.rec) {
					break
				}
			}
			if e.groupAlive(a.rec) {
				e.killGroup(pgid, syscall.SIGKILL)
				time.Sleep(e.stopPoll)
			}
		}
	}
	errs := []error{
		localshared.RemoveRoute(e.routes, a.rec.Hostname),
		localstate.Remove(a.rec.ProjectPath),
	}
	// The route is gone; drop the daemon too if it was the last one, so the
	// last project to stop leaves no orphan proxy behind.
	localshared.ReapDaemon(e.daemon)
	if opts.Clean {
		errs = append(errs, a.clean())
	}
	return errors.Join(errs...)
}

// clean removes the derived state this engine owns: AIRFLOW_HOME (metadata
// database included), the venv, and the runtime files in the state dir. The
// project's own files are untouched. AIRFLOW_HOME is removed at its default
// location; a plan-supplied override is not recorded, so a custom home is
// the plan builder's to clean once one exists.
func (a *airflow) clean() error {
	var errs []error
	for _, p := range []string{
		filepath.Join(a.rec.ProjectPath, airflowrt.StandaloneDir),
		filepath.Join(a.rec.ProjectPath, ".venv"),
	} {
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, err)
		}
	}
	if dir, err := localrt.StateDir(a.rec.ProjectPath); err == nil {
		for _, f := range []string{logFileName, jwtSecretFile} {
			if err := os.Remove(filepath.Join(dir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Run executes argv inside the project's Airflow environment: the same env
// the Airflow process runs with, venv-activated, so `airflow` and `python`
// resolve to the project venv and share the running metadata DB (v1's
// seven-line Run paired with desktop's ShellEnv).
func (a *airflow) Run(ctx context.Context, argv []string, s localrt.Stdio) error {
	if len(argv) == 0 {
		return errors.New("no command given")
	}
	env, err := a.eng.shellEnv(a.rec)
	if err != nil {
		return err
	}
	bin := airflowrt.ResolveInEnvPath(argv[0], env)
	return a.eng.cmd.Run(ctx, a.rec.ProjectPath, env, s, bin, argv[1:]...)
}

// Shell opens an interactive shell inside the project's Airflow
// environment: the user's login shell when known, else bash.
func (a *airflow) Shell(ctx context.Context, s localrt.Stdio) error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/bash"
	}
	return a.Run(ctx, []string{sh}, s)
}

// planStateDir resolves the runtime state home: the plan's when set, the
// canonical per-project location otherwise (same fallback as localdocker's
// compose-file placement).
func planStateDir(p localrt.Plan, projectPath string) (string, error) {
	if p.StateDir != "" {
		return p.StateDir, nil
	}
	return localrt.StateDir(projectPath)
}

// airflowMajor extracts the major version from a plan's Airflow version;
// empty (a minimal plan) means Airflow 3, the only version `astro init`
// scaffolds.
func airflowMajor(version string) string {
	if major, _, _ := strings.Cut(version, "."); major == "2" {
		return "2"
	}
	return "3"
}
