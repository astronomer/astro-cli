//go:build !windows

package local

import (
	"bytes"
	"context"
	"errors"
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
	if err := os.RemoveAll(filepath.Join(dir, ".venv")); err != nil {
		t.Fatal(err)
	}
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

// Past the quiet window the held lines appear, and later ones stream.
func TestSyncReporterShowsOutputOnceSlow(t *testing.T) {
	var out bytes.Buffer
	clock := time.Unix(0, 0)
	r := &syncReporter{out: &out, quietFor: time.Second, now: func() time.Time { return clock }, started: clock}
	r.line(localrt.LogLine{Text: "one"})
	if out.Len() != 0 {
		t.Fatalf("shown early: %q", out.String())
	}
	clock = clock.Add(2 * time.Second)
	r.line(localrt.LogLine{Text: "two"})
	r.line(localrt.LogLine{Text: "three"})
	if got, want := out.String(), "one\ntwo\nthree\n"; got != want {
		t.Errorf("out = %q, want %q", got, want)
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
