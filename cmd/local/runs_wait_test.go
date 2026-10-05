package local

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/cmd/cliout"
)

// A run's life as each generation reports it, one body per check.
func runBody(state string) string {
	return `{"dag_id":"orders_etl","dag_run_id":"manual__1","state":"` + state + `","run_type":"manual",
		"start_date":"2024-05-01T00:00:05Z","end_date":null}`
}

// waitStub is an Airflow of the given generation with orders_etl unpaused and
// a trigger that answers queued, then answers checks with states in turn.
func waitStub(t *testing.T, af2 bool, states ...string) (stub *airflowStub, prefix string) {
	t.Helper()
	stub, prefix = newAirflowStub(t), "/api/v2"
	if af2 {
		stub, prefix = newAirflow2Stub(t), "/api/v1"
	}
	stub.route(http.MethodGet, prefix+"/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	stub.route(http.MethodPost, prefix+"/dags/orders_etl/dagRuns", runBody("queued"))
	bodies := make([]string, 0, len(states))
	for _, state := range states {
		bodies = append(bodies, runBody(state))
	}
	stub.routeSequence(http.MethodGet, prefix+"/dags/orders_etl/dagRuns/manual__1", bodies...)
	return stub, prefix
}

const failedInstances = `{"task_instances":[
	{"task_id":"extract","dag_id":"orders_etl","dag_run_id":"manual__1","state":"success","try_number":1,"map_index":-1},
	{"task_id":"transform","dag_id":"orders_etl","dag_run_id":"manual__1","state":"failed","try_number":2,"map_index":-1},
	{"task_id":"load","dag_id":"orders_etl","dag_run_id":"manual__1","state":"upstream_failed","try_number":0,"map_index":-1},
	{"task_id":"notify","dag_id":"orders_etl","dag_run_id":"manual__1","state":null,"try_number":0,"map_index":-1}
],"total_entries":4}`

// fast is the flag pair that keeps a wait test in milliseconds.
var fast = []string{"--poll-interval", "5ms", "--timeout", "5s"}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exit *cliout.ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	t.Fatalf("err = %v, want an exit status rather than a failure", err)
	return -1
}

func TestTriggerWaitReportsASuccessfulRunOnBothGenerations(t *testing.T) {
	for _, af2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "airflow 3", true: "airflow 2"}[af2], func(t *testing.T) {
			stub, prefix := waitStub(t, af2, "running", "running", "success")
			out, errOut, err := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl", "-o", "json"}, fast...)...)
			if code := exitCode(t, err); code != 0 {
				t.Fatalf("exit = %d, want 0 for a run that succeeded", code)
			}
			v := decodeJSON(t, out)
			if v["state"] != "success" || v["timed_out"] != false || v["dag_run_id"] != "manual__1" || v["unpaused"] != false {
				t.Errorf("json = %v", v)
			}
			if _, ok := v["elapsed_seconds"].(float64); !ok {
				t.Errorf("elapsed_seconds = %v, want a number", v["elapsed_seconds"])
			}
			if _, ok := v["failed_tasks"]; ok {
				t.Errorf("a successful run reported failed tasks: %v", v)
			}
			// A success has nothing to explain, so the instances are never read.
			if stub.sawRequest(http.MethodGet, prefix+"/dags/orders_etl/dagRuns/manual__1/taskInstances") {
				t.Error("a successful run still listed its task instances")
			}
			// The wait is visible as it goes, off stdout.
			if !strings.Contains(errOut, "manual__1: running") || !strings.Contains(errOut, "manual__1: success") {
				t.Errorf("stderr = %q, want the state changes", errOut)
			}
			// Every check read the run by the id the trigger answered with.
			if got := len(stub.requestsTo(http.MethodGet, prefix+"/dags/orders_etl/dagRuns/manual__1")); got != 3 {
				t.Errorf("checked the run %d times, want 3", got)
			}
		})
	}
}

// A failed run is reported with the instances that explain it — the part the
// agent skills read first — and exits 1.
func TestTriggerWaitReportsTheFailedTasksOfAFailedRun(t *testing.T) {
	for _, af2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "airflow 3", true: "airflow 2"}[af2], func(t *testing.T) {
			stub, prefix := waitStub(t, af2, "running", "failed")
			stub.route(http.MethodGet, prefix+"/dags/orders_etl/dagRuns/manual__1/taskInstances", failedInstances)

			out, _, err := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl", "-o", "json"}, fast...)...)
			if code := exitCode(t, err); code != exitRunFailed {
				t.Fatalf("exit = %d, want %d for a failed run", code, exitRunFailed)
			}
			v := decodeJSON(t, out)
			if v["state"] != "failed" || v["timed_out"] != false {
				t.Errorf("json = %v", v)
			}
			failed, _ := v["failed_tasks"].([]any)
			var ids []string
			for _, f := range failed {
				row, _ := f.(map[string]any)
				ids = append(ids, row["task_id"].(string)+"="+row["state"].(string))
			}
			if got := strings.Join(ids, ","); got != "transform=failed,load=upstream_failed" {
				t.Errorf("failed_tasks = %s, want the failed and upstream_failed instances only", got)
			}

			// The text says which task failed and how to read why.
			stub2, prefix2 := waitStub(t, af2, "failed")
			stub2.route(http.MethodGet, prefix2+"/dags/orders_etl/dagRuns/manual__1/taskInstances", failedInstances)
			out, _, err = runQuery(t, stub2, append([]string{"runs", "trigger-wait", "orders_etl"}, fast...)...)
			if code := exitCode(t, err); code != exitRunFailed {
				t.Fatalf("text exit = %d", code)
			}
			for _, want := range []string{
				"orders_etl run manual__1: failed", "transform", "upstream_failed",
				"astro af tasks logs orders_etl manual__1 transform --try 2 --url " + stub2.URL,
			} {
				if !strings.Contains(out, want) {
					t.Errorf("text is missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// A run still going when the time runs out is reported as it last stood, with
// the wait marked timed out and exit 2 — and it is left running.
func TestTriggerWaitTimesOutAndLeavesTheRunGoing(t *testing.T) {
	for _, af2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "airflow 3", true: "airflow 2"}[af2], func(t *testing.T) {
			stub, prefix := waitStub(t, af2, "running")
			started := time.Now()
			out, _, err := runQuery(t, stub, "runs", "trigger-wait", "orders_etl", "-o", "json",
				"--poll-interval", "10ms", "--timeout", "0.05")
			if code := exitCode(t, err); code != exitWaitTimeout {
				t.Fatalf("exit = %d, want %d for a wait that timed out", code, exitWaitTimeout)
			}
			if elapsed := time.Since(started); elapsed > 3*time.Second {
				t.Errorf("a 50ms timeout took %s", elapsed)
			}
			v := decodeJSON(t, out)
			if v["timed_out"] != true || v["state"] != "running" || v["dag_run_id"] != "manual__1" {
				t.Errorf("json = %v", v)
			}
			// Nothing stops the run, and nothing lists why it failed: it has not.
			for _, req := range stub.requests() {
				if req.Method != http.MethodGet && req.Path != prefix+"/dags/orders_etl/dagRuns" {
					t.Errorf("the wait changed something: %s %s", req.Method, req.Path)
				}
			}
			if stub.sawRequest(http.MethodGet, prefix+"/dags/orders_etl/dagRuns/manual__1/taskInstances") {
				t.Error("a timed-out wait listed task instances")
			}

			stub2, _ := waitStub(t, af2, "running")
			out, _, err = runQuery(t, stub2, "runs", "trigger-wait", "orders_etl", "--poll-interval", "10ms", "--timeout", "50ms")
			if code := exitCode(t, err); code != exitWaitTimeout {
				t.Fatalf("text exit = %d", code)
			}
			if !strings.Contains(out, "still running") || !strings.Contains(out, "astro af runs get orders_etl manual__1 --url "+stub2.URL) {
				t.Errorf("text = %q, want the run's state and how to check on it", out)
			}
		})
	}
}

// A run that finishes during the last, shortened sleep is reported finished,
// not timed out: the deadline gets one more check.
func TestTriggerWaitChecksOnceMoreAtTheDeadline(t *testing.T) {
	stub, _ := waitStub(t, false, "success")
	out, _, err := runQuery(t, stub, "runs", "trigger-wait", "orders_etl", "-o", "json",
		"--poll-interval", "1h", "--timeout", "20ms")
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if v := decodeJSON(t, out); v["timed_out"] != false || v["state"] != "success" {
		t.Errorf("json = %v", v)
	}
}

func TestTriggerWaitUnpausesAndRecordsIt(t *testing.T) {
	stub, _ := waitStub(t, false, "success")
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)
	stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	args := []string{"runs", "trigger-wait", "orders_etl", "-o", "json", "--conf", `{"day":"monday"}`, "--note", "smoke"}
	out, _, err := runQuery(t, stub, append(args, fast...)...)
	if code := exitCode(t, err); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if v := decodeJSON(t, out); v["unpaused"] != true {
		t.Errorf("json = %v, want the unpause recorded", v)
	}
	// The trigger flags are the trigger's: the same body `runs trigger` sends.
	body := stub.request(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns").Body
	for _, want := range []string{`"day":"monday"`, `"note":"smoke"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %s", body, want)
		}
	}
}

// A check that fails mid-wait is an error, and says how to look at the run it
// had already started.
func TestTriggerWaitFailsWhenACheckFails(t *testing.T) {
	stub, _ := waitStub(t, false)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1", http.StatusInternalServerError, `{"detail":"boom"}`)
	_, _, err := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl"}, fast...)...)
	if err == nil {
		t.Fatal("a failed check must fail the command")
	}
	var exit *cliout.ExitError
	if errors.As(err, &exit) {
		t.Fatalf("err = %v, want a failure rather than a run status", err)
	}
	if !strings.Contains(err.Error(), "astro af runs get orders_etl manual__1") {
		t.Errorf("err = %v, want it to say how to check on the run", err)
	}
}

func TestTriggerWaitRefusesBadWaitsBeforeAnyCall(t *testing.T) {
	for _, tc := range []struct{ flag, value string }{
		{"--timeout", "soon"},
		{"--timeout", "0"},
		{"--timeout", "-5"},
		{"--poll-interval", "0s"},
		{"--poll-interval", "NaN"},
		{"--timeout", "1e300"},
	} {
		t.Run(tc.flag+"="+tc.value, func(t *testing.T) {
			stub := newAirflowStub(t)
			_, _, err := runQuery(t, stub, "runs", "trigger-wait", "orders_etl", tc.flag, tc.value)
			if err == nil || !strings.Contains(err.Error(), tc.flag) {
				t.Fatalf("err = %v, want one naming %s", err, tc.flag)
			}
			if stub.sawRequest(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns") {
				t.Error("a bad wait still triggered a run")
			}
		})
	}
}

// The skills that drive this spell --timeout in bare seconds, which is what
// the CLI it came from took; a duration reads too.
func TestParseWaitTakesSecondsOrADuration(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"300":    300 * time.Second,
		"2.5":    2500 * time.Millisecond,
		"5m":     5 * time.Minute,
		"1h30m":  90 * time.Minute,
		"3600":   time.Hour,
		"250ms":  250 * time.Millisecond,
		"0.0015": 1500 * time.Microsecond,
	} {
		got, err := parseWait("--timeout", value)
		if err != nil || got != want {
			t.Errorf("parseWait(%q) = %v, %v; want %v", value, got, err, want)
		}
	}
}

// runFinished decides when the wait stops, so each state is pinned.
func TestRunFinishedStopsOnlyOnTheTerminalStates(t *testing.T) {
	for state, want := range map[string]bool{
		"success": true, "failed": true,
		"queued": false, "running": false, "": false,
	} {
		if got := runFinished(state); got != want {
			t.Errorf("runFinished(%q) = %v, want %v", state, got, want)
		}
	}
}

// The failed-task list pages: a failure on the second page of a big run must
// not read as a clean one.
func TestFailedTasksReadEveryPage(t *testing.T) {
	stub, _ := waitStub(t, false, "failed")
	stub.routeSequence(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances",
		`{"task_instances":[{"task_id":"a","state":"success","map_index":-1},{"task_id":"b","state":"success","map_index":-1}],"total_entries":3}`,
		`{"task_instances":[{"task_id":"c","state":"failed","try_number":1,"map_index":-1}],"total_entries":3}`)
	out, _, err := runQuery(t, stub, append([]string{"runs", "trigger-wait", "orders_etl", "-o", "json"}, fast...)...)
	if code := exitCode(t, err); code != exitRunFailed {
		t.Fatalf("exit = %d", code)
	}
	failed, _ := decodeJSON(t, out)["failed_tasks"].([]any)
	if len(failed) != 1 || failed[0].(map[string]any)["task_id"] != "c" {
		t.Errorf("failed_tasks = %v, want c from the second page", failed)
	}
	pages := stub.requestsTo(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances")
	if len(pages) != 2 || !strings.Contains(pages[1].Query, "offset=2") {
		t.Errorf("pages = %v, want a second page from offset 2", pages)
	}
}
