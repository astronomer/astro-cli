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
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(validManifest), 0o600); err != nil {
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

// validManifest is the smallest manifest that loads. Both helpers write it,
// because every spelling of check now validates the manifest before it looks at
// the environment — an empty pyproject.toml marks a project directory but is not
// a project the check can report on.
const validManifest = "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\", \"pandas\"]\n[tool.astro]\nairflow = '3.1'\n"

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

// A manifest problem is cheap, certain, and offline; a missing environment is
// expensive to fix and may not be the real problem. So when both are true the
// manifest is what the user hears about — otherwise they go and build an
// environment for a project that cannot load, and nothing mentions the key that
// is actually wrong.
func TestCheckReportsAManifestProblemBeforeTheEnvironment(t *testing.T) {
	d, out := testDeps(t)
	dir := t.TempDir()
	broken := "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\"]\n[tool.astro]\nairflow = '3.1'\nbogus_key = 'x'\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	// The environment is ALSO not ready. The manifest still wins.
	d.Checks = stubParser{err: wrapNotReady()}

	err := execute(t, d, "local", "check")
	if err == nil {
		t.Fatal("a check on an invalid manifest must fail")
	}
	if !strings.Contains(err.Error(), "bogus_key") {
		t.Errorf("error should name the manifest key, got %v", err)
	}
	if strings.Contains(out.String(), "astro local start") {
		t.Errorf("should not send the user to build an environment: %q", out.String())
	}
}

// `--target astro` is documented as an alias for a plain check, so the two
// spellings have to agree about an invalid manifest. They did not: the target
// path loaded the manifest and the plain path did not, so the same project
// reported its manifest errors one way and a missing venv the other.
func TestCheckAndTargetAstroAgreeOnAnInvalidManifest(t *testing.T) {
	broken := "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\"]\n[tool.astro]\nairflow = '3.1'\nbogus_key = 'x'\n"
	for _, args := range [][]string{
		{"local", "check"},
		{"local", "check", "--target", "astro"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, _ := testDeps(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(broken), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			d.Checks = stubParser{err: wrapNotReady()}

			err := execute(t, d, args...)
			if err == nil || !strings.Contains(err.Error(), "bogus_key") {
				t.Errorf("want the manifest error, got %v", err)
			}
		})
	}
}

// The DETAIL column used to show a traceback's first line, which is the literal
// "Traceback (most recent call last):" on every traceback Python produces — the
// same nine words on every row, saying nothing about any of them. The row now
// carries the exception, and the frames print under the table.
func TestCheckImportErrorShowsTheExceptionThenTheFrames(t *testing.T) {
	d, out := checkDeps(t)
	traceback := "Traceback (most recent call last):\n" +
		"  File \"/p/dags/bad.py\", line 2, in <module>\n" +
		"    import nonexistent_module_xyz\n" +
		"ModuleNotFoundError: No module named 'nonexistent_module_xyz'"
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: traceback}},
	}}

	var exit *ExitError
	if err := execute(t, d, "local", "check"); !errors.As(err, &exit) {
		t.Fatalf("want ExitError, got %v", err)
	}

	got := out.String()
	row, ok := lineContaining(got, "import_error")
	if !ok {
		t.Fatalf("no import_error row: %q", got)
	}
	if !strings.Contains(row, "ModuleNotFoundError") {
		t.Errorf("the row should carry the exception, got %q", row)
	}
	if strings.Contains(row, "Traceback (most recent call last)") {
		t.Errorf("the row should not be the traceback banner, got %q", row)
	}
	// The frames are the half that says WHERE, so they have to survive.
	if !strings.Contains(got, "line 2, in <module>") {
		t.Errorf("frames missing from the output: %q", got)
	}
}

// lineContaining returns the first line of s holding substr.
func lineContaining(s, substr string) (string, bool) {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, substr) {
			return line, true
		}
	}
	return "", false
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
