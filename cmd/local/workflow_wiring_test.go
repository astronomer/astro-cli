package local

import (
	"net/http"
	"strings"
	"testing"
)

func TestAssetsTriggersListsTheEventsBehindARun(t *testing.T) {
	for _, tc := range []struct {
		name string
		stub func(*testing.T) *airflowStub
		path string
		body string
	}{
		{
			name: "airflow 3", stub: newAirflowStub,
			path: "/api/v2/dags/reports/dagRuns/asset_triggered__1/upstreamAssetEvents",
			body: `{"asset_events":[{"id":7,"asset_id":2,"uri":"s3://orders/gold","source_dag_id":"orders_etl",
				"source_task_id":"load","source_run_id":"manual__1","timestamp":"2024-05-01T00:00:00Z"}],"total_entries":1}`,
		},
		{
			name: "airflow 2", stub: newAirflow2Stub,
			path: "/api/v1/dags/reports/dagRuns/asset_triggered__1/upstreamDatasetEvents",
			body: `{"dataset_events":[{"id":7,"dataset_id":2,"dataset_uri":"s3://orders/gold","source_dag_id":"orders_etl",
				"source_task_id":"load","source_run_id":"manual__1","timestamp":"2024-05-01T00:00:00Z"}],"total_entries":1}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.path, tc.body)
			out, _, err := runQuery(t, stub, "assets", "triggers", "reports", "asset_triggered__1", "-o", "json")
			if err != nil {
				t.Fatalf("assets triggers: %v", err)
			}
			row := decodeNDJSON(t, out)[0]
			if row["uri"] != "s3://orders/gold" || row["asset_id"] != float64(2) || row["source_run_id"] != "manual__1" {
				t.Errorf("row = %v, want both spellings folded into the event row", row)
			}
			out, _, err = runQuery(t, stub, "assets", "triggers", "reports", "asset_triggered__1")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "orders_etl.load") || !strings.Contains(out, "s3://orders/gold") {
				t.Errorf("text = %q", out)
			}
		})
	}

	// A run nothing triggered says so.
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/reports/dagRuns/manual__1/upstreamAssetEvents", `{"asset_events":[],"total_entries":0}`)
	out, _, err := runQuery(t, stub, "assets", "triggers", "reports", "manual__1")
	if err != nil || !strings.Contains(out, "No asset events started this run.") {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestAssetsListSendsTheURIPattern(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/assets", `{"assets":[],"total_entries":0}`)
	if _, _, err := runQuery(t, stub, "assets", "list", "--uri-pattern", "s3://orders%"); err != nil {
		t.Fatal(err)
	}
	if q := stub.request(http.MethodGet, "/api/v2/assets").Query; !strings.Contains(q, "uri_pattern=s3%3A%2F%2Forders%25") {
		t.Errorf("query = %q", q)
	}
}

// Every command this port added answers under both registrations and reaches
// the Airflow each one names: `astro af …` the one --url points at, and
// `astro local af …` the one this project has running. The tree test holds the
// two registrations to the same flags; this holds them to the same behavior.
func TestPortedCommandsWorkUnderBothRegistrations(t *testing.T) {
	cases := []struct {
		args []string
		// want is a string the json output carries.
		want string
	}{
		{[]string{"dags", "errors"}, `"filename":"/dags/broken.py"`},
		{[]string{"dags", "warnings"}, `"dag_id":"orders_etl"`},
		{[]string{"dags", "explore", "orders_etl"}, `"fileloc":"/dags/orders.py"`},
		{[]string{"dags", "list", "--include-inactive"}, `"dag_id":"orders_etl"`},
		{[]string{"runs", "trigger-wait", "orders_etl", "--poll-interval", "5ms"}, `"state":"success"`},
		{[]string{"runs", "diagnose", "orders_etl", "manual__1"}, `"total_tasks":4`},
		{[]string{"assets", "triggers", "orders_etl", "manual__1"}, `"uri":"s3://orders/gold"`},
		{[]string{"assets", "list", "--uri-pattern", "s3%"}, `"uri":"s3://orders/gold"`},
		{[]string{"version"}, `"version":"3.0.3"`},
		{[]string{"providers"}, `"package_name":"apache-airflow-providers-http"`},
		{[]string{"plugins"}, `"name":"metrics"`},
		{[]string{"config"}, `"key":"executor"`},
	}
	airflow := func(t *testing.T) *airflowStub {
		t.Helper()
		stub := newAirflowStub(t)
		stub.route(http.MethodGet, "/api/v2/importErrors", importErrorsBody)
		stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[{"dag_id":"orders_etl","message":"m"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[{"dag_id":"orders_etl"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false,"fileloc":"/dags/orders.py"}`)
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl/tasks", `{"tasks":[],"total_entries":0}`)
		stub.route(http.MethodGet, "/api/v2/dagSources/orders_etl", `{"content":"x"}`)
		stub.route(http.MethodPost, "/api/v2/dags/orders_etl/dagRuns", runBody("queued"))
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1", runBody("success"))
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/taskInstances", failedInstances)
		stub.route(http.MethodGet, "/api/v2/dags/orders_etl/dagRuns/manual__1/upstreamAssetEvents",
			`{"asset_events":[{"id":1,"uri":"s3://orders/gold"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/assets", `{"assets":[{"id":1,"uri":"s3://orders/gold"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/providers", `{"providers":[{"package_name":"apache-airflow-providers-http","version":"1"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/plugins", `{"plugins":[{"name":"metrics"}],"total_entries":1}`)
		stub.route(http.MethodGet, "/api/v2/config", `{"sections":[{"name":"core","options":[{"key":"executor","value":"x"}]}]}`)
		return stub
	}
	for _, tc := range cases {
		name := strings.Join(tc.args, " ")
		t.Run("astro af "+name, func(t *testing.T) {
			out, _, err := runQuery(t, airflow(t), append(tc.args, "-o", "json")...)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %s, want %s", out, tc.want)
			}
		})
		t.Run("astro local af "+name, func(t *testing.T) {
			stub := airflow(t)
			out, _, err := runLocalQuery(t, stub, append(append([]string{"local", afName}, tc.args...), "-o", "json")...)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %s, want %s", out, tc.want)
			}
			if len(stub.requests()) == 0 {
				t.Error("the machine registration never reached this project's Airflow")
			}
		})
	}
}
