package local

import (
	"net/http"
	"strings"
	"testing"
)

// existingRunStub is an Airflow 3 with orders_etl's run manual__1 already
// going, answering checks with states in turn.
func existingRunStub(t *testing.T, states ...string) *airflowStub {
	t.Helper()
	stub := newAirflowStub(t)
	bodies := make([]string, 0, len(states))
	for _, state := range states {
		bodies = append(bodies, runBody(state))
	}
	stub.routeSequence("/api/v2/dags/orders_etl/dagRuns/manual__1", bodies...)
	return stub
}

func instanceBody(taskID string, mapIndex int, state string) string {
	s := `null`
	if state != "" {
		s = `"` + state + `"`
	}
	return `{"task_id":"` + taskID + `","dag_id":"orders_etl","dag_run_id":"manual__1","state":` + s +
		`,"map_index":` + map[bool]string{true: "-1", false: "2"}[mapIndex < 0] + `}`
}

// Waiting on a run already going reports it as trigger-wait does, and starts
// nothing.
func TestRunsWaitWaitsOnAnExistingRun(t *testing.T) {
	stub := existingRunStub(t, "running", "running", "success")
	out, errOut, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1", "-o", "json"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 for a run that succeeded", code)
	}
	v := decodeJSON(t, out)
	if v["state"] != "success" || v["timed_out"] != false || v["unpaused"] != false || v["dag_run_id"] != "manual__1" {
		t.Errorf("json = %v", v)
	}
	if stub.sawRequest(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns") {
		t.Error("runs wait started a run")
	}
	if !strings.Contains(errOut, "manual__1 is running; waiting up to") || !strings.Contains(errOut, "manual__1: success") {
		t.Errorf("stderr = %q, want the wait and its state changes", errOut)
	}
}

func TestRunsWaitReportsAFailedRunWithItsFailedTasks(t *testing.T) {
	stub := existingRunStub(t, "running", "failed")
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances", failedInstances)
	out, _, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1", "-o", "json"}, fast...)...)
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if failed, _ := decodeJSON(t, out)["failed_tasks"].([]any); len(failed) != 2 {
		t.Errorf("failed_tasks = %v, want transform and load", failed)
	}
}

// The case --until is for: act once a human-in-the-loop expansion is waiting
// for its answer.
func TestRunsWaitUntilAMappedTaskIsAwaitingInput(t *testing.T) {
	stub := existingRunStub(t, "running")
	stub.routeSequence("/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances/review/2",
		instanceBody("review", 2, "scheduled"), instanceBody("review", 2, "running"), instanceBody("review", 2, "awaiting_input"))

	out, errOut, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1", "--until", "review:2=awaiting_input", "-o", "json"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 once the task reached the state", code)
	}
	v := decodeJSON(t, out)
	if v["task_id"] != "review" || v["map_index"] != float64(2) || v["state"] != "awaiting_input" ||
		v["until"] != "awaiting_input" || v["reached"] != true || v["timed_out"] != false || v["run_state"] != "running" {
		t.Errorf("json = %v", v)
	}
	for _, want := range []string{"review[2] in orders_etl run manual__1: scheduled", "review[2] in orders_etl run manual__1: awaiting_input"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr is missing %q:\n%s", want, errOut)
		}
	}
}

// A task that settles in another state will never reach the target, so the
// wait ends at once rather than at the timeout.
func TestRunsWaitUntilEndsWhenTheTaskSettlesElsewhere(t *testing.T) {
	stub := existingRunStub(t, "running")
	stub.routeSequence("/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances/load",
		instanceBody("load", -1, "running"), instanceBody("load", -1, "failed"))

	out, _, err := runQuery(t, stub, "runs", "wait", "orders_etl", "manual__1", "--until", "load=success", "--poll-interval", "5ms", "--timeout", "1h")
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if !strings.Contains(out, "load in orders_etl run manual__1 ended failed, so it will not reach success") {
		t.Errorf("text = %q", out)
	}
}

// An expansion not mapped yet answers 404; that is waited through, and a run
// that finishes without it ends the wait.
func TestRunsWaitUntilWaitsThroughAMissingInstanceAndStopsWhenTheRunEnds(t *testing.T) {
	stub := existingRunStub(t, "running", "success")
	out, _, err := runQuery(t, stub, "runs", "wait", "orders_etl", "manual__1", "--until", "review:5=awaiting_input", "--poll-interval", "5ms", "--timeout", "1h")
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if !strings.Contains(out, "orders_etl run manual__1 finished success with review[5] -, so it will not reach awaiting_input") {
		t.Errorf("text = %q", out)
	}
}

func TestRunsWaitUntilTimesOut(t *testing.T) {
	stub := existingRunStub(t, "running")
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances/review", instanceBody("review", -1, "deferred"))
	out, _, err := runQuery(t, stub, "runs", "wait", "orders_etl", "manual__1", "--until", "review=success", "-o", "json", "--poll-interval", "5ms", "--timeout", "30ms")
	if code := exitCode(t, err); code != exitWaitTimeout {
		t.Fatalf("exit = %d, want %d", code, exitWaitTimeout)
	}
	if v := decodeJSON(t, out); v["timed_out"] != true || v["reached"] != false || v["state"] != "deferred" {
		t.Errorf("json = %v", v)
	}
}

func TestParseUntil(t *testing.T) {
	for value, want := range map[string]untilTarget{
		"load=success":                   {taskID: "load", mapIndex: -1, state: "success"},
		"review:0=awaiting_input":        {taskID: "review", mapIndex: 0, state: "awaiting_input"},
		"group.review:12=AWAITING_INPUT": {taskID: "group.review", mapIndex: 12, state: "awaiting_input"},
	} {
		got, err := parseUntil(value)
		if err != nil || got != want {
			t.Errorf("parseUntil(%q) = %+v, %v; want %+v", value, got, err, want)
		}
	}
	for _, value := range []string{"load", "load=", "=success", "review:x=success", "review:-1=success", ":1=success"} {
		if _, err := parseUntil(value); err == nil || !strings.Contains(err.Error(), "--until must be") {
			t.Errorf("parseUntil(%q) err = %v, want the format explained", value, err)
		}
	}
}

// A run that had already finished is reported without a wait that never
// happened.
func TestRunsWaitOnAFinishedRunReportsNoWait(t *testing.T) {
	stub := existingRunStub(t, "success")
	out, errOut, err := runQuery(t, stub, append([]string{"runs", "wait", "orders_etl", "manual__1"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if out != "orders_etl run manual__1: success\n" {
		t.Errorf("text = %q", out)
	}
	if strings.Contains(errOut, "waiting up to") {
		t.Errorf("stderr = %q, want no wait announced for a finished run", errOut)
	}
}
