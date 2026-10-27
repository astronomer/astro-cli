package local

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

const (
	runPath          = "/api/v2/dags/orders_etl/dagRuns/manual__1"
	reviewTaskPath   = "/api/v2/dags/orders_etl/tasks/review"
	loadTaskPath     = "/api/v2/dags/orders_etl/tasks/load"
	loadInstancePath = runPath + "/taskInstances/load"
)

// taskWaitStub is an Airflow 3 with orders_etl's run manual__1 already going,
// answering checks on it with states in turn, whose Dag defines review, a
// mapped task, and load.
func taskWaitStub(t *testing.T, states ...string) *airflowStub {
	t.Helper()
	stub := existingRunStub(t, states...)
	stub.route(http.MethodGet, reviewTaskPath, `{"task_id":"review","is_mapped":true}`)
	stub.route(http.MethodGet, loadTaskPath, `{"task_id":"load","is_mapped":false}`)
	return stub
}

func instanceBody(taskID string, mapIndex int, state string) string {
	s := `null`
	if state != "" {
		s = `"` + state + `"`
	}
	return `{"task_id":"` + taskID + `","dag_id":"orders_etl","dag_run_id":"manual__1","state":` + s +
		`,"map_index":` + strconv.Itoa(mapIndex) + `}`
}

// The case the command is for: act once a human-in-the-loop expansion is
// waiting for its answer.
func TestTasksWaitUntilAMappedTaskIsAwaitingInput(t *testing.T) {
	stub := taskWaitStub(t, "running")
	stub.routeSequence(runPath+"/taskInstances/review/2",
		instanceBody("review", 2, "scheduled"), instanceBody("review", 2, "running"), instanceBody("review", 2, "awaiting_input"))

	out, errOut, err := runQuery(t, stub, append([]string{
		"tasks", "wait", "orders_etl", "manual__1", "review",
		"--map-index", "2", "--until", "awaiting_input", "-o", "json",
	}, fast...)...)
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

// Without --until the wait is for success, and a task that settles in another
// state will never get there, so the wait ends at once rather than at the
// timeout.
func TestTasksWaitEndsWhenTheTaskSettlesElsewhere(t *testing.T) {
	stub := taskWaitStub(t, "running")
	stub.routeSequence(loadInstancePath, instanceBody("load", -1, "running"), instanceBody("load", -1, "failed"))

	out, _, err := runQuery(t, stub, "tasks", "wait", "orders_etl", "manual__1", "load", "--poll-interval", "5ms", "--timeout", "1h")
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if !strings.Contains(out, "load in orders_etl run manual__1 ended failed, so it will not reach success") {
		t.Errorf("text = %q", out)
	}
}

// An expansion not mapped yet answers 404; that is waited through, and a run
// that finishes without it ends the wait.
func TestTasksWaitWaitsThroughAMissingInstanceAndStopsWhenTheRunEnds(t *testing.T) {
	stub := taskWaitStub(t, "running", "success")
	out, _, err := runQuery(t, stub, "tasks", "wait", "orders_etl", "manual__1", "review", "-m", "5", "--until", "awaiting_input",
		"--poll-interval", "5ms", "--timeout", "1h")
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d, want %d", code, exitRunFailed)
	}
	if !strings.Contains(out, "orders_etl run manual__1 finished success with review[5] -, so it will not reach awaiting_input") {
		t.Errorf("text = %q", out)
	}
}

// A state is matched as Airflow spells it, however the flag spelled it.
func TestTasksWaitTimesOut(t *testing.T) {
	stub := taskWaitStub(t, "running")
	stub.route(http.MethodGet, runPath+"/taskInstances/review", instanceBody("review", -1, "deferred"))
	out, _, err := runQuery(t, stub, "tasks", "wait", "orders_etl", "manual__1", "review", "--until", "SUCCESS", "-o", "json",
		"--poll-interval", "5ms", "--timeout", "30ms")
	if code := exitCode(t, err); code != exitWaitTimeout {
		t.Fatalf("exit = %d, want %d", code, exitWaitTimeout)
	}
	if v := decodeJSON(t, out); v["timed_out"] != true || v["reached"] != false || v["state"] != "deferred" || v["until"] != "success" {
		t.Errorf("json = %v", v)
	}
}

// A task the Dag does not define would read as one not yet mapped and be
// waited for until the run ended; it is refused up front instead, with
// Airflow's own words.
func TestTasksWaitRefusesATaskTheDagDoesNotDefine(t *testing.T) {
	stub := existingRunStub(t, "running")
	_, _, err := runQuery(t, stub, append([]string{"tasks", "wait", "orders_etl", "manual__1", "approve"}, fast...)...)
	if !errors.Is(err, airflowapi.ErrNotFound) || !strings.Contains(err.Error(), "/tasks/approve") {
		t.Errorf("err = %v, want Airflow's not found for the task", err)
	}
	if stub.sawRequest(http.MethodGet, runPath+"/taskInstances/approve") {
		t.Error("the task instance was checked on before the task was known to exist")
	}
}

// Each poll reads the run before the task. The other way round, a task that
// reaches the state in the moment between the two reads is seen as it was,
// its run as finished, and the wait ends saying it will never get there.
func TestTasksWaitReadsTheRunBeforeTheTask(t *testing.T) {
	stub := taskWaitStub(t, "running", "success")
	runReads := func() int {
		n := 0
		for _, req := range stub.requests() {
			if req.Method == http.MethodGet && req.Path == runPath {
				n++
			}
		}
		return n
	}
	// load finishes at the same moment its run does: the second run read.
	stub.routeFunc(http.MethodGet, loadInstancePath, func(url.Values) string {
		if runReads() >= 2 {
			return instanceBody("load", -1, "success")
		}
		return instanceBody("load", -1, "running")
	})

	out, _, err := runQuery(t, stub, append([]string{"tasks", "wait", "orders_etl", "manual__1", "load", "-o", "json"}, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0 for a task that succeeded as its run finished\n%s", code, out)
	}
	if v := decodeJSON(t, out); v["reached"] != true || v["state"] != "success" || v["run_state"] != "success" {
		t.Errorf("json = %v", v)
	}
}
