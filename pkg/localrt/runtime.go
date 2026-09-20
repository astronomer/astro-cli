package localrt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localdocker"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localprune"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstandalone"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Runtime is a configured local runtime. It dispatches on Mode between the two
// engines, standalone and docker, and read paths dispatch on the mode the state
// record captured at start — so any tool stops what another started.
//
// This was cmd/local's modeRuntime, which is why the earlier move exists: the dispatch, the
// start lock, and the refuse-a-live-start rules were CLI-private, so a second
// consumer had no way to reach them without reimplementing them differently.
// Moved here verbatim, error strings included.
type Runtime struct {
	docker     *localdocker.Engine
	standalone *localstandalone.Engine
	// routes is the CLI's own view of routes.json, for the list join and the
	// --clean sweep. It carries the record-aware prune predicate so listing
	// never evicts a route whose owner is still alive.
	routes *proxy.Store
	// now stamps a claimed runtime's start time. The engines each carry their
	// own clock for the records they write; this is the same seam for the
	// records Claim writes, which have no engine behind them.
	now func() time.Time
	// containersGone confirms a docker project's containers are really absent
	// before --clean drops its record. A field for the same reason now is
	// one: the answer comes from outside the process, and a test that needs
	// no engine to be reachable cannot make the machine it runs on have none.
	containersGone func(context.Context, string) (bool, error)
}

// Config is what a consumer supplies to build a Runtime. Nothing here is
// derived, because the two consumers disagree on all three: different routes
// dirs (tests included), different proxy lifecycles, and only one of them had an
// image builder before this.
type Config struct {
	// RoutesDir is the proxy's routes.json home, normally <astro home>/proxy.
	// The CLI and the desktop must pass the same one or they stop seeing each
	// other's projects.
	RoutesDir string
	// ProxyDaemon is the reverse-proxy lifecycle. Nil is valid and means "no
	// daemon" — Windows, and tests — in which case Airflow stays reachable on
	// its direct localhost port.
	ProxyDaemon ProxyDaemon
	// Images builds the container image docker mode runs. Required for docker
	// mode; standalone never touches it.
	Images ImageBuilder
	// HermeticUVEnv strips the inherited UV_* variables that steer resolution
	// from the venv a standalone start provisions. A consumer passing python,
	// dependencies and constraints from the manifest can only be contradicted
	// by an ambient one — most concretely UV_EXCLUDE_NEWER, which filters out
	// freshly published builds until the install fails as "unsatisfiable" for a
	// reason nothing in the project explains. See uv.Options.HermeticEnv for
	// why --no-config does not reach these.
	//
	// Off by default: a uv a user configured for their own shell is theirs, and
	// the CLI honors it for the same reason it leaves --no-config off.
	HermeticUVEnv bool
	// OnUVCertFallback, when set, is called after a provisioning step that only
	// succeeded once uv was retried against the platform certificate store.
	// The start worked, so this is diagnostics — and the only way to tell a
	// machine whose trust anchor is missing from uv's bundle from a healthy
	// one, because nothing else about the start looks different.
	OnUVCertFallback func()
}

// New returns a Runtime configured by cfg.
func New(cfg Config) *Runtime {
	docker := localdocker.New(cfg.RoutesDir, cfg.ProxyDaemon, cfg.Images)
	return &Runtime{
		docker: docker,
		standalone: localstandalone.New(cfg.RoutesDir, cfg.ProxyDaemon, localstandalone.UVOptions{
			HermeticEnv:    cfg.HermeticUVEnv,
			OnCertFallback: cfg.OnUVCertFallback,
		}),
		routes:         proxy.NewStore(cfg.RoutesDir, proxy.WithRouteLiveness(localprune.RouteAlive)),
		now:            time.Now,
		containersGone: docker.ContainersGone,
	}
}

func (r *Runtime) Start(ctx context.Context, p Plan, cb Callbacks) (Airflow, error) {
	if p.Mode == "" {
		// The plan leaves Mode empty when the command asked for no specific
		// runtime (no --docker); standalone is the default.
		p.Mode = ModeStandalone
	}
	// One project starts at a time. The lock stops two concurrent starts from
	// both writing the record, where the loser's dying pid would orphan the
	// winner's live Airflow; it is held across the liveness check and the
	// engine's record write so the check-and-write is atomic.
	unlock, err := localstate.Lock(p.ProjectPath)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := r.refuseLiveStart(p); err != nil {
		return nil, err
	}
	if p.Mode == ModeDocker {
		return r.docker.Start(ctx, p, cb)
	}
	return r.standalone.Start(ctx, p, cb)
}

// refuseLiveStart refuses to start over a runtime that is already live, so a
// second start — same mode or a different one — never overwrites the record
// and orphans the running Airflow. This is the only place a cross-mode
// collision is caught, since an engine knows only its own mode. A record whose
// runtime is gone falls through to the engine, which overwrites a stale
// same-mode record and still refuses a foreign mode.
func (r *Runtime) refuseLiveStart(p Plan) error {
	rec, err := localstate.Load(p.ProjectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.statusOf(rec).State != StateRunning {
		return nil
	}
	if rec.Mode != p.Mode {
		return fmt.Errorf("local Airflow is already running for this project in %s mode; stop it first with `astro local stop`", modeLabel(rec.Mode))
	}
	return fmt.Errorf("local Airflow is already running for this project; use `astro local restart` to restart it or `astro local stop` to stop it")
}

// RunInImage runs one command in a docker-mode project's image, with no
// Airflow running and without starting any.
//
// It dispatches to the docker engine unconditionally rather than on a state
// record's mode, because the case it serves is a project that has no record:
// a stop removes it. The caller knows the project is docker mode from its
// manifest, which is the same thing Start dispatches on.
//
// Standalone mode has no image and no counterpart here. Its offline equivalent
// is to run the project's own .venv interpreter directly, which a caller can do
// without going through a runtime at all.
func (r *Runtime) RunInImage(ctx context.Context, projectPath string, req ImageRun) error {
	return r.docker.RunInImage(ctx, projectPath, req)
}

// ResetReport says what a reset actually did, so a caller can tell the user
// when part of it could not be reached.
// The bools carry no omitempty: they are status a json consumer reads, and a
// key that vanishes when false makes "did not happen" indistinguishable from
// "this version does not report it".
type ResetReport struct {
	// Stopped reports that a live Airflow was stopped as part of this.
	Stopped bool `json:"stopped"`
	// ComposeProject names the compose project taken down, empty when none was.
	ComposeProject string `json:"compose_project,omitempty"`
	// DockerUnreachable reports that no container engine answered, so a
	// docker-mode leftover could neither be removed nor ruled out.
	DockerUnreachable bool `json:"docker_unreachable"`
}

// Reset stops this project's Airflow if it is running and removes the state a
// run derives — the metadata database, the venv, the logs, the compose project
// and its volumes. The project's own files are untouched.
//
// It works on a project that is already STOPPED, which is the whole reason it
// exists rather than being Stop with a flag. A stop removes the state record,
// and every other entry point here goes through Attach, which needs one — so
// the case where someone most wants to wipe a corrupt database (they stopped,
// it misbehaved, they want to start clean) was the one case with nothing to
// attach to. Both engines can clean from the project path alone; only the
// plumbing assumed otherwise.
//
// Both engines are always cleaned from the path, whether or not there is a
// record. A record names only the mode that ran last, so a project moved
// between modes leaves the other one's state behind, and with no record at all
// nothing says which mode that was. Each engine acts only on evidence that its
// own mode ran here, so the one that did not is a no-op rather than a guess.
func (r *Runtime) Reset(ctx context.Context, projectPath string) (ResetReport, error) {
	var report ResetReport

	// The lock Start takes, for the same reason. A reset racing a start would
	// delete the venv out from under its uv sync, or tear down containers the
	// other half is still writing a record for. Neither Stop nor the engines'
	// Clean takes it, so holding it across the whole reset cannot deadlock.
	unlock, err := localstate.Lock(projectPath)
	if err != nil {
		return report, err
	}
	defer unlock()

	rec, err := localstate.Load(projectPath)
	switch {
	case err == nil:
		// Recorded, and possibly live: the engine that owns it knows how to
		// take it down and clean up after itself.
		//
		// Whether it was live has to be read before the stop, not after.
		// statusOf probes for real — a signal to the process group, a query to
		// the container engine — and the stop is precisely what makes those
		// stop answering, so asking afterwards always says "stopped" and the
		// report would never once be true.
		wasRunning := r.statusOf(rec).State == StateRunning
		af, aerr := r.Attach(projectPath)
		if aerr != nil {
			return report, aerr
		}
		if serr := af.Stop(ctx, StopOptions{Clean: true}); serr != nil {
			return report, serr
		}
		report.Stopped = wasRunning
		if rec.Mode == ModeDocker {
			report.ComposeProject = rec.ComposeProject
		}
	case !errors.Is(err, localstate.ErrNotRunning):
		return report, err
	}

	// Then clean both engines from the path alone. After a stop this is a
	// no-op for the mode that just went down — its own clean already removed
	// the evidence each of these gates on — and it is how the other mode's
	// leftovers go too.
	var errs []error
	if cerr := r.standalone.Clean(projectPath); cerr != nil {
		errs = append(errs, cerr)
	}
	name, reached, derr := r.docker.Clean(ctx, projectPath)
	if derr != nil {
		errs = append(errs, derr)
	}
	if name != "" {
		report.ComposeProject = name
	}
	report.DockerUnreachable = !reached
	return report, errors.Join(errs...)
}

// HotInstall adds dependencies to a project's environment without restarting
// the Airflow running on it, then nudges the scheduler to re-parse its Dags.
//
// Standalone only. Refusing docker mode is the honest answer rather than a gap:
// those dependencies live in an image, so making them live means rebuilding it,
// which is a restart under another name — a caller that wants that should ask.
//
// deps are the project's declared dependencies, passed in rather than read
// here, because nothing in this package reads a manifest. Plan.Dependencies
// arrives the same way for the same reason.
func (r *Runtime) HotInstall(ctx context.Context, projectPath string, deps []string, cb Callbacks) error {
	// Before anything else, including the record: a caller watching a manifest
	// cannot know a project declares no dependencies until it asks, and
	// answering "nothing to do" is truer than "no Airflow is recorded".
	if len(deps) == 0 {
		return nil
	}

	// The lock Start and Reset take, for the reason Reset states: two uv
	// processes in one venv is how it ends up half-written. A hot install
	// racing a restart would install into the venv EnsureSynced is rebuilding,
	// and racing a reset would install into one Clean is deleting. The lock is
	// non-blocking and the engines take none of their own, so holding it across
	// this cannot deadlock.
	unlock, err := localstate.Lock(projectPath)
	if err != nil {
		return err
	}
	defer unlock()

	rec, err := localstate.Load(projectPath)
	if err != nil {
		return err
	}
	if rec.Mode == ModeDocker {
		return fmt.Errorf("%w: hot install for a docker-mode project", ErrNotImplemented)
	}
	// Recorded is not running. A crash leaves the record behind, and installing
	// into a stopped project then reporting success tells the caller a package
	// is live in a scheduler that does not exist — when the entire reason to
	// call this rather than restart is that something IS running.
	if r.statusOf(rec).State != StateRunning {
		return fmt.Errorf("%w: this project's local Airflow is not running, so there is nothing to install into without restarting it", ErrNotImplemented)
	}
	return r.standalone.HotInstall(ctx, projectPath, deps, cb)
}

func (r *Runtime) Attach(projectPath string) (Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return nil, err
	}
	if rec.Mode == ModeDocker {
		return r.docker.Attach(projectPath)
	}
	return r.standalone.Attach(projectPath)
}

func (r *Runtime) LogSource(projectPath string) (Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err == nil {
		if rec.Mode == ModeDocker {
			return r.docker.Attach(projectPath)
		}
		return r.standalone.Attach(projectPath)
	}
	if errors.Is(err, localstate.ErrNotRunning) {
		// No record: only standalone leaves a log file behind, so read it
		// through the standalone engine's detached log handle.
		return r.standalone.LogHandle(projectPath)
	}
	return nil, err
}

func (r *Runtime) ReadStatus(projectPath string) (Status, error) {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return Status{ProjectPath: projectPath, State: StateStopped}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if rec.Mode == ModeDocker {
		return r.docker.ReadStatus(projectPath)
	}
	return r.standalone.ReadStatus(projectPath)
}

func (r *Runtime) List() ([]Status, error) {
	recs, err := localstate.List()
	if err != nil {
		return nil, err
	}
	// Join with the live routes: it fills a hostname the record predates, and
	// the read prunes routes.json through the record-aware predicate, so a
	// list also heals the file. Best-effort — a missing or unreadable proxy
	// dir must not hide the records.
	hostByProject := map[string]string{}
	if routes, rerr := r.routes.ListRoutes(); rerr == nil {
		for _, rt := range routes {
			hostByProject[rt.ProjectDir] = rt.Hostname
		}
	}
	statuses := make([]Status, 0, len(recs))
	for i := range recs {
		st := r.statusOf(recs[i])
		if st.Hostname == "" {
			st.Hostname = hostByProject[st.ProjectPath]
		}
		statuses = append(statuses, st)
	}
	return statuses, nil
}

// statusOf reports one record's live status through the engine that owns its
// mode, so liveness is checked the way that mode records it.
func (r *Runtime) statusOf(rec localstate.Record) Status {
	if rec.Mode == ModeDocker {
		return r.docker.StatusOf(rec)
	}
	return r.standalone.StatusOf(rec)
}

func (r *Runtime) PruneStale() ([]Status, error) {
	statuses, err := r.List()
	if err != nil {
		return nil, err
	}
	return r.pruneAll(statuses)
}

// pruneAll is PruneStale's loop over statuses already read. Separate because
// List reaches a container engine to judge docker liveness, and what this
// does with a failure is worth testing without one.
func (r *Runtime) pruneAll(statuses []Status) ([]Status, error) {
	var removed []Status
	// Best-effort, and deliberately so: this is the command someone reaches
	// for when the records are already in a state nobody intended, and
	// returning on the first failure meant one unremovable record hid every
	// other stale one behind it. What could be pruned is pruned, and what
	// could not is reported at the end.
	var errs []error
	// ContainersGone fails only when no engine is reachable at all, which is
	// one thing wrong with the machine rather than one thing wrong per
	// record. Asked once: the engine will not come back inside this loop, and
	// reporting it once beats the same sentence with a different path after
	// it, each costing another probe timeout.
	var engineErr error
	for i := range statuses {
		st := &statuses[i]
		if st.State == StateRunning {
			continue
		}
		// Docker liveness cannot tell "compose gone" from "engine down", so
		// confirm the containers are really absent before deleting. A blip in
		// the daemon must not wipe a running project's record. Standalone
		// liveness is a syscall, so its stopped verdict is trusted as-is.
		if st.Mode == ModeDocker {
			if engineErr != nil {
				continue
			}
			gone, cerr := r.containersGone(context.Background(), st.ProjectPath)
			if cerr != nil {
				// Not knowing is not the same as nothing to do. Unreported,
				// --clean says "no stale records to remove" when what
				// happened is that it could not go look.
				engineErr = cerr
				errs = append(errs, cerr)
				continue
			}
			if !gone {
				continue
			}
		}
		// Drop the route first, while the record still backs it, then the
		// record. Removing an absent record is not an error.
		//
		// A route that will not drop does not keep the record: the route
		// already names a runtime that is not there and holding the record
		// cannot revive it, while a routes file that always fails to write
		// would otherwise make every record unprunable for good. Reported,
		// and the record still goes.
		if st.Hostname != "" {
			if _, rerr := r.routes.RemoveRoute(st.Hostname); rerr != nil {
				errs = append(errs, fmt.Errorf("removing route %s: %w", st.Hostname, rerr))
			}
		}
		if rerr := localstate.Remove(st.ProjectPath); rerr != nil {
			errs = append(errs, fmt.Errorf("removing record for %s: %w", st.ProjectPath, rerr))
			continue
		}
		removed = append(removed, *st)
	}
	return removed, errors.Join(errs...)
}

// modeLabel is how a mode is named in a message to a person.
func modeLabel(m Mode) string {
	if m == ModeDocker {
		return "docker"
	}
	return "standalone"
}
