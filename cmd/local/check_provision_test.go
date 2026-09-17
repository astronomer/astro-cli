package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
)

// A project with no interpreter of its own gets one built, instead of being
// told to run a command that — in docker mode — does not build one.
func TestCheckBuildsAnEnvironmentWhenTheProjectHasNone(t *testing.T) {
	d, out := targetDeps(t)
	d.Checks = stubParser{err: checks.ErrNoInterpreter}

	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("a check with no project venv should provision one and pass: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "checks passed") {
		t.Errorf("expected a normal verdict from the provisioned run: %q", s)
	}
	// The provisioning is visible: it is the slow part of the run, and a
	// command that goes quiet for a minute reads as a hang.
	if !strings.Contains(s, "provisioning") {
		t.Errorf("expected progress about building the environment: %q", s)
	}
}

// The requirement set is the manifest's own dependencies, carrying Airflow in
// the shape the project states it. Rebuilding it from the [tool.astro] pin
// would ask for "apache-airflow==3.1", a release that does not exist.
func TestCheckProvisionsFromTheManifestsOwnDependencies(t *testing.T) {
	d, _ := targetDeps(t)
	d.Checks = stubParser{err: checks.ErrNoInterpreter}

	var got checks.VenvSpec
	d.Provisioner = func(context.Context) (checks.Provisioner, error) {
		return &fakeProvisioner{python: "/tmp/venv/bin/python", got: &got}, nil
	}

	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(got.Reqs) == 0 {
		t.Fatal("the provisioner was handed no requirements")
	}
	var airflow string
	for _, r := range got.Reqs {
		if strings.HasPrefix(r, "apache-airflow=") {
			airflow = r
		}
	}
	if airflow != "apache-airflow==3.1.*" {
		t.Errorf("airflow requirement = %q, want the manifest's own %q", airflow, "apache-airflow==3.1.*")
	}
}

// The interpreter request comes from requires-python. The venv is built
// outside the project, so uv cannot read that constraint from the manifest —
// and an Airflow 2 project checked on an interpreter its Airflow cannot run is
// the failure requires-python exists to prevent.
func TestCheckProvisionsUnderTheManifestsRequiresPython(t *testing.T) {
	// An Airflow 2 project, bounded the way the scaffold now writes them.
	const manifest = "[project]\nname = 'demo'\nrequires-python = '>=3.10,<3.13'\n" +
		"dependencies = [\"apache-airflow==2.10.*\"]\n[tool.astro]\nairflow = '2.10'\n"

	d, _ := targetDeps(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Checks = stubParser{err: checks.ErrNoInterpreter}

	var got checks.VenvSpec
	d.Provisioner = func(context.Context) (checks.Provisioner, error) {
		return &fakeProvisioner{python: "/tmp/venv/bin/python", got: &got}, nil
	}
	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("check: %v", err)
	}
	if got.Python != ">=3.10,<3.13" {
		t.Errorf("interpreter request = %q, want the manifest's requires-python", got.Python)
	}
	if got.Airflow != "2.10" {
		t.Errorf("cache key airflow = %q, want the manifest pin", got.Airflow)
	}
}

// When building one fails, the answer is still "the environment is not ready":
// the same door, the same exit code, rather than a new kind of failure for
// what is the same problem one step further on.
func TestCheckFailingToBuildAnEnvironmentStillExitsCode2(t *testing.T) {
	d, out := targetDeps(t)
	d.Checks = stubParser{err: checks.ErrNoInterpreter}
	d.Provisioner = func(context.Context) (checks.Provisioner, error) {
		return nil, errors.New("no uv on this machine")
	}

	err := execute(t, d, "local", "check")
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exit.Code != checks.ExitEnvNotReady {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitEnvNotReady)
	}
	if !strings.Contains(out.String(), "no uv on this machine") {
		t.Errorf("stdout should name why the environment could not be built: %q", out.String())
	}
}

// specRecorder is a Provisioner that records the spec it was handed.
type specRecorder struct {
	python string
	got    *checks.VenvSpec
}

func (s *specRecorder) EnsureVenv(_ context.Context, spec checks.VenvSpec, progress func(string)) (string, error) {
	*s.got = spec
	progress("provisioning")
	return s.python, nil
}

func (s *specRecorder) ResolveConstraints(context.Context, []string, string, string) error {
	return nil
}

// A project whose own .venv is broken — Airflow there will not import — is NOT
// answered by building a fresh environment. Doing so checks the project
// against dependencies it does not have installed and reports a clean pass,
// while the Airflow it actually runs stays broken and `astro local start`
// still fails.
func TestCheckDoesNotPaperOverABrokenProjectEnvironment(t *testing.T) {
	d, out := targetDeps(t)
	// ErrEnvNotReady without ErrNoInterpreter: the interpreter is there, its
	// Airflow is not importable.
	d.Checks = stubParser{err: fmt.Errorf("no module named airflow: %w", checks.ErrEnvNotReady)}

	var provisioned bool
	d.Provisioner = func(context.Context) (checks.Provisioner, error) {
		provisioned = true
		return &fakeProvisioner{python: "/tmp/venv/bin/python"}, nil
	}

	err := execute(t, d, "local", "check")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != checks.ExitEnvNotReady {
		t.Fatalf("a broken project environment should still stop the check, got %v", err)
	}
	if provisioned {
		t.Error("built a fresh environment for a project that has one; its own is the broken thing")
	}
	if !strings.Contains(out.String(), "no module named airflow") {
		t.Errorf("the original reason should survive: %q", out.String())
	}
}

// --target astro is documented as an alias for the plain check, so it has to
// take the same road: a docker-mode project passing one way and reporting
// "environment not ready" the other is the same project in the same state
// giving two answers.
func TestTargetAstroProvisionsLikeThePlainCheck(t *testing.T) {
	d, out := targetDeps(t)
	d.Checks = stubParser{err: checks.ErrNoInterpreter}

	if err := execute(t, d, "local", "check", "--target", "astro"); err != nil {
		t.Fatalf("--target astro should provision exactly as the plain check does: %v", err)
	}
	if !strings.Contains(out.String(), "check passed") {
		t.Errorf("expected a passing astro target report: %q", out.String())
	}
	// And it says so, for the same reason the json summary does: a built
	// environment is resolved from the manifest and can disagree with what the
	// project actually runs.
	if !strings.Contains(out.String(), "built from the manifest") {
		t.Errorf("the report should say the environment was built for the run: %q", out.String())
	}
}

// A json consumer has to be able to tell the two apart. A built environment is
// resolved from the manifest, so it can disagree with what the project
// actually runs — most obviously for a docker project, whose image is the real
// environment — and the payload was byte-identical either way.
func TestCheckJSONSaysWhenTheEnvironmentWasBuilt(t *testing.T) {
	d, out := targetDeps(t)
	d.Checks = stubParser{err: checks.ErrNoInterpreter}

	if err := execute(t, d, "local", "check", "--output", "json"); err != nil {
		t.Fatalf("check: %v", err)
	}
	var summary struct {
		Event       string `json:"event"`
		Provisioned bool   `json:"provisioned"`
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); err != nil {
		t.Fatalf("last line is not the summary: %v: %q", err, out.String())
	}
	if summary.Event != "summary" {
		t.Fatalf("last line is not the summary: %q", lines[len(lines)-1])
	}
	if !summary.Provisioned {
		t.Error("the summary does not say the environment was built for the run")
	}
	// And the progress notes stay out of the payload.
	if strings.Contains(out.String(), "provisioning") {
		t.Errorf("progress text leaked into json output: %q", out.String())
	}
}

// The ordinary payload is unchanged: nothing new appears when the project has
// its own environment.
func TestCheckJSONIsUnchangedForAProjectWithItsOwnEnvironment(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}}
	if err := execute(t, d, "local", "check", "--output", "json"); err != nil {
		t.Fatalf("check: %v", err)
	}
	if strings.Contains(out.String(), "provisioned") {
		t.Errorf("the key should be omitted when it is false: %q", out.String())
	}
}
