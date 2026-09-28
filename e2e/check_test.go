//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `astro local check` against a real Airflow is the best end-to-end case in the
// product: it needs no ports, no processes and no cloud, it is deterministic,
// and it is the only tier that proves the embedded parse script runs under an
// interpreter that actually has Airflow in it. Everything below is that command.

// writeDag puts a file in the project's dags directory, which is where every
// case here starts. Through write rather than its own os.WriteFile, so the
// mode and the failure message stay in one place.
func writeDag(t *testing.T, p *project, name, body string) {
	t.Helper()
	write(t, filepath.Join(p.Dir, "dags", name), body)
}

// removeDag drops one, for a case that wants the starter DAG out of the way.
func removeDag(t *testing.T, p *project, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(p.Dir, "dags", name)); err != nil {
		t.Fatalf("removing %s: %v", name, err)
	}
}

// The scaffolded project passes, which is the claim `astro init` makes when it
// prints "Next: astro local start".
func TestCheckPassesOnAScaffoldedProject(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	r := p.run("local", "check").requireSuccess()
	if !strings.Contains(r.Stdout, "checks passed") {
		t.Errorf("expected a passing summary\n%s", r.output())
	}
	if !strings.Contains(r.Stdout, "1 DAGs") {
		t.Errorf("expected the starter DAG to be counted\n%s", r.output())
	}
}

// The other half of the same guarantee: a project with no interpreter of its
// own gets one built. This is every docker-mode project, and the only tier that
// can prove the built environment actually has a working Airflow in it.
func TestCheckBuildsAnEnvironmentAndPasses(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "noenv").requireSuccess()
	if _, err := os.Stat(filepath.Join(p.Dir, ".venv")); !os.IsNotExist(err) {
		t.Fatalf("a freshly scaffolded project should have no .venv: %v", err)
	}

	r := p.run("local", "check").requireSuccess()
	if !strings.Contains(r.Stdout, "checks passed") {
		t.Errorf("expected a passing summary\n%s", r.output())
	}
	if !strings.Contains(r.Stdout, "provisioning") && !strings.Contains(r.Stdout, "reusing") {
		t.Errorf("expected a note about building or reusing an environment\n%s", r.output())
	}
	// A check is a question. Asking it must not leave a .venv in a project
	// whose author chose not to have one.
	if _, err := os.Stat(filepath.Join(p.Dir, ".venv")); !os.IsNotExist(err) {
		t.Error("the check left a .venv behind in the project")
	}
}

// A DAG that cannot import is an error, and the row names the exception that
// closed the traceback rather than the banner that opened it.
func TestCheckReportsAnImportError(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	writeDag(t, p, "broken.py", "from airflow.sdk import dag\nimport nonexistent_module_xyz\n")

	r := p.run("local", "check").requireFailure()
	if r.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1 (checked, and found a problem)", r.ExitCode)
	}
	row, ok := lineWith(r.Stdout, "import_error")
	if !ok {
		t.Fatalf("no import_error row\n%s", r.output())
	}
	if !strings.Contains(row, "dags/broken.py") {
		t.Errorf("the row does not name the file: %q", row)
	}
	if !strings.Contains(row, "ModuleNotFoundError") {
		t.Errorf("the row should name the exception that stopped the import: %q", row)
	}
	if strings.Contains(row, "Traceback") {
		t.Errorf("the row names the banner rather than the exception: %q", row)
	}
	// The frames print under the table, the way a compiler prints the source
	// line under its summary.
	if !strings.Contains(r.Stdout, "nonexistent_module_xyz") {
		t.Errorf("expected the traceback under the table\n%s", r.output())
	}
}

// The row names the exception even when the message's own last line looks like
// one.
//
// A library that ends an error with a bare documentation URL — SQLAlchemy and
// pydantic both do — produces a traceback whose last line is
// "https://example.com/...", which is a dotted name followed by a colon. That
// is the one input where reading the exception by shape and reading it by
// position disagree, so it is the one worth spending a real Airflow on: the
// cheaper shapes are covered by the unit table, which can enumerate dozens of
// them without installing anything.
func TestCheckNamesTheExceptionNotAUrlInItsMessage(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	writeDag(t, p, "urlerr.py", "from airflow.sdk import dag\n"+
		"raise ValueError('could not reach the warehouse\\n"+
		"retrying will not help\\n"+
		"https://example.com/docs/errors#e123')\n")

	r := p.run("local", "check").requireFailure()
	row, ok := lineWith(r.Stdout, "import_error")
	if !ok {
		t.Fatalf("no import_error row\n%s", r.output())
	}
	if !strings.Contains(row, "ValueError: could not reach the warehouse") {
		t.Errorf("the row should name the exception, got %q", row)
	}
	if strings.Contains(row, "https://") {
		t.Errorf("the row names a URL from the message rather than the exception: %q", row)
	}
}

// A DAG that reads the project's .env at import time passes, because a start
// gives it those values and a check that did not would report a working DAG
// as broken.
func TestCheckImportsUnderTheProjectDotenv(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	write(t, filepath.Join(p.Dir, ".env"), "ENV=sandbox\n")
	writeDag(t, p, "envdag.py", "import os\nfrom airflow.sdk import dag\n"+
		"assert os.environ['ENV'].lower() == 'sandbox'\n")

	r := p.run("local", "check").requireSuccess()
	if !strings.Contains(r.Stdout, "checks passed") {
		t.Errorf("expected a passing summary\n%s", r.output())
	}
}

// Two files defining one dag_id is a duplicate, not an import error, and it is
// reported once however many files are involved.
func TestCheckReportsADuplicateDagID(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	removeDag(t, p, "exampledag.py")
	const dag = "from airflow.sdk import DAG\nimport datetime\n" +
		"with DAG(dag_id='shared', start_date=datetime.datetime(2024, 1, 1)):\n    pass\n"
	for _, name := range []string{"a.py", "b.py", "c.py"} {
		writeDag(t, p, name, dag)
	}

	r := p.run("local", "check").requireFailure()
	if r.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", r.ExitCode)
	}
	if n := strings.Count(r.Stdout, "duplicate_dag_id"); n != 1 {
		t.Errorf("three copies of one dag_id is one problem, got %d rows\n%s", n, r.output())
	}
	row, _ := lineWith(r.Stdout, "duplicate_dag_id")
	for _, f := range []string{"dags/a.py", "dags/b.py", "dags/c.py"} {
		if !strings.Contains(row, f) {
			t.Errorf("the row does not name %s: %q", f, row)
		}
	}
	if !strings.Contains(row, "Airflow loaded") {
		t.Errorf("the row should say which copy Airflow loaded: %q", row)
	}
}

// A slow DAG is a warning, not a failure — slow is not broken — and --strict
// turns it into one.
//
// The threshold is two thirds of the import timeout in force, so the timeout is
// lowered here rather than sleeping past the default. That keeps the case at a
// few seconds instead of twenty-odd, and it asserts the tracking as well: a
// project that configures its own timeout is judged against that one.
func TestCheckWarnsAboutASlowDagAndStrictFailsOnIt(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	writeDag(t, p, "slow.py", "from airflow.sdk import dag\nimport time\ntime.sleep(5)\n")
	lowTimeout := map[string]string{"AIRFLOW__CORE__DAGBAG_IMPORT_TIMEOUT": "6"}

	r := p.runWith(lowTimeout, "local", "check").requireSuccess()
	row, ok := lineWith(r.Stdout, "slow_parse")
	if !ok {
		t.Fatalf("no slow_parse row\n%s", r.output())
	}
	if !strings.Contains(row, "dags/slow.py") {
		t.Errorf("the row does not name the file: %q", row)
	}
	// Two thirds of 6 is 4, so the threshold reported must be the derived one
	// and not the default.
	if !strings.Contains(row, "threshold 4s") {
		t.Errorf("the threshold should follow the configured timeout: %q", row)
	}

	r = p.runWith(lowTimeout, "local", "check", "--strict").requireFailure()
	if r.ExitCode != 1 {
		t.Errorf("--strict exit code = %d, want 1", r.ExitCode)
	}
	if !strings.Contains(r.Stdout, "slow_parse") {
		t.Errorf("--strict should still report the finding\n%s", r.output())
	}
}

// The JSON contract: one object per finding, then a summary, and the summary's
// verdict agrees with the exit code. This is what a CI job and Astro Desktop
// read, so it is pinned rather than left to the text.
func TestCheckJSONLIsOneFindingPerLineThenASummary(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	writeDag(t, p, "broken.py", "from airflow.sdk import dag\nimport nonexistent_module_xyz\n")

	r := p.run("local", "check", "--output", "json").requireFailure()

	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) < 2 {
		t.Fatalf("want at least a finding and a summary, got %d line(s)\n%s", len(lines), r.output())
	}

	var sawImportError bool
	for _, line := range lines[:len(lines)-1] {
		var finding struct {
			Kind     string `json:"kind"`
			Severity string `json:"severity"`
			File     string `json:"file"`
			Message  string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &finding); err != nil {
			t.Fatalf("a finding line is not JSON: %v: %q", err, line)
		}
		if finding.Kind == "import_error" {
			sawImportError = true
			if finding.File != "dags/broken.py" {
				t.Errorf("file = %q, want dags/broken.py", finding.File)
			}
			if finding.Severity != "error" {
				t.Errorf("severity = %q, want error", finding.Severity)
			}
			// The whole message, not the one line the table shows.
			if !strings.Contains(finding.Message, "Traceback") {
				t.Errorf("json should carry the full traceback, got %q", finding.Message)
			}
		}
	}
	if !sawImportError {
		t.Errorf("no import_error finding in the payload\n%s", r.output())
	}

	var summary struct {
		Event    string `json:"event"`
		Dags     int    `json:"dags"`
		Errors   int    `json:"errors"`
		Warnings int    `json:"warnings"`
		Strict   bool   `json:"strict"`
		Passed   bool   `json:"passed"`
	}
	last := lines[len(lines)-1]
	if err := json.Unmarshal([]byte(last), &summary); err != nil {
		t.Fatalf("the last line is not the summary: %v: %q", err, last)
	}
	if summary.Event != "summary" {
		t.Fatalf("last line event = %q, want summary", summary.Event)
	}
	if summary.Errors != 1 {
		t.Errorf("errors = %d, want 1", summary.Errors)
	}
	// The verdict and the exit code are two renderings of one answer, and a
	// consumer that trusts one over the other must not be able to tell them
	// apart.
	if summary.Passed {
		t.Error("summary says passed while the command exited non-zero")
	}
}

// A platform target checks the project against the Airflow that platform
// actually runs, in a scratch environment built for it.
//
// Both branches are accepted on the constraints step, on purpose. Fetching
// MWAA's constraints file needs the network, and the CLI treats an unreachable
// one as a skip rather than a failure — so a case that demanded a successful
// fetch would fail on an offline machine for a reason that says nothing about
// this product. What is asserted either way is the part that is ours: that a
// pin the platform does not offer is mapped down, with a note saying so.
func TestCheckAgainstAPlatformTarget(t *testing.T) {
	tier(t, 1)
	needsUV(t)

	p := newProject(t)
	p.run("init", "--name", "target").requireSuccess()

	r := p.run("local", "check", "--target", "mwaa").requireSuccess()

	if !strings.Contains(r.Stdout, "== mwaa ==") {
		t.Errorf("expected a target block\n%s", r.output())
	}
	// The scaffold pins an Airflow MWAA does not offer, so the run maps down
	// and says which version it checked and why.
	//
	// That precondition is data, not logic: pkg/platformversions.MWAA is a
	// hand-maintained list its own comment calls "data to refresh", and the pin
	// comes from runtimeversions.FallbackAirflowSeries, since the harness keeps
	// init offline. If MWAA ever lists the pinned version there is nothing to
	// map down from and this stops being the case it is — so the failure says
	// that, rather than leaving the next person to work out why a passing
	// feature looks broken.
	if !strings.Contains(r.Stdout, "mapped down from the manifest pin") {
		t.Errorf("expected the version-mapping note. If MWAA now lists the "+
			"scaffold's pinned Airflow, this case needs a pin that it does "+
			"not, rather than a fix\n%s", r.output())
	}
	if !strings.Contains(r.Stdout, "check passed for mwaa") {
		t.Errorf("expected a passing target verdict\n%s", r.output())
	}
	// Whichever way the constraints step went, it reported an outcome rather
	// than going quiet.
	if !strings.Contains(r.Stdout, "constraints:") {
		t.Errorf("expected a constraints line\n%s", r.output())
	}
}

// --target astro is documented as an alias for the plain check, and a tier-1
// run is where that can be proved against a real environment rather than a
// stub.
func TestCheckTargetAstroMatchesThePlainCheck(t *testing.T) {
	tier(t, 1)

	p := airflowProject(t)
	plain := p.run("local", "check").requireSuccess()
	astro := p.run("local", "check", "--target", "astro").requireSuccess()

	if !strings.Contains(plain.Stdout, "checks passed") {
		t.Errorf("plain check did not pass\n%s", plain.output())
	}
	if !strings.Contains(astro.Stdout, "check passed for astro") {
		t.Errorf("--target astro did not pass\n%s", astro.output())
	}
}

// lineWith returns the first line containing want.
func lineWith(s, want string) (string, bool) {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, want) {
			return line, true
		}
	}
	return "", false
}
