package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
)

// stubParser is the check seam under test: it returns a canned report (or
// error) so no real Python runs.
type stubParser struct {
	report checks.ParseReport
	err    error
}

func (p stubParser) Parse(context.Context, checks.ParseInput) (checks.ParseReport, error) {
	return p.report, p.err
}

// checkDeps is testDeps with a working directory that is a real project root,
// so project.Discover (which the check command resolves the path through)
// finds a marker instead of failing.
func checkDeps(t *testing.T) (Deps, *bytes.Buffer) {
	t.Helper()
	d, out := testDeps(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	return d, out
}

// targetDeps is checkDeps with a manifest that carries an Airflow pin and
// dependencies (a target check reads them), plus fake preflight seams so no uv
// runs.
func targetDeps(t *testing.T) (Deps, *bytes.Buffer) {
	t.Helper()
	d, out := testDeps(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(composerManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	prov := &fakeProvisioner{python: filepath.Join(dir, ".venv", "bin", "python")}
	d.Provisioner = func(context.Context) (checks.Provisioner, error) { return prov, nil }
	d.CheckVenv = stubTargetParser{report: checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}}
	return d, out
}

// fakeProvisioner is the cmd-level checks.Provisioner stub: it records the spec
// and returns a canned interpreter and constraints outcome.
type fakeProvisioner struct {
	python     string
	resolveErr error
}

func (f *fakeProvisioner) EnsureVenv(_ context.Context, _ checks.VenvSpec, progress func(string)) (string, error) {
	progress("provisioning")
	return f.python, nil
}

func (f *fakeProvisioner) ResolveConstraints(context.Context, []string, string, string) error {
	return f.resolveErr
}

// stubTargetParser returns a canned report from ParseWith, standing in for the
// scratch-venv parser.
type stubTargetParser struct {
	report checks.ParseReport
	err    error
}

func (p stubTargetParser) ParseWith(context.Context, string, checks.ParseInput) (checks.ParseReport, error) {
	return p.report, p.err
}

const composerManifest = "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\", \"pandas\"]\n[tool.astro]\nairflow = '3.1'\n"

func TestTargetComposerPassesCleanly(t *testing.T) {
	d, out := targetDeps(t)
	if err := execute(t, d, "local", "check", "--target", "composer"); err != nil {
		t.Fatalf("composer 3.1 maps to 3.1.8 and should pass: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Airflow 3.1.8") {
		t.Errorf("output should name the checked version 3.1.8: %q", s)
	}
	if !strings.Contains(s, "check passed") {
		t.Errorf("clean target check should report passed: %q", s)
	}
}

func TestTargetMWAAShowsClosestLowerNote(t *testing.T) {
	d, out := targetDeps(t)
	if err := execute(t, d, "local", "check", "--target", "mwaa"); err != nil {
		t.Fatalf("mwaa clean parse should pass: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "3.0.6") || !strings.Contains(s, "mapped down from the manifest pin 3.1") {
		t.Errorf("mwaa should note the 3.1 -> 3.0.6 downgrade: %q", s)
	}
	if !strings.Contains(s, "constraints:") {
		t.Errorf("mwaa should report a constraints outcome: %q", s)
	}
}

func TestTargetCheckJSONEmitsPerTargetNDJSON(t *testing.T) {
	d, out := targetDeps(t)
	if err := execute(t, d, "local", "check", "--target", "composer,mwaa", "--output", "json"); err != nil {
		t.Fatalf("clean multi-target check should pass: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one NDJSON object per target, got %d: %q", len(lines), out.String())
	}
	var got []string
	for _, l := range lines {
		var rep checks.TargetReport
		if err := json.Unmarshal([]byte(l), &rep); err != nil {
			t.Fatalf("line is not a standalone TargetReport: %v: %q", err, l)
		}
		got = append(got, rep.Target)
	}
	if got[0] != "composer" || got[1] != "mwaa" {
		t.Errorf("targets should render in order, got %v", got)
	}
}

func TestTargetCheckUnknownTargetErrors(t *testing.T) {
	d, _ := targetDeps(t)
	err := execute(t, d, "local", "check", "--target", "gcp")
	if err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("an unknown target should error clearly, got %v", err)
	}
}

func TestTargetCheckImportErrorExitsCode1(t *testing.T) {
	d, _ := targetDeps(t)
	d.CheckVenv = stubTargetParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "boom"}},
	}}
	err := execute(t, d, "local", "check", "--target", "composer")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != checks.ExitChecksFailed {
		t.Fatalf("an import error in a target check should exit 1, got %v", err)
	}
}

func TestTargetAstroIsThePlainCheck(t *testing.T) {
	d, out := targetDeps(t)
	// astro uses the project-venv parser (d.Checks), not the scratch one.
	d.Checks = stubParser{report: checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}}
	if err := execute(t, d, "local", "check", "--target", "astro"); err != nil {
		t.Fatalf("astro target is the default check and should pass: %v", err)
	}
	if !strings.Contains(out.String(), "== astro ==") {
		t.Errorf("astro target should render as a target block: %q", out.String())
	}
}

func TestCheckCleanExitsZeroAndPrintsSummary(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		Dags:  []checks.ReportDag{{DagID: "a", File: "dags/a.py"}},
		Files: []checks.ReportFile{{File: "dags/a.py", ParseSeconds: 0.1, DagIDs: []string{"a"}}},
	}}
	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("clean check must exit zero: %v", err)
	}
	if !strings.Contains(out.String(), "checks passed") {
		t.Errorf("summary missing: %q", out.String())
	}
}

func TestCheckImportErrorFailsWithCode1(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "boom\ntraceback"}},
	}}
	err := execute(t, d, "local", "check")
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exit.Code != checks.ExitChecksFailed {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitChecksFailed)
	}
	if !strings.Contains(out.String(), "dags/bad.py") || !strings.Contains(out.String(), "boom") {
		t.Errorf("table missing the import error: %q", out.String())
	}
}

func TestCheckStrictTurnsWarningIntoFailure(t *testing.T) {
	slow := checks.ParseReport{
		Files: []checks.ReportFile{{File: "dags/slow.py", ParseSeconds: checks.ParseTimeWarnThreshold.Seconds() + 1}},
	}
	// Not strict: a slow parse is a warning, exit zero.
	d, _ := checkDeps(t)
	d.Checks = stubParser{report: slow}
	if err := execute(t, d, "local", "check"); err != nil {
		t.Fatalf("non-strict slow parse must exit zero: %v", err)
	}
	// Strict: the same warning fails.
	d2, _ := checkDeps(t)
	d2.Checks = stubParser{report: slow}
	err := execute(t, d2, "local", "check", "--strict")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != checks.ExitChecksFailed {
		t.Fatalf("strict slow parse must fail with code 1, got %v", err)
	}
}

func TestCheckEnvNotReadyExitsCode2(t *testing.T) {
	d, out := checkDeps(t)
	// The error must wrap ErrEnvNotReady so errors.Is routes it to code 2.
	d.Checks = stubParser{err: wrapNotReady()}
	err := execute(t, d, "local", "check")
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exit.Code != checks.ExitEnvNotReady {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitEnvNotReady)
	}
	if !strings.Contains(out.String(), "astro local start") {
		t.Errorf("env-not-ready output should guide the user: %q", out.String())
	}
}

func wrapNotReady() error {
	return errWrap{msg: "no Python found at .venv/bin/python — run `astro local start` first", err: checks.ErrEnvNotReady}
}

type errWrap struct {
	msg string
	err error
}

func (e errWrap) Error() string { return e.msg }
func (e errWrap) Unwrap() error { return e.err }

func TestCheckJSONEmitsNDJSONFindingsAndSummary(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "boom"}},
		Files: []checks.ReportFile{
			{File: "dags/x.py", DagIDs: []string{"dup"}},
			{File: "dags/y.py", DagIDs: []string{"dup"}},
		},
	}}
	err := execute(t, d, "local", "check", "--output", "json")
	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("failing check should still carry an exit code: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// import_error + duplicate_dag_id findings, then the summary.
	if len(lines) != 3 {
		t.Fatalf("want 3 NDJSON lines, got %d: %q", len(lines), out.String())
	}
	for _, l := range lines {
		var obj map[string]any
		if jerr := json.Unmarshal([]byte(l), &obj); jerr != nil {
			t.Errorf("line is not standalone JSON: %v: %q", jerr, l)
		}
	}
	var summary checkSummary
	if jerr := json.Unmarshal([]byte(lines[len(lines)-1]), &summary); jerr != nil || summary.Event != "summary" {
		t.Errorf("last line should be the summary: %v: %q", jerr, lines[len(lines)-1])
	}
	if summary.Passed {
		t.Error("summary should report a failure")
	}
}
