package local

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const xcomPath = "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances/review/xcomEntries/return_value"

func TestTasksXComPrintsTheValueAlone(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, xcomPath,
		`{"key":"return_value","task_id":"review","dag_id":"orders_etl","run_id":"manual__1","map_index":2,
		"timestamp":"2024-05-01T00:00:05Z","value":{"chosen_options":["Pass"],"rows":9007199254740993}}`)

	out, _, err := runQuery(t, stub, "tasks", "xcom", "orders_etl", "manual__1", "review", "--map-index", "2")
	if err != nil {
		t.Fatal(err)
	}
	// Indented JSON a pipe can read, with the big integer's digits intact.
	want := "{\n  \"chosen_options\": [\n    \"Pass\"\n  ],\n  \"rows\": 9007199254740993\n}\n"
	if out != want {
		t.Errorf("text = %q, want %q", out, want)
	}
	query, _ := url.ParseQuery(stub.request(http.MethodGet, xcomPath).Query)
	if query.Get("map_index") != "2" || query.Get("deserialize") != "true" {
		t.Errorf("query = %v, want expansion 2, deserialized", query)
	}
}

func TestTasksXComPrintsAStringWithoutQuotes(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances/load/xcomEntries/rows_loaded",
		`{"key":"rows_loaded","task_id":"load","dag_id":"orders_etl","value":"42 rows"}`)

	out, _, err := runQuery(t, stub, "tasks", "xcom", "orders_etl", "manual__1", "load", "--key", "rows_loaded")
	if err != nil {
		t.Fatal(err)
	}
	if out != "42 rows\n" {
		t.Errorf("text = %q", out)
	}
}

func TestTasksXComJSONCarriesTheIDs(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags/orders_etl/dagRuns/manual__1/taskInstances/load/xcomEntries/return_value",
		`{"key":"return_value","task_id":"load","dag_id":"orders_etl","value":"[1, 2]"}`)

	out, _, err := runQuery(t, stub, "tasks", "xcom", "orders_etl", "manual__1", "load", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	v := decodeJSON(t, out)
	// Airflow 2 does not echo the run id; it comes from the command line.
	if v["dag_run_id"] != "manual__1" || v["task_id"] != "load" || v["map_index"] != float64(-1) ||
		v["key"] != "return_value" || v["value"] != "[1, 2]" {
		t.Errorf("json = %v", v)
	}
}

func TestTasksXComNamesAMissingKey(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, xcomPath, http.StatusNotFound, `{"detail":"XCom entry with key: `+"`return_value`"+` not found"}`)
	_, _, err := runQuery(t, stub, "tasks", "xcom", "orders_etl", "manual__1", "review")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want Airflow's not found", err)
	}
}
