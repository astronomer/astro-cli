// Package local is the v2 command tree: `astro local *`, `astro init`, the
// root aliases `astro start/stop/logs`, and the `astro dev` removal stub.
// It follows the cmd/ layer rules in docs/v2-architecture.md: parse flags,
// call one function, render output. All process state (stdio, the runtime,
// the working directory) arrives through Deps, built once in main; nothing
// here reads config at import time or holds mutable package state.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/browser"
	"github.com/spf13/cobra"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/internal/checks"
	"github.com/astronomer/astro-cli/internal/localdocker"
	"github.com/astronomer/astro-cli/internal/localprune"
	"github.com/astronomer/astro-cli/internal/localshared"
	"github.com/astronomer/astro-cli/internal/localstandalone"
	"github.com/astronomer/astro-cli/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// Deps is everything the v2 commands need from the process. The composition
// root (the v1 root, cmd/root.go) builds it once through NewDeps and hands it
// down.
type Deps struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// Runtime is the pkg/localrt surface the commands call.
	Runtime Runtime

	// Checks runs `astro local check`'s DAG parse against the project venv.
	Checks checks.Parser

	// CheckVenv parses DAGs in a scratch venv for a `--target` pre-flight
	// check. Production is the same *checks.VenvRunner as Checks; a test
	// injects a stub.
	CheckVenv checks.TargetParser

	// Provisioner builds and caches the scratch venvs a target check needs.
	// nil uses the uv-backed production provisioner; a test injects a fake.
	Provisioner func(ctx context.Context) (checks.Provisioner, error)

	// WorkingDir resolves the project path. Commands never call os.Getwd
	// themselves so tests can pin it.
	WorkingDir func() (string, error)

	// OpenURL opens a URL in the user's browser (`astro local open`).
	OpenURL func(url string) error
}

// Runtime mirrors the package-level functions of pkg/localrt as an
// interface, so commands can be tested without a real runtime.
type Runtime interface {
	Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error)
	Attach(projectPath string) (localrt.Airflow, error)
	// LogSource returns a handle for reading a project's logs. Unlike Attach
	// it also serves a stopped standalone project, whose log file outlives its
	// record; the handle's Logs reads that file.
	LogSource(projectPath string) (localrt.Airflow, error)
	ReadStatus(projectPath string) (localrt.Status, error)
	List() ([]localrt.Status, error)
	// PruneStale removes the records (and their routes) whose runtime is
	// gone, returning what it removed. It backs `astro local list --clean`.
	PruneStale() ([]localrt.Status, error)
}

// NewDeps builds the production Deps. Call it once, from main.
func NewDeps() Deps {
	runner := checks.NewVenvRunner()
	return Deps{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Runtime:     newModeRuntime(),
		Checks:      runner,
		CheckVenv:   runner,
		Provisioner: newUVProvisioner,
		WorkingDir:  os.Getwd,
		OpenURL:     browser.OpenURL,
	}
}

// modeRuntime is the production Runtime: it dispatches on localrt.Mode
// between the two engines, standalone (internal/localstandalone, an earlier fix)
// and docker (internal/localdocker, an earlier fix). Read paths dispatch on the
// mode the state record captured at start, so any tool stops what another
// started.
type modeRuntime struct {
	docker     *localdocker.Engine
	standalone *localstandalone.Engine
	// routes is the CLI's own view of routes.json, for the list join and the
	// --clean sweep. It carries the record-aware prune predicate so listing
	// never evicts a route whose owner is still alive.
	routes *proxy.Store
}

func newModeRuntime() modeRuntime {
	dir := routesDir()
	daemon := newProxyDaemon()
	return modeRuntime{
		docker:     localdocker.New(dir, daemon),
		standalone: localstandalone.New(dir, daemon),
		routes:     proxy.NewStore(dir, proxy.WithRouteLiveness(localprune.RouteAlive)),
	}
}

// proxyDaemon adapts airflow/proxy's daemon lifecycle to the engines'
// localshared.ProxyDaemon seam. It lives at the composition layer because
// airflow/proxy pulls in config, which the v2 engines must not import; they
// see only the interface.
type proxyDaemon struct{}

func (proxyDaemon) EnsureRunning() (string, error) {
	return proxydaemon.EnsureRunning(proxy.DefaultPort)
}

func (proxyDaemon) StopIfEmpty() { proxydaemon.StopIfEmpty() }

// newProxyDaemon returns the daemon seam, or nil on Windows, where the proxy
// is unsupported (decision 12): the engines skip the daemon and Airflow stays
// reachable on its direct localhost port.
func newProxyDaemon() localshared.ProxyDaemon {
	if runtime.GOOS == "windows" {
		return nil
	}
	return proxyDaemon{}
}

// routesDir is where pkg/proxy keeps routes.json: <astro home>/proxy, the
// same location v1 uses, honoring the same ASTRO_HOME override — v1 and v2
// must see each other's routes.
func routesDir() string {
	home := os.Getenv("ASTRO_HOME")
	if home == "" {
		home, _ = os.UserHomeDir() //nolint:errcheck // falls back to a relative path, matching v1's ASTRO_HOME handling
	}
	return filepath.Join(home, ".astro", "proxy")
}

func (r modeRuntime) Start(ctx context.Context, p localrt.Plan, cb localrt.Callbacks) (localrt.Airflow, error) {
	if p.Mode == "" {
		// The plan leaves Mode empty when the command asked for no specific
		// runtime (no --docker); standalone is the default.
		p.Mode = localrt.ModeStandalone
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
	if p.Mode == localrt.ModeDocker {
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
func (r modeRuntime) refuseLiveStart(p localrt.Plan) error {
	rec, err := localstate.Load(p.ProjectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.statusOf(rec).State != localrt.StateRunning {
		return nil
	}
	if rec.Mode != p.Mode {
		return fmt.Errorf("local Airflow is already running for this project in %s mode; stop it first with `astro local stop`", modeLabel(rec.Mode))
	}
	return fmt.Errorf("local Airflow is already running for this project; use `astro local restart` to restart it or `astro local stop` to stop it")
}

func (r modeRuntime) Attach(projectPath string) (localrt.Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err != nil {
		return nil, err
	}
	if rec.Mode == localrt.ModeDocker {
		return r.docker.Attach(projectPath)
	}
	return r.standalone.Attach(projectPath)
}

func (r modeRuntime) LogSource(projectPath string) (localrt.Airflow, error) {
	rec, err := localstate.Load(projectPath)
	if err == nil {
		if rec.Mode == localrt.ModeDocker {
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

func (r modeRuntime) ReadStatus(projectPath string) (localrt.Status, error) {
	rec, err := localstate.Load(projectPath)
	if errors.Is(err, localstate.ErrNotRunning) {
		return localrt.Status{ProjectPath: projectPath, State: localrt.StateStopped}, nil
	}
	if err != nil {
		return localrt.Status{}, err
	}
	if rec.Mode == localrt.ModeDocker {
		return r.docker.ReadStatus(projectPath)
	}
	return r.standalone.ReadStatus(projectPath)
}

func (r modeRuntime) List() ([]localrt.Status, error) {
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
	statuses := make([]localrt.Status, 0, len(recs))
	for _, rec := range recs {
		st := r.statusOf(rec)
		if st.Hostname == "" {
			st.Hostname = hostByProject[st.ProjectPath]
		}
		statuses = append(statuses, st)
	}
	return statuses, nil
}

// statusOf reports one record's live status through the engine that owns its
// mode, so liveness is checked the way that mode records it.
func (r modeRuntime) statusOf(rec localstate.Record) localrt.Status {
	if rec.Mode == localrt.ModeDocker {
		return r.docker.StatusOf(rec)
	}
	return r.standalone.StatusOf(rec)
}

func (r modeRuntime) PruneStale() ([]localrt.Status, error) {
	statuses, err := r.List()
	if err != nil {
		return nil, err
	}
	var removed []localrt.Status
	for _, st := range statuses {
		if st.State == localrt.StateRunning {
			continue
		}
		// Docker liveness cannot tell "compose gone" from "engine down", so
		// confirm the containers are really absent before deleting. A blip in
		// the daemon must not wipe a running project's record. Standalone
		// liveness is a syscall, so its stopped verdict is trusted as-is.
		if st.Mode == localrt.ModeDocker {
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
		removed = append(removed, st)
	}
	return removed, nil
}

// skipPreRunAnnotation mirrors internal/telemetry.SkipPreRunAnnotation. It
// is spelled out here because v2 packages never import config/, which
// internal/telemetry pulls in. The v1 root's PersistentPreRunE checks this
// annotation on the invoked command, so `astro local` stays offline: no
// network call runs before the command does.
const skipPreRunAnnotation = "skipPreRun"

// markSkipPreRun annotates cmd and every descendant. Cobra annotations do
// not inherit, and the v1 root reads the annotation off the leaf command.
func markSkipPreRun(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[skipPreRunAnnotation] = "true"
	for _, sub := range cmd.Commands() {
		markSkipPreRun(sub)
	}
}

// errAborted reports a confirmation answered "no".
var errAborted = errors.New("aborted")
