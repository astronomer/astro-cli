package localdocker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// hangingUp makes `compose up --detach` block until its context ends, as one
// waiting on a migration that never completes does.
func hangingUp(call string) time.Duration {
	if strings.Contains(call, " up --detach ") {
		return time.Hour
	}
	return 0
}

// An up that never returns is given up on at the health timeout and rolled
// back like any failed up: logs read first, then the containers this start
// created taken down, and no record written. Not as a health timeout, which
// says Airflow may still be coming up: here nothing is.
func TestAnUpThatOutlivesItsDeadlineIsGivenUpOn(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)
	var healthCalled bool
	e.health = func(context.Context, []string, time.Duration) error {
		healthCalled = true
		return nil
	}
	p := testPlan(t)

	began := time.Now()
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.ErrorIs(t, err, ErrStartTimedOut)
	assert.NotErrorIs(t, err, ErrHealthTimeout)
	assert.Less(t, time.Since(began), 10*time.Second, "the up must be cut off at its deadline, not waited out")
	assert.Contains(t, err.Error(), "did not finish starting in time after 50ms")
	assert.False(t, healthCalled, "there is nothing to wait for once the up has failed")
	assert.True(t, hasCall(cmd.calls, " logs "), "the logs are the diagnosis; calls were %v", cmd.calls)
	assert.True(t, hasCall(cmd.calls, " down "), "a timed-out up must be taken down; calls were %v", cmd.calls)
	if _, lerr := localstate.Load(p.ProjectPath); !errors.Is(lerr, localstate.ErrNotRunning) {
		t.Errorf("a timed-out up must leave no state record, got %v", lerr)
	}
}

// A revision the logs name is the precise failure, and is not masked by the
// timeout that ended the up.
func TestATimedOutUpStillNamesADatabaseNewerThanAirflow(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, delayFor: hangingUp}
	cmd.run = func(call string, s rt.Stdio) error {
		if strings.Contains(call, " logs ") {
			fmt.Fprintln(s.Out, "db-migration-1  | 2026-07-21T10:00:00Z alembic.util.exc.CommandError: Can't locate revision identified by '1d6611b6ab7c'")
		}
		return nil
	}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, airflowrt.ErrDatabaseNewerThanAirflow)
	assert.NotErrorIs(t, err, ErrStartTimedOut)
}

// The pull is not charged to the up's deadline, and the health wait gets the
// whole timeout after the up, as it did before the up had a deadline.
func TestThePullIsFreeAndTheWaitGetsTheWholeTimeout(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	cmd.delayFor = func(call string) time.Duration {
		if strings.Contains(call, " up --no-start ") {
			// Longer than the whole timeout: a cold pull on a slow link.
			return 200 * time.Millisecond
		}
		return 0
	}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(100 * time.Millisecond)
	var told time.Duration
	e.health = func(_ context.Context, _ []string, timeout time.Duration) error {
		told = timeout
		return nil
	}

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err, "a slow pull is not a timeout")
	assert.Equal(t, 100*time.Millisecond, told)
}

// Ctrl-C during the up is an interruption, not a timeout, and still cleans up.
func TestAnInterruptedUpIsNotATimeoutAndStillCleansUp(t *testing.T) {
	cmd := &fakeCmd{output: noProjects, delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	stop := time.AfterFunc(50*time.Millisecond, cancel)
	defer stop.Stop()

	_, err := e.Start(ctx, testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrStartTimedOut, "an interrupt is not the deadline")
	assert.True(t, hasCall(cmd.calls, " down "), "an interrupted up must still be taken down; calls were %v", cmd.calls)
}
