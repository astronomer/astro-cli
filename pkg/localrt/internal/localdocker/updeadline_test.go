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

// stuckStack is what `compose ps -a --format json` reports for the hang this
// deadline exists for: postgres crash-looping, the migration waiting on it, and
// the components never started because they wait on the migration. One object
// per line, the spelling compose has used since 2.21.
const stuckStack = `{"Service":"api-server","State":"created","ExitCode":0}
{"Service":"db-migration","State":"running","ExitCode":0}
{"Service":"postgres","State":"restarting","ExitCode":1}
{"Service":"scheduler","State":"created","ExitCode":0}
`

// podmanStuckStack is the same stack as rootless podman 5.8 reports it
// (fields trimmed to the ones read, values as sampled): no "restarting" — the
// crash-looping postgres shows as running with the code it last exited with,
// or as stopped.
const podmanStuckStack = `{"ExitCode":0,"Name":"astro-p-api-server-1","Service":"api-server","State":"created","Status":"Created"}
{"ExitCode":0,"Name":"astro-p-db-migration-1","Service":"db-migration","State":"running","Status":"Up 2 seconds"}
{"ExitCode":1,"Name":"astro-p-postgres-1","Service":"postgres","State":"running","Status":"Up Less than a second"}
{"ExitCode":0,"Name":"astro-p-scheduler-1","Service":"scheduler","State":"created","Status":"Created"}
`

// slowStack is an up that ran out of time with nothing failing: the migration
// is still running, the components still waiting on it.
const slowStack = `{"Service":"db-migration","State":"running","ExitCode":0}
{"Service":"postgres","State":"running","ExitCode":0}
{"Service":"scheduler","State":"created","ExitCode":0}
`

// postgresLogs is what a crash-looping postgres leaves in `compose logs`: the
// same refusal once per restart, prefixed the way compose prefixes every line.
const postgresLogs = "postgres-1  | FATAL:  database files are incompatible with server\n" +
	"postgres-1  | DETAIL:  initialized by version 16\n" +
	"postgres-1  | FATAL:  database files are incompatible with server\n" +
	"postgres-1  | DETAIL:  initialized by version 16\n"

// stackOutput answers the engine's queries about a stuck stack: nothing
// pre-existing, the given states, and logs per service.
func stackOutput(stack, logs string) func(string) ([]byte, error) {
	return func(call string) ([]byte, error) {
		switch {
		case strings.Contains(call, " ps -a --format json"):
			return []byte(stack), nil
		case strings.Contains(call, " logs "):
			return []byte(logs), nil
		}
		return nil, nil
	}
}

// hangingUp makes `compose up --detach` block until its context ends, as a
// real one waiting on a migration that never completes does.
func hangingUp(call string) time.Duration {
	if strings.Contains(call, " up --detach") {
		return time.Hour
	}
	return 0
}

func calls(cmd *fakeCmd, substr string) []string {
	var found []string
	for _, c := range cmd.calls {
		if strings.Contains(c, substr) {
			found = append(found, c)
		}
	}
	return found
}

// An up that never returns because a service keeps failing is given up on at
// the budget, says which service and what it said, and takes down what it
// created. Not as a health timeout: no budget would have been enough, so
// neither the CLI's "wait longer" advice nor --output json's "may still be
// coming up" applies.
//
// The hang this is about: the components depend on the migration completing,
// so `compose up --detach` waits inside itself, and a postgres that never comes
// up made it wait for good — while the health timeout, applied only after the
// up, never started.
func TestAnUpStuckOnAFailingServiceFailsAsThatService(t *testing.T) {
	cmd := &fakeCmd{output: stackOutput(stuckStack, postgresLogs), delayFor: hangingUp}
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

	require.ErrorIs(t, err, ErrServiceFailed)
	assert.NotErrorIs(t, err, ErrHealthTimeout, "a crash loop is not slow, and waiting longer does not fix it")
	msg := err.Error()
	for _, want := range []string{
		"gave up after 50ms",
		"postgres is restarting (last exit code 1)",
		"never started, waiting on a dependency: api-server, scheduler",
		"Last output from postgres: FATAL:  database files are incompatible with server | DETAIL:  initialized by version 16",
	} {
		assert.Contains(t, msg, want)
	}
	assert.NotContains(t, msg, "Last output from db-migration",
		"the failing service's logs, not the migration waiting on it")
	assert.False(t, healthCalled, "there is nothing to wait for once the up has failed")

	// The stuck service's logs read once, in one call, and not again by the
	// rollback: twice would repeat them in this error, and spend a second
	// capture budget on a daemon that may be why the up hung.
	logs := calls(cmd, " logs ")
	require.Len(t, logs, 1, "calls were %v", cmd.calls)
	assert.True(t, strings.HasSuffix(logs[0], " postgres"), logs[0])

	// Cleaned up as any failed up is: the containers this start created go,
	// the record was never written.
	assert.NotEmpty(t, calls(cmd, " down "), "a timed-out up must be taken down; calls were %v", cmd.calls)
	if _, lerr := localstate.Load(p.ProjectPath); !errors.Is(lerr, localstate.ErrNotRunning) {
		t.Errorf("a timed-out up must leave no state record, got %v", lerr)
	}
}

// On podman too, the error names the crashing service and quotes it, rather
// than the migration waiting on it — which is what it did before podman's
// spelling of a crash loop was recognized.
func TestAnUpStuckOnAFailingServiceOnPodmanNamesIt(t *testing.T) {
	cmd := &fakeCmd{output: stackOutput(podmanStuckStack, postgresLogs), delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, ErrServiceFailed)
	msg := err.Error()
	assert.Contains(t, msg, "postgres keeps exiting (last exit code 1)")
	assert.Contains(t, msg, "Last output from postgres: FATAL:  database files are incompatible with server")
	assert.NotContains(t, msg, "db-migration")
}

// An up that ran out of time with nothing failing IS a health timeout: slow,
// and a longer budget is the remedy. It names what was still running and
// quotes all of it, in one read.
func TestASlowUpFailsAsAHealthTimeout(t *testing.T) {
	logs := "db-migration-1  | Running upgrade a -> b\n" +
		"postgres-1  | database system is ready to accept connections\n"
	cmd := &fakeCmd{output: stackOutput(slowStack, logs), delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, ErrHealthTimeout)
	assert.NotErrorIs(t, err, ErrServiceFailed)
	msg := err.Error()
	for _, want := range []string{
		"timed out after 50ms, before the containers finished starting",
		"never started, waiting on a dependency: scheduler",
		"still running: db-migration, postgres",
		"Last output from db-migration: Running upgrade a -> b",
		"Last output from postgres: database system is ready to accept connections",
	} {
		assert.Contains(t, msg, want)
	}
	logCalls := calls(cmd, " logs ")
	require.Len(t, logCalls, 1, "one read for every stuck service; calls were %v", cmd.calls)
	assert.True(t, strings.HasSuffix(logCalls[0], " db-migration postgres"), logCalls[0])
}

// Containers that were there before this start are not removed after a
// timed-out up — but they are stopped, so a crash-looping postgres does not
// restart for as long as the engine runs with no record to stop it by.
func TestATimedOutUpStopsButKeepsPreexistingContainers(t *testing.T) {
	out := stackOutput(stuckStack, postgresLogs)
	cmd := &fakeCmd{output: func(call string) ([]byte, error) {
		if strings.HasSuffix(call, " ps -aq") {
			return []byte("abc123\n"), nil
		}
		return out(call)
	}, delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, ErrServiceFailed)
	assert.Empty(t, calls(cmd, " down"), "pre-existing containers must not be removed; calls were %v", cmd.calls)
	stops := calls(cmd, " stop ")
	require.Len(t, stops, 1, "calls were %v", cmd.calls)
	name, nerr := composeProjectName(testPlanPath(t, cmd))
	require.NoError(t, nerr)
	assert.Equal(t, fmt.Sprintf("docker compose --project-name %s stop --timeout %d", name, gracefulStopTimeout), stops[0])
	assert.Contains(t, err.Error(), "so they were stopped, not removed")
}

// testPlanPath recovers the project path a Start ran against from its up call.
func testPlanPath(t *testing.T, cmd *fakeCmd) string {
	t.Helper()
	for _, c := range cmd.calls {
		if _, rest, ok := strings.Cut(c, " --project-directory "); ok {
			dir, _, _ := strings.Cut(rest, " ")
			return dir
		}
	}
	t.Fatalf("no up call in %v", cmd.calls)
	return ""
}

// A migration revision the logs name joins the diagnosis rather than replacing
// it: both what was pending and the database's own failure survive, and each
// one's sentinel.
func TestATimedOutUpKeepsItsDiagnosisBesideADatabaseNewerFailure(t *testing.T) {
	logs := postgresLogs +
		"db-migration-1  | alembic.util.exc.CommandError: Can't locate revision identified by '1d6611b6ab7c'\n"
	stack := strings.Replace(stuckStack, `"db-migration","State":"running","ExitCode":0`,
		`"db-migration","State":"restarting","ExitCode":1`, 1)
	cmd := &fakeCmd{output: stackOutput(stack, logs), delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(50 * time.Millisecond)

	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.ErrorIs(t, err, airflowrt.ErrDatabaseNewerThanAirflow)
	require.ErrorIs(t, err, ErrServiceFailed)
	assert.Contains(t, err.Error(), "1d6611b6ab7c")
	assert.Contains(t, err.Error(), "Last output from postgres")
}

// steppingClock is a fake clock a test moves forward by hand.
type steppingClock struct{ t time.Time }

func (c *steppingClock) now() time.Time          { return c.t }
func (c *steppingClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newSteppingClock() *steppingClock {
	return &steppingClock{t: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)}
}
func (c *steppingClock) at(d time.Duration) func() { return func() { c.advance(d) } }

// The health wait gets what the up left of the budget, on the engine's clock,
// and the pull is not charged to it. And a timeout says so, rather than
// claiming it waited the whole budget.
func TestTheHealthWaitGetsWhatTheUpLeft(t *testing.T) {
	const budget = time.Minute
	clock := newSteppingClock()
	pull, up := clock.at(5*time.Minute), clock.at(40*time.Second)
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		switch {
		case strings.Contains(call, " up --no-start "):
			// Longer than the whole budget: a cold pull on a slow link. It
			// must not count, or a first start fails having made progress
			// the whole time.
			pull()
		case strings.Contains(call, " up --detach "):
			up()
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
	clock := newSteppingClock()
	cmd := &fakeCmd{output: noProjects}
	cmd.run = func(call string, _ rt.Stdio) error {
		if strings.Contains(call, " up --detach ") {
			clock.advance(time.Minute)
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

// The second up is quiet about pulls too.
func TestBothUpsPassQuietPull(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	_, err := e.Start(context.Background(), testPlan(t), rt.Callbacks{})
	require.NoError(t, err)
	ups := calls(cmd, " up ")
	require.Len(t, ups, 2, "calls were %v", cmd.calls)
	for _, c := range ups {
		assert.True(t, strings.HasSuffix(c, " --quiet-pull"), c)
	}
}

// Ctrl-C during the up is an interruption, not a timeout, and still cleans up.
func TestAnInterruptedUpIsNotATimeoutAndStillCleansUp(t *testing.T) {
	cmd := &fakeCmd{output: stackOutput(stuckStack, postgresLogs), delayFor: hangingUp}
	e := testEngine(t, cmd)
	e.SetHealthTimeout(time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	stop := time.AfterFunc(50*time.Millisecond, cancel)
	defer stop.Stop()

	_, err := e.Start(ctx, testPlan(t), rt.Callbacks{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrHealthTimeout, "an interrupt is not the budget running out")
	assert.NotErrorIs(t, err, ErrServiceFailed)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotEmpty(t, calls(cmd, " down "), "an interrupted up must still be taken down; calls were %v", cmd.calls)
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

// What counts as pending, whose logs explain it, and whether it is failing.
func TestPendingServices(t *testing.T) {
	t.Run("an exited service is failing, and its logs are the ones read", func(t *testing.T) {
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "exited", ExitCode: 3},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, []string{"postgres exited (3)", "never started, waiting on a dependency: scheduler"}, pending)
		assert.Equal(t, []string{"postgres"}, stuck)
		assert.True(t, failing)
	})
	t.Run("with nothing failing, what is still running is what is slow", func(t *testing.T) {
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "running"},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, []string{
			"never started, waiting on a dependency: scheduler",
			"still running: db-migration, postgres",
		}, pending)
		assert.Equal(t, []string{"db-migration", "postgres"}, stuck)
		assert.False(t, failing)
	})
	t.Run("podman's crash loop: running with the last exit code", func(t *testing.T) {
		// Podman has no "restarting": it restarts at once, and mid-loop ps
		// reports the container running with the code it last exited with.
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "running", ExitCode: 1},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, []string{
			"postgres keeps exiting (last exit code 1)",
			"never started, waiting on a dependency: scheduler",
		}, pending)
		assert.Equal(t, []string{"postgres"}, stuck, "the crashing service's logs, not the migration waiting on it")
		assert.True(t, failing)
	})
	t.Run("podman's crash loop: stopped with its exit code", func(t *testing.T) {
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "stopped", ExitCode: 1},
			{Service: "db-migration", State: "running"},
		})
		assert.Equal(t, "postgres is stopped (last exit code 1)", pending[0])
		assert.Equal(t, []string{"postgres"}, stuck)
		assert.True(t, failing)
	})
	t.Run("stopped with 0 is not failing: pending for a service", func(t *testing.T) {
		// Podman, mid-transition between restarts. Nothing says it failed.
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "scheduler", State: "created"},
			{Service: "postgres", State: "stopped"},
			{Service: "db-migration", State: "running"},
		})
		assert.False(t, failing)
		assert.Equal(t, []string{
			"never started, waiting on a dependency: scheduler",
			"still running: db-migration, postgres (stopped)",
		}, pending)
		assert.Equal(t, []string{"db-migration", "postgres"}, stuck)
	})
	t.Run("stopped with 0 is done for the migration", func(t *testing.T) {
		// Podman's word for a one-shot that finished.
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "db-migration", State: "stopped"},
			{Service: "postgres", State: "running"},
		})
		assert.Empty(t, pending)
		assert.Empty(t, stuck)
		assert.False(t, failing)
	})
	t.Run("a completed migration is done, not pending", func(t *testing.T) {
		pending, stuck, failing := pendingServices([]composeService{
			{Service: "db-migration", State: "exited"},
			{Service: "postgres", State: "running"},
		})
		assert.Empty(t, pending)
		assert.Empty(t, stuck)
		assert.False(t, failing)
	})
}

// The newest line is never the one dropped: a crash loop's FATAL repeats on
// every restart, and keeping its first occurrence pushed it out of the window.
func TestLastDistinctKeepsTheLastOccurrence(t *testing.T) {
	lines := []string{"FATAL", "a", "b", "c", "d", "e", "FATAL"}
	assert.Equal(t, []string{"b", "c", "d", "e", "FATAL"}, lastDistinct(lines, 5))
	assert.Equal(t, []string{"x", "y"}, lastDistinct([]string{"x", "x", "y", "y"}, 5))
}
