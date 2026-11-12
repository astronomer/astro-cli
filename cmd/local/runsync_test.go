//go:build !windows

package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// With nothing running, run and shell sync the venv with the plan a start would
// build, and only then hand back the handle.
func TestRunWithNothingRunningSyncsFirst(t *testing.T) {
	for _, cmd := range [][]string{{"local", "run", "pytest"}, {"local", "shell"}} {
		t.Run(cmd[1], func(t *testing.T) {
			t.Setenv("UV_NO_SYNC", "")
			d, _, calls := stoppedProject(t, "")
			if err := execute(t, d, cmd...); err != nil {
				t.Fatal(err)
			}
			dir, _ := d.WorkingDir()
			if len(calls.synced) != 1 || calls.synced[0].ProjectPath != dir {
				t.Fatalf("synced = %+v, want one sync of %q", calls.synced, dir)
			}
			if want := []string{"sync", "stopped"}; !reflect.DeepEqual(calls.order, want) {
				t.Errorf("order = %v, want %v", calls.order, want)
			}
		})
	}
}

// --no-sync and UV_NO_SYNC keep the old behavior: Stopped alone.
func TestRunNoSyncSkipsTheSync(t *testing.T) {
	cases := map[string]struct {
		args []string
		env  string
	}{
		"run flag":   {args: []string{"local", "run", "--no-sync", "pytest"}},
		"shell flag": {args: []string{"local", "shell", "--no-sync"}},
		"uv env":     {args: []string{"local", "run", "pytest"}, env: "1"},
		"uv env on":  {args: []string{"local", "run", "pytest"}, env: "true"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("UV_NO_SYNC", tc.env)
			d, _, calls := stoppedProject(t, "")
			if err := execute(t, d, tc.args...); err != nil {
				t.Fatal(err)
			}
			if len(calls.synced) != 0 {
				t.Errorf("synced %d times, want none", len(calls.synced))
			}
			if want := []string{"stopped"}; !reflect.DeepEqual(calls.order, want) {
				t.Errorf("order = %v, want %v", calls.order, want)
			}
		})
	}
}

// UV_NO_SYNC=0 is uv's spelling of off, and does not skip the sync.
func TestRunUVNoSyncFalseStillSyncs(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "0")
	d, _, calls := stoppedProject(t, "")
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	if len(calls.synced) != 1 {
		t.Errorf("synced %d times, want 1", len(calls.synced))
	}
}

// A running Airflow is attached to, never synced under.
func TestRunWithAirflowRunningDoesNotSync(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	d.Runtime = syncCountingRuntime{attachRuntime: attachRuntime{af: runAirflow{}}, calls: calls}
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	if len(calls.synced) != 0 {
		t.Errorf("synced a running project %d times", len(calls.synced))
	}
}

type syncCountingRuntime struct {
	attachRuntime
	calls *stoppedCalls
}

func (r syncCountingRuntime) Sync(_ context.Context, p localrt.Plan, _ localrt.Callbacks) error {
	r.calls.synced = append(r.calls.synced, p)
	return nil
}

// Docker mode has no venv: it is never synced, and its refusal is unchanged.
func TestRunInADockerProjectDoesNotSync(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, localrt.ModeDocker)
	if err := execute(t, d, "local", "run", "pytest"); !errors.Is(err, localrt.ErrNotRunning) {
		t.Fatalf("err = %v, want not running", err)
	}
	if len(calls.synced) != 0 {
		t.Errorf("synced a docker project %d times", len(calls.synced))
	}
}

// A failed sync fails the run before any command starts, and shows all of what
// uv said, even when it finished inside the quiet window.
func TestRunSyncFailureStopsTheRunAndShowsUVOutput(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, stdout, calls := stoppedProject(t, "")
	calls.syncLines = []string{"Resolved 3 packages", "error: no solution"}
	calls.syncErr = errors.New("preparing the project environment: boom")
	err := execute(t, d, "local", "run", "pytest")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the sync's error", err)
	}
	if len(calls.order) != 1 || calls.order[0] != "sync" {
		t.Errorf("order = %v, want only the sync", calls.order)
	}
	if got := stderrOf(d); !strings.Contains(got, "error: no solution") {
		t.Errorf("stderr = %q, want uv's output", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

// First build: announced, and uv's lines stream to stderr. stdout stays empty
// for the wrapped command.
func TestRunFirstBuildReportsOnStderrOnly(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, stdout, calls := stoppedProject(t, "")
	dir, _ := d.WorkingDir()
	dropVenv(t, dir)
	calls.syncLines = []string{"Installed 120 packages"}
	if err := execute(t, d, "local", "run", "python", "-c", "1"); err != nil {
		t.Fatal(err)
	}
	got := stderrOf(d)
	requireInOrder(t, got, "Building this project's environment", "Installed 120 packages")
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
}

// An update of an existing venv that finishes quickly says nothing.
func TestRunQuietWhenTheVenvIsCurrent(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	calls.syncLines = []string{"Resolved 120 packages", "Audited 120 packages"}
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	if got := stderrOf(d); got != "" {
		t.Errorf("stderr = %q, want nothing", got)
	}
}

// The quiet timer's call shows what was held with no new line arriving, and
// later lines stream.
func TestSyncQuietTimerFiresWithoutALine(t *testing.T) {
	var out bytes.Buffer
	r := &syncReporter{out: &out, note: "Updating..."}
	r.line(localrt.LogLine{Text: "one"})
	if out.Len() != 0 {
		t.Fatalf("shown early: %q", out.String())
	}
	r.slow() // what the timer calls, with no line since "one"
	if got, want := out.String(), "Updating...\none\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	r.line(localrt.LogLine{Text: "two"})
	r.slow()
	if got, want := out.String(), "Updating...\none\ntwo\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
}

// The syncer arms the timer for a complete venv and stops it when the sync
// ends; firing it mid-sync narrates.
func TestSyncArmsAndStopsTheTimer(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	calls.syncLines = []string{"Resolved"}
	var fire func()
	stopped := false
	withSeams(t, syncer{
		after: func(_ time.Duration, f func()) func() { fire = f; return func() { stopped = true } },
		sleep: func(context.Context, time.Duration) error { return nil },
		wait:  time.Minute,
	})
	calls.onSync = func() { fire() } // a slow install, with no line after the first
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Error("timer not stopped when the sync ended")
	}
	requireInOrder(t, stderrOf(d), "Updating this project's environment...", "Resolved")
}

func withSeams(t *testing.T, s syncer) {
	t.Helper()
	old := syncSeams
	syncSeams = s
	t.Cleanup(func() { syncSeams = old })
}

// A venv astro did not mark complete is wiped by the sync, so say so and show
// the output.
func TestRunAnnouncesRebuildOfAnUnmarkedVenv(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	dir, _ := d.WorkingDir()
	dropVenv(t, dir)
	makeVenv(t, dir, false)
	calls.syncLines = []string{"Installed 3 packages"}
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	requireInOrder(t, stderrOf(d), "Rebuilding this project's environment (it was not built by astro)", "Installed 3 packages")
}

func dropVenv(t *testing.T, dir string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(dir, ".venv")); err != nil {
		t.Fatal(err)
	}
}

// Another command holds the lock and the venv is complete: run with it as it
// is, and say so.
func TestRunProceedsWhenTheLockIsHeldAndTheVenvIsComplete(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, stdout, calls := stoppedProject(t, "")
	calls.syncErr = localrt.ErrStartInProgress
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatalf("run with the lock held: %v", err)
	}
	if !strings.Contains(stderrOf(d), "using it as it is") || stdout.Len() != 0 {
		t.Errorf("stderr = %q, stdout = %q", stderrOf(d), stdout.String())
	}
	if want := []string{"sync", "stopped"}; !reflect.DeepEqual(calls.order, want) {
		t.Errorf("order = %v, want %v", calls.order, want)
	}
}

// Lock held and no complete venv: wait, retrying the sync, and run once it
// goes through.
func TestRunWaitsForTheLockWhenThereIsNoCompleteVenv(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	dir, _ := d.WorkingDir()
	dropVenv(t, dir)
	calls.syncErrs = []error{localrt.ErrStartInProgress, localrt.ErrStartInProgress}
	sleeps := 0
	withSeams(t, syncer{
		after: realAfter,
		sleep: func(context.Context, time.Duration) error { sleeps++; return nil },
		wait:  time.Minute,
	})
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatal(err)
	}
	if sleeps != 2 || len(calls.synced) != 3 {
		t.Errorf("sleeps = %d, syncs = %d, want 2 and 3", sleeps, len(calls.synced))
	}
	if !strings.Contains(stderrOf(d), "Waiting for another astro command") {
		t.Errorf("stderr = %q", stderrOf(d))
	}
}

// The wait is bounded.
func TestRunGivesUpWaitingForTheLock(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	dir, _ := d.WorkingDir()
	dropVenv(t, dir)
	calls.syncErr = localrt.ErrStartInProgress
	withSeams(t, syncer{after: realAfter, sleep: func(context.Context, time.Duration) error { return nil }, wait: 0})
	err := execute(t, d, "local", "run", "pytest")
	if !errors.Is(err, localrt.ErrStartInProgress) || !strings.Contains(err.Error(), "waited") {
		t.Fatalf("err = %v", err)
	}
}

// Airflow came up between the status read and the sync: the sync refuses, and
// the run attaches to it.
func TestRunAttachesWhenAirflowStartedDuringTheSync(t *testing.T) {
	t.Setenv("UV_NO_SYNC", "")
	d, _, calls := stoppedProject(t, "")
	calls.syncErr = fmt.Errorf("%w: this project's local Airflow is running", localrt.ErrNotImplemented)
	rt := &racingRuntime{stoppedRuntime: stoppedRuntime{calls: calls}}
	d.Runtime = rt
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !rt.attached {
		t.Error("did not attach to the Airflow that came up")
	}
	if want := []string{"sync"}; !reflect.DeepEqual(calls.order, want) {
		t.Errorf("order = %v, want only the failed sync", calls.order)
	}
}

// racingRuntime reads stopped once, then running.
type racingRuntime struct {
	stoppedRuntime
	reads    int
	attached bool
}

func (r *racingRuntime) ReadStatus(dir string) (localrt.Status, error) {
	r.reads++
	if r.reads == 1 {
		return localrt.Status{ProjectPath: dir, State: localrt.StateStopped}, nil
	}
	return localrt.Status{ProjectPath: dir, State: localrt.StateRunning, Mode: localrt.ModeStandalone}, nil
}

func (r *racingRuntime) Attach(string) (localrt.Airflow, error) {
	r.attached = true
	return runAirflow{}, nil
}

// With the sync skipped and no venv, the refusal stands and names both ways to
// skip it.
func TestRunNoSyncRefusesAProjectWithNoVenv(t *testing.T) {
	for name, tc := range map[string]struct{ env, flag string }{"flag": {"", "--no-sync"}, "env": {"1", ""}} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("UV_NO_SYNC", tc.env)
			d, _, calls := stoppedProject(t, "")
			dir, _ := d.WorkingDir()
			dropVenv(t, dir)
			args := []string{"local", "run"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			err := execute(t, d, append(args, "pytest")...)
			if !errors.Is(err, localrt.ErrNotRunning) || !strings.Contains(err.Error(), "no environment yet") || !strings.Contains(err.Error(), "UV_NO_SYNC") {
				t.Fatalf("err = %v", err)
			}
			if len(calls.synced) != 0 {
				t.Error("synced despite the skip")
			}
		})
	}
}

func TestUVNoSyncSpellings(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "1": true, "TRUE": true, "yes": true, "on": true} {
		t.Setenv("UV_NO_SYNC", v)
		if got := uvNoSyncSet(); got != want {
			t.Errorf("UV_NO_SYNC=%q: %v, want %v", v, got, want)
		}
	}
}
