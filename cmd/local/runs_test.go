package local

import (
	"net/http"
	"strings"
	"testing"
)

const twoRunsAF3 = `{"dag_runs":[
	{"dag_id":"orders_etl","dag_run_id":"manual__2024-05-01","state":"success","run_type":"manual",
	 "logical_date":"2024-05-01T00:00:00Z","start_date":"2024-05-01T00:00:05Z","end_date":"2024-05-01T00:01:05Z",
	 "triggered_by":"rest_api"},
	{"dag_id":"orders_etl","dag_run_id":"scheduled__2024-04-30","state":"failed","run_type":"scheduled",
	 "logical_date":"2024-04-30T00:00:00Z"}
],"total_entries":2}`

func TestRunsListRendersAndAsksMostRecentFirst(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/~/dagRuns", twoRunsAF3)

	out, _, err := runQuery(t, stub, "runs", "list")
	if err != nil {
		t.Fatalf("runs list: %v", err)
	}
	for _, want := range []string{"RUN_ID", "manual__2024-05-01", "success", "1m"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}
	// The table renders the duration; json keeps the number it rendered from.
	if strings.Contains(out, "duration_seconds") {
		t.Errorf("the table printed a json key:\n%s", out)
	}
	// A bare listing is "what just happened", which Airflow's own default order
	// is not.
	if got := stub.request(http.MethodGet, "/api/v2/dags/~/dagRuns").Query; !strings.Contains(got, "order_by=-start_date") {
		t.Errorf("query = %q, want the most-recent-first order", got)
	}

	out, _, err = runQuery(t, stub, "runs", "list", "-o", "json")
	if err != nil {
		t.Fatalf("runs list -o json: %v", err)
	}
	row := decodeNDJSON(t, out)[0]
	if row["dag_run_id"] != "manual__2024-05-01" || row["state"] != "success" {
		t.Errorf("row = %v", row)
	}
	// Seconds as a number, not a pre-rendered "1m": a consumer has to be able
	// to compare and sum durations without parsing our prose.
	if row["duration_seconds"] != float64(60) {
		t.Errorf("duration_seconds = %v (%T), want the number 60", row["duration_seconds"], row["duration_seconds"])
	}
}

func TestRunsListFiltersByStateAndDAG(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns", twoRunsAF3)

	if _, _, err := runQuery(t, stub, "runs", "list", "-d", "orders_etl", "-s", "failed", "-s", "running", "-l", "5"); err != nil {
		t.Fatalf("runs list: %v", err)
	}
	query := stub.request(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns").Query
	for _, want := range []string{"state=failed", "state=running", "limit=5"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to carry %s", query, want)
		}
	}
}

func TestRunsListFiltersByStartDate(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/~/dagRuns", twoRunsAF3)

	_, _, err := runQuery(t, stub, "runs", "list",
		"--start-date-gte", "2024-04-01T00:00:00Z", "--start-date-lte", "2024-05-01T00:00:00Z")
	if err != nil {
		t.Fatalf("runs list with date bounds: %v", err)
	}
	query := stub.request(http.MethodGet, "/api/v2/dags/~/dagRuns").Query
	for _, want := range []string{"start_date_gte=2024-04-01T00%3A00%3A00Z", "start_date_lte=2024-05-01T00%3A00%3A00Z"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to carry %s", query, want)
		}
	}

	// A time nobody can parse fails before the request, naming the flag.
	bad := newAirflowStub(t)
	_, _, err = runQuery(t, bad, "runs", "list", "--start-date-gte", "last tuesday")
	if err == nil || !strings.Contains(err.Error(), "--start-date-gte") {
		t.Fatalf("err = %v, want one naming the flag", err)
	}
	if bad.sawRequest(http.MethodGet, "/api/v2/dags/~/dagRuns") {
		t.Error("an unparseable bound still reached Airflow")
	}
}

func TestRunsTriggerSendsConfAndLogicalDate(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	stub.route(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns",
		`{"dag_id":"orders_etl","dag_run_id":"manual__x","state":"queued"}`)

	out, _, err := runQuery(t, stub, "runs", "trigger", "orders_etl",
		"--conf", `{"day":"monday"}`, "--logical-date", "2024-05-01T00:00:00Z")
	if err != nil {
		t.Fatalf("runs trigger: %v", err)
	}
	if !strings.Contains(out, "triggered orders_etl run manual__x (queued)") {
		t.Errorf("stdout = %q", out)
	}
	body := stub.request(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns").Body
	for _, want := range []string{`"day":"monday"`, "2024-05-01T00:00:00Z"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want it to carry %s", body, want)
		}
	}
}

func TestRunsTriggerRefusesBadFlagValuesBeforeAnyCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"conf", []string{"--conf", "{not json"}, "--conf is not valid JSON"},
		{"logical date", []string{"--logical-date", "yesterday"}, "RFC 3339"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := newAirflowStub(t)
			_, _, err := runQuery(t, stub, append([]string{"runs", "trigger", "orders_etl"}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one naming %s", err, tc.want)
			}
			if stub.sawRequest(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns") {
				t.Error("a bad flag value still reached Airflow")
			}
		})
	}
}

// Triggering a paused DAG is accepted by Airflow and then never scheduled, so
// the DAG is unpaused first — loudly, on stderr, and recorded in the result.
func TestRunsTriggerUnpausesAPausedDAG(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)
	stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	stub.route(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns",
		`{"dag_id":"orders_etl","dag_run_id":"manual__x","state":"queued"}`)

	out, errOut, err := runQuery(t, stub, "runs", "trigger", "orders_etl", "-o", "json")
	if err != nil {
		t.Fatalf("runs trigger: %v", err)
	}
	if !strings.Contains(errOut, "unpaused orders_etl") {
		t.Errorf("the unpause was not announced: %q", errOut)
	}
	if v := decodeJSON(t, out); v["unpaused"] != true {
		t.Errorf("json = %v, want the unpause recorded", v)
	}

	// --no-auto-unpause is the way to say "fail instead", and it changes nothing.
	stub2 := newAirflowStub(t)
	stub2.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)
	_, _, err = runQuery(t, stub2, "runs", "trigger", "orders_etl", "--no-auto-unpause")
	if err == nil || !strings.Contains(err.Error(), "is paused") {
		t.Fatalf("err = %v, want the paused refusal", err)
	}
	if stub2.sawRequest(http.MethodPatch, "/api/v2/dags/orders_etl") {
		t.Error("--no-auto-unpause still unpaused the DAG")
	}
}

// The unpause outlives a trigger that then fails, so a failed command must not
// leave the DAG quietly changed without saying so.
func TestRunsTriggerReportsTheUnpauseItLeavesBehind(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)
	stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	// The trigger itself is left unrouted, so it fails after the unpause landed.

	_, _, err := runQuery(t, stub, "runs", "trigger", "orders_etl")
	if err == nil {
		t.Fatal("the trigger must fail")
	}
	if !strings.Contains(err.Error(), "astro dags pause orders_etl") {
		t.Errorf("err = %q, want it to name the state it left and how to undo it", err)
	}
}

// Delete and clear are destructive, so a run that cannot be asked and did not
// pass --yes fails without touching the instance.
func TestRunsDeleteAndClearNeedConsent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		method string
		path   string
	}{
		{"delete", []string{"runs", "delete", "orders_etl", "run_1"}, http.MethodDelete, "/api/v2/dags/orders_etl/dagRuns/run_1"},
		{"clear", []string{"runs", "clear", "orders_etl", "run_1"}, http.MethodPost, "/api/v2/dags/orders_etl/dagRuns/run_1/clear"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := newAirflowStub(t)
			stub.route(tc.method, tc.path, `{"dag_run_id":"run_1","state":"queued"}`)

			_, _, err := runQuery(t, stub, tc.args...)
			if err == nil || !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("err = %v, want the refusal naming --yes", err)
			}
			if stub.sawRequest(tc.method, tc.path) {
				t.Fatal("the instance was changed without consent")
			}

			if _, _, err := runQuery(t, stub, append(tc.args, "--yes")...); err != nil {
				t.Fatalf("with --yes: %v", err)
			}
			if !stub.sawRequest(tc.method, tc.path) {
				t.Error("--yes did not carry the call through")
			}
		})
	}
}

// A dry run changes nothing, so it asks nothing — which also makes it the way
// to see what a clear would do from a script.
func TestRunsClearDryRunAsksNothing(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns/run_1/clear",
		`{"task_instances":[{"task_id":"load","dag_id":"orders_etl","dag_run_id":"run_1","state":"success"}],"total_entries":1}`)

	out, _, err := runQuery(t, stub, "runs", "clear", "orders_etl", "run_1", "--dry-run")
	if err != nil {
		t.Fatalf("runs clear --dry-run: %v", err)
	}
	if !strings.Contains(out, "would clear 1 task instance(s)") || !strings.Contains(out, "load") {
		t.Errorf("stdout = %q", out)
	}
	body := stub.request(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns/run_1/clear").Body
	if !strings.Contains(body, `"dry_run":true`) {
		t.Errorf("body = %q, want the dry run asked for", body)
	}
}
