//go:build !windows

package localstandalone

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/internal/localstate"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func TestParseLogLine(t *testing.T) {
	t.Parallel()

	l := parseLogLine("scheduler  | 2026-03-16T18:33:51.933149Z [info     ] Adopting or resetting orphaned tasks [airflow.jobs.scheduler_job_runner.SchedulerJobRunner] loc=scheduler_job_runner.py:123")
	assert.Equal(t, "scheduler", l.Component)
	assert.Equal(t, "18:33:51 Adopting or resetting orphaned tasks", l.Text)
	assert.Equal(t, time.Date(2026, 3, 16, 18, 33, 51, 933149000, time.UTC), l.Time)

	// Uvicorn access logs collapse to method/path/status.
	l = parseLogLine(`api-server INFO:     127.0.0.1:49553 - "GET /api/v2/dags HTTP/1.1" 200 OK`)
	assert.Equal(t, "api-server", l.Component)
	assert.Equal(t, "GET /api/v2/dags 200", l.Text)

	// Unprefixed lines (tracebacks, banners) are "system" with no time.
	l = parseLogLine("Traceback (most recent call last):")
	assert.Equal(t, "system", l.Component)
	assert.True(t, l.Time.IsZero())
}

// What `airflow standalone` actually writes: the component name is colored, so
// the line begins with an escape sequence rather than the name.
//
// Every fixture above is uncolored, which is why this went unnoticed — and the
// symptom was silent. The prefix match missed, every line became "system", and
// `astro local logs --component dag-processor` printed nothing at all rather
// than saying the component was unknown.
//
// Captured from a real run: Airflow 3.1.8, `astro local logs --output json`.
func TestParseLogLineStripsTheColorAirflowWrites(t *testing.T) {
	t.Parallel()

	const colored = "\x1b[33mdag-processor\x1b[0m | \x1b[2m2026-09-16T01:03:24.123456Z\x1b[0m " +
		"[\x1b[32m\x1b[1minfo \x1b[0m] \x1b[1mSync 1 DAGs\x1b[0m " +
		"[\x1b[34mairflow.serialization.serialized_objects\x1b[0m] \x1b[36mloc\x1b[0m=\x1b[35mserialized_objects.py:2949\x1b[0m"

	l := parseLogLine(colored)
	assert.Equal(t, "dag-processor", l.Component, "the colored prefix must still identify the component")
	assert.Equal(t, "01:03:24 Sync 1 DAGs", l.Text)
	assert.NotContains(t, l.Text, "\x1b", "an escape sequence must not reach the rendered text or --output json")
	assert.Equal(t, time.Date(2026, 9, 16, 1, 3, 24, 123456000, time.UTC), l.Time)
}

// Every component standalone multiplexes, colored the way it arrives, so the
// filter works on all of them rather than on whichever one a fixture used.
func TestParseLogMetaFindsEveryColoredComponent(t *testing.T) {
	t.Parallel()

	for _, c := range logComponents {
		got, rest := parseLogMeta("\x1b[33m" + c + "\x1b[0m | some message")
		assert.Equal(t, c, got)
		assert.Equal(t, "| some message", rest)
	}
	// A name that is not a component is still system, colored or not.
	got, _ := parseLogMeta("\x1b[33mpostgres\x1b[0m | some message")
	assert.Equal(t, "system", got)
}

// writeTestLog writes a three-component log for a fake running project and
// returns its handle.
func logsFixture(t *testing.T, lines []string) (*airflow, *fakeProcs) {
	t.Helper()
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	var buf bytes.Buffer
	for _, l := range lines {
		fmt.Fprintln(&buf, l)
	}
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, logFileName), buf.Bytes(), 0o600))
	return af.(*airflow), procs
}

func testLogLines() []string {
	return []string{
		"scheduler  | 2026-07-21T10:00:01.000000Z one",
		"triggerer  | 2026-07-21T10:00:02.000000Z two",
		"scheduler  | 2026-07-21T10:00:03.000000Z three",
		"scheduler  | 2026-07-21T10:00:04.000000Z four",
	}
}

func TestLogsParsesAndFilters(t *testing.T) {
	af, _ := logsFixture(t, testLogLines())

	var got []rt.LogLine
	err := af.Logs(context.Background(), rt.LogOptions{
		Components: []string{"scheduler"},
		OnLine:     func(l rt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "10:00:01 one", got[0].Text)
	assert.Equal(t, "scheduler", got[0].Component)
}

func TestLogsTailAndSince(t *testing.T) {
	af, _ := logsFixture(t, testLogLines())

	var got []rt.LogLine
	err := af.Logs(context.Background(), rt.LogOptions{
		Tail:   2,
		OnLine: func(l rt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "10:00:03 three", got[0].Text)

	got = nil
	err = af.Logs(context.Background(), rt.LogOptions{
		Since:  time.Date(2026, 7, 21, 10, 0, 3, 0, time.UTC),
		OnLine: func(l rt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "10:00:03 three", got[0].Text)
}

// Writer mode hands its lines straight to the caller's io.Writer, so it has to
// strip escapes too. Suppressing colour at the launch site keeps them out of
// new log files; a file written before that, or by anything else, still has
// them, and forwarding control codes through one output while stripping them
// from the other is the kind of half-true that reads as fixed.
func TestLogsWriterStripsEscapesToo(t *testing.T) {
	af, _ := logsFixture(t, []string{
		"\x1b[33mscheduler\x1b[0m  | 2026-07-21T10:00:01.000000Z \x1b[1mone\x1b[0m",
	})

	var buf bytes.Buffer
	require.NoError(t, af.Logs(context.Background(), rt.LogOptions{Writer: &buf}))
	assert.NotContains(t, buf.String(), "\x1b", "an escape must not reach the caller's writer")
	assert.Contains(t, buf.String(), "scheduler  | 2026-07-21T10:00:01.000000Z one")
}

func TestLogsWriterGetsRawLines(t *testing.T) {
	af, _ := logsFixture(t, testLogLines()[:1])

	var buf bytes.Buffer
	require.NoError(t, af.Logs(context.Background(), rt.LogOptions{Writer: &buf}))
	assert.Equal(t, testLogLines()[0]+"\n", buf.String())

	// Exactly one sink.
	err := af.Logs(context.Background(), rt.LogOptions{})
	assert.ErrorContains(t, err, "exactly one")
	err = af.Logs(context.Background(), rt.LogOptions{Writer: &buf, OnLine: func(rt.LogLine) {}})
	assert.ErrorContains(t, err, "exactly one")
}

func TestLogsFollowDrainsAfterProcessDies(t *testing.T) {
	af, procs := logsFixture(t, testLogLines())
	procs.alive[fakePID] = false

	var got []rt.LogLine
	done := make(chan error, 1)
	go func() {
		done <- af.Logs(context.Background(), rt.LogOptions{
			Follow: true,
			OnLine: func(l rt.LogLine) { got = append(got, l) },
		})
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end after the process died")
	}
	assert.Len(t, got, 4)
}

func TestLogsFollowStopsOnCancel(t *testing.T) {
	af, _ := logsFixture(t, testLogLines())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- af.Logs(ctx, rt.LogOptions{Follow: true, OnLine: func(rt.LogLine) {}})
	}()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end on cancel")
	}
}

func TestLogsReadableAfterStop(t *testing.T) {
	// Stop removes the record but keeps the log file, so `astro local logs`
	// must still read it through a detached handle (no live record needed).
	e, _, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)

	stateDir, err := rt.StateDir(p.ProjectPath)
	require.NoError(t, err)
	var buf bytes.Buffer
	for _, l := range testLogLines() {
		fmt.Fprintln(&buf, l)
	}
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, logFileName), buf.Bytes(), 0o600))

	require.NoError(t, af.Stop(context.Background(), rt.StopOptions{}))
	_, err = localstate.Load(p.ProjectPath)
	require.ErrorIs(t, err, localstate.ErrNotRunning)

	// LogHandle reads the persisted file with no record, honoring --tail.
	handle, err := e.LogHandle(p.ProjectPath)
	require.NoError(t, err)
	var got []rt.LogLine
	err = handle.Logs(context.Background(), rt.LogOptions{
		Tail:   2,
		OnLine: func(l rt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "10:00:03 three", got[0].Text)
}

func TestLogsWithoutFileSaysStartFirst(t *testing.T) {
	e, _, _ := testEngine(t)
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, rt.Callbacks{})
	require.NoError(t, err)
	af, err := e.Attach(p.ProjectPath)
	require.NoError(t, err)
	err = af.Logs(context.Background(), rt.LogOptions{OnLine: func(rt.LogLine) {}})
	assert.ErrorContains(t, err, "no local Airflow logs yet")
}
