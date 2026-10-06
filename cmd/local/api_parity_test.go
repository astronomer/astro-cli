package local

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These drive `astro local api` through the real root and the real machine
// resolution (runLocalQuery hands the stub over as a runtime record), for the
// flags the standalone af's `af api` takes and the skills use: -F, -f (as --raw-field), -H, -i,
// --raw, `ls --filter`, and `spec`.

// -F on a GET is the query string, typed the way af types it: a number, a
// bool and null arrive as themselves, not as whatever --raw-field would send.
func TestLocalAPITypedFieldsGoInTheQueryOnAGet(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)

	if _, _, err := runLocalQuery(t, stub, "local", "api", "dags",
		"-F", "limit=10", "-F", "only_active=true", "-F", "ratio=1.5", "-F", "cursor=null", "--raw-field", "tags=10"); err != nil {
		t.Fatalf("local api: %v", err)
	}
	req := stub.request(http.MethodGet, "/api/v2/dags")
	got, err := url.ParseQuery(req.Query)
	if err != nil {
		t.Fatalf("query %q: %v", req.Query, err)
	}
	want := url.Values{"limit": {"10"}, "only_active": {"true"}, "ratio": {"1.5"}, "cursor": {""}, "tags": {"10"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query = %v, want %v", got, want)
	}
	if req.Body != "" {
		t.Errorf("a GET carried a body: %q", req.Body)
	}
}

// Fields on anything but a GET are the JSON body, and there the types are what
// a reader can see: 1.5 and 8 are numbers, true a bool, null a null, and --raw-field
// keeps 8080 a string. @file reads the file's contents in.
func TestLocalAPITypedFieldsAreTheBodyOnAPost(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/variables", `{"key":"port"}`)
	file := filepath.Join(t.TempDir(), "description.txt")
	if err := os.WriteFile(file, []byte("from a file"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runLocalQuery(t, stub, "local", "api", "variables", "-X", "POST",
		"-F", "key=port", "--raw-field", "value=8080", "-F", "weight=1.5", "-F", "slots=8",
		"-F", "is_encrypted=false", "-F", "overwrite=true", "-F", "note=null", "-F", "description=@"+file); err != nil {
		t.Fatalf("local api: %v", err)
	}
	var body map[string]any
	req := stub.request(http.MethodPost, "/api/v2/variables")
	if err := json.Unmarshal([]byte(req.Body), &body); err != nil {
		t.Fatalf("body %q is not json: %v", req.Body, err)
	}
	want := map[string]any{
		"key": "port", "value": "8080", "weight": 1.5, "slots": float64(8),
		"is_encrypted": false, "overwrite": true, "note": nil, "description": "from a file",
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body = %v, want %v", body, want)
	}
	if req.Query != "" {
		t.Errorf("fields leaked into the query: %q", req.Query)
	}
}

// A field that does not parse fails before anything is sent.
func TestLocalAPIRefusesAFieldWithNoValue(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "variables", "-X", "POST", "-F", "key")
	if err == nil || !strings.Contains(err.Error(), "parsing fields") {
		t.Fatalf("err = %v, want the field refused", err)
	}
	if stub.sawRequest(http.MethodPost, "/api/v2/variables") {
		t.Error("the request went out anyway")
	}
}

// --body carries the payload, so fields beside it ride in the query string —
// the rule `astro api airflow` has for --input — rather than vanishing.
func TestLocalAPIFieldsBesideABodyGoInTheQuery(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodPost, "/api/v2/backfills", `{}`)

	if _, _, err := runLocalQuery(t, stub, "local", "api", "backfills", "-X", "POST",
		"--body", `{"dag_id":"etl"}`, "-F", "dry_run=true"); err != nil {
		t.Fatalf("local api: %v", err)
	}
	req := stub.request(http.MethodPost, "/api/v2/backfills")
	if req.Body != `{"dag_id":"etl"}` {
		t.Errorf("body = %q, want --body unchanged", req.Body)
	}
	if req.Query != "dry_run=true" {
		t.Errorf("query = %q", req.Query)
	}
}

// -H reaches the wire, and an Authorization of the caller's own wins over the
// credential the engine minted.
func TestLocalAPISendsHeaders(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[]}`)

	if _, _, err := runLocalQuery(t, stub, "local", "api", "/dags",
		"-H", "X-Trace: abc123", "-H", "Authorization: Bearer mine"); err != nil {
		t.Fatalf("local api: %v", err)
	}
	req := stub.request(http.MethodGet, "/api/v2/dags")
	if got := req.Header.Get("X-Trace"); got != "abc123" {
		t.Errorf("X-Trace = %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer mine" {
		t.Errorf("Authorization = %q, want the caller's own", got)
	}
}

func TestLocalAPIRefusesAHeaderWithNoColon(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "/dags", "-H", "X-Trace abc")
	if err == nil || !strings.Contains(err.Error(), "invalid header format") {
		t.Fatalf("err = %v, want the header refused", err)
	}
}

// -i puts the status line and the headers in front of the body, the way an
// HTTP response arrives.
func TestLocalAPIIncludePrintsTheStatusAndHeaders(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[]}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "dags", "-i")
	if err != nil {
		t.Fatalf("local api -i: %v", err)
	}
	head, body, found := strings.Cut(out, "\n\n")
	if !found {
		t.Fatalf("no blank line between the head and the body:\n%s", out)
	}
	if !strings.HasPrefix(head, "HTTP/1.1 200 OK\n") {
		t.Errorf("head does not start with the status line:\n%s", head)
	}
	if !strings.Contains(head, "Content-Type: application/json") {
		t.Errorf("head is missing the headers:\n%s", head)
	}
	if strings.TrimSpace(body) != `{"dags":[]}` {
		t.Errorf("body = %q", body)
	}
}

// With --output json, -i is the one object af prints for `af api -i`.
func TestLocalAPIIncludeInJSONModeIsOneObject(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "dags", "-i", "-o", "json")
	if err != nil {
		t.Fatalf("local api -i -o json: %v", err)
	}
	got := decodeJSON(t, out)
	if got["status_code"] != float64(200) {
		t.Errorf("status_code = %v", got["status_code"])
	}
	headers, _ := got["headers"].(map[string]any)
	if headers["content-type"] != "application/json" {
		t.Errorf("headers = %v, want them keyed in lower case", got["headers"])
	}
	body, _ := got["body"].(map[string]any)
	if body["total_entries"] != float64(0) {
		t.Errorf("body = %v, want Airflow's JSON decoded", got["body"])
	}
}

// A failure under -i still shows its status and still fails.
func TestLocalAPIIncludeStillFailsOnANon2xx(t *testing.T) {
	stub := newAirflowStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/missing", http.StatusNotFound, `{"detail":"DAG not found"}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "dags/missing", "-i")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want the 404", err)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 404 Not Found\n") || !strings.Contains(out, "DAG not found") {
		t.Errorf("stdout = %q", out)
	}
}

// --raw is af's name for --root.
func TestLocalAPIRawIsRoot(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/health", `{"metadatabase":{"status":"healthy"}}`)

	out, _, err := runLocalQuery(t, stub, "local", "api", "health", "--raw")
	if err != nil {
		t.Fatalf("local api health --raw: %v", err)
	}
	if !strings.Contains(out, "metadatabase") {
		t.Errorf("stdout = %q", out)
	}
}

// airflow3Spec is a trimmed /openapi.json in Airflow 3's shape: every path
// carries its /api/v2 prefix.
const airflow3Spec = `{"openapi":"3.1.0","info":{"title":"Airflow API","version":"2"},"paths":{` +
	`"/api/v2/variables":{"get":{"operationId":"get_variables","summary":"Get Variables","tags":["Variable"]}},` +
	`"/api/v2/variables/{variable_key}":{"delete":{"operationId":"delete_variable","tags":["Variable"]}},` +
	`"/api/v2/dags":{"get":{"operationId":"get_dags","tags":["DAG"]}}}}`

// airflow2Spec is Airflow 2's, served as YAML under /api/v1 with unprefixed
// paths.
const airflow2Spec = "openapi: 3.0.3\ninfo:\n  title: Airflow API (Stable)\n  version: '1.0.0'\n" +
	"servers:\n  - url: /api/v1\npaths:\n" +
	"  /variables:\n    get:\n      operationId: get_variables\n      tags: [Variable]\n" +
	"  /dags:\n    get:\n      operationId: get_dags\n      tags: [DAG]\n"

// ls reads the running Airflow's own spec and lists paths the way this command
// takes them, without the version prefix. --filter is af's spelling.
func TestLocalAPIListFiltersTheAirflowsOwnSpec(t *testing.T) {
	for _, args := range [][]string{{"--filter", "variable"}, {"variable"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stub := newAirflowStub(t)
			stub.route(http.MethodGet, "/openapi.json", airflow3Spec)

			out, _, err := runLocalQuery(t, stub, append([]string{"local", "api", "ls", "-o", "json"}, args...)...)
			if err != nil {
				t.Fatalf("local api ls: %v", err)
			}
			rows := decodeRows(t, out, "endpoints")
			var paths []string
			for _, row := range rows {
				paths = append(paths, row["path"].(string))
			}
			want := []string{"/variables", "/variables/{variable_key}"}
			if !reflect.DeepEqual(paths, want) {
				t.Errorf("paths = %v, want %v", paths, want)
			}
		})
	}
}

// The text listing is the table `astro api airflow ls` prints.
func TestLocalAPIListPrintsATable(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/openapi.json", airflow3Spec)

	out, _, err := runLocalQuery(t, stub, "local", "api", "ls")
	if err != nil {
		t.Fatalf("local api ls: %v", err)
	}
	for _, want := range []string{"Variable", "/variables/{variable_key}", "delete_variable", "Found 3 endpoints"} {
		if !strings.Contains(out, want) {
			t.Errorf("listing is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/api/v2") {
		t.Errorf("the version prefix was left on:\n%s", out)
	}
}

// Airflow 2 serves its spec as YAML under the API prefix; ls reads it there.
func TestLocalAPIListReadsAirflow2sSpec(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/openapi.yaml", airflow2Spec)

	out, _, err := runLocalQuery(t, stub, "local", "api", "ls", "dags", "-o", "json")
	if err != nil {
		t.Fatalf("local api ls: %v", err)
	}
	rows := decodeRows(t, out, "endpoints")
	if len(rows) != 1 || rows[0]["path"] != "/dags" || rows[0]["operationId"] != "get_dags" {
		t.Errorf("rows = %v", rows)
	}
}

func TestLocalAPIListRefusesTwoFilters(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "ls", "dags", "--filter", "variable")
	if err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("err = %v", err)
	}
}

// spec prints the running Airflow's own document, as JSON.
func TestLocalAPISpecPrintsTheAirflowsOwnDocument(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/openapi.json", airflow3Spec)

	out, _, err := runLocalQuery(t, stub, "local", "api", "spec")
	if err != nil {
		t.Fatalf("local api spec: %v", err)
	}
	doc := decodeJSON(t, out)
	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/api/v2/dags"]; !ok {
		t.Errorf("spec is not the document served: %v", doc)
	}
}

// Airflow 2's YAML comes out as JSON, so a pipe into jq works on both.
func TestLocalAPISpecConvertsAirflow2sYAML(t *testing.T) {
	stub := newAirflow2Stub(t)
	stub.route(http.MethodGet, "/api/v1/openapi.yaml", airflow2Spec)

	out, _, err := runLocalQuery(t, stub, "local", "api", "spec")
	if err != nil {
		t.Fatalf("local api spec: %v", err)
	}
	doc := decodeJSON(t, out)
	if doc["openapi"] != "3.0.3" {
		t.Errorf("spec = %v", doc)
	}
}

// An Airflow that serves no spec is a failure that says what was being read.
func TestLocalAPISpecReportsAMissingDocument(t *testing.T) {
	stub := newAirflowStub(t)
	_, _, err := runLocalQuery(t, stub, "local", "api", "spec")
	if err == nil || !strings.Contains(err.Error(), "API specification") || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v", err)
	}
}
