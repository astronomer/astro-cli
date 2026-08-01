package local

import (
	"net/http"
	"strings"
	"testing"
)

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
// `astro local health` and passing no selector at all.
func TestLocalHealthReadsTheMachine(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[],"total_entries":0}`)

	out, errOut, err := runLocalQuery(t, stub, "local", "health", "-o", "json")
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
