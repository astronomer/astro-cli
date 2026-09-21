package localrt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/proxy"
)

// No build constraint, unlike most of this package's tests: the reap wiring is
// the same on every platform, and a `daemon: cfg.ProxyDaemon` dropped from New
// would otherwise go uncaught on the Windows job. The one case here that needs
// a real process group lives in reapafterclean_live_test.go.

// recordingDaemon answers the ProxyDaemon seam and records WHEN it was asked,
// not just how often.
//
// The count alone cannot tell "reaped after the route went" from "reaped
// before it", and the difference decides whether the daemon actually stops:
// StopIfEmpty stops only when no routes remain, so a reap that runs before the
// removal it is reacting to sees the route still there and leaves the daemon
// up. Recording how many routes were registered at each call is what makes
// that ordering visible — a correct reap sees zero.
type recordingDaemon struct {
	routes *proxy.Store
	// whenAsked is the number of routes still registered at each ask.
	whenAsked []int
	ensured   int
}

func (d *recordingDaemon) EnsureRunning() (string, error) {
	d.ensured++
	return "6563", nil
}

func (d *recordingDaemon) StopIfEmpty() {
	// Safe to read here: the removal released the routes lock before asking.
	n := -1
	if rs, err := d.routes.ListRoutes(); err == nil {
		n = len(rs)
	}
	d.whenAsked = append(d.whenAsked, n)
}

func (d *recordingDaemon) asks() int { return len(d.whenAsked) }

// runtimeWatchingItsProxy is isolatedRuntime with a daemon that records.
func runtimeWatchingItsProxy(t *testing.T) (*Runtime, *recordingDaemon) {
	t.Helper()
	d := &recordingDaemon{}
	r := isolatedRuntime(t, d)
	d.routes = r.routes
	return r, d
}

// `astro local list --clean` puts the proxy away with the last route.
//
// Stop has reaped since it was written — "so the last project to stop leaves
// no orphan proxy behind" — and --clean, the other way a route is removed, did
// not. So the one command whose job is tidying up after a runtime that died
// badly left the proxy for that runtime alive and listening with nothing to
// serve, and nothing would ever collect it: the next start adopts the daemon
// it finds rather than counting them. One leaked per run of the e2e suite;
// twenty-six had accumulated on one machine before anybody looked.
func TestCleanPutsTheProxyAwayWithTheLastRoute(t *testing.T) {
	rtime, daemon := runtimeWatchingItsProxy(t)
	gone := staleRecordFor(t, t.TempDir())

	removed, err := rtime.PruneStale()
	require.NoError(t, err)
	require.Len(t, removed, 1, "the stale record should have been pruned")
	assert.Equal(t, gone, removed[0].ProjectPath)

	require.Equal(t, 1, daemon.asks(),
		"--clean took the last route and never asked the proxy to stand down")
	assert.Equal(t, 0, daemon.whenAsked[0],
		"asked before the route was actually gone, so StopIfEmpty would decline")
}

// A docker record, which is the case ordering can break.
//
// localprune keeps a docker route unconditionally — containers outlive the
// process that started them, so the recorded PID says nothing — which means
// List does not evict it and only this sweep ever removes it. A reap that ran
// before the removal would find that route still registered, leave the daemon
// up, and the bug would survive for every docker project while a call-count
// assertion stayed green.
func TestCleanPutsTheProxyAwayAfterADockerRouteGoes(t *testing.T) {
	rtime, daemon := runtimeWatchingItsProxy(t)
	rtime.containersGone = func(_ context.Context, paths []string) (map[string]bool, error) {
		return allGone(paths), nil
	}

	require.NoError(t, rtime.routes.AddRoute(&proxy.Route{
		Hostname:   "boxed.localhost",
		Port:       "8080",
		ProjectDir: "/p/boxed",
		Mode:       proxy.RouteModeDocker,
	}))

	removed, err := rtime.pruneAll([]Status{
		{ProjectPath: "/p/boxed", Mode: ModeDocker, State: StateStopped, Hostname: "boxed.localhost"},
	})
	require.NoError(t, err)
	require.Len(t, removed, 1)

	require.Equal(t, 1, daemon.asks(), "the docker route went and the proxy was not asked")
	assert.Equal(t, 0, daemon.whenAsked[0],
		"asked while the docker route was still registered, so the daemon would stay up")
}

// And it leaves the proxy alone while anything still needs it.
//
// This is the assertion that keeps the reap honest. Asking the daemon to
// decide for itself meant airflow/proxy.StopIfEmpty, which re-lists through a
// store built with no liveness predicate: the default one judges a route by
// its recorded PID, and a standalone master exits before the rest of its group
// during shutdown, so that store calls a live project's route dead. ListRoutes
// writes back what it pruned, so the wrong answer was also destructive. The
// count comes from the store that carries the record-aware predicate instead,
// and a remaining route means no ask at all.
func TestCleanLeavesTheProxyWhileARouteRemains(t *testing.T) {
	rtime, daemon := runtimeWatchingItsProxy(t)
	rtime.containersGone = func(_ context.Context, paths []string) (map[string]bool, error) {
		return allGone(paths), nil
	}

	// One route that survives the sweep, and one that does not.
	for _, r := range []*proxy.Route{
		{Hostname: "stays.localhost", Port: "8081", ProjectDir: "/p/stays", Mode: proxy.RouteModeDocker},
		{Hostname: "goes.localhost", Port: "8082", ProjectDir: "/p/goes", Mode: proxy.RouteModeDocker},
	} {
		require.NoError(t, rtime.routes.AddRoute(r))
	}

	removed, err := rtime.pruneAll([]Status{
		{ProjectPath: "/p/goes", Mode: ModeDocker, State: StateStopped, Hostname: "goes.localhost"},
	})
	require.NoError(t, err)
	require.Len(t, removed, 1)

	assert.Zero(t, daemon.asks(),
		"a route is still registered; stopping the proxy would strand the project it serves")
	assert.Zero(t, daemon.ensured, "--clean has no reason to start a proxy")
}
