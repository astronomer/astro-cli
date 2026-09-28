package local

import (
	"net/http"
	"strings"
	"testing"
)

const importErrorsBody = `{"import_errors":[{"import_error_id":7,"filename":"/dags/broken.py","bundle_name":"dags-folder",
	"timestamp":"2024-05-01T00:00:00Z","stack_trace":"Traceback (most recent call last):\n  File \"/dags/broken.py\", line 3\nNameError: name 'x' is not defined\n"}],
	"total_entries":1}`

func TestDagsErrorsListsImportErrorsOnBothGenerations(t *testing.T) {
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
			stub.route(http.MethodGet, tc.prefix+"/importErrors", importErrorsBody)

			out, _, err := runQuery(t, stub, "dags", "errors", "-o", "json", "-l", "5")
			if err != nil {
				t.Fatalf("dags errors: %v", err)
			}
			rows := decodeNDJSON(t, out)
			if len(rows) != 1 {
				t.Fatalf("rows = %v", rows)
			}
			row := rows[0]
			if row["filename"] != "/dags/broken.py" || row["import_error_id"] != float64(7) ||
				row["bundle_name"] != "dags-folder" || !strings.Contains(row["stack_trace"].(string), "NameError") {
				t.Errorf("row = %v", row)
			}
			if q := stub.request(http.MethodGet, tc.prefix+"/importErrors").Query; !strings.Contains(q, "limit=5") {
				t.Errorf("query = %q", q)
			}

			// The text keeps the whole traceback: it is what fixing the file needs.
			out, _, err = runQuery(t, stub, "dags", "errors")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"/dags/broken.py (bundle dags-folder, 2024-05-01T00:00:00Z)",
				`    File "/dags/broken.py", line 3`, "    NameError: name 'x' is not defined",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("text is missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestDagsErrorsAndWarningsSayWhenThereAreNone(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/importErrors", `{"import_errors":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/dagWarnings", `{"dag_warnings":[],"total_entries":0}`)
	for args, want := range map[string]string{
		"errors":   "No import errors on this Airflow.",
		"warnings": "No DAG warnings on this Airflow.",
	} {
		out, _, err := runQuery(t, stub, "dags", args)
		if err != nil {
			t.Fatalf("dags %s: %v", args, err)
		}
		if !strings.Contains(out, want) {
			t.Errorf("dags %s = %q, want %q", args, out, want)
		}
		// And json prints no rows at all, rather than an empty object.
		out, _, err = runQuery(t, stub, "dags", args, "-o", "json")
		if err != nil || strings.TrimSpace(out) != "" {
			t.Errorf("dags %s -o json = %q, %v; want nothing", args, out, err)
		}
	}
}

func TestDagsWarningsListsWarnings(t *testing.T) {
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
			stub.route(http.MethodGet, tc.prefix+"/dagWarnings", `{"dag_warnings":[{"dag_id":"orders_etl",
				"warning_type":"non-existent pool","message":"Dag 'orders_etl' references pool 'big'\nwhich does not exist",
				"timestamp":"2024-05-01T00:00:00Z"}],"total_entries":1}`)
			out, _, err := runQuery(t, stub, "dags", "warnings")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"orders_etl", "non-existent pool", "references pool 'big'"} {
				if !strings.Contains(out, want) {
					t.Errorf("text is missing %q:\n%s", want, out)
				}
			}
			out, _, err = runQuery(t, stub, "dags", "warnings", "-o", "json")
			if err != nil {
				t.Fatal(err)
			}
			if row := decodeNDJSON(t, out)[0]; row["dag_id"] != "orders_etl" || row["warning_type"] != "non-existent pool" {
				t.Errorf("row = %v", row)
			}
		})
	}
}

func TestDagsExploreCombinesTheDAGItsTasksAndItsSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stub   func(*testing.T) *airflowStub
		prefix string
		source string
	}{
		{"airflow 3", newAirflowStub, "/api/v2", "/api/v2/dagSources/orders_etl"},
		// Airflow 2 addresses source by the file token on the DAG.
		{"airflow 2", newAirflow2Stub, "/api/v1", "/api/v1/dagSources/tok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.prefix+"/dags/orders_etl",
				`{"dag_id":"orders_etl","is_paused":false,"fileloc":"/dags/orders.py","file_token":"tok","owners":["data"]}`)
			stub.route(http.MethodGet, tc.prefix+"/dags/orders_etl/tasks",
				`{"tasks":[{"task_id":"extract","operator_name":"PythonOperator","downstream_task_ids":["load"]},{"task_id":"load"}],"total_entries":2}`)
			stub.route(http.MethodGet, tc.source, `{"content":"from airflow import DAG\n"}`)

			out, _, err := runQuery(t, stub, "dags", "explore", "orders_etl", "-o", "json")
			if err != nil {
				t.Fatalf("dags explore: %v", err)
			}
			v := decodeJSON(t, out)
			if dag, _ := v["dag"].(map[string]any); dag["fileloc"] != "/dags/orders.py" {
				t.Errorf("dag = %v", v["dag"])
			}
			if tasks, _ := v["tasks"].([]any); len(tasks) != 2 {
				t.Errorf("tasks = %v", v["tasks"])
			}
			if v["source"] != "from airflow import DAG\n" {
				t.Errorf("source = %q", v["source"])
			}
			for _, key := range []string{"dag_error", "tasks_error", "source_error"} {
				if _, ok := v[key]; ok {
					t.Errorf("%s set on a clean read: %v", key, v)
				}
			}

			out, _, err = runQuery(t, stub, "dags", "explore", "orders_etl")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"dag id:", "orders_etl", "tasks (2):", "PythonOperator",
				"source (/dags/orders.py):", "from airflow import DAG",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("text is missing %q:\n%s", want, out)
				}
			}
		})
	}
}

// One part failing costs that part; all three failing is a DAG Airflow does not
// know, and fails the command.
func TestDagsExploreReportsAFailedPartInItsPlace(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl", `{"dag_id":"orders_etl"}`)
	stub.route(http.MethodGet, "/api/v2/dags/orders_etl/tasks", `{"tasks":[],"total_entries":0}`)
	out, _, err := runQuery(t, stub, "dags", "explore", "orders_etl", "-o", "json")
	if err != nil {
		t.Fatalf("a missing source must not fail the command: %v", err)
	}
	v := decodeJSON(t, out)
	if v["source_error"] == nil || v["dag"] == nil {
		t.Errorf("json = %v, want the dag and the source's error", v)
	}

	_, _, err = runQuery(t, newAirflowStub(t), "dags", "explore", "missing")
	if err == nil {
		t.Fatal("a DAG nothing could be read about must fail")
	}
}

func TestDagsListActiveFilterSpellsEachGenerationsParameter(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, param string
		stub                func(*testing.T) *airflowStub
	}{
		{"airflow 3", "/api/v2", "exclude_stale", newAirflowStub},
		{"airflow 2", "/api/v1", "only_active", newAirflow2Stub},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for flag, want := range map[string]string{"--only-active": "true", "--include-inactive": "false"} {
				stub := tc.stub(t)
				stub.route(http.MethodGet, tc.prefix+"/dags", `{"dags":[],"total_entries":0}`)
				if _, _, err := runQuery(t, stub, "dags", "list", flag); err != nil {
					t.Fatalf("dags list %s: %v", flag, err)
				}
				if q := stub.request(http.MethodGet, tc.prefix+"/dags").Query; !strings.Contains(q, tc.param+"="+want) {
					t.Errorf("%s: query = %q, want %s=%s", flag, q, tc.param, want)
				}
			}
		})
	}

	stub := newAirflowStub(t)
	_, _, err := runQuery(t, stub, "dags", "list", "--only-active", "--include-inactive")
	if err == nil {
		t.Error("--only-active and --include-inactive together must be refused")
	}
}
