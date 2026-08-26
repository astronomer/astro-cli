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
}

// New returns a Runtime configured by cfg.
func New(cfg Config) *Runtime {
	return &Runtime{
		docker:     localdocker.New(cfg.RoutesDir, cfg.ProxyDaemon, cfg.Images),
		standalone: localstandalone.New(cfg.RoutesDir, cfg.ProxyDaemon),
		routes:     proxy.NewStore(cfg.RoutesDir, proxy.WithRouteLiveness(localprune.RouteAlive)),
		now:        time.Now,
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
	var removed []Status
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
			gone, cerr := r.docker.ContainersGone(context.Background(), st.ProjectPath)
			if cerr != nil || !gone {
				continue
			}
		}
		// Drop the route first, while the record still backs it, then the
		// record. Removing an absent record is not an error.
		if st.Hostname != "" {
			if _, rerr := r.routes.RemoveRoute(st.Hostname); rerr != nil {
				return removed, rerr
			}
		}
		if rerr := localstate.Remove(st.ProjectPath); rerr != nil {
			return removed, rerr
		}
		removed = append(removed, *st)
	}
	return removed, nil
}

// modeLabel is how a mode is named in a message to a person.
func modeLabel(m Mode) string {
	if m == ModeDocker {
		return "docker"
	}
	return "standalone"
}
