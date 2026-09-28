package local

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// The password is not omitted here — it is never decoded at all, one layer
// down. This checks the promise holds through both renderings anyway, because
// that promise is the reason the command is safe to run against production.
func TestConnectionsNeverShowAPassword(t *testing.T) {
	stub := newAirflowStub(t)
	body := `{"connections":[{"connection_id":"warehouse","conn_type":"postgres","host":"db.corp",
		"port":5432,"schema":"public","login":"astro","password":"hunter2"}],"total_entries":1}`
	stub.route(http.MethodGet, "/api/v2/connections", body)
	stub.route(http.MethodGet, "/api/v2/connections/warehouse",
		`{"connection_id":"warehouse","conn_type":"postgres","host":"db.corp","port":5432,"login":"astro","password":"hunter2"}`)

	for _, args := range [][]string{
		{"connections", "list"},
		{"connections", "list", "-o", "json"},
		{"connections", "get", "warehouse"},
		{"connections", "get", "warehouse", "-o", "json"},
	} {
		out, _, err := runQuery(t, stub, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "hunter2") || strings.Contains(out, "password") {
			t.Errorf("%v leaked the password:\n%s", args, out)
		}
		if !strings.Contains(out, "warehouse") {
			t.Errorf("%v showed nothing:\n%s", args, out)
		}
	}
}

// `extra` is a free-form blob that routinely holds tokens and keys under names
// Airflow does not mask, so listing every connection must not hand it out. The
// text table never showed it; this is about --output json, where the leak was.
func TestConnectionsListWithholdsExtraAndGetShowsIt(t *testing.T) {
	stub := newAirflowStub(t)
	const extra = `{"private_key":"-----BEGIN KEY-----"}`
	body := `{"connections":[{"connection_id":"warehouse","conn_type":"postgres","host":"db.corp",
		"extra":"{\"private_key\":\"-----BEGIN KEY-----\"}"}],"total_entries":1}`
	stub.route(http.MethodGet, "/api/v2/connections", body)
	stub.route(http.MethodGet, "/api/v2/connections/warehouse",
		`{"connection_id":"warehouse","conn_type":"postgres","extra":"{\"private_key\":\"-----BEGIN KEY-----\"}"}`)

	for _, args := range [][]string{
		{"connections", "list"},
		{"connections", "list", "-o", "json"},
	} {
		out, _, err := runQuery(t, stub, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "private_key") || strings.Contains(out, "BEGIN KEY") {
			t.Errorf("%v leaked extra:\n%s", args, out)
		}
		if !strings.Contains(out, "warehouse") {
			t.Errorf("%v showed nothing:\n%s", args, out)
		}
	}

	// Asking for one connection by name is deliberate, so extra comes back.
	out, _, err := runQuery(t, stub, "connections", "get", "warehouse", "-o", "json")
	if err != nil {
		t.Fatalf("connections get: %v", err)
	}
	if v := decodeJSON(t, out); v["extra"] != extra {
		t.Errorf("extra = %v, want it on the deliberate read", v["extra"])
	}
}

// A Variable holds whatever someone put in it, so listing them prints keys and
// stops. Reading one is a deliberate act.
func TestVariablesListHasNoValuesAndGetDoes(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/variables",
		`{"variables":[{"key":"api_key","value":"s3cret","description":"upstream key"}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v2/variables/api_key",
		`{"key":"api_key","value":"s3cret","description":"upstream key"}`)

	for _, args := range [][]string{{"variables", "list"}, {"variables", "list", "-o", "json"}} {
		out, _, err := runQuery(t, stub, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "s3cret") {
			t.Errorf("%v printed a value:\n%s", args, out)
		}
		if !strings.Contains(out, "api_key") {
			t.Errorf("%v showed no keys:\n%s", args, out)
		}
	}

	out, _, err := runQuery(t, stub, "variables", "get", "api_key", "-o", "json")
	if err != nil {
		t.Fatalf("variables get: %v", err)
	}
	if v := decodeJSON(t, out); v["value"] != "s3cret" {
		t.Errorf("json = %v, want the value that was asked for", v)
	}
}

func TestPoolsListAndGet(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/pools", `{"pools":[
		{"name":"default_pool","slots":128,"running_slots":3,"queued_slots":1,"open_slots":124}],"total_entries":1}`)
	stub.route(http.MethodGet, "/api/v2/pools/default_pool",
		`{"name":"default_pool","slots":128,"occupied_slots":4,"running_slots":3,"queued_slots":1,"open_slots":124}`)

	out, _, err := runQuery(t, stub, "pools", "list")
	if err != nil {
		t.Fatalf("pools list: %v", err)
	}
	for _, want := range []string{"NAME", "default_pool", "128", "124"} {
		if !strings.Contains(out, want) {
			t.Errorf("table is missing %q:\n%s", want, out)
		}
	}

	out, _, err = runQuery(t, stub, "pools", "get", "default_pool", "-o", "json")
	if err != nil {
		t.Fatalf("pools get: %v", err)
	}
	v := decodeJSON(t, out)
	if v["name"] != "default_pool" || v["open_slots"] != float64(124) {
		t.Errorf("json = %v", v)
	}
}

// runAstroLinkQuery drives a top-level query command at the manifest's astro
// link, whose Airflow the stub stands in for.
func runAstroLinkQuery(t *testing.T, stub *airflowStub, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	d, out, errOut := instanceDeps(t, instanceProject(t, cloudManifest))
	d.Session = func(context.Context, string) (string, error) { return "Bearer session-token", nil }
	d.Locator = anyDomain(locatorFunc(func(context.Context, instances.Instance) (string, error) {
		return stub.URL, nil
	}))
	err = execute(t, d, append(append([]string{afName}, args...), "-d", "prod")...)
	return out.String(), errOut.String(), err
}

// Airflow's API lists only what its database holds, and every value `env ...
// set` stores reaches Airflow as an environment variable instead. So right
// after a set, the list is missing the value and get answers 404; both have to
// say where the value is, naming the command for the surface the reader is on.
// An Airflow that is neither an astro link nor this machine's gets its values
// some other way, so the note names no command there.
func TestEnvSourcedValuesArePointedAt(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/connections", `{"connections":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/variables",
		`{"variables":[{"key":"api_key","value":"s3cret"}],"total_entries":1}`)

	cases := []struct {
		run     func(t *testing.T, stub *airflowStub, args ...string) (string, string, error)
		prefix  []string
		family  string
		want    string
		notWant string
	}{
		{runAstroLinkQuery, nil, "connections", "`astro env connection list --deployment-id clm2xk9dq000108l7a2b3c4d5`", ""},
		{runAstroLinkQuery, nil, "variables", "`astro env airflow-variable list --deployment-id clm2xk9dq000108l7a2b3c4d5`", ""},
		{runQuery, nil, "connections", "not in Airflow's database", "astro env"},
		{runQuery, nil, "variables", "not in Airflow's database", "astro env"},
		{runLocalQuery, []string{"local", afName}, "connections", "`astro local env connection list`", ""},
		{runLocalQuery, []string{"local", afName}, "variables", "`astro local env airflow-variable list`", ""},
	}
	for _, tc := range cases {
		args := append(append([]string(nil), tc.prefix...), tc.family)

		out, errOut, err := tc.run(t, stub, append(args, "list")...)
		if err != nil {
			t.Fatalf("%v list: %v", args, err)
		}
		if !strings.Contains(errOut, tc.want) {
			t.Errorf("%v list stderr = %q, want it to name %s", args, errOut, tc.want)
		}
		if tc.notWant != "" && strings.Contains(errOut, tc.notWant) {
			t.Errorf("%v list stderr = %q, want no %s", args, errOut, tc.notWant)
		}
		if strings.Contains(out, tc.want) {
			t.Errorf("%v list put the note on stdout:\n%s", args, out)
		}

		out, errOut, err = tc.run(t, stub, append(args, "list", "-o", "json")...)
		if err != nil {
			t.Fatalf("%v list -o json: %v", args, err)
		}
		if strings.Contains(errOut, tc.want) || strings.Contains(out, tc.want) {
			t.Errorf("%v list -o json added the note: stdout %q, stderr %q", args, out, errOut)
		}

		_, _, err = tc.run(t, stub, append(args, "get", "missing")...)
		if !errors.Is(err, airflowapi.ErrNotFound) {
			t.Fatalf("%v get missing: err = %v, want it to stay a not-found", args, err)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v get missing: err = %q, want it to name %s", args, err, tc.want)
		}
		if tc.notWant != "" && strings.Contains(err.Error(), tc.notWant) {
			t.Errorf("%v get missing: err = %q, want no %s", args, err, tc.notWant)
		}
	}
}

// Only a not-found earns the note. Any other failure is about reaching the
// Airflow, and where env values live has nothing to do with it.
func TestEnvSourcedNoteOnlyOnNotFound(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/connections/warehouse", http.StatusForbidden, `{"detail":"Forbidden"}`)

	_, _, err := runQuery(t, stub, "connections", "get", "warehouse")
	if err == nil || strings.Contains(err.Error(), "environment variables") {
		t.Errorf("err = %v, want a forbidden error with no env note", err)
	}
}
