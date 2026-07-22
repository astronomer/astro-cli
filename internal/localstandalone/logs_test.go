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

	"github.com/astronomer/astro-cli/pkg/localrt"
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

// writeTestLog writes a three-component log for a fake running project and
// returns its handle.
func logsFixture(t *testing.T, lines []string) (*airflow, *fakeProcs) {
	t.Helper()
	e, procs, _ := testEngine(t)
	p := testPlan(t)
	af, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	stateDir, err := localrt.StateDir(p.ProjectPath)
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

	var got []localrt.LogLine
	err := af.Logs(context.Background(), localrt.LogOptions{
		Components: []string{"scheduler"},
		OnLine:     func(l localrt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "10:00:01 one", got[0].Text)
	assert.Equal(t, "scheduler", got[0].Component)
}

func TestLogsTailAndSince(t *testing.T) {
	af, _ := logsFixture(t, testLogLines())

	var got []localrt.LogLine
	err := af.Logs(context.Background(), localrt.LogOptions{
		Tail:   2,
		OnLine: func(l localrt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "10:00:03 three", got[0].Text)

	got = nil
	err = af.Logs(context.Background(), localrt.LogOptions{
		Since:  time.Date(2026, 7, 21, 10, 0, 3, 0, time.UTC),
		OnLine: func(l localrt.LogLine) { got = append(got, l) },
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "10:00:03 three", got[0].Text)
}

func TestLogsWriterGetsRawLines(t *testing.T) {
	af, _ := logsFixture(t, testLogLines()[:1])

	var buf bytes.Buffer
	require.NoError(t, af.Logs(context.Background(), localrt.LogOptions{Writer: &buf}))
	assert.Equal(t, testLogLines()[0]+"\n", buf.String())

	// Exactly one sink.
	err := af.Logs(context.Background(), localrt.LogOptions{})
	assert.ErrorContains(t, err, "exactly one")
	err = af.Logs(context.Background(), localrt.LogOptions{Writer: &buf, OnLine: func(localrt.LogLine) {}})
	assert.ErrorContains(t, err, "exactly one")
}

func TestLogsFollowDrainsAfterProcessDies(t *testing.T) {
	af, procs := logsFixture(t, testLogLines())
	procs.alive[fakePID] = false

	var got []localrt.LogLine
	done := make(chan error, 1)
	go func() {
		done <- af.Logs(context.Background(), localrt.LogOptions{
			Follow: true,
			OnLine: func(l localrt.LogLine) { got = append(got, l) },
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
		done <- af.Logs(ctx, localrt.LogOptions{Follow: true, OnLine: func(localrt.LogLine) {}})
	}()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end on cancel")
	}
}

func TestLogsWithoutFileSaysStartFirst(t *testing.T) {
	e, _, _ := testEngine(t)
	p := testPlan(t)
	_, err := e.Start(context.Background(), p, localrt.Callbacks{})
	require.NoError(t, err)
	af, err := e.Attach(p.ProjectPath)
	require.NoError(t, err)
	err = af.Logs(context.Background(), localrt.LogOptions{OnLine: func(localrt.LogLine) {}})
	assert.ErrorContains(t, err, "no local Airflow logs yet")
}
