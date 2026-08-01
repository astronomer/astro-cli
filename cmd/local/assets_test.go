package local

import (
	"net/http"
	"strings"
	"testing"
)

// Airflow 2 serves the same idea under another name. One command covers both,
// which is the whole reason the client adapts.
func TestAssetsListReadsDatasetsOnAirflow2(t *testing.T) {
	af3 := newAirflowStub(t)
	af3.route(http.MethodGet, "/api/v2/assets", `{"assets":[
		{"id":1,"uri":"s3://orders/gold","name":"gold","producing_tasks":[{"dag_id":"orders_etl","task_id":"load"}],
		 "scheduled_dags":[{"dag_id":"reports"}],"updated_at":"2024-05-01T00:00:00Z"}],"total_entries":1}`)
	out, _, err := runQuery(t, af3, "assets", "list")
	if err != nil {
		t.Fatalf("assets list: %v", err)
	}
	for _, want := range []string{"s3://orders/gold", "orders_etl.load", "reports"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}

	af2 := newAirflow2Stub(t)
	af2.route(http.MethodGet, "/api/v1/datasets", `{"datasets":[{"id":1,"uri":"s3://orders/gold"}],"total_entries":1}`)
	out, _, err = runQuery(t, af2, "assets", "list", "-o", "json")
	if err != nil {
		t.Fatalf("assets list on airflow 2: %v", err)
	}
	if row := decodeNDJSON(t, out)[0]; row["uri"] != "s3://orders/gold" {
		t.Errorf("row = %v", row)
	}
}

func TestAssetsEventsFilterBySource(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/assets/events", `{"asset_events":[
		{"id":7,"asset_uri":"s3://orders/gold","source_dag_id":"orders_etl","source_task_id":"load",
		 "timestamp":"2024-05-01T00:00:00Z","created_dagruns":[{"dag_id":"reports","run_id":"run_9"}]}
	],"total_entries":1}`)

	out, _, err := runQuery(t, stub, "assets", "events", "--dag-id", "orders_etl", "--task-id", "load")
	if err != nil {
		t.Fatalf("assets events: %v", err)
	}
	for _, want := range []string{"orders_etl.load", "reports/run_9"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}
	query := stub.request(http.MethodGet, "/api/v2/assets/events").Query
	for _, want := range []string{"source_dag_id=orders_etl", "source_task_id=load"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, want it to carry %s", query, want)
		}
	}
}
