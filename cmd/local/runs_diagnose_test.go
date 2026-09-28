package local

import (
	"net/http"
	"strings"
	"testing"
)

func TestRunsDiagnoseSummarizesAFailedRunOnBothGenerations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stub   func(*testing.T) *airflowStub
		prefix string
	}{
		{"airflow 3", newAirflowStub, "/api/v2"},
		{"airflow 2", newAirflow2Stub, "/api/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.prefix+"/dags/orders_etl/dagRuns/manual__1", runBody("failed"))
			stub.route(http.MethodGet, tc.prefix+"/dags/orders_etl/dagRuns/manual__1/taskInstances", failedInstances)

			out, _, err := runQuery(t, stub, "runs", "diagnose", "orders_etl", "manual__1", "-o", "json")
			if err != nil {
				t.Fatalf("runs diagnose: %v", err)
			}
			checkFailedDiagnosis(t, decodeJSON(t, out))

			out, _, err = runQuery(t, stub, "runs", "diagnose", "orders_etl", "manual__1")
			if err != nil {
				t.Fatalf("runs diagnose (text): %v", err)
			}
			for _, want := range []string{
				"state:", "failed", "tasks: 4 (failed=1 none=1 success=1 upstream_failed=1)",
				"failed tasks:", "transform", "tasks logs orders_etl manual__1 transform --try 2",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("text is missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// checkFailedDiagnosis holds the json of `runs diagnose` over failedInstances:
// the run, all four instances, the counts, and the two that explain it.
func checkFailedDiagnosis(t *testing.T, v map[string]any) {
	t.Helper()
	if run, _ := v["run"].(map[string]any); run["state"] != "failed" || run["dag_run_id"] != "manual__1" {
		t.Errorf("run = %v", v["run"])
	}
	if instances, _ := v["task_instances"].([]any); len(instances) != 4 {
		t.Errorf("task_instances = %v, want all four", v["task_instances"])
	}
	summary, _ := v["summary"].(map[string]any)
	if summary["total_tasks"] != float64(4) {
		t.Errorf("total_tasks = %v", summary["total_tasks"])
	}
	counts, _ := summary["state_counts"].(map[string]any)
	for state, want := range map[string]float64{"success": 1, "failed": 1, "upstream_failed": 1, noState: 1} {
		if counts[state] != want {
			t.Errorf("state_counts[%s] = %v, want %v (all %v)", state, counts[state], want, counts)
		}
	}
	failed, _ := summary["failed_tasks"].([]any)
	if len(failed) != 2 || failed[0].(map[string]any)["task_id"] != "transform" || failed[1].(map[string]any)["task_id"] != "load" {
		t.Errorf("failed_tasks = %v, want transform then load", failed)
	}
}

// A clean run reports an empty failure list, not a missing one: "none failed"
// is the answer, and a consumer should not have to infer it from an absent key.
func TestRunsDiagnoseOfACleanRunHasAnEmptyFailureList(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1", runBody("success"))
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances",
		`{"task_instances":[{"task_id":"extract","state":"success","map_index":-1}],"total_entries":1}`)
	out, _, err := runQuery(t, stub, "runs", "diagnose", "orders_etl", "manual__1", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"failed_tasks":[]`) {
		t.Errorf("json = %s, want failed_tasks present and empty", out)
	}
}

// The run is what the diagnosis rests on, so a missing run fails; a task
// listing that fails is reported beside the run instead.
func TestRunsDiagnoseFailsWithoutTheRunButNotWithoutItsTasks(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runQuery(t, stub, "runs", "diagnose", "orders_etl", "nope", "-o", "json")
	if err == nil {
		t.Fatal("a run Airflow does not know must fail the command")
	}

	stub2 := newAirflowStub(t)
	stub2.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1", runBody("failed"))
	stub2.routeStatus(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances", http.StatusForbidden, `{"detail":"no"}`)
	out, _, err := runQuery(t, stub2, "runs", "diagnose", "orders_etl", "manual__1", "-o", "json")
	if err != nil {
		t.Fatalf("a failed task listing must not fail the diagnosis: %v", err)
	}
	v := decodeJSON(t, out)
	if v["task_instances_error"] == nil || v["run"] == nil {
		t.Errorf("json = %v, want the run and the reason its tasks are missing", v)
	}
}
