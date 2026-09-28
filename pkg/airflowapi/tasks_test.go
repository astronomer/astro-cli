package airflowapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

func TestListTaskInstancesWildcardsEmptyIDs(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/~/dagRuns/~/taskInstances",
		`{"task_instances":[{"task_id":"load","state":"failed"}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ListTaskInstances(t.Context(), "", "", ListOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.TaskInstances) != 1 || list.TaskInstances[0].State != "failed" {
		t.Fatalf("list = %+v, want the one instance", list)
	}
	if got := stub.lastRequest().Query.Get("limit"); got != "5" {
		t.Errorf("limit = %q, want 5", got)
	}
}

func TestGetTaskInstanceReadsOne(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/etl/dagRuns/r1/taskInstances/load",
		`{"task_id":"load","dag_id":"etl","try_number":2,"duration":1.5}`)
	client := stub.client()

	instance, err := client.GetTaskInstance(t.Context(), "etl", "r1", "load")
	if err != nil {
		t.Fatal(err)
	}
	if instance.TryNumber != 2 || instance.Duration != 1.5 {
		t.Errorf("instance = %+v, want the reported try and duration", instance)
	}
}

func TestGetMappedTaskInstanceAddressesOneExpansion(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1/taskInstances/greet/3",
		`{"task_id":"greet","dag_id":"etl","map_index":3,"state":"success"}`)
	client := stub.client()

	instance, err := client.GetMappedTaskInstance(t.Context(), "etl", "r1", "greet", 3)
	if err != nil {
		t.Fatal(err)
	}
	if instance.MapIndex != 3 || instance.State != "success" {
		t.Errorf("instance = %+v, want expansion 3", instance)
	}
}

// Airflow 2 answers a clear with references that carry no map_index. Read as
// 0, every one of them would look like the first expansion of a mapped task.
func TestTaskInstanceWithoutAMapIndexIsUnmapped(t *testing.T) {
	var list TaskInstanceList
	if err := json.Unmarshal([]byte(`{"task_instances":[{"task_id":"load"},{"task_id":"greet","map_index":0}]}`), &list); err != nil {
		t.Fatal(err)
	}
	if got := []int{list.TaskInstances[0].MapIndex, list.TaskInstances[1].MapIndex}; got[0] != -1 || got[1] != 0 {
		t.Errorf("map indexes = %v, want [-1 0]", got)
	}
}

func TestTaskLogsDefaultsToTheFirstTryAndTheWholeLog(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1/taskInstances/load/logs/1", `{"content":"line"}`)
	client := stub.client()

	if _, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{}); err != nil {
		t.Fatal(err)
	}
	got := stub.lastRequest()
	if got.Query.Get("full_content") != "true" {
		t.Errorf("full_content = %q, want the whole log by default", got.Query.Get("full_content"))
	}
	if _, ok := got.Query["map_index"]; ok {
		t.Error("map_index was sent for an unmapped task")
	}
}

func TestTaskLogsSendsTheTryAndMapIndex(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1/taskInstances/load/logs/3", `{"content":"line"}`)
	client := stub.client()

	mapIndex := 0
	_, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{
		TryNumber: 3,
		MapIndex:  &mapIndex,
		Tail:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stub.lastRequest()
	if got.Query.Get("map_index") != "0" {
		t.Errorf("map_index = %q, want index 0 to be sent", got.Query.Get("map_index"))
	}
	if got.Query.Get("full_content") != "false" {
		t.Errorf("full_content = %q, want the tail when Tail is set", got.Query.Get("full_content"))
	}
}

func TestTaskLogTextRendersBothShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"airflow 2 sends one string", `{"content":"line one\nline two"}`, "line one\nline two"},
		{"airflow 3 sends entries", `{"content":[{"event":"line one"},{"event":"line two"}]}`, "line one\nline two"},
		{"entries that are plain strings", `{"content":["line one","line two"]}`, "line one\nline two"},
		{"an entry with no event keeps its json", `{"content":[{"level":"info"}]}`, `{"level":"info"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newAF3Stub(t)
			stub.route(http.MethodGet, "/api/v2/dags/etl/dagRuns/r1/taskInstances/load/logs/1", tt.body)
			client := stub.client()

			log, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := log.Text(); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Airflow 2's JSON answer wraps the log in the Python repr of its
// (hostname, log) pairs, which is one line holding every real line as an
// escape. The endpoint has served text/plain since 2.0.0, so the client asks
// for that and the reader gets the log itself.
func TestTaskLogsAskAirflow2ForPlainText(t *testing.T) {
	const plain = "[2026-01-01T00:00:00+0000] INFO - Started\n[2026-01-01T00:00:01+0000] INFO - Done\n"
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/etl/dagRuns/r1/taskInstances/load/logs/1", plain)
	client := stub.client()

	log, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Header.Get("Accept"); got != textMediaType {
		t.Errorf("Accept = %q, want %q", got, textMediaType)
	}
	if log.Text() != plain {
		t.Errorf("Text() = %q, want the log as served", log.Text())
	}
}

// A door with nowhere to put a header cannot ask for text — MWAA under
// InvokeRestApi. The log still has to arrive, so the client asks again without
// the header and hands back the shape Airflow 2's JSON gives it.
func TestTaskLogsFallBackWhenTheDoorTakesNoHeaders(t *testing.T) {
	const repr = `[('worker-1', '[2026-01-01T00:00:00+0000] INFO - Started\n')]`
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/etl/dagRuns/r1/taskInstances/load/logs/1",
		`{"content":`+strconv.Quote(repr)+`,"continuation_token":"abc"}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatalf("build transport: %v", err)
	}
	client := New(headerlessTransport{transport})

	log, err := client.TaskLogs(t.Context(), "etl", "r1", "load", TaskLogsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if log.Text() != repr {
		t.Errorf("Text() = %q, want airflow 2's answer untouched", log.Text())
	}
	if log.ContinuationToken != "abc" {
		t.Errorf("continuation token = %q, want it kept", log.ContinuationToken)
	}
}

// headerlessTransport refuses a request carrying headers, the way MWAA's does.
type headerlessTransport struct{ Transport }

func (t headerlessTransport) Do(ctx context.Context, req Request) (Response, error) {
	if len(req.Header) > 0 {
		return Response{}, fmt.Errorf("%w: this door carries none", ErrHeadersUnsupported)
	}
	return t.Transport.Do(ctx, req)
}

func TestClearTaskInstancesSendsEveryFlagAsGiven(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodPost, "/api/v1/dags/etl/clearTaskInstances",
		`{"task_instances":[{"task_id":"load"}],"total_entries":1}`)
	client := stub.client()

	list, err := client.ClearTaskInstances(t.Context(), "etl", ClearTaskInstancesOptions{
		DAGRunID:     "r1",
		TaskIDs:      []string{"load"},
		DryRun:       true,
		ResetDAGRuns: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.TaskInstances) != 1 {
		t.Fatalf("list = %+v, want the cleared instance", list)
	}
	want := `{"dag_run_id":"r1","dry_run":true,"include_downstream":false,"include_upstream":false,"only_failed":false,"reset_dag_runs":true,"task_ids":["load"]}`
	if got := stub.lastRequest().Body; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestClearTaskInstancesReadsAMissingDAGAsMissing(t *testing.T) {
	// The path names a dag, so a 404 is that dag being absent — not an
	// Airflow without the endpoint.
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodPost, "/api/v2/dags/nope/clearTaskInstances",
		http.StatusNotFound, `{"detail":"Dag with id nope was not found"}`)
	client := stub.client()

	_, err := client.ClearTaskInstances(t.Context(), "nope", ClearTaskInstancesOptions{TaskIDs: []string{"load"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want it to read as not found", err)
	}
	if errors.Is(err, ErrNotServed) {
		t.Errorf("err = %v, want a missing dag not to read as a missing endpoint", err)
	}
}

func TestClearTaskInstancesReportsAnInstanceThatCannotDoIt(t *testing.T) {
	// An Airflow without the endpoint refuses the method, which does read as
	// not served.
	stub := newAF2Stub(t)
	stub.routeStatus(http.MethodPost, "/api/v1/dags/etl/clearTaskInstances",
		http.StatusMethodNotAllowed, `{"detail":"Method Not Allowed"}`)
	client := stub.client()

	_, err := client.ClearTaskInstances(t.Context(), "etl", ClearTaskInstancesOptions{TaskIDs: []string{"load"}})
	if !errors.Is(err, ErrNotServed) {
		t.Errorf("err = %v, want it to read as not served", err)
	}
}
