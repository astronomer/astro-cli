package localdocker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// stuckStack is what `compose ps -a --format json` reports for the hang this
// deadline exists for: postgres crash-looping, the migration waiting on it, and
// the components never started because they wait on the migration. One object
// per line, the spelling compose has used since 2.21.
const stuckStack = `{"Service":"api-server","State":"created","ExitCode":0}
{"Service":"db-migration","State":"running","ExitCode":0}
{"Service":"postgres","State":"restarting","ExitCode":1}
{"Service":"scheduler","State":"created","ExitCode":0}
`

// stuckOutput answers the engine's queries about a stack stuck on postgres:
// nothing pre-existing, the states above, and postgres's refusal in its logs.
func stuckOutput(call string) ([]byte, error) {
	switch {
	case strings.Contains(call, " ps -a --format json"):
		return []byte(stuckStack), nil
	case strings.Contains(call, " logs ") && strings.HasSuffix(call, " postgres"):
		// Twice, as a crash loop prints it once per restart.
		return []byte("FATAL:  database files are incompatible with server\nDETAIL:  initialized by version 16\n" +
			"FATAL:  database files are incompatible with server\nDETAIL:  initialized by version 16\n"), nil
	}
	return nil, nil
}

// An up that never returns fails at the health timeout, says what it was
// waiting on, and takes down what it created.
//
// The hang this is about: the components depend on the migration completing,
// so `compose up --detach` waits inside itself, and a postgres that never comes
// up made it wait for good — while the health timeout, applied only after the
// up, never started.
func TestAnUpThatOutlivesTheBudgetFailsAsAHealthTimeout(t *testing.T) {
	cmd := &fakeCmd{output: stuckOutput}
	cmd.delayFor = func(call string) time.Duration {
		if strings.HasSuffix(call, " up --detach") {
			return time.Hour
		}
		return 0
	}
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
	require.Error(t, err)
	assert.Less(t, time.Since(began), 10*time.Second, "the up must be cut off at the budget, not waited out")

	// Classified as the health timeout, so --output json and the CLI's advice
	// treat it as one.
	require.ErrorIs(t, err, ErrHealthTimeout)
	msg := err.Error()
	for _, want := range []string{
		"after 50ms",
		"postgres is restarting",
		"never started, waiting on a dependency: api-server, scheduler",
		"Last output from postgres: FATAL:  database files are incompatible with server | DETAIL:  initialized by version 16",
	} {
		assert.Contains(t, msg, want)
	}
	// The failing service's logs, not the one merely waiting on it: the
	// migration saying "waiting for postgres" explains nothing.
	assert.NotContains(t, msg, "Last output from db-migration")
	assert.False(t, healthCalled, "there is nothing to wait for once the up has failed")

	// Cleaned up as any failed up is: the containers this start created go,
	// the record was never written.
	var down bool
	for _, c := range cmd.calls {
		down = down || strings.Contains(c, " down ")
	}
	assert.True(t, down, "a timed-out up must be taken down; calls were %v", cmd.calls)
	if _, lerr := localstate.Load(p.ProjectPath); !errors.Is(lerr, localstate.ErrNotRunning) {
		t.Errorf("a timed-out up must leave no state record, got %v", lerr)
	}
}

// The timeout does not license removing containers that were not this start's.
func TestAnUpThatOutlivesTheBudgetLeavesPreexistingContainersAlone(t *testing.T) {
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasSuffix(call, " ps -aq") {
			return []byte("abc123\n"), nil
		}
		return stuckOutput(call)
	}}
	cmd.delayFor = func(call string) time.Duration {
		if strings.HasSuffix(call, " up --detach") {
			return time.Hour
		}
		return 0
	}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, ErrHealthTimeout)
	for _, c := range cmd.calls {
		assert.NotContains(t, c, " down", "pre-existing containers must not be torn down; calls were %v", cmd.calls)
	}
	assert.Contains(t, err.Error(), "containers may remain under compose project")
}

// The health wait gets what the up left of the budget, and the pull is not
// charged to it.
func TestTheHealthWaitGetsWhatTheUpLeft(t *testing.T) {
	const budget = 2 * time.Second
	const upTook = 500 * time.Millisecond
	cmd := &fakeCmd{output: noProjects}
	cmd.delayFor = func(call string) time.Duration {
		switch {
		case strings.HasSuffix(call, " up --no-start --quiet-pull"):
			// Longer than the whole budget: a cold pull on a slow link. It
			// must not count, or a first start fails having made progress
			// the whole time.
			return budget + 500*time.Millisecond
		case strings.HasSuffix(call, " up --detach"):
			return upTook
		}
		return 0
	}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(budget)
	var (
		left    time.Duration
		bounded bool
		told    time.Duration
	)
	e.health = func(ctx context.Context, _ []string, timeout time.Duration) error {
		var deadline time.Time
		deadline, bounded = ctx.Deadline()
		left = time.Until(deadline)
		told = timeout
		return nil
	}

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err, "a slow pull is not a timeout")
	require.True(t, bounded, "the health wait must run under the start's deadline")
	assert.LessOrEqual(t, left, budget-upTook, "the wait must not get the whole budget again after the up spent some")
	assert.Positive(t, left)
	// Told the whole budget, which is what its timeout message reports.
	assert.Equal(t, budget, told)
}

// Ctrl-C during the up is an interruption, not a timeout, and still cleans up.
func TestAnInterruptedUpIsNotATimeoutAndStillCleansUp(t *testing.T) {
	cmd := &fakeCmd{output: stuckOutput}
	cmd.delayFor = func(call string) time.Duration {
		if strings.HasSuffix(call, " up --detach") {
			return time.Hour
		}
		return 0
	}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	stop := time.AfterFunc(50*time.Millisecond, cancel)
	defer stop.Stop()

	_, err := e.Start(ctx, testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrHealthTimeout, "an interrupt is not the budget running out")
	assert.ErrorIs(t, err, context.Canceled)
	var down bool
	for _, c := range cmd.calls {
		down = down || strings.Contains(c, " down ")
	}
	assert.True(t, down, "an interrupted up must still be taken down; calls were %v", cmd.calls)
}

// Both spellings compose has used for `ps --format json` are read.
func TestComposeServicesReadsBothJSONSpellings(t *testing.T) {
	want := []composeService{
		{Service: "postgres", State: "restarting", ExitCode: 1},
		{Service: "db-migration", State: "running"},
	}
	for name, out := range map[string]string{
		"array": `[{"Service":"postgres","State":"restarting","ExitCode":1},{"Service":"db-migration","State":"running","ExitCode":0}]`,
		"lines": "{\"Service\":\"postgres\",\"State\":\"restarting\",\"ExitCode\":1}\n{\"Service\":\"db-migration\",\"State\":\"running\",\"ExitCode\":0}\n",
	} {
		t.Run(name, func(t *testing.T) {
			cmd := &fakeCmd{output: func(string) ([]byte, error) { return []byte(out), nil }}
			e := testEngine(t, cmd)
			got, err := e.composeServices(context.Background(), composeLine{conn: engineConn{bin: "docker"}, name: "p"})
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

// What counts as pending, and whose logs explain it.
func TestPendingServices(t *testing.T) {
	t.Run("a failing service is named and its logs are the ones read", func(t *testing.T) {
		pending, stuck := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "exited", ExitCode: 3},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, []string{"postgres is exited with code 3", "never started, waiting on a dependency: scheduler"}, pending)
		assert.Equal(t, []string{"postgres"}, stuck)
	})
	t.Run("with nothing failing, what is still running is what is slow", func(t *testing.T) {
		pending, stuck := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "running"},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, []string{
			"never started, waiting on a dependency: scheduler",
			"still running: db-migration, postgres",
		}, pending)
		assert.Equal(t, []string{"db-migration", "postgres"}, stuck)
	})
	t.Run("a completed migration is done, not pending", func(t *testing.T) {
		pending, stuck := pendingServices([]composeService{
			{Service: "db-migration", State: "exited"},
			{Service: "postgres", State: "running"},
		})
		assert.Empty(t, pending)
		assert.Empty(t, stuck)
	})
}
