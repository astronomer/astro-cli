package airflowapi

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
)

func TestDetectsAirflow3FromTheV2API(t *testing.T) {
	stub := newAF3Stub(t)
	client := stub.client()

	info, err := client.Version(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if info.Generation != Airflow3 {
		t.Errorf("generation = %v, want 3", info.Generation)
	}
	if info.Version != "3.0.3" || info.GitVersion != "abc" {
		t.Errorf("version = %+v, want the reported one", info)
	}
	if stub.countRequests(http.MethodGet, "/api/v1/version") != 0 {
		t.Error("probed /api/v1 after /api/v2 answered")
	}
}

func TestDetectsAirflow2AfterTheV2ProbeMisses(t *testing.T) {
	stub := newAF2Stub(t)
	client := stub.client()

	version, err := client.Generation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if version != Airflow2 {
		t.Errorf("Generation = %v, want 2", version)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/version") != 1 {
		t.Error("did not try /api/v2 first")
	}
}

func TestDetectionFollowsTheReportedVersionNotTheProbe(t *testing.T) {
	// An Astronomer-patched Airflow 2 runtime answers on paths Airflow 3
	// introduced, so the version it reports decides which API it has.
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/version", `{"version":"2.10.5+astro.4"}`)
	client := stub.client()

	version, err := client.Generation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if version != Airflow2 {
		t.Errorf("Generation = %v, want 2 for a patched airflow 2", version)
	}
}

func TestDetectionFallsBackToTheProbeWhenTheVersionIsUnreadable(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/version", `{"version":"unreleased"}`)
	client := stub.client()

	version, err := client.Generation(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if version != Airflow3 {
		t.Errorf("Generation = %v, want the generation that answered", version)
	}
}

func TestDetectionRunsOnce(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":0}`)
	client := stub.client()

	for range 3 {
		if _, err := client.ListDAGs(t.Context(), ListDAGsOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := stub.countRequests(http.MethodGet, "/api/v2/version"); got != 1 {
		t.Errorf("probed the version %d times, want once per client", got)
	}
}

func TestDetectionFailureNamesBothProbes(t *testing.T) {
	stub := newStub(t)
	client := stub.client()

	_, err := client.Generation(t.Context())
	var detect *DetectError
	if !errors.As(err, &detect) {
		t.Fatalf("err = %v, want a *DetectError", err)
	}
	if len(detect.Probes) != 2 {
		t.Fatalf("probes = %+v, want both generations", detect.Probes)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Error("a detection failure should carry what the probes said")
	}
}

func TestDetectionSurfacesACredentialProblem(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/version", http.StatusUnauthorized, `{"detail":"no token"}`)
	stub.routeStatus(http.MethodGet, "/api/v1/version", http.StatusUnauthorized, `{"detail":"no token"}`)
	client := stub.client()

	_, err := client.Generation(t.Context())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want it to read as unauthorized", err)
	}
}

func TestDetectionReportsACredentialProblemWithoutClaimingNotFound(t *testing.T) {
	// An Airflow 3 with a bad credential answers 401 on /api/v2/version and
	// 404 on /api/v1/version, which it does not serve at all. Reporting both
	// would let a caller checking not-found first say there is no Airflow
	// here.
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/version", http.StatusUnauthorized, `{"detail":"Not authenticated"}`)
	client := stub.client()

	_, err := client.Generation(t.Context())
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want it to read as unauthorized", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want a refused credential not to read as a missing airflow", err)
	}
}

func TestVersionIsReadOnceEvenWhenAirflowReportsNoVersion(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/version", `{"git_version":"abc"}`)
	client := stub.client()

	for range 3 {
		if _, err := client.Version(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := stub.countRequests(http.MethodGet, "/api/v2/version"); got != 1 {
		t.Errorf("read the version %d times, want detection to cost one call", got)
	}
}

func TestDetectionIsNotCachedAfterAFailure(t *testing.T) {
	stub := newStub(t)
	client := stub.client()

	if _, err := client.Generation(t.Context()); err == nil {
		t.Fatal("expected the first detection to fail")
	}
	stub.route(http.MethodGet, "/api/v2/version", `{"version":"3.1.0"}`)
	version, err := client.Generation(t.Context())
	if err != nil {
		t.Fatalf("second detection: %v", err)
	}
	if version != Airflow3 {
		t.Errorf("Generation = %v, want 3 once airflow came up", version)
	}
}

func TestDoPassesThroughWhateverTheStatusIs(t *testing.T) {
	stub := newAF3Stub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags/missing", http.StatusNotFound, `{"detail":"gone"}`)
	client := stub.client()

	resp, err := client.Do(t.Context(), Request{Method: http.MethodGet, Path: "/dags/missing"})
	if err != nil {
		t.Fatalf("passthrough turned a 404 into an error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want the 404 handed back", resp.StatusCode)
	}
	if string(resp.Body) != `{"detail":"gone"}` {
		t.Errorf("body = %q, want the raw payload", resp.Body)
	}
}

func TestDoUsesTheDetectedGeneration(t *testing.T) {
	stub := newAF2Stub(t)
	stub.route(http.MethodGet, "/api/v1/plugins", `{"total_entries":0}`)
	client := stub.client()

	resp, err := client.Do(t.Context(), Request{Method: http.MethodGet, Path: "/plugins"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want the airflow 2 path to be used", resp.StatusCode)
	}
}

func TestDoRootSkipsTheVersionPrefix(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/openapi.json", `{"openapi":"3.1.0"}`)
	client := stub.client()

	resp, err := client.DoRoot(t.Context(), Request{Method: http.MethodGet, Path: "/openapi.json"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want the root path to be reached", resp.StatusCode)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/version") != 0 {
		t.Error("a root request should not need version detection")
	}
}

func TestListOptionsAlwaysSendAPageSize(t *testing.T) {
	// The generations default differently, so the page size is never left to
	// the server.
	query := ListOptions{}.query()
	if query.Get("limit") != strconv.Itoa(DefaultLimit) {
		t.Errorf("query = %v, want the default page size sent", query)
	}
	if _, ok := query["offset"]; ok {
		t.Errorf("query = %v, want no offset when none was asked for", query)
	}
	query = ListOptions{Limit: 25, Offset: 50, OrderBy: "-start_date"}.query()
	if query.Get("limit") != "25" || query.Get("offset") != "50" || query.Get("order_by") != "-start_date" {
		t.Errorf("query = %v, want every field set", query)
	}
}
