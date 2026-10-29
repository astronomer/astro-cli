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
