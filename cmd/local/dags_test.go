package local

import (
	"net/http"
	"strings"
	"testing"
)

const twoDAGsAF3 = `{"dags":[
	{"dag_id":"orders_etl","is_paused":false,"owners":["data"],"tags":[{"name":"gold"}],
	 "timetable_summary":"@daily","next_dagrun_logical_date":"2024-05-01T00:00:00Z"},
	{"dag_id":"reports","is_paused":true,"owners":["analytics"],"timetable_summary":"@weekly"}
],"total_entries":2}`

func TestDagsListRendersATableAndNDJSON(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", twoDAGsAF3)

	out, errOut, err := runQuery(t, stub, "dags", "list")
	if err != nil {
		t.Fatalf("dags list: %v", err)
	}
	for _, want := range []string{"DAG_ID", "orders_etl", "@daily", "gold", "reports", "yes"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}
	// The resolved instance is announced on stderr, never mixed into stdout.
	if !strings.Contains(errOut, "→ "+stub.URL) {
		t.Errorf("the target was not announced: %q", errOut)
	}

	out, _, err = runQuery(t, stub, "dags", "list", "-o", "json")
	if err != nil {
		t.Fatalf("dags list -o json: %v", err)
	}
	rows := decodeNDJSON(t, out)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2:\n%s", len(rows), out)
	}
	// Snake case, and no Go spelling anywhere in the object.
	if rows[0]["dag_id"] != "orders_etl" || rows[0]["is_paused"] != false || rows[0]["schedule"] != "@daily" {
		t.Errorf("row = %v", rows[0])
	}
	if _, leaked := rows[0]["DAGID"]; leaked {
		t.Errorf("Go field names leaked into json: %v", rows[0])
	}
}

// The two generations spell a DAG's schedule and next run differently. Both
// land in one field, so nothing downstream has to know which Airflow answered.
func TestDagsListFoldsTheGenerationsIntoOneShape(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/dags", `{"dags":[
		{"dag_id":"orders_etl","is_paused":false,"timetable_description":"Every day",
		 "next_dagrun":"2024-05-01T00:00:00Z"}],"total_entries":1}`)

	out, _, err := runQuery(t, stub, "dags", "list", "-o", "json")
	if err != nil {
		t.Fatalf("dags list: %v", err)
	}
	row := decodeNDJSON(t, out)[0]
	if row["schedule"] != "Every day" {
		t.Errorf("airflow 2's timetable_description did not reach schedule: %v", row)
	}
	if row["next_run"] != "2024-05-01T00:00:00Z" {
		t.Errorf("airflow 2's next_dagrun did not reach next_run: %v", row)
	}
}

func TestDagsListPausedFilters(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want string
	}{
		{"--paused", "paused=true"},
		{"--not-paused", "paused=false"},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			stub := newAirflowStub(t)
			stub.route(http.MethodGet, "/api/v2/dags", twoDAGsAF3)
			if _, _, err := runQuery(t, stub, "dags", "list", tc.flag); err != nil {
				t.Fatalf("dags list %s: %v", tc.flag, err)
			}
			if got := stub.request(http.MethodGet, "/api/v2/dags").Query; !strings.Contains(got, tc.want) {
				t.Errorf("query = %q, want it to carry %s", got, tc.want)
			}
		})
	}

	// Asking for both at once is a contradiction, refused before any call.
	stub := newAirflowStub(t)
	_, _, err := runQuery(t, stub, "dags", "list", "--paused", "--not-paused")
	if err == nil {
		t.Fatal("--paused with --not-paused must fail")
	}
}

func TestDagsSourcePrintsTheFile(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dagSources/orders_etl", `{"dag_id":"orders_etl","content":"from airflow import DAG"}`)

	out, _, err := runQuery(t, stub, "dags", "source", "orders_etl")
	if err != nil {
		t.Fatalf("dags source: %v", err)
	}
	if out != "from airflow import DAG\n" {
		t.Errorf("stdout = %q, want the file alone", out)
	}

	out, _, err = runQuery(t, stub, "dags", "source", "orders_etl", "-o", "json")
	if err != nil {
		t.Fatalf("dags source -o json: %v", err)
	}
	if v := decodeJSON(t, out); v["dag_id"] != "orders_etl" || v["content"] != "from airflow import DAG" {
		t.Errorf("json = %v", v)
	}
}

func TestDagsStatsCountsRunsByState(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dagStats", `{"dags":[
		{"dag_id":"orders_etl","stats":[{"state":"success","count":12},{"state":"failed","count":1}]}
	],"total_entries":1}`)

	out, _, err := runQuery(t, stub, "dags", "stats")
	if err != nil {
		t.Fatalf("dags stats: %v", err)
	}
	for _, want := range []string{"DAG_ID", "SUCCESS", "FAILED", "orders_etl", "12"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}

	out, _, err = runQuery(t, stub, "dags", "stats", "-o", "json")
	if err != nil {
		t.Fatalf("dags stats -o json: %v", err)
	}
	row := decodeNDJSON(t, out)[0]
	stats, ok := row["stats"].(map[string]any)
	if !ok || stats["success"] != float64(12) {
		t.Errorf("row = %v", row)
	}
}

// An Airflow with no dagStats endpoint gets a sentence, not a status dump.
func TestDagsStatsSaysSoWhenNotServed(t *testing.T) {
	stub := newAirflowStub(t)
	// /api/v2/dagStats is unregistered, so the stub 404s it — which from a list
	// endpoint means "not served here".
	_, _, err := runQuery(t, stub, "dags", "stats")
	if err == nil {
		t.Fatal("dags stats against an Airflow without the endpoint must fail")
	}
	if !strings.Contains(err.Error(), "does not serve DAG run statistics") {
		t.Errorf("err = %q, want the plain sentence", err)
	}
	if strings.Contains(err.Error(), "404") {
		t.Errorf("err = %q, want no status code in it", err)
	}
}

func TestDagsPauseAndUnpause(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":true}`)

	out, _, err := runQuery(t, stub, "dags", "pause", "orders_etl")
	if err != nil {
		t.Fatalf("dags pause: %v", err)
	}
	if !strings.Contains(out, "paused orders_etl") {
		t.Errorf("stdout = %q", out)
	}

	stub.route(http.MethodPatch, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl","is_paused":false}`)
	out, _, err = runQuery(t, stub, "dags", "unpause", "orders_etl")
	if err != nil {
		t.Fatalf("dags unpause: %v", err)
	}
	if !strings.Contains(out, "unpaused orders_etl") {
		t.Errorf("stdout = %q", out)
	}
}
