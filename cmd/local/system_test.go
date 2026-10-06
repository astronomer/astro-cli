package local

import (
	"net/http"
	"strings"
	"testing"
)

func TestVersionReportsTheVersionAndGeneration(t *testing.T) {
	for _, tc := range []struct {
		name, generation, version string
		stub                      func(*testing.T) *airflowStub
	}{
		{"airflow 3", "3", "3.0.3", newAirflowStub},
		{"airflow 2", "2", "2.10.5", newAirflow2Stub},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			out, _, err := runQuery(t, stub, "version", "-o", "json")
			if err != nil {
				t.Fatal(err)
			}
			if v := decodeJSON(t, out); v["version"] != tc.version || v["generation"] != tc.generation {
				t.Errorf("json = %v", v)
			}
			out, _, err = runQuery(t, stub, "version")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "version:") || !strings.Contains(out, tc.version) || !strings.Contains(out, "api generation:") {
				t.Errorf("text = %q", out)
			}
		})
	}
}

func TestProvidersListsEachInstalledProvider(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		stub         func(*testing.T) *airflowStub
	}{
		{"airflow 3", "/api/v2", newAirflowStub},
		{"airflow 2", "/api/v1", newAirflow2Stub},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := tc.stub(t)
			stub.route(http.MethodGet, tc.prefix+"/providers", `{"providers":[
				{"package_name":"apache-airflow-providers-standard","version":"1.2.0","description":"Standard operators\nand more"}],
				"total_entries":1}`)
			out, _, err := runQuery(t, stub, "providers", "-o", "json")
			if err != nil {
				t.Fatal(err)
			}
			row := decodeRows(t, out, "providers")[0]
			if row["package_name"] != "apache-airflow-providers-standard" || row["version"] != "1.2.0" {
				t.Errorf("row = %v", row)
			}
			out, _, err = runQuery(t, stub, "providers")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "PACKAGE") || !strings.Contains(out, "1.2.0") || strings.Contains(out, "and more") {
				t.Errorf("text = %q, want one line per provider", out)
			}
		})
	}
}

func TestPluginsListsWhatEachContributes(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/plugins", `{"plugins":[{"name":"metrics","source":"$PLUGINS_FOLDER/metrics.py",
		"macros":["b","a"],"fastapi_apps":[{"name":"metrics_api"}],"listeners":[]}],"total_entries":1}`)
	out, _, err := runQuery(t, stub, "plugins", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	row := decodeRows(t, out, "plugins")[0]
	components, _ := row["components"].(map[string]any)
	if row["name"] != "metrics" || len(components) != 2 || components["listeners"] != nil {
		t.Errorf("row = %v, want the two kinds it contributes and not the empty one", row)
	}
	out, _, err = runQuery(t, stub, "plugins")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fastapi_apps: metrics_api; macros: a,b") {
		t.Errorf("text = %q", out)
	}
}

func TestConfigListsOptionsAndExplainsARefusal(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/config", `{"sections":[
		{"name":"core","options":[{"key":"executor","value":"LocalExecutor"},{"key":"parallelism","value":"32"}]},
		{"name":"api","options":[{"key":"expose_config","value":["True","env var"]}]}]}`)

	out, _, err := runQuery(t, stub, "config", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	rows := decodeRows(t, out, "options")
	if len(rows) != 3 || rows[0]["section"] != "core" || rows[0]["key"] != "executor" || rows[2]["source"] != "env var" {
		t.Errorf("rows = %v", rows)
	}
	out, _, err = runQuery(t, stub, "config", "--section", "core")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[core]\nexecutor = LocalExecutor\nparallelism = 32\n\n[api]\nexpose_config = True  # env var") {
		t.Errorf("text = %q", out)
	}
	if reqs := stub.requestsTo(http.MethodGet, "/api/v2/config"); !strings.Contains(reqs[len(reqs)-1].Query, "section=core") {
		t.Errorf("--section did not reach Airflow: %v", reqs)
	}

	// Off by default on both generations, so the refusal names the setting.
	af2 := newAirflow2Stub(t)
	af2.routeStatus(http.MethodGet, "/api/v1/config", http.StatusForbidden, `{"detail":"Your Airflow administrator chose not to expose the configuration"}`)
	_, _, err = runQuery(t, af2, "config")
	if err == nil || !strings.Contains(err.Error(), "expose_config") {
		t.Fatalf("err = %v, want it to name expose_config", err)
	}
}
