//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"strconv"
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

	// The message has to say so too. Everything above can hold while the
	// person who pressed Ctrl-C is told "context canceled" and has no way to
	// know an Airflow is now running on their machine — which is what the bare
	// context error said before.
	assert.Contains(t, err.Error(), "still starting",
		"the message must say the runtime was left up")
	assert.Contains(t, err.Error(), strconv.Itoa(rec.Port),
		"the message must name the port it will answer on")
	assert.Contains(t, err.Error(), "astro local stop",
		"the message must say how to end what it left running")
	assert.NotContains(t, err.Error(), "context canceled",
		"the mechanism is not the message")
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

// A session-tied start does not claim to have left anything running.
//
// --stop-with-session arms the supervisor's parent watch, so Airflow is killed
// the moment this process exits — a beat after the interrupt. Telling the
// reader it is "still starting on port N" is false by the time they finish
// reading it, and the record left behind turns up as "stopped (stale)" in the
// next `astro local list`. So this start is torn down like any other that did
// not finish, and says so.
func TestASessionTiedStartIsTornDownWhenInterrupted(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	p.StopWithSession = true

	ctx, cancel := context.WithCancel(context.Background())
	e.health = func(hctx context.Context, _ string, _ time.Duration, _ airflowrt.HealthCheckConfig) error {
		cancel()
		return hctx.Err()
	}

	af, err := e.Start(ctx, p, rt.Callbacks{})
	require.ErrorIs(t, err, context.Canceled, "a canceled start still reports the cancellation")
	assert.Nil(t, af)

	assert.Contains(t, err.Error(), "stopped with this session",
		"the message must not promise a runtime the supervisor is about to reap")
	assert.NotContains(t, err.Error(), "still starting",
		"this is the claim that was false under --stop-with-session")

	assert.False(t, procs.alive[fakePID], "the runtime is not left for the supervisor to reap")
	_, lerr := localstate.Load(p.ProjectPath)
	assert.Error(t, lerr, "no record is left to show up as stopped (stale)")
	assert.Empty(t, hostnames(t, e), "no route is left pointing at a killed port")
}

// An interrupt that coincides with Airflow dying reports the death, not the
// interrupt.
//
// waitHealthy knows the difference and says so, with the log path. Preferring
// the cancellation threw that away and announced a runtime on a port where
// nothing was listening — the previous message was vague, this one would have
// been wrong.
func TestADeadAirflowIsReportedEvenWhenTheStartWasInterrupted(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)

	ctx, cancel := context.WithCancel(context.Background())
	e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error {
		// Both at once: Airflow is gone AND the user pressed Ctrl-C.
		procs.alive[fakePID] = false
		cancel()
		return errors.New("Airflow exited while starting /tmp/airflow.log")
	}

	af, err := e.Start(ctx, p, rt.Callbacks{})
	require.Error(t, err)
	assert.Nil(t, af)

	assert.Contains(t, err.Error(), "exited while starting",
		"the health wait knew what happened and its answer is the better one")
	assert.NotContains(t, err.Error(), "still starting on port",
		"nothing is listening, so nothing may be advertised")
}

// An interrupt during the sync does not go on to start an Airflow.
//
// uv can finish in the same moment the signal arrives, and a command that
// succeeded reports no error whatever the context says — so without a check
// between the two, a start canceled during provisioning launched Airflow
// anyway and the health-wait branch then left it running. That is the opposite
// of what an interrupt during a sync promises, and it would have shown up as a
// flake rather than a failure.
func TestAnInterruptDuringTheSyncNeverLaunches(t *testing.T) {
	e, procs, launches := testEngine(t)
	p := testPlan(t)

	ctx, cancel := context.WithCancel(context.Background())
	e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
		// The sync succeeds, and the interrupt lands as it returns.
		cancel()
		return fakeUV{}, nil
	}

	af, err := e.Start(ctx, p, rt.Callbacks{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, af)
	assert.Contains(t, err.Error(), "not started")

	assert.Empty(t, *launches, "nothing may be launched under a context that is already done")
	assert.False(t, procs.alive[fakePID])
	_, lerr := localstate.Load(p.ProjectPath)
	assert.Error(t, lerr, "no record for a runtime that was never started")
	assert.Empty(t, hostnames(t, e), "no route for a runtime that was never started")
}

// Every way a start can end reports a terminal state.
//
// A consumer driving a UI from Callbacks watches for one, and an exit that
// emits nothing leaves the stream on StateStarting forever — silence it has to
// guess about. Both interrupted exits were such an exit, and so were the ten
// or so failure paths that were never given a per-site emit; the deferred one
// in Start covers them by construction.
//
// Driven over the exits rather than asserted on one, because the thing being
// checked is that no exit is missed.
func TestEveryFailingExitReportsATerminalState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setUp func(e *Engine, p *rt.Plan, cancel context.CancelFunc)
	}{
		{
			name: "interrupted with the runtime left up",
			setUp: func(e *Engine, _ *rt.Plan, cancel context.CancelFunc) {
				e.health = func(hctx context.Context, _ string, _ time.Duration, _ airflowrt.HealthCheckConfig) error {
					cancel()
					return hctx.Err()
				}
			},
		},
		{
			name: "interrupted on a session-tied start",
			setUp: func(e *Engine, p *rt.Plan, cancel context.CancelFunc) {
				p.StopWithSession = true
				e.health = func(hctx context.Context, _ string, _ time.Duration, _ airflowrt.HealthCheckConfig) error {
					cancel()
					return hctx.Err()
				}
			},
		},
		{
			name: "interrupted before anything was launched",
			setUp: func(e *Engine, _ *rt.Plan, cancel context.CancelFunc) {
				e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
					cancel()
					return fakeUV{}, nil
				}
			},
		},
		{
			name: "the environment could not be built",
			setUp: func(e *Engine, _ *rt.Plan, _ context.CancelFunc) {
				e.uv = func(context.Context, func(rt.LogLine)) (venvSyncer, error) {
					return nil, errors.New("no uv on this machine")
				}
			},
		},
		{
			name: "airflow never became healthy",
			setUp: func(e *Engine, _ *rt.Plan, _ context.CancelFunc) {
				e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error {
					return errors.New("Airflow exited while starting /tmp/airflow.log")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			p := testPlan(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tc.setUp(e, &p, cancel)

			var states []rt.State
			var reported error
			cb := rt.Callbacks{OnState: func(s rt.State, err error) {
				states = append(states, s)
				if s == rt.StateError {
					reported = err
				}
			}}

			_, err := e.Start(ctx, p, cb)
			require.Error(t, err, "this case is supposed to fail")

			require.NotEmpty(t, states)
			assert.Equal(t, rt.StateError, states[len(states)-1],
				"the stream ends on %v, so a consumer is left waiting", states)
			assert.Equal(t, err, reported,
				"the state carries the error the caller got, not a prefix of it")
		})
	}
}
