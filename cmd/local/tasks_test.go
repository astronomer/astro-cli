package local

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestTasksListAndGet(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/tasks", `{"tasks":[
		{"task_id":"extract","operator_name":"PythonOperator","owner":"data","trigger_rule":"all_success",
		 "downstream_task_ids":["load"]},
		{"task_id":"load","operator_name":"PythonOperator","owner":"data"}
	],"total_entries":2}`)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/tasks/extract",
		`{"task_id":"extract","operator_name":"PythonOperator","owner":"data","retries":2,"downstream_task_ids":["load"]}`)

	out, _, err := runQuery(t, stub, "tasks", "list", "orders_etl")
	if err != nil {
		t.Fatalf("tasks list: %v", err)
	}
	for _, want := range []string{"TASK_ID", "extract", "PythonOperator", "load"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}

	out, _, err = runQuery(t, stub, "tasks", "get", "orders_etl", "extract", "-o", "json")
	if err != nil {
		t.Fatalf("tasks get: %v", err)
	}
	v := decodeJSON(t, out)
	if v["task_id"] != "extract" || v["operator_name"] != "PythonOperator" || v["retries"] != float64(2) {
		t.Errorf("json = %v", v)
	}
}

func TestTasksInstanceShowsWhatOneRunDid(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances/load",
		`{"task_id":"load","dag_id":"orders_etl","dag_run_id":"run_1","state":"failed","try_number":2,
		  "max_tries":2,"duration":95.0,"start_date":"2024-05-01T00:00:00Z"}`)

	out, _, err := runQuery(t, stub, "tasks", "instance", "orders_etl", "run_1", "load")
	if err != nil {
		t.Fatalf("tasks instance: %v", err)
	}
	for _, want := range []string{"state:", "failed", "try:", "2 of 3", "duration:", "1m"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
}

// A mapped task has no single instance, so Airflow only answers for one
// expansion, addressed by its index in the path.
func TestTasksInstanceReadsOneExpansionOfAMappedTask(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/greeter/dagRuns/run_1/taskInstances/greet/2",
		`{"task_id":"greet","dag_id":"greeter","dag_run_id":"run_1","state":"success","map_index":2,"try_number":1}`)

	out, _, err := runQuery(t, stub, "tasks", "instance", "greeter", "run_1", "greet", "--map-index", "2")
	if err != nil {
		t.Fatalf("tasks instance --map-index: %v", err)
	}
	if !regexp.MustCompile(`(?m)^\s*map index:\s+2$`).MatchString(out) {
		t.Errorf("detail is missing the map index:\n%s", out)
	}
}

// Airflow 2's clear answer carries no map_index, which must not read as a
// mapped task.
func TestTasksClearOnAirflow2HasNoMapIndexColumn(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodPost, "/api/v1/dags/orders_etl/clearTaskInstances",
		`{"task_instances":[{"task_id":"load","dag_id":"orders_etl","dag_run_id":"run_1"}]}`)

	out, _, err := runQuery(t, stub, "tasks", "clear", "orders_etl", "run_1", "load", "--dry-run")
	if err != nil {
		t.Fatalf("tasks clear --dry-run: %v", err)
	}
	if strings.Contains(out, "MAP_INDEX") {
		t.Errorf("an unmapped clear has no MAP_INDEX column:\n%s", out)
	}
}

// The two generations answer a log request in their own shape — Airflow 3 with
// structured entries, Airflow 2 with the plain text the client asks it for —
// and both reach the reader as lines.
func TestTasksLogsRenderBothGenerations(t *testing.T) {
	af3 := newAirflowStub(t)
	af3.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances/load/logs/1",
		`{"content":[{"event":"first line"},{"event":"second line"}]}`)
	out, _, err := runQuery(t, af3, "tasks", "logs", "orders_etl", "run_1", "load")
	if err != nil {
		t.Fatalf("tasks logs on airflow 3: %v", err)
	}
	if out != "first line\nsecond line\n" {
		t.Errorf("stdout = %q", out)
	}

	af2 := newAirflow2Stub(t)
	af2.route(http.MethodGet, "/api/v1/dags/orders_etl/dagRuns/run_1/taskInstances/load/logs/1",
		"a plain airflow 2 log")
	out, _, err = runQuery(t, af2, "tasks", "logs", "orders_etl", "run_1", "load")
	if err != nil {
		t.Fatalf("tasks logs on airflow 2: %v", err)
	}
	if out != "a plain airflow 2 log\n" {
		t.Errorf("stdout = %q", out)
	}
}

// The whole log, always. Airflow's full_content=false is an incremental-read
// mode meant to be driven with a continuation token, not a tail — the default
// file handler ignores it and some handlers return the head — so there is no
// flag for it and the request always asks for everything.
func TestTasksLogsAlwaysFetchTheWholeLog(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances/load/logs/1",
		`{"content":"whole log"}`)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances/load/logs/3",
		`{"content":"third try"}`)

	const logPath = "/api/v2/dags/orders_etl/dagRuns/run_1/taskInstances/load/logs/1"
	if _, _, err := runQuery(t, stub, "tasks", "logs", "orders_etl", "run_1", "load"); err != nil {
		t.Fatalf("tasks logs: %v", err)
	}
	if got := stub.request(http.MethodGet, logPath).Query; !strings.Contains(got, "full_content=true") {
		t.Errorf("query = %q, want the whole log by default", got)
	}
	// An unmapped task sends no map index at all: index 0 is a real expansion,
	// so the flag's -1 has to mean "leave it out" rather than "send zero".
	if got := stub.request(http.MethodGet, logPath).Query; strings.Contains(got, "map_index") {
		t.Errorf("query = %q, want no map index for an unmapped task", got)
	}

	mapped := newAirflowStub(t)
	mapped.route(http.MethodGet, logPath, `{"content":"one expansion"}`)
	if _, _, err := runQuery(t, mapped, "tasks", "logs", "orders_etl", "run_1", "load", "-m", "0"); err != nil {
		t.Fatalf("tasks logs -m 0: %v", err)
	}
	if got := mapped.request(http.MethodGet, logPath).Query; !strings.Contains(got, "map_index=0") {
		t.Errorf("query = %q, want the expansion that was asked for", got)
	}

	out, _, err := runQuery(t, stub, "tasks", "logs", "orders_etl", "run_1", "load", "--try", "3")
	if err != nil {
		t.Fatalf("tasks logs --try 3: %v", err)
	}
	if !strings.Contains(out, "third try") {
		t.Errorf("stdout = %q, want the try that was asked for", out)
	}

	// Attempts count from 1, so a zero or negative try is refused rather than
	// quietly reading attempt 1 and reporting the number that was typed.
	if _, _, err := runQuery(t, stub, "tasks", "logs", "orders_etl", "run_1", "load", "--try", "0"); err == nil {
		t.Error("--try 0 must fail")
	}
}

// An empty id widens a request rather than narrowing it: a clear with no
// dag_run_id clears the task in every run of the DAG. A shell variable that
// failed to fill passes exactly that, so every leaf refuses it by name before
// anything is sent.
func TestAnEmptyArgumentIsRefusedBeforeAnyRequest(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"tasks", "clear", "orders_etl", "", "load", "--dry-run"}, "RUN_ID is empty"},
		{[]string{"tasks", "clear", "orders_etl", "run_1", "load", " ", "--yes"}, "TASK_ID is empty"},
		{[]string{"runs", "get", "", "run_1"}, "DAG_ID is empty"},
		{[]string{"runs", "list", ""}, "DAG_ID is empty"},
	} {
		stub := newAirflowStub(t)
		_, _, err := runQuery(t, stub, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
		if got := stub.requests(); len(got) != 0 {
			t.Errorf("%v: requests went out: %v", tc.args, got)
		}
	}
}

// A clear that leaves the run in a terminal state never re-runs anything, so
// the run is reset by default and the flag is how you decline.
func TestTasksClearResetsTheRunAndNeedsConsent(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/dags/orders_etl/clearTaskInstances",
		`{"task_instances":[{"task_id":"load","dag_id":"orders_etl","dag_run_id":"run_1","state":"failed"}],"total_entries":1}`)

	_, _, err := runQuery(t, stub, "tasks", "clear", "orders_etl", "run_1", "load")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("err = %v, want the refusal naming --yes", err)
	}
	if stub.sawRequest(http.MethodPost, "/api/v2/dags/orders_etl/clearTaskInstances") {
		t.Fatal("tasks were cleared without consent")
	}

	out, _, err := runQuery(t, stub, "tasks", "clear", "orders_etl", "run_1", "load", "--yes")
	if err != nil {
		t.Fatalf("tasks clear --yes: %v", err)
	}
	if !strings.Contains(out, "cleared 1 task instance(s)") {
		t.Errorf("stdout = %q", out)
	}
	body := stub.request(http.MethodPost, "/api/v2/dags/orders_etl/clearTaskInstances").Body
	if !strings.Contains(body, `"reset_dag_runs":true`) {
		t.Errorf("body = %q, want the run reset so the scheduler picks the tasks up", body)
	}
	if !strings.Contains(body, `"load"`) || !strings.Contains(body, `"run_1"`) {
		t.Errorf("body = %q, want the run and the task named", body)
	}

	// A dry run changes nothing, so it asks nothing.
	out, _, err = runQuery(t, stub, "tasks", "clear", "orders_etl", "run_1", "load", "--dry-run")
	if err != nil {
		t.Fatalf("tasks clear --dry-run: %v", err)
	}
	if !strings.Contains(out, "would clear 1 task instance(s)") {
		t.Errorf("stdout = %q", out)
	}
}
