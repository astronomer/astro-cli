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

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/pkg/localrt"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
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

// stoppedWorkspaceProject is a stopped standalone project whose manifest
// declares workspace-source values, with the Environment Manager faked by
// warehouseClient. A test picks the login with testUtil.InitTestConfig first.
func stoppedWorkspaceProject(t *testing.T, manifest string) (d Deps, stderr *bytes.Buffer, calls *stoppedCalls) {
	t.Helper()
	d, _, calls = stoppedProject(t, "")
	isolateEnvSources(t, "API_TOKEN", "DATA_WAREHOUSE_URI")
	dir, _ := d.WorkingDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ASTRO_HOME", t.TempDir())
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	d.WorkspaceClients = workspaceClients(warehouseClient())
	return d, d.Stderr.(*bytes.Buffer), calls
}

// Offline by default: every workspace-source value the run goes without is
// named on one warning line that says how to fetch them.
func TestRunOfflineNamesSkippedWorkspaceValuesOnOneLine(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	manifest := workspaceEnvManifest + "API_TOKEN = { source = 'workspace' }\n"
	for _, cmd := range [][]string{{"local", "run", "pytest"}, {"local", "shell"}} {
		t.Run(cmd[1], func(t *testing.T) {
			d, stderr, calls := stoppedWorkspaceProject(t, manifest)
			if err := execute(t, d, cmd...); err != nil {
				t.Fatalf("offline %s: %v", cmd[1], err)
			}
			want := "warning: running without env var API_TOKEN, env var DATA_WAREHOUSE_URI: declared source = \"workspace\"; pass --with-workspace to fetch them\n"
			if got := stderr.String(); got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
			if _, ok := calls.stopped.SecretEnv["DATA_WAREHOUSE_URI"]; ok {
				t.Error("an offline run fetched a workspace value")
			}
		})
	}
}

// --with-workspace fetches the values the way a start does.
func TestRunWithWorkspaceFetchesWorkspaceValues(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	for _, cmd := range [][]string{{"local", "run", "--with-workspace", "pytest"}, {"local", "shell", "--with-workspace"}} {
		t.Run(cmd[1], func(t *testing.T) {
			d, stderr, calls := stoppedWorkspaceProject(t, workspaceEnvManifest)
			if err := execute(t, d, cmd...); err != nil {
				t.Fatalf("%s --with-workspace: %v", cmd[1], err)
			}
			if got := calls.stopped.SecretEnv["DATA_WAREHOUSE_URI"]; got != "postgres://cloud" {
				t.Errorf("DATA_WAREHOUSE_URI = %q, want the workspace value", got)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", stderr.String())
			}
		})
	}
}

// The flag asks for workspace values only: a required local value with no
// source still warns, and the command runs.
func TestRunWithWorkspaceStillRunsWithoutALocalValue(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	isolateEnvSources(t, "ASTRO_TEST_LOCAL_ONLY")
	manifest := workspaceEnvManifest + "ASTRO_TEST_LOCAL_ONLY = { secret = true }\n"
	d, stderr, calls := stoppedWorkspaceProject(t, manifest)
	if err := execute(t, d, "local", "run", "--with-workspace", "pytest"); err != nil {
		t.Fatalf("run --with-workspace: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: running without env var ASTRO_TEST_LOCAL_ONLY") {
		t.Errorf("stderr = %q, want a warning for the local value", stderr.String())
	}
	if calls.stopped.SecretEnv["DATA_WAREHOUSE_URI"] != "postgres://cloud" {
		t.Error("the workspace value was not fetched")
	}
}

// Asked for explicitly, a workspace value that cannot be fetched fails the run
// with the report a start gives, and nothing runs.
func TestRunWithWorkspaceFailsWhenAValueCannotBeFetched(t *testing.T) {
	testUtil.InitTestConfig(testUtil.Initial)
	d, _, calls := stoppedWorkspaceProject(t, workspaceEnvManifest)
	err := execute(t, d, "local", "run", "--with-workspace", "pytest")
	var missing *plan.MissingEnvError
	if !errors.As(err, &missing) {
		t.Fatalf("want *plan.MissingEnvError, got %T: %v", err, err)
	}
	if len(missing.Missing) != 1 || missing.Missing[0].Name != "DATA_WAREHOUSE_URI" {
		t.Errorf("missing = %+v, want DATA_WAREHOUSE_URI alone", missing.Missing)
	}
	if !strings.Contains(err.Error(), "leave off `--with-workspace`") {
		t.Errorf("error does not name the way out:\n%v", err)
	}
	if calls.stopped.ProjectPath != "" {
		t.Error("the command ran although a requested value was missing")
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
