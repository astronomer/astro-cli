package local

import (
	"net/http"
	"strings"
	"testing"
)

// The two generations answer this endpoint differently and the report has to be
// true of both.
//
// Airflow 3 builds its response from the rows of a DagRun query, so only DAGs
// with runs come back. Airflow 2 builds it from the REQUESTED ids, and
// pkg/airflowapi asks it for every DAG on the instance — so every DAG comes
// back, run or not. Both zero-fill every state on every row they return, which
// is why neither len(DAGs) nor "did a state key appear" answers "has anything
// run here".
//
// Driven through the real command against wire-shaped payloads, because the
// original bug was invisible to a test that built healthDAGStats by hand: no
// hand-written fixture carries the zero-filled states the endpoint always
// sends, and every other health test stubs this endpoint as `{"dags":[]}`.
func TestHealthDAGStatsIsTrueOnBothGenerations(t *testing.T) {
	// One DAG, never run — what a fresh project looks like. Airflow 2 returns
	// the row anyway, zero-filled.
	const af2NoRuns = `{"dags":[{"dag_id":"example_dag","stats":[
		{"state":"queued","count":0},{"state":"running","count":0},
		{"state":"success","count":0},{"state":"failed","count":0}]}],"total_entries":1}`
	// Ten DAGs, two of which have ever run.
	const af2SomeRuns = `{"dags":[
		{"dag_id":"a","stats":[{"state":"success","count":3},{"state":"failed","count":0}]},
		{"dag_id":"b","stats":[{"state":"success","count":2},{"state":"failed","count":1}]},
		{"dag_id":"c","stats":[{"state":"success","count":0},{"state":"failed","count":0}]},
		{"dag_id":"d","stats":[{"state":"success","count":0},{"state":"failed","count":0}]}],"total_entries":4}`

	for _, tc := range []struct {
		name    string
		af2     bool
		dags    string
		payload string
		want    string
		absent  string
	}{
		{
			// Before: "1 DAG(s) with runs, failed=0 queued=0 running=0
			// success=0" — a claim that a DAG has runs, with four zeros
			// disproving it.
			name: "airflow 2, one DAG, never run", af2: true,
			dags:    `{"dags":[{"dag_id":"example_dag"}],"total_entries":1}`,
			payload: af2NoRuns,
			want:    "dag stats: no runs", absent: "DAG(s)",
		},
		{
			// Before: "4 DAG(s) with runs" — two of them never ran.
			name: "airflow 2, four DAGs, two with runs", af2: true,
			dags:    `{"dags":[{"dag_id":"a"},{"dag_id":"b"},{"dag_id":"c"},{"dag_id":"d"}],"total_entries":4}`,
			payload: af2SomeRuns,
			want:    "dag stats: 2 DAG(s) with runs, failed=1 success=5",
		},
		{
			name: "airflow 3, no DAG has runs", payload: `{"dags":[],"total_entries":0}`,
			want: "dag stats: no runs", absent: "DAG(s)",
		},
		{
			name: "airflow 3, two DAGs with runs",
			payload: `{"dags":[
				{"dag_id":"a","stats":[{"state":"success","count":2},{"state":"failed","count":0}]},
				{"dag_id":"b","stats":[{"state":"success","count":0},{"state":"failed","count":1}]}],"total_entries":2}`,
			want: "dag stats: 2 DAG(s) with runs, failed=1 success=2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stub *airflowStub
			if tc.af2 {
				stub = newAirflow2Stub(t)
				stub.route(http.MethodGet, "/api/v1/importErrors", `{"import_errors":[],"total_entries":0}`)
				stub.route(http.MethodGet, "/api/v1/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
				stub.route(http.MethodGet, "/api/v1/dags", tc.dags)
				stub.route(http.MethodGet, "/api/v1/dagStats", tc.payload)
			} else {
				stub = newAirflowStub(t)
				stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
				stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
				stub.route(http.MethodGet, "/api/v2/dagStats", tc.payload)
			}

			out, _, err := runQuery(t, stub, "health")
			if err != nil {
				t.Fatalf("health: %v", err)
			}
			line, ok := lineContaining(out, "dag stats:")
			if !ok {
				t.Fatalf("no dag stats line: %q", out)
			}
			if !strings.Contains(line, tc.want) {
				t.Errorf("got %q, want it to contain %q", line, tc.want)
			}
			if tc.absent != "" && strings.Contains(line, tc.absent) {
				t.Errorf("got %q, should not contain %q", line, tc.absent)
			}
		})
	}
}

// The text and the json must describe the same array the same way: the old text
// called it "DAGs with runs" while the json shipped rows of all-zero counts.
func TestHealthDAGStatsJSONAgreesWithTheText(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v1/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v1/dags", `{"dags":[{"dag_id":"a"},{"dag_id":"b"}],"total_entries":2}`)
	stub.route(http.MethodGet, "/api/v1/dagStats", `{"dags":[
		{"dag_id":"a","stats":[{"state":"success","count":3}]},
		{"dag_id":"b","stats":[{"state":"success","count":0}]}],"total_entries":2}`)

	out, _, err := runQuery(t, stub, "health", "-o", "json")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	stats, _ := decodeJSON(t, out)["dag_stats"].(map[string]any)
	if stats["runs"] != float64(3) {
		t.Errorf("runs = %v, want 3", stats["runs"])
	}
	// Two rows came back; one of them has never run.
	if stats["dags_with_runs"] != float64(1) {
		t.Errorf("dags_with_runs = %v, want 1", stats["dags_with_runs"])
	}
	dags, _ := stats["dags"].([]any)
	if len(dags) != 2 {
		t.Fatalf("dags = %v, want both rows kept", stats["dags"])
	}
	// The rows are dags stats' rows, Airflow's {state, count} list, which is
	// what af's health passes through as well.
	first, _ := dags[0].(map[string]any)
	counts, _ := first["stats"].([]any)
	if len(counts) != 1 {
		t.Fatalf("dags[0].stats = %v, want one {state, count}", first["stats"])
	}
	if c, _ := counts[0].(map[string]any); c["state"] != "success" || c["count"] != float64(3) {
		t.Errorf("dags[0].stats[0] = %v, want success 3", counts[0])
	}
}

// health reads four things and each one stands or falls alone: an Airflow
// serving three of them still answers with three.
func TestHealthComposesAndDegrades(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[
		{"dag_id":"orders_etl","warning_type":"non-existent pool","message":"pool 'bulk' does not exist"}],"total_entries":1}`)
	// /api/v2/dagStats is unregistered, so this instance does not serve it.

	out, _, err := runQuery(t, stub, "health", "-o", "json")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	report := decodeJSON(t, out)
	if report["overall_status"] != healthWarning {
		t.Errorf("overall_status = %v, want %s", report["overall_status"], healthWarning)
	}
	if reason, _ := report["status_reason"].(string); !strings.Contains(reason, "1 DAG warning(s)") {
		t.Errorf("status_reason = %v", report["status_reason"])
	}
	// The version section carries Airflow's own account of itself, so a reader
	// gets it without a second command.
	version, _ := report["version"].(map[string]any)
	if version["version"] != "3.0.3" || version["generation"] != "3" || version["git_version"] != "abc123" {
		t.Errorf("version section = %v", version)
	}
	// The section that is not served says so and carries no error: an absent
	// endpoint is an answer, not a failure.
	stats, _ := report["dag_stats"].(map[string]any)
	if stats["available"] != false || stats["error"] != nil {
		t.Errorf("dag_stats = %v, want it marked unavailable without an error", stats)
	}
	if note, _ := stats["note"].(string); !strings.Contains(note, "does not serve") {
		t.Errorf("dag_stats note = %v", stats["note"])
	}
}

// The same report against the machine, reached by spelling the command
// `astro local af health` and passing no selector at all.
func TestLocalHealthReadsTheMachine(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, errOut, err := runLocalQuery(t, stub, "local", "af", "health", "-o", "json")
	if err != nil {
		t.Fatalf("local health: %v", err)
	}
	if got := decodeJSON(t, out)["overall_status"]; got != healthHealthy {
		t.Errorf("overall_status = %v, want %s", got, healthHealthy)
	}
	if !strings.Contains(errOut, "→ local") {
		t.Errorf("the machine was not announced: %q", errOut)
	}
}

func TestHealthVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		importErrors string
		warnings     string
		want         string
		reason       string
	}{
		{
			name:         "clean",
			importErrors: `{"import_errors":[],"total_entries":0}`,
			warnings:     `{"dag_warnings":[],"total_entries":0}`,
			want:         healthHealthy,
			reason:       "no import errors or DAG warnings",
		},
		{
			name:         "import errors outrank warnings",
			importErrors: `{"import_errors":[{"filename":"dags/broken.py","stack_trace":"SyntaxError\n  line 3"}],"total_entries":1}`,
			warnings:     `{"dag_warnings":[{"dag_id":"orders_etl","message":"m"}],"total_entries":1}`,
			want:         healthUnhealthy,
			reason:       "1 DAG file(s) failed to parse",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := newAirflowStub(t)
			stub.route(http.MethodGet, "/api/v2/importErrors", tc.importErrors)
			stub.route(http.MethodGet, "/api/v2/dagWarnings", tc.warnings)
			stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

			out, _, err := runQuery(t, stub, "health", "-o", "json")
			if err != nil {
				t.Fatalf("health: %v", err)
			}
			report := decodeJSON(t, out)
			if report["overall_status"] != tc.want {
				t.Errorf("overall_status = %v, want %s", report["overall_status"], tc.want)
			}
			if reason, _ := report["status_reason"].(string); !strings.Contains(reason, tc.reason) {
				t.Errorf("status_reason = %v, want it to carry %q", report["status_reason"], tc.reason)
			}
		})
	}
}

// A page is not a total. An Airflow with more import errors than fit in one
// request still has that many broken files, and the verdict has to say the
// real number rather than the size of the page it read.
func TestHealthCountsWhatTheInstanceHasNotWhatFitOnThePage(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors",
		`{"import_errors":[{"filename":"dags/broken.py"}],"total_entries":250}`)
	stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, _, err := runQuery(t, stub, "health", "-o", "json")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	report := decodeJSON(t, out)
	section, _ := report["import_errors"].(map[string]any)
	if section["count"] != float64(250) {
		t.Errorf("count = %v, want the instance's total of 250", section["count"])
	}
	if reason, _ := report["status_reason"].(string); !strings.Contains(reason, "250 DAG file(s)") {
		t.Errorf("status_reason = %v, want the real number", report["status_reason"])
	}
}

// "I checked and it was clean" and "I could not check" are different claims,
// and only the first is good news. An unread section holds the verdict back to
// warning rather than letting a clean partial read pass as healthy.
func TestHealthWithholdsHealthyWhenASectionIsUnread(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.routeStatus(http.MethodGet, "/api/v2/dagWarnings", http.StatusInternalServerError, `{"detail":"boom"}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, _, err := runQuery(t, stub, "health", "-o", "json")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	report := decodeJSON(t, out)
	if report["overall_status"] != healthWarning {
		t.Errorf("overall_status = %v, want %s — nothing readable was wrong, but not everything was readable",
			report["overall_status"], healthWarning)
	}
	if reason, _ := report["status_reason"].(string); !strings.Contains(reason, "not enough was read to judge") {
		t.Errorf("status_reason = %v", report["status_reason"])
	}
}

// A refusal reads as one about the token, not about the Airflow. Some
// deployments grant no role the permission /dagWarnings needs, so this is the
// shape a reader meets rather than an exotic case — and it must not turn a
// health report into five lines of raw problem+json. The section still counts
// as unread, because not being allowed to look is not a clean bill.
func TestHealthSaysARefusalIsAboutTheToken(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.routeStatus(http.MethodGet, "/api/v2/dagWarnings", http.StatusForbidden,
		`{"detail":null,"status":403,"title":"Forbidden","type":"http://apache-airflow-docs.s3-website.eu-central-1.amazonaws.com/docs/apache-airflow/stable/stable-rest-api-ref.html#section/Errors/PermissionDenied"}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, _, err := runQuery(t, stub, "health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if want := "dag warnings: could not be read (this token is not allowed to read it)"; !strings.Contains(out, want) {
		t.Errorf("stdout is missing %q:\n%s", want, out)
	}
	for _, unwanted := range []string{"apache-airflow-docs", `"status": 403`, "PermissionDenied"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the raw problem document leaked into the report (%q):\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "not enough was read to judge") {
		t.Errorf("a refused section must still withhold a clean bill:\n%s", out)
	}
}

// A report of nothing is not a report: when every section fails the command
// fails too, so a script cannot read a verdict off an Airflow it never reached.
func TestHealthFailsWhenNothingCouldBeRead(t *testing.T) {
	stub := newAirflowStub(t)
	// Detection answers, so a client opens; every section below it 404s.
	stub.routeStatus(http.MethodGet, "/api/v2/version", http.StatusInternalServerError, `{"detail":"down"}`)

	out, _, err := runQuery(t, stub, "health")
	if err == nil {
		t.Fatal("a report with no readable section must fail")
	}
	if !strings.Contains(err.Error(), "could not read anything") {
		t.Errorf("err = %q, want it to say there was nothing to report", err)
	}
	if strings.Contains(out, healthHealthy) {
		t.Errorf("a verdict was printed with no evidence behind it:\n%s", out)
	}
}

// A verdict that rests on less than the whole report says so, so "healthy"
// never quietly stands in for "not checked".
func TestHealthNamesWhatItCouldNotRead(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.routeStatus(http.MethodGet, "/api/v2/dagWarnings", http.StatusInternalServerError, `{"detail":"boom"}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, _, err := runQuery(t, stub, "health")
	if err != nil {
		t.Fatalf("one failed section must not end the command: %v", err)
	}
	if !strings.Contains(out, "dag warnings: could not be read") {
		t.Errorf("output does not report the failed section:\n%s", out)
	}
	if !strings.Contains(out, "DAG warnings could not be read") {
		t.Errorf("the verdict does not name the gap:\n%s", out)
	}
	if !strings.Contains(out, "version: 3.0.3") {
		t.Errorf("the sections that worked were lost:\n%s", out)
	}

	// The prose reason is not the only signal: a script filtering on
	// overall_status needs the gap as data, not as a sentence to parse.
	jsonOut, _, err := runQuery(t, stub, "health", "-o", "json")
	if err != nil {
		t.Fatalf("health -o json: %v", err)
	}
	unread, _ := decodeJSON(t, jsonOut)["unread"].([]any)
	if len(unread) != 1 || unread[0] != "DAG warnings" {
		t.Errorf("unread = %v, want the one section that failed", unread)
	}
}
