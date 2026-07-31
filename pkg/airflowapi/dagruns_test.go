package airflowapi

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestListDAGRunsWildcardsAnEmptyDAGID(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/~/dagRuns", `{"dag_runs":[{"dag_run_id":"r1","dag_id":"etl"}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ListDAGRuns(t.Context(), "", ListDAGRunsOptions{States: []string{"failed", "running"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.DAGRuns) != 1 || list.DAGRuns[0].DAGRunID != "r1" {
		t.Fatalf("list = %+v, want the one run", list)
	}
	if got := stub.lastRequest().Query["state"]; !reflect.DeepEqual(got, []string{"failed", "running"}) {
		t.Errorf("state = %v, want both states", got)
	}
}

// The start-date bounds are one of the few filters both generations spell the
// same way, so the same options reach the same parameters on either.
func TestListDAGRunsSendsStartDateBounds(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 31, 23, 59, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
	}{
		{"airflow 3", newAF3Stub, "/api/v2/dags/etl/dagRuns"},
		{"airflow 2", newAF2Stub, "/api/v1/dags/etl/dagRuns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, `{"dag_runs":[],"total_entries":0}`)
			client := stub.client()

			_, err := client.ListDAGRuns(t.Context(), "etl", ListDAGRunsOptions{StartDateFrom: from, StartDateTo: to})
			if err != nil {
				t.Fatal(err)
			}
			query := stub.lastRequest().Query
			if got := query.Get("start_date_gte"); got != "2026-07-01T00:00:00Z" {
				t.Errorf("start_date_gte = %q", got)
			}
			if got := query.Get("start_date_lte"); got != "2026-07-31T23:59:00Z" {
				t.Errorf("start_date_lte = %q", got)
			}
		})
	}
}

// Either end may stand alone, and an unset one is left out rather than sent as
// year one — which would filter out every run there is.
func TestListDAGRunsLeavesAnUnsetBoundOut(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/~/dagRuns", `{"dag_runs":[],"total_entries":0}`)
	client := stub.client()

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := client.ListDAGRuns(t.Context(), "", ListDAGRunsOptions{StartDateFrom: from}); err != nil {
		t.Fatal(err)
	}
	query := stub.lastRequest().Query
	if query.Get("start_date_gte") == "" {
		t.Error("the bound that was set was not sent")
	}
	if _, ok := query["start_date_lte"]; ok {
		t.Errorf("an unset bound was sent as %q", query.Get("start_date_lte"))
	}
}

func TestGetDAGRunReadsTheDates(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1",
		`{"dag_run_id":"r1","dag_id":"etl","state":"success","logical_date":"2026-07-30T10:00:00Z","end_date":null}`)
	client := stub.client()

	run, err := client.GetDAGRun(t.Context(), "etl", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if !run.LogicalDate.Equal(time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("logical date = %v, want the parsed one", run.LogicalDate)
	}
	if !run.EndDate.IsZero() {
		t.Errorf("end date = %v, want zero for a null", run.EndDate)
	}
}

func TestTriggerDAGRunNamesTheDateEachGenerationsWay(t *testing.T) {
	when := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
		date time.Time
		want string
	}{
		{
			name: "airflow 3 sends logical_date",
			stub: newAF3Stub, path: "/api/v2/dags/etl/dagRuns", date: when,
			want: `{"logical_date":"2026-07-30T10:00:00Z"}`,
		},
		{
			name: "airflow 3 sends a null logical_date when there is no date",
			stub: newAF3Stub, path: "/api/v2/dags/etl/dagRuns",
			want: `{"logical_date":null}`,
		},
		{
			name: "airflow 2 sends execution_date",
			stub: newAF2Stub, path: "/api/v1/dags/etl/dagRuns", date: when,
			want: `{"execution_date":"2026-07-30T10:00:00Z"}`,
		},
		{
			name: "airflow 2 sends nothing when there is no date",
			stub: newAF2Stub, path: "/api/v1/dags/etl/dagRuns",
			want: `{}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := tt.stub(t)
			stub.route(http.MethodPost, tt.path, `{"dag_run_id":"r1"}`)
			client := stub.client()

			run, err := client.TriggerDAGRun(t.Context(), "etl", TriggerDAGRunOptions{LogicalDate: tt.date})
			if err != nil {
				t.Fatal(err)
			}
			if run.DAGRunID != "r1" {
				t.Errorf("run = %+v, want the triggered run", run)
			}
			if got := stub.lastRequest().Body; got != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTriggerDAGRunCarriesTheRunIDConfAndNote(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns", `{"dag_run_id":"manual"}`)
	client := stub.client()

	_, err := client.TriggerDAGRun(t.Context(), "etl", TriggerDAGRunOptions{
		DAGRunID: "manual",
		Conf:     map[string]any{"region": "us"},
		Note:     "backfill",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"conf":{"region":"us"},"dag_run_id":"manual","logical_date":null,"note":"backfill"}`
	if got := stub.lastRequest().Body; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestDeleteDAGRunAcceptsAnEmptyAnswer(t *testing.T) {
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodDelete, "/api/v2/dags/etl/dagRuns/r1", http.StatusNoContent, "")
	client := stub.client()

	if err := client.DeleteDAGRun(t.Context(), "etl", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestClearDAGRunReadsBothAnswerShapes(t *testing.T) {
	t.Run("dry run lists the task instances", func(t *testing.T) {
		stub := newAF3Stub(t)
		stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns/r1/clear",
			`{"task_instances":[{"task_id":"load"}],"total_entries":1}`)
		client := stub.client()

		result, err := client.ClearDAGRun(t.Context(), "etl", "r1", ClearDAGRunOptions{DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.TaskInstances) != 1 || result.TaskInstances[0].TaskID != "load" {
			t.Fatalf("result = %+v, want the task instances", result)
		}
		if result.DAGRun != nil {
			t.Error("a dry run should not report a run")
		}
		if got := stub.lastRequest().Body; got != `{"dry_run":true}` {
			t.Errorf("body = %q, want the dry run flag", got)
		}
	})

	t.Run("a real clear returns the run", func(t *testing.T) {
		stub := newAF3Stub(t)
		stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns/r1/clear",
			`{"dag_run_id":"r1","dag_id":"etl","state":"queued"}`)
		client := stub.client()

		result, err := client.ClearDAGRun(t.Context(), "etl", "r1", ClearDAGRunOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if result.DAGRun == nil || result.DAGRun.State != "queued" {
			t.Fatalf("result = %+v, want the updated run", result)
		}
		if got := stub.lastRequest().Body; got != `{"dry_run":false}` {
			t.Errorf("body = %q, want the flag sent as given", got)
		}
	})
}
