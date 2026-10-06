package airflowapi

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestListDAGsReadsBothGenerations(t *testing.T) {
	tests := []struct {
		name  string
		stub  func(*testing.T) *airflowStub
		path  string
		state string
	}{
		{"airflow 3", newAF3Stub, "/api/v2/dags", `{"dags":[{"dag_id":"etl","is_paused":false,"timetable_summary":"@daily"}],"total_entries":1}`},
		{"airflow 2", newAF2Stub, "/api/v1/dags", `{"dags":[{"dag_id":"etl","is_paused":false,"timetable_description":"Once a day"}],"total_entries":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := tt.stub(t)
			stub.route(http.MethodGet, tt.path, tt.state)
			client := stub.client()

			list, err := client.ListDAGs(t.Context(), ListDAGsOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if list.TotalEntries != 1 || len(list.DAGs) != 1 || list.DAGs[0].DAGID != "etl" {
				t.Fatalf("list = %+v, want the one dag", list)
			}
		})
	}
}

func TestListDAGsSendsItsFilters(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{}`)
	client := stub.client()
	paused := true

	_, err := client.ListDAGs(t.Context(), ListDAGsOptions{
		ListOptions:  ListOptions{Limit: 10, Offset: 20, OrderBy: "dag_id"},
		Tags:         []string{"core", "hourly"},
		DAGIDPattern: "etl",
		Paused:       &paused,
	})
	if err != nil {
		t.Fatal(err)
	}
	query := stub.lastRequest().Query
	if got := query["tags"]; !reflect.DeepEqual(got, []string{"core", "hourly"}) {
		t.Errorf("tags = %v, want both", got)
	}
	if query.Get("dag_id_pattern") != "etl" || query.Get("paused") != "true" {
		t.Errorf("query = %v, want the filters set", query)
	}
	if query.Get("limit") != "10" || query.Get("offset") != "20" || query.Get("order_by") != "dag_id" {
		t.Errorf("query = %v, want the pagination set", query)
	}
}

func TestGetDAGSourceUsesTheFileTokenOnAirflow2(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/etl", `{"dag_id":"etl","file_token":"tok123"}`)
	stub.route(http.MethodGet, "/api/v1/dagSources/tok123", `{"content":"print('hi')"}`)
	client := stub.client()

	source, err := client.GetDAGSource(t.Context(), "etl")
	if err != nil {
		t.Fatal(err)
	}
	if source.Content != "print('hi')" {
		t.Errorf("content = %q, want the dag file", source.Content)
	}
	if source.DAGID != "etl" {
		t.Errorf("dag id = %q, want etl even though airflow 2 omits it", source.DAGID)
	}
}

func TestGetDAGSourceUsesTheDAGIDOnAirflow3(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dagSources/etl", `{"dag_id":"etl","content":"print('hi')"}`)
	client := stub.client()

	source, err := client.GetDAGSource(t.Context(), "etl")
	if err != nil {
		t.Fatal(err)
	}
	if source.Content != "print('hi')" {
		t.Errorf("content = %q, want the dag file", source.Content)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/dags/etl") != 0 {
		t.Error("airflow 3 needs no extra call for a file token")
	}
}

func TestGetDAGSourceSaysWhenAirflow2HasNoFileToken(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/etl", `{"dag_id":"etl"}`)
	client := stub.client()

	_, err := client.GetDAGSource(t.Context(), "etl")
	if err == nil || !strings.Contains(err.Error(), "file token") {
		t.Errorf("err = %v, want it to name the missing file token", err)
	}
}

func TestDAGStatsListsTheDAGsFirstOnAirflow2(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags", `{"dags":[{"dag_id":"etl"},{"dag_id":"load"}],"total_entries":2}`)
	stub.route(http.MethodGet, "/api/v1/dagStats", `{"dags":[{"dag_id":"etl","stats":[{"state":"success","count":3}]}],"total_entries":1}`)
	client := stub.client()

	stats, err := client.DAGStats(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.DAGs) != 1 || stats.DAGs[0].Stats[0].Count != 3 {
		t.Fatalf("stats = %+v, want the counts", stats)
	}
	if got := stub.lastRequest().Query.Get("dag_ids"); got != "etl,load" {
		t.Errorf("dag_ids = %q, want every dag listed", got)
	}
}

func TestDAGStatsReturnsNothingWhenAirflow2HasNoDAGs(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags", `{"dags":[],"total_entries":0}`)
	client := stub.client()

	stats, err := client.DAGStats(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.DAGs) != 0 {
		t.Errorf("stats = %+v, want nothing", stats)
	}
	if stub.countRequests(http.MethodGet, "/api/v1/dagStats") != 0 {
		t.Error("asked for stats with no dag ids, which airflow 2 refuses")
	}
}

// Airflow 3 reads dag_ids as an optional array, so "all DAGs" is the parameter
// left off. Sending it empty asks for the DAG whose id is "" and answers with
// nothing every time — which is silent, because an Airflow with no runs answers
// the same way. The stub returns real counts here so the empty answer cannot
// pass for the true one.
func TestDAGStatsAsksAirflow3ForAllDAGsWithNoFilterAtAll(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[{"dag_id":"etl","dag_display_name":"ETL","stats":[{"state":"success","count":4}]}],"total_entries":1}`)
	client := stub.client()

	stats, err := client.DAGStats(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, sent := stub.lastRequest().Query["dag_ids"]; sent {
		t.Errorf("dag_ids = %v, want it absent: an empty filter selects no DAG", stub.lastRequest().Query["dag_ids"])
	}
	if len(stats.DAGs) != 1 || stats.DAGs[0].Stats[0].Count != 4 || stats.DAGs[0].DAGDisplayName != "ETL" {
		t.Errorf("stats = %+v, want the counts and the display name", stats)
	}
}

func TestDAGStatsAsksAirflow3OneDAGAtATime(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[{"dag_id":"etl","stats":[{"state":"failed","count":1}]}],"total_entries":1}`)
	client := stub.client()

	stats, err := client.DAGStats(t.Context(), []string{"etl", "load"})
	if err != nil {
		t.Fatal(err)
	}
	if got := stub.countRequests(http.MethodGet, "/api/v2/dagStats"); got != 2 {
		t.Errorf("made %d calls, want one per dag", got)
	}
	if len(stats.DAGs) != 2 || stats.TotalEntries != 2 {
		t.Errorf("stats = %+v, want the answers joined", stats)
	}
}

func TestListTasksAndGetTaskReadDefinitions(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/tasks",
		`{"tasks":[{"task_id":"load","operator_name":"PythonOperator","downstream_task_ids":["report"]}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v2/dags/etl/tasks/load",
		`{"task_id":"load","operator_name":"PythonOperator","trigger_rule":"all_success"}`)
	client := stub.client()

	list, err := client.ListTasks(t.Context(), "etl")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tasks) != 1 || list.Tasks[0].Operator != "PythonOperator" {
		t.Fatalf("list = %+v, want the task definition", list)
	}
	if len(list.Tasks[0].DownstreamTasks) != 1 {
		t.Errorf("downstream = %v, want the dependency", list.Tasks[0].DownstreamTasks)
	}

	task, err := client.GetTask(t.Context(), "etl", "load")
	if err != nil {
		t.Fatal(err)
	}
	if task.TriggerRule != "all_success" {
		t.Errorf("task = %+v, want its trigger rule", task)
	}
}

func TestListImportErrorsAndWarnings(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/importErrors",
		`{"import_errors":[{"import_error_id":1,"filename":"dags/etl.py","stack_trace":"SyntaxError"}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v1/dagWarnings",
		`{"dag_warnings":[{"dag_id":"etl","warning_type":"non-existent pool","message":"Dag uses pool x"}],"total_entries":1}`)
	client := stub.client()

	errs, err := client.ListImportErrors(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(errs.ImportErrors) != 1 || errs.ImportErrors[0].Filename != "dags/etl.py" {
		t.Fatalf("import errors = %+v, want the failing file", errs)
	}

	warnings, err := client.ListDAGWarnings(t.Context(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings.DAGWarnings) != 1 || warnings.DAGWarnings[0].DAGID != "etl" {
		t.Fatalf("warnings = %+v, want the warning", warnings)
	}
}

func TestListsReadAMissingEndpointAsNotServed(t *testing.T) {
	// A list endpoint's path names nothing that could be missing, so a 404
	// is the endpoint being absent.
	stub := newAF2Stub(t)
	client := stub.client()

	for _, call := range []struct {
		name string
		err  func() error
	}{
		{"import errors", func() error { _, err := client.ListImportErrors(t.Context(), ListOptions{}); return err }},
		{"dag warnings", func() error { _, err := client.ListDAGWarnings(t.Context(), ListOptions{}); return err }},
		{"connections", func() error { _, err := client.ListConnections(t.Context(), ListOptions{}); return err }},
		{"variables", func() error { _, err := client.ListVariables(t.Context(), ListOptions{}); return err }},
		{"pools", func() error { _, err := client.ListPools(t.Context(), ListOptions{}); return err }},
		{"dags", func() error { _, err := client.ListDAGs(t.Context(), ListDAGsOptions{}); return err }},
	} {
		if err := call.err(); !errors.Is(err, ErrNotServed) {
			t.Errorf("%s: err = %v, want it to read as not served", call.name, err)
		}
	}
}

func TestPauseAndUnpauseSendTheFlag(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client) error
		want string
	}{
		{"pause", func(c *Client) error { _, err := c.PauseDAG(t.Context(), "etl"); return err }, `{"is_paused":true}`},
		{"unpause", func(c *Client) error { _, err := c.UnpauseDAG(t.Context(), "etl"); return err }, `{"is_paused":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newAF3Stub(t)
			stub.route(http.MethodPatch, "/api/v2/dags/etl", `{"dag_id":"etl"}`)
			if err := tt.call(stub.client()); err != nil {
				t.Fatal(err)
			}
			if got := stub.lastRequest().Body; got != tt.want {
				t.Errorf("body = %q, want %q", got, tt.want)
			}
		})
	}
}
