//go:build !windows

package local

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

// attachRuntime hands every attach the same fake Airflow.
type attachRuntime struct {
	fakeRuntime
	af localrt.Airflow
}

func (r attachRuntime) Attach(string) (localrt.Airflow, error) { return r.af, nil }

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
