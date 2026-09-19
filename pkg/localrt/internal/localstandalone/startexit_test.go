//go:build !windows

package localstandalone

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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
	e, _, _ := testEngine(t)
	p := testPlan(t)
	e.healthTimeout = time.Hour
	e.health = neverHealthy

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := e.Start(ctx, p, rt.Callbacks{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "got %v", err)
	assert.False(t, strings.Contains(err.Error(), "exited while starting"),
		"a canceled start is not an exited one")
}
