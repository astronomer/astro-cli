// Package localprune decides which proxy routes survive pruning, using the
// state record as the source of truth. pkg/proxy prunes routes.json on every
// write, and two owners write it (the CLI on 6563, Astro Desktop on 6564).
// The default PID check evicts a route whenever the PID stored in it is not
// alive — but a route's PID and its owner's live PID diverge when the owner
// restarts, so a cross-owner prune can drop a route whose Airflow is still
// running. This predicate resolves each route to its record and keeps the
// route while the record's own mode-liveness says the runtime is alive.
package localprune

import (
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// groupAlive reports whether a standalone process group is still running. It
// is a package variable so tests can replace the syscall.
var groupAlive = defaultGroupAlive

// RouteAlive is the record-aware prune predicate wired into the CLI's proxy
// Store (see cmd/local). It keeps a route while the owning state record
// reports the runtime alive by that record's own mode's liveness check.
func RouteAlive(r proxy.Route) bool {
	return routeAlive(r, localstate.Load, groupAlive)
}

// routeAlive is RouteAlive with its record lookup and group check injected,
// so the decision is testable without touching disk or real processes.
func routeAlive(r proxy.Route, load func(string) (localstate.Record, error), alive func(pgid int) bool) bool {
	rec, err := load(r.ProjectDir)
	if err != nil {
		// No v2 record backs this route: it was written by a v1 CLI, or the
		// project directory is gone. Fall back to the route's own liveness,
		// the same call the default predicate makes.
		return r.Mode == proxy.RouteModeDocker || proxy.IsPIDAlive(r.PID)
	}
	switch rec.Mode {
	case rt.ModeDocker:
		// Containers outlive their starter, so the recorded PID is not a
		// liveness signal. A docker route is removed only by an explicit stop
		// or by `astro local list --clean`, which checks real compose state.
		return true
	case rt.ModeStandalone:
		return standaloneAlive(rec, alive)
	default:
		// Empty mode is treated as standalone, matching the Route.Mode contract.
		return standaloneAlive(rec, alive)
	}
}

// standaloneAlive checks the record's process group — its pgid, falling back
// to the master PID — not the route's stored PID, which is what lets the
// route survive an owner restart.
func standaloneAlive(rec localstate.Record, alive func(pgid int) bool) bool {
	pgid := rec.Pgid
	if pgid == 0 {
		pgid = rec.PID
	}
	return pgid > 0 && alive(pgid)
}
