package local

import (
	"net/http"
	"reflect"
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

	if _, _, err := runQuery(t, stub, "runs", "list", "--dag-id", "orders_etl", "-s", "failed", "-s", "running", "-l", "5"); err != nil {
		t.Fatalf("runs list: %v", err)
	}
	query := stub.request(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns").Query
	for _, want := range []string{"state=failed", "state=running", "limit=5"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to carry %s", query, want)
		}
	}
}

// The DAG can be named as the argument, as `runs get` and `runs trigger` take
// it, or with --dag-id; naming two different ones is refused.
func TestRunsListTakesTheDAGAsAnArgument(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns", twoRunsAF3)

	out, _, err := runQuery(t, stub, "runs", "list", "orders_etl")
	if err != nil {
		t.Fatalf("runs list orders_etl: %v", err)
	}
	if !strings.Contains(out, "manual__2024-05-01") {
		t.Errorf("stdout = %q", out)
	}
	if _, _, err := runQuery(t, stub, "runs", "list", "orders_etl", "--dag-id", "orders_etl"); err != nil {
		t.Errorf("the argument and a matching --dag-id: %v", err)
	}

	before := len(stub.requests())
	_, _, err = runQuery(t, stub, "runs", "list", "orders_etl", "--dag-id", "billing")
	if err == nil || !strings.Contains(err.Error(), "different DAGs") {
		t.Fatalf("err = %v, want the two DAGs refused", err)
	}
	if len(stub.requests()) != before {
		t.Error("a request went out for a DAG the command could not settle on")
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

	// runQuery targets the stub with --url, so the undo has to carry --url too:
	// without it the suggested command resolves through the project's own rule
	// and would pause a DAG on a different Airflow entirely.
	_, _, err := runQuery(t, stub, "runs", "trigger", "orders_etl")
	if err == nil {
		t.Fatal("the trigger must fail")
	}
	want := "astro af dags pause orders_etl --url " + stub.URL
	if !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to name the state it left and how to undo it on this Airflow (%q)", err, want)
	}
}

// The undo names the Airflow that errored, whichever way the run named it —
// so a fix pasted from a failure lands where the failure did.
func TestRecoveryCommandsCarryTheSelector(t *testing.T) {
	pausedDAG := func(t *testing.T) *airflowStub {
		t.Helper()
		stub := newAirflowStub(t)
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)
		// The trigger is left unrouted, so it fails after the unpause landed.
		stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
		return stub
	}

	t.Run("--url carries through", func(t *testing.T) {
		stub := pausedDAG(t)
		_, _, err := runQuery(t, stub, "runs", "trigger", "orders_etl")
		if err == nil {
			t.Fatal("the trigger must fail")
		}
		if !strings.Contains(err.Error(), "--url "+stub.URL) {
			t.Errorf("err = %q, want the undo to carry --url", err)
		}
	})

	t.Run("-d carries through", func(t *testing.T) {
		stub := pausedDAG(t)
		dir := instanceProject(t, "\n[tool.astro.deployments.staging]\nurl = '"+stub.URL+"'\nauth = { method = 'none' }\n")
		d, _, _ := queryDeps(t)
		d.WorkingDir = func() (string, error) { return dir, nil }
		err := execute(t, d, "af", "runs", "trigger", "orders_etl", "-d", "staging")
		if err == nil {
			t.Fatal("the trigger must fail")
		}
		if !strings.Contains(err.Error(), "astro af dags pause orders_etl -d staging") {
			t.Errorf("err = %q, want the undo to carry -d staging", err)
		}
	})

	t.Run("the machine keeps its own spelling", func(t *testing.T) {
		stub := pausedDAG(t)
		_, _, err := runLocalQuery(t, stub, "local", "af", "runs", "trigger", "orders_etl")
		if err == nil {
			t.Fatal("the trigger must fail")
		}
		if !strings.Contains(err.Error(), "astro local af dags pause orders_etl") {
			t.Errorf("err = %q, want the undo spelled for this machine", err)
		}
		if strings.Contains(err.Error(), "--url") || strings.Contains(err.Error(), "-d ") {
			t.Errorf("err = %q, want no selector: the machine takes none", err)
		}
	})
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

// The view a failed run sends you to: which task, and how it ended. The dag and
// run columns are dropped because both are on the command line, and a run id is
// long enough to push the states off a terminal.
func TestRunsTasksListsWhatEachTaskDid(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances",
		`{"task_instances":[
			{"task_id":"extract","dag_id":"orders_etl","dag_run_id":"run_1","state":"success","try_number":1,"duration":2.5},
			{"task_id":"transform","dag_id":"orders_etl","dag_run_id":"run_1","state":"failed","try_number":1,"duration":0.068},
			{"task_id":"load","dag_id":"orders_etl","dag_run_id":"run_1","state":"upstream_failed","try_number":0}
		],"total_entries":3}`)

	out, _, err := runQuery(t, stub, "runs", "tasks", "orders_etl", "run_1")
	if err != nil {
		t.Fatalf("runs tasks: %v", err)
	}
	for _, want := range []string{"TASK_ID", "STATE", "transform", "failed", "upstream_failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "RUN_ID") {
		t.Errorf("table repeats the run id, which is already on the command line:\n%s", out)
	}
}

// The expansions of a mapped task are otherwise identical rows, so a run with
// one gains a MAP_INDEX column, and each task's expansions sit together in
// index order.
func TestRunsTasksTellsMappedExpansionsApart(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/greeter/dagRuns/run_1/taskInstances",
		`{"task_instances":[
			{"task_id":"get_names","state":"success","map_index":-1,"try_number":1},
			{"task_id":"greet","state":"success","map_index":2,"try_number":1},
			{"task_id":"say_bye","state":"success","map_index":-1,"try_number":1},
			{"task_id":"greet","state":"success","map_index":0,"try_number":1},
			{"task_id":"greet","state":"failed","map_index":1,"try_number":1}
		],"total_entries":5}`)

	out, _, err := runQuery(t, stub, "runs", "tasks", "greeter", "run_1")
	if err != nil {
		t.Fatalf("runs tasks: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var got [][]string
	for _, line := range lines {
		got = append(got, strings.Fields(line)[:3])
	}
	want := [][]string{
		{"TASK_ID", "MAP_INDEX", "STATE"},
		{"get_names", "-", "success"},
		{"greet", "0", "success"},
		{"greet", "1", "failed"},
		{"greet", "2", "success"},
		{"say_bye", "-", "success"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v\n%s", got, want, out)
	}

	out, _, err = runQuery(t, stub, "runs", "tasks", "greeter", "run_1", "--order-by", "-start_date")
	if err != nil {
		t.Fatalf("runs tasks --order-by: %v", err)
	}
	if first := strings.Fields(strings.Split(out, "\n")[2])[1]; first != "2" {
		t.Errorf("an explicit --order-by keeps Airflow's order, got second row index %q:\n%s", first, out)
	}
}

func TestRunsTasksKeepsItsColumnsWithoutAMappedTask(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances",
		`{"task_instances":[{"task_id":"extract","state":"success","map_index":-1,"try_number":1}],"total_entries":1}`)

	out, _, err := runQuery(t, stub, "runs", "tasks", "orders_etl", "run_1")
	if err != nil {
		t.Fatalf("runs tasks: %v", err)
	}
	if strings.Contains(out, "MAP_INDEX") {
		t.Errorf("a run with no mapped task has no MAP_INDEX column:\n%s", out)
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
