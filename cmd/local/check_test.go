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

	"github.com/astronomer/astro-cli/internal/checks"
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
