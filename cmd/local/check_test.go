package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/cmd/cliout"
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
	// got records the spec it was handed, for a test that cares which
	// environment was asked for rather than only that one was.
	got *checks.VenvSpec
}

func (f *fakeProvisioner) EnsureVenv(_ context.Context, spec checks.VenvSpec, progress func(string)) (string, error) {
	if f.got != nil {
		*f.got = spec
	}
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
const validManifest = "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\", \"pandas\"]\n[tool.astro]\n"

// brokenManifest fails manifest.Load on an unknown [tool.astro] key. Named
// beside its valid counterpart so a change to Load's strictness is one edit.
const brokenManifest = "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\"]\n[tool.astro]\nbogus_key = 'x'\n"

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

func TestTargetMWAAConstraintsConflictFails(t *testing.T) {
	d, out := targetDeps(t)
	prov := &fakeProvisioner{python: "/p", resolveErr: &checks.ConstraintConflict{Summary: "scikit-learn==1.5.2 and scikit-learn==1.8.0 cannot both hold"}}
	d.Provisioner = func(context.Context) (checks.Provisioner, error) { return prov, nil }

	err := execute(t, d, "local", "check", "--target", "mwaa")
	var exit *cliout.ExitError
	if !errors.As(err, &exit) || exit.Code != checks.ExitChecksFailed {
		t.Fatalf("a constraints conflict should exit 1, got %v", err)
	}
	if want := "check failed for mwaa: dependencies conflict with MWAA's constraints; 1 DAGs, 0 errors, 0 warnings"; !strings.Contains(out.String(), want) {
		t.Errorf("verdict should say the check failed and why, want %q in:\n%s", want, out.String())
	}
}

func TestTargetMWAAConstraintsSkipStillPasses(t *testing.T) {
	d, out := targetDeps(t)
	prov := &fakeProvisioner{python: "/p", resolveErr: checks.ErrConstraintsUnavailable}
	d.Provisioner = func(context.Context) (checks.Provisioner, error) { return prov, nil }

	if err := execute(t, d, "local", "check", "--target", "mwaa"); err != nil {
		t.Fatalf("an offline constraints step is a skip, not a failure: %v", err)
	}
	if !strings.Contains(out.String(), "check passed for mwaa: 1 DAGs") {
		t.Errorf("verdict should pass with no reason attached:\n%s", out.String())
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
	d, out := targetDeps(t)
	err := execute(t, d, "local", "check", "--target", "gcp")
	// No verdict was reached, so it takes the same door as every other outcome
	// of that kind: message on stdout, exit 2 rather than the DAG-failure 1.
	var exit *cliout.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	if exit.Code != checks.ExitEnvNotReady {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitEnvNotReady)
	}
	if !strings.Contains(out.String(), "unknown target") {
		t.Errorf("stdout should name the problem, got %q", out.String())
	}
}

func TestTargetCheckImportErrorExitsCode1(t *testing.T) {
	d, _ := targetDeps(t)
	d.CheckVenv = stubTargetParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "boom"}},
	}}
	err := execute(t, d, "local", "check", "--target", "composer")
	var exit *cliout.ExitError
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

// What the project gets locally without declaring it is reported, as info that
// fails nothing: not a plain check, and not --strict either.
func TestCheckNotesUndeclaredLocalValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ASTRO_HOME", "")
	d, out := checkDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("AIRFLOW_CONN_LOCAL_DB=postgres://h\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := execute(t, d, "local", "check", "--strict"); err != nil {
		t.Fatalf("an undeclared local value must not fail the check: %v", err)
	}
	line, ok := lineContaining(out.String(), "info: ")
	if !ok || !strings.Contains(line, "AIRFLOW_CONN_LOCAL_DB") || !strings.Contains(line, "will not follow it to a Deployment") {
		t.Errorf("want an info line naming AIRFLOW_CONN_LOCAL_DB, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "checks passed") {
		t.Errorf("summary missing: %q", out.String())
	}

	d, out = checkDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }
	if err := execute(t, d, "local", "check", "--output", "json"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"undeclared_env":["AIRFLOW_CONN_LOCAL_DB"]`) {
		t.Errorf("json summary missing undeclared_env: %s", out.String())
	}
}

func TestCheckImportErrorFailsWithCode1(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "boom\ntraceback"}},
	}}
	err := execute(t, d, "local", "check")
	var exit *cliout.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	if exit.Code != checks.ExitChecksFailed {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitChecksFailed)
	}
	// Asserted against the ROW, not the whole output. Checking the output as a
	// whole stopped testing anything once the frames began printing under the
	// table: the substring matched the traceback block whatever the row said,
	// so a regression that emptied the DETAIL column would have passed.
	row, ok := lineContaining(out.String(), "import_error")
	if !ok {
		t.Fatalf("no import_error row: %q", out.String())
	}
	if !strings.Contains(row, "dags/bad.py") || !strings.Contains(row, "boom") {
		t.Errorf("row missing the file or the message: %q", row)
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
	var exit *cliout.ExitError
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
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(brokenManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	// The environment is ALSO not ready. The manifest still wins.
	d.Checks = stubParser{err: wrapNotReady()}

	err := execute(t, d, "local", "check")
	var exit *cliout.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	// Exit 2, not 1: no verdict was reached. 1 means the DAGs failed a check,
	// and a CI job branching on the two must not read a manifest typo as one.
	if exit.Code != checks.ExitEnvNotReady {
		t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitEnvNotReady)
	}
	// On stdout, where docs/install.md tells an agent to read it.
	if !strings.Contains(out.String(), "bogus_key") {
		t.Errorf("stdout should name the manifest key, got %q", out.String())
	}
	if strings.Contains(out.String(), "astro local start") {
		t.Errorf("should not send the user to build an environment: %q", out.String())
	}
}

// The manifest problem reaches a json consumer in the shape it switches on,
// rather than cobra's generic error object.
func TestCheckManifestProblemJSONCarriesAnErrorEvent(t *testing.T) {
	d, out := testDeps(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(brokenManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	// Set even though the manifest should fail first: without it, a regression
	// that reordered the two would dereference a nil parser and report a panic
	// instead of "the manifest was not reported first".
	d.Checks = stubParser{err: wrapNotReady()}

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check", "--output", "json"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	var got struct {
		Event   string `json:"event"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &got); err != nil {
		t.Fatalf("stdout is not one json object: %v: %q", err, out.String())
	}
	if got.Event != "error" {
		t.Errorf("event = %q, want \"error\"", got.Event)
	}
	if !strings.Contains(got.Message, "bogus_key") {
		t.Errorf("message should name the key, got %q", got.Message)
	}
}

// `--target astro` is documented as an alias for a plain check, so the two
// spellings have to agree about an invalid manifest. They did not: the target
// path loaded the manifest and the plain path did not, so the same project
// reported its manifest errors one way and a missing venv the other.
func TestCheckAndTargetAstroAgreeOnAnInvalidManifest(t *testing.T) {
	for _, args := range [][]string{
		{"local", "check"},
		{"local", "check", "--target", "astro"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			d, out := testDeps(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(brokenManifest), 0o600); err != nil {
				t.Fatal(err)
			}
			d.WorkingDir = func() (string, error) { return dir, nil }
			d.Checks = stubParser{err: wrapNotReady()}

			// Both spellings agree on all three: the problem on stdout, the
			// key named, and exit 2 — "no verdict reached" — rather than 1,
			// which means the DAGs failed a check.
			err := execute(t, d, args...)
			var exit *cliout.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("want cliout.ExitError, got %v", err)
			}
			if exit.Code != checks.ExitEnvNotReady {
				t.Errorf("exit code = %d, want %d", exit.Code, checks.ExitEnvNotReady)
			}
			if !strings.Contains(out.String(), "bogus_key") {
				t.Errorf("stdout should name the manifest key, got %q", out.String())
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

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
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

// Every traceback shape the review of #177 turned up, plus the degenerate
// ones. The rule is "the line closing the final frame block", not "the last
// line" — see exceptionLine.
func TestExceptionLine(t *testing.T) {
	const frames = "Traceback (most recent call last):\n" +
		"  File \"/p/dags/bad.py\", line 2, in <module>\n" +
		"    boom()\n"

	for _, tc := range []struct{ name, in, want string }{
		{
			"plain traceback",
			frames + "ModuleNotFoundError: No module named 'nope'",
			"ModuleNotFoundError: No module named 'nope'",
		},
		{
			// SQLAlchemy appends this on every error it raises.
			"exception with a trailing note",
			frames + "sqlalchemy.exc.OperationalError: (psycopg2.OperationalError) could not connect\n" +
				"(Background on this error at: https://sqlalche.me/e/20/e3q8)",
			"sqlalchemy.exc.OperationalError: (psycopg2.OperationalError) could not connect",
		},
		{
			// A message whose last line is the URL and nothing else. The two
			// cases above survive only because their URL follows other text:
			// "(Background on this error at: …" starts with a paren, pydantic's
			// with a word. A bare one is a dotted name and a colon — the same
			// shape as "ValueError:" — and this scan runs bottom-up, so the
			// scheme was reported as the exception that stopped the run.
			"exception whose last line is a bare url",
			frames + "ValueError: could not reach the warehouse\n" +
				"retrying will not help\n" +
				"https://example.com/docs/errors#e123",
			"ValueError: could not reach the warehouse",
		},
		{
			// The same, with no path after the host, so the colon is followed
			// by a digit rather than a slash.
			"exception whose last line is a bare url with a port",
			frames + "ConnectionError: refused\n" +
				"http://localhost:8080",
			"ConnectionError: refused",
		},
		{
			// pydantic's per-field detail, whose own last line is a URL.
			"exception with an indented detail block",
			frames + "pydantic_core.ValidationError: 1 validation error for Settings\n" +
				"api_key\n" +
				"  Field required [type=missing]\n" +
				"    For further information visit https://errors.pydantic.dev/2.6/v/missing",
			"pydantic_core.ValidationError: 1 validation error for Settings",
		},
		{
			// The OUTER exception is the one to report.
			"chained traceback",
			frames + "ValueError: inner\n\n" +
				"The above exception was the direct cause of the following exception:\n\n" +
				"Traceback (most recent call last):\n" +
				"  File \"/p/dags/bad.py\", line 9, in <module>\n" +
				"    outer()\n" +
				"RuntimeError: outer",
			"RuntimeError: outer",
		},
		{
			// No frames: Airflow's own import-error shape in health_test.
			"headline with an indented detail and no frames",
			"SyntaxError\n  line 3",
			"SyntaxError",
		},
		{
			// An exception raised with no message renders as the bare type, so
			// no line in the whole block carries a colon. Reported as the
			// banner until the no-colon fallback learned to read a traceback
			// from the bottom.
			"exception with no message",
			frames + "AssertionError",
			"AssertionError",
		},
		{
			// The same, dotted.
			"dotted exception with no message",
			frames + "airflow.exceptions.AirflowNotFoundException",
			"airflow.exceptions.AirflowNotFoundException",
		},
		{
			// Not a traceback at all — no banner — so the first line is the
			// headline, as it was. A caller can pass any multi-line text here.
			"multi-line text that is not a traceback",
			"boom\nsomething else",
			"boom",
		},
		{
			// TWO invalid fields. The second field name follows the first
			// field's indented detail, so a positional rule reads it as the
			// exception and reports a bare "db_url".
			"pydantic with two invalid fields",
			frames + "pydantic_core.ValidationError: 2 validation errors for Settings\n" +
				"api_key\n  Field required [type=missing]\n    For further information visit https://x\n" +
				"db_url\n  Field required [type=missing]\n    For further information visit https://x",
			"pydantic_core.ValidationError: 2 validation errors for Settings",
		},
		{
			// Python 3.11+ draws a margin down the left of an ExceptionGroup, so
			// EVERY line is indented. The sub-exception is reported rather than
			// the group header: "ExceptionGroup: eg (1 sub-exception)" names a
			// container, "ValueError: 1" names what actually broke, and on
			// `af health` — one line, no frames — that is the whole diagnosis.
			"exception group reports the sub-exception",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 5, in <module>\n" +
				"  |     raise ExceptionGroup(\"eg\", [ValueError(1)])\n" +
				"  | ExceptionGroup: eg (1 sub-exception)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +------------------------------------",
			"ValueError: 1",
		},
		{
			// With several, that reasoning inverts. The backwards scan returns
			// the LAST sub-exception, which is the third here only because it
			// is last — it says nothing about the other two, and any of the
			// three would be as arbitrary. The count is the honest summary, and
			// the frames printed underneath carry the detail.
			"exception group of several reports the group",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 5, in <module>\n" +
				"  |     raise ExceptionGroup(\"eg\", [ValueError(1), TypeError(2), KeyError(3)])\n" +
				"  | ExceptionGroup: eg (3 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +---------------- 3 ----------------\n" +
				"    | KeyError: 3\n" +
				"    +------------------------------------",
			"ExceptionGroup: eg (3 sub-exceptions)",
		},
		{
			// Groups nest, and the outermost describes the whole failure; an
			// inner one describes a part of it.
			// Captured from CPython 3.12: a nested group repeats no banner, so
			// the outer one is the only announcement in the message.
			"nested groups report the outermost",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 10, in <module>\n" +
				"  |     raise exc\n" +
				"  | ExceptionGroup: outer (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | ExceptionGroup: inner (2 sub-exceptions)\n" +
				"    +-+---------------- 1 ----------------\n" +
				"      | TypeError: 2\n" +
				"      +---------------- 2 ----------------\n" +
				"      | KeyError: 3\n" +
				"      +------------------------------------",
			"ExceptionGroup: outer (2 sub-exceptions)",
		},
		{
			// A group that was CAUGHT, with something else raised while
			// handling it. Python renders chains oldest-first, so the group is
			// the FIRST block and the exception that killed the run is the last
			// — taking the first group reported a handled exception, which is
			// the failure #184 exists to prevent.
			//
			// Captured from CPython 3.12.
			"a handled group does not outrank what actually failed",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 6, in <module>\n" +
				"  |     raise ExceptionGroup(\"eg\", [ValueError(1), TypeError(2)])\n" +
				"  | ExceptionGroup: eg (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +------------------------------------\n" +
				"\n" +
				"During handling of the above exception, another exception occurred:\n" +
				"\n" +
				"Traceback (most recent call last):\n" +
				"  File \"/t.py\", line 8, in <module>\n" +
				"    raise RuntimeError(\"gave up\")\n" +
				"RuntimeError: gave up",
			"RuntimeError: gave up",
		},
		{
			// A chain whose FINAL link is the group. The earlier link's own
			// "Traceback (most recent call last):" must not pull the window
			// back over it — which is why the two chain signals combine by
			// taking the later position rather than either one alone.
			"a chain ending in a group reports that group",
			"Traceback (most recent call last):\n" +
				"  File \"x.py\", line 1, in <module>\n" +
				"ValueError: handled\n" +
				"\n" +
				"During handling of the above exception, another exception occurred:\n" +
				"\n" +
				"  + Exception Group Traceback (most recent call last):\n" +
				"  | ExceptionGroup: eg (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | TypeError: a\n" +
				"    +---------------- 2 ----------------\n" +
				"    | KeyError: b\n" +
				"    +------------------------------------",
			"ExceptionGroup: eg (2 sub-exceptions)",
		},
		{
			// Two chained groups, captured from CPython 3.12. Neither link has
			// a bare "Traceback (most recent call last):" of its own, so the
			// separator sentence is the ONLY signal that finds the final one —
			// the case that earns it its place beside the traceback header.
			// Without it the first, handled group is reported.
			"a chain of two groups reports the second",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 5, in <module>\n" +
				"  |     raise ExceptionGroup(\"first\", [ValueError(1), TypeError(2)])\n" +
				"  | ExceptionGroup: first (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +------------------------------------\n" +
				"\n" +
				"During handling of the above exception, another exception occurred:\n" +
				"\n" +
				"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 7, in <module>\n" +
				"  |     raise ExceptionGroup(\"second\", [KeyError(3), IndexError(4), OSError(5)])\n" +
				"  | ExceptionGroup: second (3 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | KeyError: 3\n" +
				"    +---------------- 2 ----------------\n" +
				"    | IndexError: 4\n" +
				"    +---------------- 3 ----------------\n" +
				"    | OSError: 5\n" +
				"    +------------------------------------",
			"ExceptionGroup: second (3 sub-exceptions)",
		},
		{
			// Drift insurance, and the reason the chain boundary has two
			// signals. Every other string matched here degrades to an older,
			// defensible answer when Python rewords it; this one degraded to
			// naming an exception that was HANDLED, which is wrong rather than
			// merely coarse. The invented sentence stands in for a future
			// CPython that rewords the real one: the top-level traceback header
			// still marks the final link.
			"a reworded chain separator still finds the last link",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  | ExceptionGroup: eg (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +------------------------------------\n" +
				"\n" +
				"While handling the above, another exception was raised:\n" +
				"\n" +
				"Traceback (most recent call last):\n" +
				"  File \"y.py\", line 2, in <module>\n" +
				"RuntimeError: gave up",
			"RuntimeError: gave up",
		},
		{
			// The same, for the other sentence Python uses to chain.
			"a direct cause does not outrank its effect either",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  | ExceptionGroup: eg (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +------------------------------------\n" +
				"\n" +
				"The above exception was the direct cause of the following exception:\n" +
				"\n" +
				"Traceback (most recent call last):\n" +
				"RuntimeError: gave up",
			"RuntimeError: gave up",
		},
		{
			// An outer group of one says nothing its sub-exception does not say
			// better — but that sub-exception is itself a group of three, and
			// then the inner header is the summary. Returning at the outer one
			// left the backwards scan to pick the last leaf, which is the
			// arbitrary answer this whole rule exists to avoid.
			//
			// Captured from CPython 3.12.
			"a group of one wrapping a group of several reports the inner group",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  |   File \"/t.py\", line 14, in <module>\n" +
				"  |     raise ExceptionGroup(\"outer\", [ExceptionGroup(\"inner\", [ValueError(1), TypeError(2), KeyError(3)])])\n" +
				"  | ExceptionGroup: outer (1 sub-exception)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ExceptionGroup: inner (3 sub-exceptions)\n" +
				"    +-+---------------- 1 ----------------\n" +
				"      | ValueError: 1\n" +
				"      +---------------- 2 ----------------\n" +
				"      | TypeError: 2\n" +
				"      +---------------- 3 ----------------\n" +
				"      | KeyError: 3\n" +
				"      +------------------------------------",
			"ExceptionGroup: inner (3 sub-exceptions)",
		},
		{
			// The count comes from Python, the class name from whoever raised
			// it, so the suffix is what identifies a group header.
			"a BaseExceptionGroup subclass is still a group",
			"  + Exception Group Traceback (most recent call last):\n" +
				"  | acme.Fanout: two workers failed (2 sub-exceptions)\n" +
				"  +-+---------------- 1 ----------------\n" +
				"    | ValueError: 1\n" +
				"    +---------------- 2 ----------------\n" +
				"    | TypeError: 2\n" +
				"    +------------------------------------",
			"acme.Fanout: two workers failed (2 sub-exceptions)",
		},
		{
			// A plain exception whose message happens to end that way is not a
			// group header. Placed mid-chain, where the two rules disagree: the
			// ordinary one reports what the run actually died of, and mistaking
			// this for a group would report a handled exception instead.
			"a message ending in the same shape is not a group",
			"Traceback (most recent call last):\n" +
				"  File \"x.py\", line 1\n" +
				"ValueError: retried (3 sub-exceptions)\n" +
				"\n" +
				"During handling of the above exception, another exception occurred:\n" +
				"\n" +
				"Traceback (most recent call last):\n" +
				"  File \"y.py\", line 2\n" +
				"RuntimeError: gave up",
			"RuntimeError: gave up",
		},
		{
			// A blank line between the last frame and the exception leaves a
			// positional rule with an empty predecessor.
			"blank line before the exception",
			"Traceback (most recent call last):\n  File \"x.py\", line 1\n\nValueError: bad",
			"ValueError: bad",
		},
		{
			// An echoed source line carrying an annotation looks like an
			// exception once trimmed, so indentation without a gutter is skipped.
			"annotated source line is not an exception",
			"Traceback (most recent call last):\n  File \"x.py\", line 2, in <module>\n    x: int = 1\nTypeError: nope",
			"TypeError: nope",
		},
		{"single line", "boom", "boom"},
		{"single line with trailing newline", "boom\n", "boom"},
		{"empty", "", ""},
		{"whitespace only", "   \n\t\n", ""},
		{
			"crlf line endings",
			"Traceback (most recent call last):\r\n  File \"x.py\", line 1\r\nValueError: bad\r\n",
			"ValueError: bad",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := exceptionLine(tc.in); got != tc.want {
				t.Errorf("exceptionLine:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}

// One missing dependency makes every dag file an import error with the same
// traceback, so the frames are capped. The table still lists them all.
func TestCheckCapsTheTracebackDump(t *testing.T) {
	traceback := "Traceback (most recent call last):\n  File \"x.py\", line 1\nModuleNotFoundError: No module named 'nope'"
	report := checks.ParseReport{}
	for i := range maxTracebacks + 3 {
		report.ImportErrors = append(report.ImportErrors, checks.ReportImportErr{
			File:    fmt.Sprintf("dags/d%d.py", i),
			Message: traceback,
		})
	}
	d, out := checkDeps(t)
	d.Checks = stubParser{report: report}

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}

	got := out.String()
	if n := strings.Count(got, "Traceback (most recent call last)"); n != maxTracebacks {
		t.Errorf("printed %d frame blocks, want %d", n, maxTracebacks)
	}
	// "traceback(s)", not "import error(s)": every import error is still listed
	// in the table, and only the frames were suppressed.
	if !strings.Contains(got, "3 more traceback(s) not shown") {
		t.Errorf("nothing said how many tracebacks were suppressed: %q", got)
	}
	if strings.Contains(got, "import error(s) not shown") {
		t.Errorf("suppression line claims findings are missing from the table: %q", got)
	}
	// Every finding is still in the table, capped frames or not.
	for i := range maxTracebacks + 3 {
		if _, ok := lineContaining(got, fmt.Sprintf("dags/d%d.py", i)); !ok {
			t.Errorf("dags/d%d.py missing from the table", i)
		}
	}
}

// The budget is per RUN, not per report: two targets sharing one traceback must
// not print five blocks each with two contradictory suppression counts.
func TestCheckTracebackBudgetIsSharedAcrossTargets(t *testing.T) {
	traceback := "Traceback (most recent call last):\n  File \"x.py\", line 1\nModuleNotFoundError: No module named 'nope'"
	report := checks.ParseReport{}
	for i := range maxTracebacks {
		report.ImportErrors = append(report.ImportErrors, checks.ReportImportErr{
			File:    fmt.Sprintf("dags/d%d.py", i),
			Message: traceback,
		})
	}
	d, out := targetDeps(t)
	d.CheckVenv = stubTargetParser{report: report}

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check", "--target", "composer,mwaa"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}

	got := out.String()
	if n := strings.Count(got, "Traceback (most recent call last)"); n != maxTracebacks {
		t.Errorf("printed %d frame blocks across both targets, want %d", n, maxTracebacks)
	}
	if n := strings.Count(got, "traceback(s) not shown"); n != 1 {
		t.Errorf("printed %d suppression lines, want 1", n)
	}
}

// A message that is one line plus a trailing newline has nothing to show under
// the table, and printing it anyway repeated the row verbatim.
func TestCheckPrintsNoTracebackForASingleLineMessage(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{{File: "dags/bad.py", Message: "ModuleNotFoundError: no module\n"}},
	}}

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	if n := strings.Count(out.String(), "ModuleNotFoundError"); n != 1 {
		t.Errorf("message appears %d times, want once (in the row): %q", n, out.String())
	}
}

// A tab in an exception message would open a new tabwriter column and shift
// every row under it.
func TestCheckKeepsTheTableAlignedAroundATab(t *testing.T) {
	d, out := checkDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{
		ImportErrors: []checks.ReportImportErr{
			{File: "dags/a.py", Message: "ValueError: bad\tvalue here"},
			{File: "dags/b.py", Message: "ValueError: plain"},
		},
	}}

	var exit *cliout.ExitError
	if err := execute(t, d, "local", "check"); !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
	}
	rowA, ok := lineContaining(out.String(), "dags/a.py")
	if !ok {
		t.Fatalf("no row for dags/a.py: %q", out.String())
	}
	if strings.Contains(rowA, "\t") {
		t.Errorf("row carries a tab, which shifts the table: %q", rowA)
	}
	if !strings.Contains(rowA, "bad value here") {
		t.Errorf("the tab should become a space, got %q", rowA)
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
	var exit *cliout.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want cliout.ExitError, got %v", err)
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
	var exit *cliout.ExitError
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

// envCapturingParser records the environment each parse was handed.
type envCapturingParser struct {
	got *[]string
}

func (p envCapturingParser) Parse(_ context.Context, in checks.ParseInput) (checks.ParseReport, error) {
	*p.got = in.Env
	return checks.ParseReport{}, nil
}

func (p envCapturingParser) ParseWith(_ context.Context, _ string, in checks.ParseInput) (checks.ParseReport, error) {
	*p.got = in.Env
	return checks.ParseReport{}, nil
}

// A check imports the DAGs under the values a start would give them: the
// project's .env and a declared default, as well as the shell. A DAG that reads
// its .env at import time otherwise fails here and runs fine under a start.
func TestCheckParsesUnderTheProjectEnvironment(t *testing.T) {
	const m = validManifest + "\n[tool.astro.env]\nASTRO_TEST_DEFAULTED = { default = 'from-manifest' }\n"
	for _, args := range [][]string{
		{"local", "check"},
		{"local", "check", "--target", "astro"},
		{"local", "check", "--target", "composer"},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
			isolateEnvSources(t, "ASTRO_TEST_DOTENV", "ASTRO_TEST_DEFAULTED")
			t.Setenv("ASTRO_TEST_SHELL", "from-shell")
			d, _ := targetDeps(t)
			dir, err := d.WorkingDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ASTRO_TEST_DOTENV=sandbox\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var got []string
			d.Checks = envCapturingParser{got: &got}
			d.CheckVenv = envCapturingParser{got: &got}

			_ = execute(t, d, args...)

			for _, want := range []string{"ASTRO_TEST_DOTENV=sandbox", "ASTRO_TEST_DEFAULTED=from-manifest", "ASTRO_TEST_SHELL=from-shell"} {
				if !slices.Contains(got, want) {
					t.Errorf("the parse env is missing %s", want)
				}
			}
		})
	}
}

// A [tool.astro.env] declaration that does not parse stops a check the way it
// stops a start, because the check can no longer tell what a start would give
// the DAGs.
func TestCheckIsBlockedByAnEnvDeclarationThatDoesNotParse(t *testing.T) {
	isolateEnvSources(t)
	d, out := checkDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	m := validManifest + "\n[tool.astro.env]\nASTRO_TEST_BAD = 5\n"
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}

	err = execute(t, d, "local", "check")
	var exit *cliout.ExitError
	if !errors.As(err, &exit) || exit.Code != checks.ExitEnvNotReady {
		t.Fatalf("want exit %d, got %v", checks.ExitEnvNotReady, err)
	}
	if !strings.Contains(out.String(), "ASTRO_TEST_BAD") {
		t.Errorf("stdout should name the declaration, got %q", out.String())
	}
}
