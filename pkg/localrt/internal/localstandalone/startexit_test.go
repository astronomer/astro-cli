//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// neverHealthy is what a dead Airflow looks like from the port's side: nothing
// ever answers, and the wait ends only when something else ends it.
func neverHealthy(ctx context.Context, _ string, _ time.Duration, _ airflowrt.HealthCheckConfig) error {
	<-ctx.Done()
	return ctx.Err()
}

// exitsAfter makes the process group stop existing once d has passed, which is
// how a child that cannot import behaves. Driven through the kill seam rather
// than by writing fakeProcs from another goroutine: every liveness probe is a
// signal 0 on the waiting goroutine, so there is nothing to race.
func exitsAfter(e *Engine, procs *fakeProcs, d time.Duration) {
	gone := time.Now().Add(d)
	e.kill = func(pid int, sig syscall.Signal) error {
		if sig == 0 && time.Now().After(gone) {
			return syscall.ESRCH
		}
		return procs.kill(pid, sig)
	}
}

// A start whose Airflow dies has to say so when it dies, not when the timeout
// runs out.
//
// The health check cannot tell "not up yet" from "never coming up", so the
// wait ran its full five minutes and then reported that it had timed out. An
// Airflow 2 project on a Python it cannot run dies in about five seconds: the
// report was right about nothing except that time had passed.
func TestStartStopsWaitingOnceAirflowHasExited(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)

	// Long enough that returning promptly can only be the exit check.
	e.healthTimeout = time.Hour
	e.health = neverHealthy
	exitsAfter(e, procs, 20*time.Millisecond)

	start := time.Now()
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited while starting",
		"the error should name what happened, not that a wait elapsed")
	assert.Less(t, elapsed, 30*time.Second,
		"it waited out the timeout instead of noticing the exit")
}

// Wherever the failure is reported, the output explaining it has to be
// findable. `astro local logs` cannot reach it: a failed start clears the
// record, and the record is what logs resolves through.
func TestStartNamesTheLogWhenItGivesUp(t *testing.T) {
	t.Run("the child exited", func(t *testing.T) {
		e, procs, _ := testEngine(t)
		p := testPlan(t)
		e.healthTimeout = time.Hour
		e.health = neverHealthy
		exitsAfter(e, procs, 20*time.Millisecond)

		_, err := e.Start(context.Background(), p, rt.Callbacks{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), logFileName, "the error should name the log file")
	})

	t.Run("the health check timed out", func(t *testing.T) {
		e, _, _ := testEngine(t)
		p := testPlan(t)
		// Alive throughout, so the only way out is the health error itself.
		e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error {
			return errors.New("health check timed out after 5m0s")
		}

		_, err := e.Start(context.Background(), p, rt.Callbacks{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "health check timed out", "the original reason survives")
		assert.Contains(t, err.Error(), logFileName, "and now it says where to look")
	})
}

// A start that never becomes healthy has to take Airflow down with it, even
// when Airflow will not go quietly.
//
// The teardown sent one SIGTERM and immediately removed the record and the
// route. `airflow standalone` forwards the signal to the scheduler, the
// api-server and the triggerer, and a component wedged in an import does not
// take it — which is not a corner case here, because a wedged component is one
// of the ordinary reasons a start never becomes healthy in the first place. So
// the most likely failure left a live process group that nothing on disk named:
// status showed nothing, stop had nothing to stop, and the port stayed held.
func TestAFailedStartKillsAGroupThatIgnoresSIGTERM(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)

	procs.onTerm = func(int) bool { return false }
	e.stopTimeout = 50 * time.Millisecond
	e.stopPoll = 5 * time.Millisecond
	// Alive throughout: the only way out of the wait is the health error, so
	// this is the timeout path rather than the exited-child path.
	e.health = func(context.Context, string, time.Duration, airflowrt.HealthCheckConfig) error {
		return fmt.Errorf("%w after 20ms", airflowrt.ErrHealthTimeout)
	}

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.Error(t, err)

	// Reality, not bookkeeping. That the record and the route were removed was
	// already asserted elsewhere and was true the whole time the orphan ran.
	assert.False(t, procs.alive[fakePID],
		"the failed start returned with Airflow still running and nothing left naming it")
	assert.Equal(t,
		[]string{
			fmt.Sprintf("%d:%v", -fakePID, syscall.SIGTERM),
			fmt.Sprintf("%d:%v", -fakePID, syscall.SIGKILL),
		},
		procs.sigs,
		"ask first, then insist — the same sequence `astro local stop` uses")
}

// The log it names has to be the log it wrote.
//
// Plan.StateDir exists so an embedder can put the runtime state somewhere else,
// and planStateDir honors it — so deriving the path a second time from
// rt.StateDir, which is only the fallback, named a file that does not exist
// while the real output sat where the caller had asked for it.
func TestStartNamesTheLogTheCallerAskedFor(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	p.StateDir = t.TempDir()

	e.healthTimeout = time.Hour
	e.health = neverHealthy
	exitsAfter(e, procs, 20*time.Millisecond)

	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Join(p.StateDir, logFileName),
		"the error must name the log under the state directory the plan chose")
}

// Cancellation is not failure. The caller distinguishes them, and a start
// interrupted with Ctrl-C deliberately leaves Airflow coming up — so the exit
// check must not turn that into an error about a dead child.
func TestStartStillReportsCancellationAsCancellation(t *testing.T) {
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	e.healthTimeout = time.Hour
	e.health = neverHealthy

	ctx, cancel := context.WithCancel(context.Background())
	// The interrupt lands on the exit check's first liveness probe. That is
	// the moment worth testing and the only one that can confuse the two
	// outcomes: the wait is genuinely in progress, the poll has just found
	// the group alive, and the cancellation arrives on top of it.
	//
	// It used to land 20 milliseconds after Start was called, which is a race
	// against everything Start does before the wait — the sync, the venv
	// check, the launch. Lose that race and the cancel is seen by the
	// pre-launch check instead, which correctly reports "Airflow was not
	// started": a different branch, a different outcome, and an assertion
	// failure saying the message is wrong when what happened is that the case
	// measured something else. It failed on a loaded machine, and ten times
	// out of ten under -count, having passed every quiet single run.
	//
	// Driven through the kill seam for the reason exitsAfter gives: a
	// liveness probe is a signal 0 on the waiting goroutine, so hanging the
	// cancellation off one orders it against the wait by construction rather
	// than by the clock.
	var once sync.Once
	e.kill = func(pid int, sig syscall.Signal) error {
		if sig == 0 {
			once.Do(cancel)
		}
		return procs.kill(pid, sig)
	}

	_, err := e.Start(ctx, p, rt.Callbacks{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "got %v", err)

	// Asserted on what the start decided, not on a phrase the message no
	// longer has any way to contain. This used to check that the text did not
	// say "exited while starting", which stopped being able to fail once a
	// canceled start got an error type of its own: the string is unreachable
	// by construction, so the guard passed for the wrong reason. Whether the
	// cancellation was mistaken for a dead child is visible in the outcome —
	// a dead child is torn down, a canceled start is left running and says so.
	assert.Contains(t, err.Error(), "still starting",
		"a canceled start leaves Airflow coming up, and the message says which outcome it took")
	assert.True(t, procs.alive[fakePID], "a canceled start is not an exited one")
}
