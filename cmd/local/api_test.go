package local

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The passthrough prints what Airflow sent, unchanged, under the generation it
// detected — no spec, no rewriting, no wrapper object.
func TestLocalAPIPrintsTheAnswerVerbatim(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/config", `{"sections":[{"name":"core"}]}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "/config")
	if err != nil {
		t.Fatalf("local api /config: %v", err)
	}
	if strings.TrimSpace(out) != `{"sections":[{"name":"core"}]}` {
		t.Errorf("stdout = %q, want the body as it came", out)
	}
}

// A query string typed onto the path is sent as one, so the command does what
// it looks like it does.
func TestLocalAPISendsTheQueryString(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)

	if _, _, err := runLocalQuery(t, stub, "local", "api", "/dags?limit=5&order_by=dag_id"); err != nil {
		t.Fatalf("local api: %v", err)
	}
	if got := stub.request(http.MethodGet, "/api/v2/dags").Query; got != "limit=5&order_by=dag_id" {
		t.Errorf("query = %q", got)
	}
}

// --root addresses the server below the version prefix, for the paths Airflow
// serves unversioned.
func TestLocalAPIRootSkipsTheVersionPrefix(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/health", `{"metadatabase":{"status":"healthy"}}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "--root", "/health")
	if err != nil {
		t.Fatalf("local api --root /health: %v", err)
	}
	if !strings.Contains(out, "metadatabase") {
		t.Errorf("stdout = %q", out)
	}
}

func TestLocalAPISendsAMethodAndABody(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns", `{"dag_run_id":"manual__1"}`)

	if _, _, err := runLocalQuery(t, stub, "local", "api", "/dags/etl/dagRuns",
		"-X", "POST", "--body", `{"logical_date":null}`); err != nil {
		t.Fatalf("local api: %v", err)
	}
	if got := stub.request(http.MethodPost, "/api/v2/dags/etl/dagRuns").Body; !strings.Contains(got, "logical_date") {
		t.Errorf("body = %q", got)
	}
}

// A non-2xx prints its body and still fails, so a script can branch on the
// exit code without parsing anything.
func TestLocalAPIFailsOnANon2xxButStillPrintsIt(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/missing", http.StatusNotFound, `{"detail":"DAG not found"}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "/dags/missing")
	if err == nil {
		t.Fatal("a 404 must fail the command")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %q, want the status", err)
	}
	if !strings.Contains(out, "DAG not found") {
		t.Errorf("the body was swallowed: %q", out)
	}
}

func TestLocalAPIRefusesABodyThatIsNotJSON(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "/dags", "--body", "not json")
	if err == nil || !strings.Contains(err.Error(), "valid json") {
		t.Fatalf("err = %v, want the body refused", err)
	}
}

// --body - reads the request from stdin, for a payload too big or too quoted to
// type on a command line.
func TestLocalAPIReadsABodyFromStdin(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns", `{"dag_run_id":"manual__1"}`)

	dir := instanceProject(t, twoLinkManifest)
	d, _, _ := queryDeps(t)
	d.Runtime = stubRuntime{list: []localrt.Status{{
		ProjectPath: dir, State: localrt.StateRunning, Port: stubPort(t, stub), AirflowMajor: "3",
	}}}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.Stdin = strings.NewReader(`{"conf":{"from":"stdin"}}`)

	if err := execute(t, d, "local", "api", "/dags/etl/dagRuns", "-X", "POST", "--body", "-"); err != nil {
		t.Fatalf("local api: %v", err)
	}
	if got := stub.request(http.MethodPost, "/api/v2/dags/etl/dagRuns").Body; !strings.Contains(got, "stdin") {
		t.Errorf("body = %q, want the one read from stdin", got)
	}
}

// The response is Airflow's own, so --output json changes how a failure is
// reported rather than how the body is rendered.
func TestLocalAPIReportsAFailureAsJSONInJSONMode(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/missing", http.StatusNotFound, `{"detail":"DAG not found"}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "/dags/missing", "-o", "json")
	if err == nil {
		t.Fatal("a 404 must fail the command")
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("want the body then one error object, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "DAG not found") {
		t.Errorf("first line is not Airflow's body: %q", lines[0])
	}
	var failure map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &failure); err != nil {
		t.Fatalf("second line is not json: %v", err)
	}
	if failure["code"] != float64(1) {
		t.Errorf("error object = %v", failure)
	}
}

func TestLocalAPIRefusesAQueryStringThatDoesNotParse(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "/dags?limit=%zz")
	if err == nil || !strings.Contains(err.Error(), "does not parse") {
		t.Fatalf("err = %v, want the query string refused", err)
	}
}
