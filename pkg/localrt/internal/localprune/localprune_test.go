package localprune

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// loader builds a record lookup that returns rec for any project, or
// ErrNotRunning when found is false.
func loader(rec localstate.Record, found bool) func(string) (localstate.Record, error) {
	return func(string) (localstate.Record, error) {
		if !found {
			return localstate.Record{}, localstate.ErrNotRunning
		}
		return rec, nil
	}
}

func TestRouteAlive_StandaloneKeepsLiveRecordDespiteStaleRoutePID(t *testing.T) {
	// The route's stored PID is dead, but the owning record's process group
	// is alive under a different PID — the owner restarted. The route must
	// survive: this is the cross-owner eviction the default check gets wrong.
	rec := localstate.Record{Mode: rt.ModeStandalone, PID: 4242, Pgid: 4242}
	route := proxy.Route{Mode: proxy.RouteModeStandalone, PID: 999999, ProjectDir: "/p"}

	got := routeAlive(route, loader(rec, true), func(pgid int) bool { return pgid == 4242 })
	assert.True(t, got, "a route whose record's group is alive must be kept")
}

func TestRouteAlive_StandalonePrunesDeadRecord(t *testing.T) {
	rec := localstate.Record{Mode: rt.ModeStandalone, PID: 4242, Pgid: 4242}
	route := proxy.Route{Mode: proxy.RouteModeStandalone, PID: 4242, ProjectDir: "/p"}

	got := routeAlive(route, loader(rec, true), func(int) bool { return false })
	assert.False(t, got, "a route whose record's group is gone must be pruned")
}

func TestRouteAlive_DockerRecordAlwaysKept(t *testing.T) {
	// Containers outlive their starter, so a docker record is kept regardless
	// of any PID; explicit stop or `list --clean` removes it.
	rec := localstate.Record{Mode: rt.ModeDocker, ComposeProject: "astro-x"}
	route := proxy.Route{Mode: proxy.RouteModeDocker, PID: 999999, ProjectDir: "/p"}

	got := routeAlive(route, loader(rec, true), func(int) bool { return false })
	assert.True(t, got, "docker routes are not pruned by PID")
}

func TestRouteAlive_NoRecordFallsBackToRoutePID(t *testing.T) {
	// A v1-written route has no v2 record; fall back to the route's own
	// liveness — the same call the default predicate makes.
	dockerRoute := proxy.Route{Mode: proxy.RouteModeDocker, PID: 999999, ProjectDir: "/gone"}
	assert.True(t, routeAlive(dockerRoute, loader(localstate.Record{}, false), func(int) bool { return false }),
		"with no record, a docker route falls back to kept")

	origIsPIDAlive := proxy.IsPIDAlive
	defer func() { proxy.IsPIDAlive = origIsPIDAlive }()
	proxy.IsPIDAlive = func(int) bool { return false }
	standaloneRoute := proxy.Route{Mode: proxy.RouteModeStandalone, PID: 999999, ProjectDir: "/gone"}
	assert.False(t, routeAlive(standaloneRoute, loader(localstate.Record{}, false), func(int) bool { return true }),
		"with no record, a standalone route falls back to its own PID check")
}

func TestRouteAlive_EmptyModeTreatedAsStandalone(t *testing.T) {
	rec := localstate.Record{Mode: "", PID: 4242}
	route := proxy.Route{ProjectDir: "/p"}
	assert.True(t, routeAlive(route, loader(rec, true), func(pgid int) bool { return pgid == 4242 }))
}
