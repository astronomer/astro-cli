//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// hostnames lists the proxy routes currently registered.
func hostnames(t *testing.T, e *Engine) []string {
	t.Helper()
	routes, err := e.routes.ReadRoutes()
	require.NoError(t, err)
	out := make([]string, 0, len(routes))
	for _, r := range routes {
		out = append(out, r.Hostname)
	}
	return out
}

// A start interrupted during the health wait leaves the project REACHABLE.
//
// Canceling deliberately leaves Airflow running with its record, so that
// Ctrl-C a moment before readiness does not throw away a working instance.
// What it must not do is leave it half-registered. Registering the route after
// the wait did exactly that: the record said running, `astro local status`
// printed a <name>.localhost URL, and no route existed to serve it — so that
// URL 404d for as long as the project stayed up, while `astro local start`
// refused to fix it because it could see a live runtime.
func TestStartInterruptedDuringHealthStillRegistersTheRoute(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Stand in for Ctrl-C arriving while the wait is in progress.
	e.health = func(hctx context.Context, _ string, _ time.Duration, _ airflowrt.HealthCheckConfig) error {
		cancel()
		return hctx.Err()
	}

	af, err := e.Start(ctx, p, rt.Callbacks{})
	require.ErrorIs(t, err, context.Canceled, "a canceled start reports the cancellation")
	assert.Nil(t, af)

	// Left running on purpose: the record and the process stay, so a later
	// stop can reap them.
	rec, lerr := localstate.Load(p.ProjectPath)
	require.NoError(t, lerr, "the record is kept so `astro local stop` can reap it")
	assert.True(t, procs.alive[fakePID], "Airflow is left starting, not killed")

	// And the part that was broken: it is routable.
	assert.Equal(t, []string{rec.Hostname}, hostnames(t, e),
		"an interrupted start must leave the route the record's url promises")
}

// A start that fails health is torn down completely — including the route,
// which is now added before the wait.
//
// Leaving it would point the proxy at a port this engine has just killed, and
// the next project to be allocated that port would answer for this hostname.
func TestStartWithFailedHealthRemovesTheRouteWithTheRecord(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error {
		return errors.New("never became healthy")
	}

	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.ErrorContains(t, err, "never became healthy")
	assert.Nil(t, af)

	_, lerr := localstate.Load(p.ProjectPath)
	assert.ErrorIs(t, lerr, localstate.ErrNotRunning, "the record is cleared on failed health")
	assert.False(t, procs.alive[fakePID], "the process group is killed")
	assert.Empty(t, hostnames(t, e), "the route goes with it, or it points at a killed port")
}

// The ordinary path is unchanged by the move: one route, once.
func TestStartRegistersExactlyOneRoute(t *testing.T) {
	e, _, _ := testEngine(t)
	p := testPlan(t)

	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	require.NotNil(t, af)

	rec, err := localstate.Load(p.ProjectPath)
	require.NoError(t, err)
	assert.Equal(t, []string{rec.Hostname}, hostnames(t, e))
}
