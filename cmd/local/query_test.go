package local

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// The query surface is tested end to end through the root command against an
// httptest Airflow, the way pkg/airflowapi tests its client: a stub is the only
// way to cover what the two API generations disagree about, and the whole point
// of these commands is that a reader never sees that disagreement.

// airflowStub stands in for an Airflow. Routes are keyed "METHOD /full/path",
// so a case says which generation's path it expects and anything unregistered
// answers 404, as Airflow does.
type airflowStub struct {
	*httptest.Server
	t *testing.T

	mu       sync.Mutex
	routes   map[string]stubRoute
	received []stubRequest
}

type stubRoute struct {
	status int
	body   string
}

type stubRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// newAirflowStub is an Airflow 3 stub: version detection answers on /api/v2,
// which is what every case here wants unless it says otherwise.
func newAirflowStub(t *testing.T) *airflowStub {
	t.Helper()
	stub := &airflowStub{t: t, routes: map[string]stubRoute{}}
	stub.Server = httptest.NewServer(stub)
	t.Cleanup(stub.Close)
	stub.route(http.MethodGet, "/api/v2/version", `{"version":"3.0.3","git_version":"abc123"}`)
	return stub
}

// newAirflow2Stub answers detection as Airflow 2: no /api/v2 at all.
func newAirflow2Stub(t *testing.T) *airflowStub {
	t.Helper()
	stub := &airflowStub{t: t, routes: map[string]stubRoute{}}
	stub.Server = httptest.NewServer(stub)
	t.Cleanup(stub.Close)
	stub.route(http.MethodGet, "/api/v1/version", `{"version":"2.10.5"}`)
	return stub
}

func (s *airflowStub) route(method, path, body string) {
	s.routeStatus(method, path, http.StatusOK, body)
}

func (s *airflowStub) routeStatus(method, path string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = stubRoute{status: status, body: body}
}

func (s *airflowStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read stub request body: %v", err)
	}
	s.mu.Lock()
	s.received = append(s.received, stubRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   string(body),
	})
	route, ok := s.routes[r.Method+" "+r.URL.Path]
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"detail":"Not Found"}`)
		return
	}
	w.WriteHeader(route.status)
	_, _ = io.WriteString(w, route.body)
}

func (s *airflowStub) requests() []stubRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubRequest(nil), s.received...)
}

// request finds the one call to a method and path, failing when it never came.
func (s *airflowStub) request(method, path string) stubRequest {
	s.t.Helper()
	for _, req := range s.requests() {
		if req.Method == method && req.Path == path {
			return req
		}
	}
	s.t.Fatalf("stub never received %s %s; it saw %v", method, path, s.requests())
	return stubRequest{}
}

// sawRequest reports whether a call was made at all.
func (s *airflowStub) sawRequest(method, path string) bool {
	for _, req := range s.requests() {
		if req.Method == method && req.Path == path {
			return true
		}
	}
	return false
}

// runQuery drives a top-level query command against the stub through the real
// root, with --url so nothing depends on a project or a manifest.
func runQuery(t *testing.T, stub *airflowStub, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	d, out, errOut := queryDeps(t)
	err = execute(t, d, append(args, "--url", stub.URL)...)
	return out.String(), errOut.String(), err
}

// runLocalQuery drives an `astro local` query command against the stub, which
// stands in for the Airflow this project has running. There is no flag to pass:
// the machine registration takes none, so the stub has to arrive as a runtime
// record, exactly as `astro local start` would leave one.
func runLocalQuery(t *testing.T, stub *airflowStub, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	dir := instanceProject(t, twoLinkManifest)
	d, out, errOut := queryDeps(t)
	d.Runtime = stubRuntime{list: []localrt.Status{{
		ProjectPath:  dir,
		State:        localrt.StateRunning,
		Port:         stubPort(t, stub),
		AirflowMajor: "3",
	}}}
	d.WorkingDir = func() (string, error) { return dir, nil }
	err = execute(t, d, args...)
	return out.String(), errOut.String(), err
}

// stubPort is the port the stub listens on, which is all a runtime record
// carries — the machine's URL is rebuilt from it.
func stubPort(t *testing.T, stub *airflowStub) int {
	t.Helper()
	u, err := url.Parse(stub.URL)
	if err != nil {
		t.Fatalf("stub url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("stub port: %v", err)
	}
	return port
}

// queryDeps is the process a query command sees: buffers for both streams, a
// non-interactive run, no local Airflow alive, and no credentials in the
// environment — an ASTRO_AIRFLOW_TOKEN exported on the developer's machine
// would otherwise change what the transport sends.
func queryDeps(t *testing.T) (d Deps, stdout, stderr *bytes.Buffer) {
	t.Helper()
	t.Setenv(instances.EnvToken, "")
	t.Setenv(instances.EnvUsername, "")
	t.Setenv(instances.EnvPassword, "")
	t.Setenv(instances.EnvVar, "")
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	d, _ = testDeps(t)
	d.Stdout = stdout
	d.Stderr = stderr
	d.Runtime = stubRuntime{}
	return d, stdout, stderr
}

// decodeNDJSON reads a json-mode listing: one object per line, which is what
// emitRows writes so a consumer can stream it.
func decodeNDJSON(t *testing.T, out string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("line %q is not json: %v", line, err)
		}
		rows = append(rows, row)
	}
	return rows
}

// decodeJSON reads a json-mode single object.
func decodeJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output %q is not json: %v", out, err)
	}
	return v
}

// The families all resolve through --url above, which is the escape hatch. This
// is the everyday path instead: a link the manifest declares, named with -d.
// It proves the family's persistent flag actually reaches the composition root
// rather than being registered and ignored.
func TestQueryCommandResolvesAManifestLink(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/pools", `{"pools":[{"name":"default_pool","slots":128}],"total_entries":1}`)

	dir := instanceProject(t, "\n[tool.astro.deployments.staging]\nurl = '"+stub.URL+"'\nauth = { method = 'none' }\n")
	d, out, errOut := queryDeps(t)
	d.WorkingDir = func() (string, error) { return dir, nil }

	if err := execute(t, d, "pools", "list", "-d", "staging"); err != nil {
		t.Fatalf("pools list -d staging: %v", err)
	}
	if !strings.Contains(out.String(), "default_pool") {
		t.Errorf("stdout = %q", out)
	}
	// The link is named on stderr, not the bare URL: the whole point of a name
	// is that it is what the reader recognizes.
	if !strings.Contains(errOut.String(), "→ staging") {
		t.Errorf("stderr = %q", errOut)
	}

	// A name no link declares fails, and says what there is.
	d2, _, _ := queryDeps(t)
	d2.WorkingDir = func() (string, error) { return dir, nil }
	err := execute(t, d2, "pools", "list", "-d", "nope")
	if err == nil || !strings.Contains(err.Error(), "staging") {
		t.Fatalf("err = %v, want it to name the links that exist", err)
	}
}

// A failed command in json mode writes one error object on stdout, so a
// consumer parsing the stream is not handed a plaintext line on stderr.
func TestQueryFailureIsJSONInJSONMode(t *testing.T) {
	stub := newAirflowStub(t)
	out, _, err := runQuery(t, stub, "dags", "get", "missing", "-o", "json")
	if err == nil {
		t.Fatal("a missing DAG must fail")
	}
	v := decodeJSON(t, out)
	if v["error"] == nil || v["code"] != float64(1) {
		t.Errorf("json error object = %v", v)
	}
}

// An empty listing is a sentence in text mode, not a blank screen.
func TestEmptyListingsSaySo(t *testing.T) {
	stub := newAirflowStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"dags":[],"total_entries":0}`)
	stub.route(http.MethodGet, "/api/v2/pools", `{"pools":[],"total_entries":0}`)

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"dags", "list"}, "No DAGs on this Airflow."},
		{[]string{"pools", "list"}, "No pools on this Airflow."},
	} {
		out, _, err := runQuery(t, stub, tc.args...)
		if err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%v printed %q, want %q", tc.args, out, tc.want)
		}
	}
}

// Durations are a number in json and prose in text, so a consumer can do
// arithmetic while a reader gets something scannable. These are the edges the
// rendering has to get right.
func TestFormatDuration(t *testing.T) {
	for _, tc := range []struct {
		seconds float64
		want    string
	}{
		{0, ""},         // not finished, or never started
		{-5, ""},        // clock skew between workers, not negative time
		{0.45, "450ms"}, // plenty of tasks finish inside a second
		{0.0004, "1ms"}, // still not "0s"
		{1, "1s"},
		{95, "1m"},
		{3720, "1h2m"},
	} {
		if got := formatDuration(tc.seconds); got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.seconds, got, tc.want)
		}
	}
}

// span is the seconds between two ends, and is zero until both are known — a
// running task reports no duration rather than one measured against nothing.
func TestSpanNeedsBothEnds(t *testing.T) {
	start := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Second)
	if got := span(start, end); got != 90 {
		t.Errorf("span = %v, want 90", got)
	}
	if got := span(start, time.Time{}); got != 0 {
		t.Errorf("span of a running task = %v, want 0", got)
	}
	if got := span(end, start); got != 0 {
		t.Errorf("span of a skewed pair = %v, want 0", got)
	}
}
