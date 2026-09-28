//go:build !windows

package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// runAirflow is a fake localrt.Airflow whose Run returns a fixed error, so a
// command's exit-code handling can be tested without a real runtime.
type runAirflow struct {
	runErr error
}

func (a runAirflow) Stop(context.Context, localrt.StopOptions) error { return nil }
func (a runAirflow) Status() (localrt.Status, error)                 { return localrt.Status{}, nil }
func (a runAirflow) Logs(context.Context, localrt.LogOptions) error  { return nil }
func (a runAirflow) Run(context.Context, []string, localrt.Stdio) error {
	return a.runErr
}
func (a runAirflow) Shell(context.Context, localrt.Stdio) error { return nil }
func (a runAirflow) Env() ([]string, error)                     { return nil, nil }

// attachRuntime hands every attach the same fake Airflow.
type attachRuntime struct {
	fakeRuntime
	af localrt.Airflow
}

func (r attachRuntime) Attach(string) (localrt.Airflow, error) { return r.af, nil }

func (r attachRuntime) ReadStatus(string) (localrt.Status, error) {
	return localrt.Status{State: localrt.StateRunning, Mode: localrt.ModeStandalone}, nil
}

// stoppedRuntime has nothing running, in the mode a leftover record would
// name. It records the plans handed to Stopped and Start.
type stoppedRuntime struct {
	fakeRuntime
	mode  localrt.Mode
	calls *stoppedCalls
}

type stoppedCalls struct{ stopped, started localrt.Plan }

func (r stoppedRuntime) ReadStatus(dir string) (localrt.Status, error) {
	return localrt.Status{ProjectPath: dir, State: localrt.StateStopped, Mode: r.mode}, nil
}

func (r stoppedRuntime) Attach(string) (localrt.Airflow, error) { return nil, localrt.ErrNotRunning }

func (r stoppedRuntime) Stopped(p localrt.Plan) (localrt.Airflow, error) {
	r.calls.stopped = p
	return runAirflow{}, nil
}

func (r stoppedRuntime) Start(_ context.Context, p localrt.Plan, _ localrt.Callbacks) (localrt.Airflow, error) {
	r.calls.started = p
	return nil, localrt.ErrNotImplemented
}

func stoppedProject(t *testing.T, mode localrt.Mode) (d Deps, stdout *bytes.Buffer, calls *stoppedCalls) {
	t.Helper()
	isolateEnvSources(t)
	d, stdout = testDeps(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return project, nil }
	calls = &stoppedCalls{}
	d.Runtime = stoppedRuntime{mode: mode, calls: calls}
	return d, stdout, calls
}

// `astro local run pytest` works offline: with nothing running, a standalone
// project's command runs in its venv under the plan a start would build.
func TestRunWithNothingRunningUsesTheProjectEnvironment(t *testing.T) {
	d, _, calls := stoppedProject(t, "")
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatalf("run in a stopped standalone project: %v", err)
	}
	dir, _ := d.WorkingDir()
	if calls.stopped.ProjectPath != dir {
		t.Errorf("Stopped got a plan for %q, want %q", calls.stopped.ProjectPath, dir)
	}
}

// Docker mode runs commands inside its containers, so it still needs them up,
// and says how to start them.
func TestRunInAStoppedDockerProjectSaysHowToStartIt(t *testing.T) {
	d, _, _ := stoppedProject(t, localrt.ModeDocker)
	err := execute(t, d, "local", "run", "pytest")
	if !errors.Is(err, localrt.ErrNotRunning) || !strings.Contains(err.Error(), "astro local start --docker") {
		t.Fatalf("want a not-running error naming the docker start, got %v", err)
	}
}

// unansweredDockerRuntime has a docker record whose probe missed its deadline,
// so the status reads stopped while the containers are up.
type unansweredDockerRuntime struct {
	stoppedRuntime
	af localrt.Airflow
}

func (r unansweredDockerRuntime) Attach(string) (localrt.Airflow, error) { return r.af, nil }

func TestRunInADockerProjectWhoseProbeMissedAttaches(t *testing.T) {
	d, _, calls := stoppedProject(t, localrt.ModeDocker)
	d.Runtime = unansweredDockerRuntime{
		stoppedRuntime: stoppedRuntime{mode: localrt.ModeDocker, calls: calls},
		af:             runAirflow{},
	}
	if err := execute(t, d, "local", "run", "pytest"); err != nil {
		t.Fatalf("run with a docker record: %v", err)
	}
	if calls.stopped.ProjectPath != "" {
		t.Error("a docker project fell back to the stopped standalone handle")
	}
}

// A stop with nothing running succeeds, and says so, in both renderings.
func TestStopWithNothingRunningSucceeds(t *testing.T) {
	d, stdout, _ := stoppedProject(t, "")
	if err := execute(t, d, "local", "stop"); err != nil {
		t.Fatalf("stop with nothing running: %v", err)
	}
	if got := stdout.String(); got != "airflow: already stopped\n" {
		t.Errorf("text output = %q", got)
	}

	d, stdout, _ = stoppedProject(t, "")
	if err := execute(t, d, "local", "stop", "-o", "json"); err != nil {
		t.Fatalf("stop -o json with nothing running: %v", err)
	}
	var e event
	if err := json.Unmarshal(stdout.Bytes(), &e); err != nil {
		t.Fatalf("not one JSON object: %q (%v)", stdout.String(), err)
	}
	if e.Event != "state" || e.State != localrt.StateStopped || !e.AlreadyStopped {
		t.Errorf("event = %+v, want a stopped state marked already_stopped", e)
	}
}

// --clean on a stopped project still fails: it asked for state to be removed,
// and a stop that finds no record has no mode to remove it for.
func TestStopCleanWithNothingRunningNamesReset(t *testing.T) {
	d, _, _ := stoppedProject(t, "")
	err := execute(t, d, "local", "stop", "--clean")
	if !errors.Is(err, localrt.ErrNotRunning) || !strings.Contains(err.Error(), "astro local reset") {
		t.Fatalf("want a not-running error naming reset, got %v", err)
	}
}

// A restart with nothing running is a start, in the mode a leftover record
// names.
func TestRestartWithNothingRunningStartsInTheRecordedMode(t *testing.T) {
	d, _, calls := stoppedProject(t, localrt.ModeDocker)
	if err := execute(t, d, "local", "restart"); !errors.Is(err, localrt.ErrNotImplemented) {
		t.Fatalf("restart did not reach Start: %v", err)
	}
	if calls.started.Mode != localrt.ModeDocker {
		t.Errorf("restart started mode %q, want docker", calls.started.Mode)
	}
}

func TestRunPropagatesChildExitCode(t *testing.T) {
	// A real failing command gives us a genuine *exec.ExitError to propagate.
	childErr := exec.Command("sh", "-c", "exit 7").Run()
	var exitErr *exec.ExitError
	if !errors.As(childErr, &exitErr) {
		t.Fatalf("setup: want *exec.ExitError, got %T", childErr)
	}

	d, _ := testDeps(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte("[project]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return project, nil }
	d.Runtime = attachRuntime{af: runAirflow{runErr: childErr}}

	err := execute(t, d, "local", "run", "false")
	var carried *ExitError
	if !errors.As(err, &carried) {
		t.Fatalf("want *ExitError, got %T: %v", err, err)
	}
	if carried.Code != 7 {
		t.Errorf("exit code = %d, want 7", carried.Code)
	}
}

func TestRunSuccessReturnsNil(t *testing.T) {
	d, _ := testDeps(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte("[project]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return project, nil }
	d.Runtime = attachRuntime{af: runAirflow{runErr: nil}}

	if err := execute(t, d, "local", "run", "true"); err != nil {
		t.Errorf("a successful run must return nil, got %v", err)
	}
}
