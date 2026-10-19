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

func hasCallWith(cmd *fakeCmd, substr string) bool {
	for _, c := range cmd.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// An up that never returns fails at the health timeout and is rolled back
// like any failed up: logs read first, then the containers this start created
// taken down, and no record written.
func TestAnUpThatOutlivesTheBudgetFailsAsAHealthTimeout(t *testing.T) {
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
	require.ErrorIs(t, err, ErrHealthTimeout)
	assert.Less(t, time.Since(began), 10*time.Second, "the up must be cut off at the budget, not waited out")
	assert.Contains(t, err.Error(), "timed out after 50ms, before the containers finished starting")
	assert.False(t, healthCalled, "there is nothing to wait for once the up has failed")
	assert.True(t, hasCallWith(cmd, " logs "), "the logs are the diagnosis; calls were %v", cmd.calls)
	assert.True(t, hasCallWith(cmd, " down "), "a timed-out up must be taken down; calls were %v", cmd.calls)
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
	assert.NotErrorIs(t, err, ErrHealthTimeout)
}

// steppingClock is a fake clock a test moves forward by hand.
type steppingClock struct{ t time.Time }

func (c *steppingClock) now() time.Time { return c.t }

// The health wait gets what the up left of the budget, on the engine's clock;
// the pull is not charged to it; and a timeout says so, rather than claiming
// it waited the whole budget.
func TestTheHealthWaitGetsWhatTheUpLeft(t *testing.T) {
	const budget = time.Minute
	clock := &steppingClock{t: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)}
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		switch {
		case strings.Contains(call, " up --no-start "):
			// Longer than the whole budget: a cold pull on a slow link.
			clock.t = clock.t.Add(5 * time.Minute)
		case strings.Contains(call, " up --detach "):
			clock.t = clock.t.Add(40 * time.Second)
		}
		return nil
	}
	e := testEngine(t, cmd)
	e.now = clock.now
	e.SetHealthTimeout(budget)
	var told time.Duration
	e.health = func(_ context.Context, _ []string, timeout time.Duration) error {
		told = timeout
		return fmt.Errorf("%w after %s", ErrHealthTimeout, timeout)
	}

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	assert.Equal(t, 20*time.Second, told, "the wait gets the budget less what the up took, and the pull is free")
	require.ErrorIs(t, err, ErrHealthTimeout)
	assert.Contains(t, err.Error(), "after 20s (what was left of the 1m0s budget: the containers took 40s of it to start)")
}

// An up that used the whole budget still leaves the wait time for a probe.
func TestTheHealthWaitAlwaysGetsAProbe(t *testing.T) {
	clock := &steppingClock{t: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)}
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up --detach ") {
			clock.t = clock.t.Add(time.Minute)
		}
		return nil
	}
	e := testEngine(t, cmd)
	e.now = clock.now
	e.SetHealthTimeout(time.Minute)
	var told time.Duration
	e.health = func(_ context.Context, _ []string, timeout time.Duration) error {
		told = timeout
		return nil
	}
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, minHealthWait, told)
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
	assert.NotErrorIs(t, err, ErrHealthTimeout, "an interrupt is not the budget running out")
	assert.True(t, hasCallWith(cmd, " down "), "an interrupted up must still be taken down; calls were %v", cmd.calls)
}
