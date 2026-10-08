package localrt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localdocker"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localprune"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localshared"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstandalone"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Runtime is a configured local runtime. It dispatches on Mode between the two
// engines, standalone and docker, and read paths dispatch on the mode the state
// record captured at start — so any tool stops what another started.
//
// This was cmd/local's modeRuntime, which is why it moved here: the dispatch, the
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
	// containersGone confirms docker projects' containers are really absent
	// before --clean drops their records. A field for the same reason now is
	// one: the answer comes from outside the process, and a test that needs
	// no engine to be reachable cannot make the machine it runs on have none.
	//
	// Batched, because the sweep that calls it runs over every stale record
	// and each call shells out twice. Asked per record it kept the whole cost
	// the listing had just shed: 18 seconds to --clean the 26 records that
	// prompted this, after List itself was down to one call.
	containersGone func(context.Context, []string) (map[string]bool, error)
	// dockerStatuses answers a batch of docker records. A seam beside
	// containersGone and for the same reason — without it the listing tests
	// reach the host's real docker and podman, which pkg/localdocker's
	// Commander doc says explicitly must never happen in a test.
	dockerStatuses func(context.Context, []localstate.Record) ([]Status, error)
	// daemon is the proxy the routes point at. The engines each hold it for
	// the start and stop they drive; the Runtime holds it for --clean, which
	// removes routes without going through either.
	daemon ProxyDaemon
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
	// The three UV* fields below configure the uv that provisions a standalone
	// venv. Standalone only: docker mode builds an image instead and never
	// sees them, and Windows has no standalone engine, so all three are inert
	// in both.
	//
	// UVBinDir is where to look for uv before PATH, for an embedder shipping
	// its own copy. The desktop bundles one inside its .app so a user who never
	// installed uv can still build an environment; unset, the search falls
	// through to PATH and the installer locations and never finds it.
	UVBinDir string
	// UVCacheDir overrides the shared cache under the astro cache root. An
	// embedder with a cache of its own wants one cache, not two, or the Python
	// toolchain and the Airflow wheel are downloaded once per cache.
	//
	// Must be absolute; a relative path is refused rather than resolved
	// against the project. Creating it is the embedder's job — uv makes it at
	// its own mode on first use, and a caller that wants a particular one has
	// to get there first.
	//
	// Worth being clear about the scope of "one cache": this keeps the
	// EMBEDDER's two consumers on one, not the machine's. A user who also runs
	// `astro local start` in a terminal still has the CLI's cache under the
	// astro root alongside it.
	UVCacheDir string
	// UVNoConfig passes --no-config, so uv ignores a uv.toml or a [tool.uv]
	// table discovered from the project upwards.
	//
	// Off by default for the reason HermeticUVEnv is: a uv a user configured
	// for their own shell is theirs. An embedder that supplies python,
	// dependencies and constraints itself can only be contradicted by one.
	//
	// It discards the PROJECT's [tool.uv] as well as the user's, which is the
	// half that surprises: a project pinning a private index there resolves
	// against public PyPI instead, and the failure names no flag.
	UVNoConfig bool
	// HealthTimeout bounds how long a start waits for Airflow to answer before
	// giving up. Zero takes each engine's own default, five minutes, which is
	// generous because a first run initializes the metadata database.
	//
	// Configurable because five minutes is a guess about somebody else's
	// machine. A cold Docker pull on a slow link can outlast it, and a caller
	// that would rather fail fast — CI, a scripted check, the suite that tests
	// what a start leaves behind when it does not finish — has no way to ask
	// for that otherwise.
	HealthTimeout time.Duration
	// ContainerBinary returns the container.binary setting for a project —
	// "docker" or "podman" pins the engine docker mode drives, anything else
	// (or a nil func) auto-detects from PATH, docker first. A func of the
	// project path because the setting can be per project (a v1
	// .astro/config.yaml over the global config), and the consumer owns
	// reading it: the CLI from its config package, Astro Desktop from its own
	// mirror of it. Called with "" where no one project is in question, which
	// asks for the global value.
	ContainerBinary func(projectPath string) string
}

// ErrHealthTimeout reports a start whose Airflow did not answer in the time
// allowed. Exported here because Config.HealthTimeout is set here: an embedder
// that offers its user a way to change the wait is the one that can usefully
// say so, and this is the error to say it on.
var ErrHealthTimeout = airflowrt.ErrHealthTimeout

// ErrUnsupportedBase reports a start refused because the project's declared
// Dockerfile does not build on an Astro Runtime image. Docker mode only: the
// refusal is about the compose file this runtime writes, and standalone builds
// no image at all.
var ErrUnsupportedBase = airflowrt.ErrUnsupportedBase

// ErrDatabaseNewerThanAirflow reports a start whose Airflow refused the
// project's metadata database because a newer Airflow had already upgraded it.
// Docker mode only for now: it is recognized from the migration container's
// output, which standalone does not have.
var ErrDatabaseNewerThanAirflow = airflowrt.ErrDatabaseNewerThanAirflow

// New returns a Runtime configured by cfg.
func New(cfg Config) *Runtime {
	docker := localdocker.New(cfg.RoutesDir, cfg.ProxyDaemon, cfg.Images)
	standalone := localstandalone.New(cfg.RoutesDir, cfg.ProxyDaemon, localstandalone.UVOptions{
		HermeticEnv:    cfg.HermeticUVEnv,
		OnCertFallback: cfg.OnUVCertFallback,
		BinDir:         cfg.UVBinDir,
		CacheDir:       cfg.UVCacheDir,
		NoConfig:       cfg.UVNoConfig,
	})
	// Applied to both engines or neither: a caller asking a start to give up
	// sooner means the start, not the standalone one.
	if cfg.HealthTimeout > 0 {
		docker.SetHealthTimeout(cfg.HealthTimeout)
		standalone.SetHealthTimeout(cfg.HealthTimeout)
	}
	docker.SetContainerBinary(cfg.ContainerBinary)
	return &Runtime{
		docker:         docker,
		standalone:     standalone,
		routes:         proxy.NewStore(cfg.RoutesDir, proxy.WithRouteLiveness(localprune.RouteAlive)),
		now:            time.Now,
		containersGone: docker.ContainersGoneAll,
		dockerStatuses: docker.StatusOfAll,
		daemon:         cfg.ProxyDaemon,
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
	st, reached := r.statusOfReached(rec)
	if !reached {
		// Bounding the probe gave a busy engine a way to miss its deadline,
		// and a missed deadline is indistinguishable from "no containers" —
		// which here would be permission to start a second Airflow over a
		// live one. Docker mode has no other double-start guard, and the
		// damage is not recoverable by retrying: publish overwrites the
		// record, the in-use port sends allocPort to a new one, and the
		// original stack is left running behind a stale route. So an engine
		// that did not answer refuses the start rather than waving it
		// through, the same way --clean refuses to delete what it could not
		// confirm.
		return fmt.Errorf(
			"cannot tell whether %s is already running: no container engine answered in time", p.ProjectPath)
	}
	if st.State != StateRunning {
		return nil
	}
	// Both wrap the sentinels a claim returns for the same two conditions.
	// external.go says the two refusals must not diverge and then they did:
	// a claim reported ErrForeignMode and ErrAlreadyRunning while this
	// reported prose, so the same condition was machine-readable through one
	// door and not the other. The wording still differs on purpose — Start
	// tells a person which command to type, a claim lets its consumer decide
	// — but what happened is now the same value either way.
	if rec.Mode != p.Mode {
		return fmt.Errorf("%w: local Airflow is already running for this project in %s mode; stop it first with `astro local stop`",
			ErrForeignMode, modeLabel(rec.Mode))
	}
	return fmt.Errorf("%w; use `astro local restart` to restart it or `astro local stop` to stop it", ErrAlreadyRunning)
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
// here: which requirements to install is the caller's decision, and
// Plan.Dependencies arrives the same way. The engine reads the manifest for one
// thing only, its [tool.uv] constraint-dependencies, which it hands to uv as a
// constraints file so the install stays within what the project allows. A
// manifest that does not load costs those constraints, not the install.
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

// Sync provisions a project's environment without starting Airflow on it.
//
// Start does this as its first step; this is that step alone, for a caller
// that needs the venv to exist for some reason other than running something.
// The editor is the case in hand: import resolution needs a populated venv,
// and making someone start Airflow to get code intelligence is a strange
// price for opening a file.
//
// It does NOT require a record, which makes it the only method here that
// works on a project nobody has started — exactly what it serves. It does
// refuse one that IS running, which is not the same question: uv sync
// resolves the whole set, removes what the manifest no longer names, and
// deletes the venv outright when a previous run left the completion marker
// off. Doing any of that under a live scheduler executing from that venv is
// not a caller's decision to weigh, because the caller here is an editor
// opening a file rather than anyone who asked. A caller that does want
// packages added to a running Airflow wants HotInstall, which leaves it up.
//
// Standalone only. A docker-mode project's packages live in an image, so the
// equivalent is a build, which is a different operation with a different cost.
func (r *Runtime) Sync(ctx context.Context, p Plan, cb Callbacks) error {
	if p.ProjectPath == "" {
		// filepath.Abs("") is the process's working directory, so without
		// this an empty plan provisions a venv wherever the embedder happens
		// to be standing.
		return errors.New("Sync needs a project path")
	}
	// The same normalization Start applies, and for the same reason: a plan
	// built for a command that asked for no particular runtime leaves Mode
	// empty, and standalone is the default. Re-deriving it here rather than
	// sharing Start's would make Sync refuse plans Start accepts.
	if planMode(p) == ModeDocker {
		return fmt.Errorf("%w: preparing the environment of a docker-mode project", ErrNotImplemented)
	}

	unlock, err := localstate.Lock(p.ProjectPath)
	if err != nil {
		return err
	}
	defer unlock()

	// After the lock, so the answer cannot go stale between reading it and
	// acting on it. A missing record is the ordinary case here and not an
	// error — that is the project this exists for.
	if rec, rerr := localstate.Load(p.ProjectPath); rerr == nil && r.statusOf(rec).State == StateRunning {
		return fmt.Errorf("%w: this project's local Airflow is running, and preparing its environment again would rebuild the venv underneath it", ErrNotImplemented)
	}

	return r.standalone.Sync(ctx, p, cb)
}

// planMode is Start's mode defaulting, shared so the read paths agree with it.
// A plan built for a command that asked for no specific runtime leaves Mode
// empty; standalone is the default.
func planMode(p Plan) Mode {
	if p.Mode == "" {
		return ModeStandalone
	}
	return p.Mode
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

// Stopped returns a handle whose Run, Shell and Env work in a standalone
// project's venv while no Airflow is running on it, under the environment a
// start of p would give Airflow. Commands that need only the project's Python —
// pytest, a script — then work with nothing started.
//
// It takes no lock and reads no record: the caller has already seen that
// nothing is running. Docker mode has no environment outside its containers,
// so a docker plan is refused.
func (r *Runtime) Stopped(p Plan) (Airflow, error) {
	if planMode(p) == ModeDocker {
		return nil, fmt.Errorf("%w: running a command in a stopped docker-mode project", ErrNotImplemented)
	}
	return r.standalone.Stopped(p)
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
	statuses, err := r.statusOfAll(recs)
	if err != nil {
		return nil, err
	}
	for i := range statuses {
		if statuses[i].Hostname == "" {
			statuses[i].Hostname = hostByProject[statuses[i].ProjectPath]
		}
	}
	return statuses, nil
}

// statusOfAll reports every record's live status, asking the container engines
// once for the whole list instead of once per docker record.
//
// Standalone liveness is a syscall, so those stay one at a time — it is the
// docker probes that shell out, and a long list multiplied them. Asking per
// record cost a machine with 26 of them 18 seconds for a command that prints a
// table, because each record that was not running also paid for a fall-through
// probe of the other engine.
//
// Order is preserved: the answers go back to the slots their records came
// from, so a caller reading the two lists together still lines them up.
func (r *Runtime) statusOfAll(recs []localstate.Record) ([]Status, error) {
	statuses := make([]Status, len(recs))
	var docker []localstate.Record
	var dockerAt []int
	for i := range recs {
		if recs[i].Mode == ModeDocker {
			docker = append(docker, recs[i])
			dockerAt = append(dockerAt, i)
			continue
		}
		statuses[i] = r.standalone.StatusOf(recs[i])
	}
	if len(docker) == 0 {
		// Nothing to ask an engine about, so no engine is asked. Structural
		// rather than left to the engine's own early return: the sweep is one
		// call instead of N now, but one call against a wedged podman is
		// still the wait this change exists to remove.
		return statuses, nil
	}
	answered, err := r.dockerStatuses(context.Background(), docker)
	if err != nil {
		return nil, err
	}
	for j := range answered {
		statuses[dockerAt[j]] = answered[j]
	}
	return statuses, nil
}

// statusOfReached is statusOf plus whether the engine behind it answered.
// Standalone liveness is a syscall on this machine's own process table, so it
// always answers; only a container engine can go quiet.
func (r *Runtime) statusOfReached(rec localstate.Record) (Status, bool) {
	if rec.Mode == ModeDocker {
		return r.docker.StatusOfReached(rec)
	}
	return r.standalone.StatusOf(rec), true
}

// statusOf reports one record's live status through the engine that owns its
// mode, so liveness is checked the way that mode records it.
func (r *Runtime) statusOf(rec localstate.Record) Status {
	if rec.Mode == ModeDocker {
		return r.docker.StatusOf(rec)
	}
	return r.standalone.StatusOf(rec)
}

// PruneStale removes the records, and the routes, of every runtime that is
// gone, and returns what it removed. It backs `astro local list --clean`.
//
// It can also stop the proxy: removing the last route puts the daemon away,
// the same rule Stop follows. Worth knowing for an embedder whose ProxyDaemon
// is an in-process server rather than a forked one — Astro Desktop's is — and
// which might otherwise call this on a timer without expecting its own proxy
// to go down whenever no project happens to be running.
//
// A sweep it could not start reaps nothing: the listing has to succeed before
// there is anything to remove, and a reap follows a removal rather than
// replacing one.
func (r *Runtime) PruneStale() ([]Status, error) {
	statuses, err := r.List()
	if err != nil {
		return nil, err
	}
	return r.pruneAll(statuses)
}

// staleDockerPaths is every docker record the sweep might remove: the ones
// already reporting stopped. A running project is not a candidate, so asking
// the engine about it would buy nothing.
func staleDockerPaths(statuses []Status) []string {
	var paths []string
	for i := range statuses {
		if statuses[i].Mode == ModeDocker && statuses[i].State != StateRunning {
			paths = append(paths, statuses[i].ProjectPath)
		}
	}
	return paths
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
	// Docker liveness cannot tell "compose gone" from "engine down", so the
	// containers are confirmed really absent before anything is deleted: a
	// blip in the daemon must not wipe a running project's record. Standalone
	// liveness is a syscall, so its stopped verdict is trusted as-is.
	//
	// Asked once for every candidate at once. Not only because an engine that
	// is down will not come back inside this loop — that was already true when
	// the question was asked per record — but because each asking shells out
	// twice, and a sweep of 26 stale records spent 18 seconds on it. One
	// machine-wide cause is also reported once rather than once per record.
	gone, engineErr := r.containersGone(context.Background(), staleDockerPaths(statuses))
	if engineErr != nil {
		// Not knowing is not the same as nothing to do. Unreported, --clean
		// says "no stale records to remove" when what happened is that it
		// could not go look.
		errs = append(errs, engineErr)
	}
	for i := range statuses {
		st := &statuses[i]
		if st.State == StateRunning {
			continue
		}
		if st.Mode == ModeDocker && !gone[st.ProjectPath] {
			continue
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
			// Through localshared, which is where a localrt route removal
			// reaps the proxy when it takes the last one. --clean used not to
			// go through it: it removed the route, removed the record, and
			// left the daemon for that runtime alive and listening with
			// nothing to serve — and nothing would collect it later, since the
			// next start adopts the daemon it finds rather than counting them.
			// One leaked per run of the e2e suite, which has a single --clean
			// case; twenty-six had accumulated on one machine before anybody
			// looked at a process list.
			if rerr := localshared.RemoveRoute(r.routes, st.Hostname, r.daemon); rerr != nil {
				errs = append(errs, rerr)
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
