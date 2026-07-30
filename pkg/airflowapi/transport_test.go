package airflowapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeBaseURLReducesTheShapesCallersHold(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing slash", "http://localhost:8080/", "http://localhost:8080"},
		{"several trailing slashes", "http://localhost:8080//", "http://localhost:8080"},
		{"trailing slashes under a path", "http://host/dep//", "http://host/dep"},
		{"query string", "https://host/dep?orgId=abc", "https://host/dep"},
		{"fragment", "https://host/dep#top", "https://host/dep"},
		{"path kept", "https://host/deployments/clm2xk", "https://host/deployments/clm2xk"},
		{"airflow 3 api suffix", "http://localhost:8080/api/v2", "http://localhost:8080"},
		{"airflow 2 api suffix", "http://localhost:8080/api/v1/", "http://localhost:8080"},
		{"api suffix under a path", "https://host/dep/api/v2", "https://host/dep"},
		{"no scheme", "airflow.corp.dev", "https://airflow.corp.dev"},
		{"no scheme with a path", "host/dep/api/v2", "https://host/dep"},
		{"surrounding space", "  http://localhost:8080  ", "http://localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeBaseURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeBaseURL(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewHTTPTransportRejectsUnusableURLs(t *testing.T) {
	for _, raw := range []string{"", "   ", "ftp://host", "https://"} {
		if _, err := NewHTTPTransport(raw); err == nil {
			t.Errorf("NewHTTPTransport(%q) accepted an unusable url", raw)
		}
	}
}

func TestHTTPTransportBuildsVersionedPathAndQuery(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":0}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{
		Path:       "/dags",
		Generation: Airflow3,
		Query:      url.Values{"limit": {"5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := stub.lastRequest()
	if got.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", got.Method)
	}
	if got.Path != "/api/v2/dags" {
		t.Errorf("path = %q, want /api/v2/dags", got.Path)
	}
	if got.Query.Get("limit") != "5" {
		t.Errorf("limit = %q, want 5", got.Query.Get("limit"))
	}
	if got.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept = %q, want application/json", got.Header.Get("Accept"))
	}
}

func TestHTTPTransportVersionNoneReachesServerRoot(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/health", `{"metadatabase":{"status":"healthy"}}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.Do(t.Context(), Request{Path: "/health"}); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Path; got != "/health" {
		t.Errorf("path = %q, want /health", got)
	}
}

func TestHTTPTransportSendsJSONBodyAndContentType(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodPost, "/api/v2/dags/etl/dagRuns", `{"dag_run_id":"r1"}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = transport.Do(t.Context(), Request{
		Method:     http.MethodPost,
		Path:       "/dags/etl/dagRuns",
		Generation: Airflow3,
		Body:       map[string]any{"logical_date": nil},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stub.lastRequest()
	if got.Body != `{"logical_date":null}` {
		t.Errorf("body = %q, want the encoded json", got.Body)
	}
	if got.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.Header.Get("Content-Type"))
	}
}

func TestHTTPTransportSendsNoBodyWhenNoneGiven(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v1/dags", `{}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow2}); err != nil {
		t.Fatal(err)
	}
	got := stub.lastRequest()
	if got.Body != "" {
		t.Errorf("body = %q, want empty", got.Body)
	}
	if got.Header.Get("Content-Type") != "" {
		t.Errorf("Content-Type = %q, want unset", got.Header.Get("Content-Type"))
	}
}

func TestHTTPTransportAppliesCredentialsPerRequest(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{}`)
	calls := 0
	transport, err := NewHTTPTransport(stub.URL, WithCredentials(func(context.Context) (string, string, error) {
		calls++
		return bearerScheme, "token-" + strings.Repeat("x", calls), nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Errorf("credential source called %d times, want one per request", calls)
	}
	if got := stub.lastRequest().Header.Get("Authorization"); got != "Bearer token-xx" {
		t.Errorf("Authorization = %q, want the second token", got)
	}
}

func TestBasicAuthEncodesCredentials(t *testing.T) {
	scheme, value, err := BasicAuth("user", "pass")(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if scheme != basicScheme {
		t.Errorf("scheme = %q, want %q", scheme, basicScheme)
	}
	if want := base64.StdEncoding.EncodeToString([]byte("user:pass")); value != want {
		t.Errorf("value = %q, want %q", value, want)
	}
}

func TestHTTPTransportSendsNoAuthorizationWithoutCredentials(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3}); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want unset", got)
	}
}

func TestHTTPTransportFailsWhenCredentialsCannotBeResolved(t *testing.T) {
	stub := newStub(t)
	want := errors.New("no astro session")
	transport, err := NewHTTPTransport(stub.URL, WithCredentials(func(context.Context) (string, string, error) {
		return "", "", want
	}))
	if err != nil {
		t.Fatal(err)
	}

	_, err = transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
	if len(stub.requests()) != 0 {
		t.Error("request was sent despite unresolvable credentials")
	}
}

func TestHTTPTransportRetriesOnceAfterRefreshingOn401(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags", http.StatusUnauthorized, `{"detail":"expired"}`)
	refreshed := false
	transport, err := NewHTTPTransport(stub.URL,
		WithCredentials(func(context.Context) (string, string, error) {
			if refreshed {
				return bearerScheme, "fresh", nil
			}
			return bearerScheme, "stale", nil
		}),
		WithRefresh(func(context.Context) error {
			refreshed = true
			stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":1}`)
			return nil
		}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the retry to succeed", resp.StatusCode)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/dags") != 2 {
		t.Errorf("sent %d requests, want the original plus one retry", stub.countRequests(http.MethodGet, "/api/v2/dags"))
	}
	if got := stub.lastRequest().Header.Get("Authorization"); got != "Bearer fresh" {
		t.Errorf("retry sent %q, want the refreshed credential", got)
	}
}

func TestHTTPTransportDoesNotRetryWithoutARefreshHook(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags", http.StatusUnauthorized, `{"detail":"expired"}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want the 401 handed back", resp.StatusCode)
	}
	if stub.countRequests(http.MethodGet, "/api/v2/dags") != 1 {
		t.Error("request was retried without a refresh hook")
	}
}

func TestHTTPTransportReportsAFailedRefresh(t *testing.T) {
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags", http.StatusUnauthorized, `{"detail":"expired"}`)
	want := errors.New("refresh token rejected")
	transport, err := NewHTTPTransport(stub.URL, WithRefresh(func(context.Context) error { return want }))
	if err != nil {
		t.Fatal(err)
	}

	_, err = transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
}

func TestHTTPTransportRefreshesOnA403(t *testing.T) {
	// Airflow 3's bearer guard answers 403, not 401, when the token is
	// missing or malformed.
	stub := newStub(t)
	stub.routeStatus(http.MethodGet, "/api/v2/dags", http.StatusForbidden, `{"detail":"Not authenticated"}`)
	transport, err := NewHTTPTransport(stub.URL, WithRefresh(func(context.Context) error {
		stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":0}`)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want the retry after a 403 to succeed", resp.StatusCode)
	}
}

func TestHTTPTransportReadsALoginRedirectAsARefusal(t *testing.T) {
	// Airflow 2 under session auth bounces an unauthenticated API call to its
	// login page. Followed, that is a 200 HTML page wearing a success.
	stub := newStub(t)
	stub.redirect(http.MethodGet, "/api/v2/dags", "/login")
	refreshes := 0
	transport, err := NewHTTPTransport(stub.URL,
		WithCredentials(BearerToken("stale")),
		WithRefresh(func(context.Context) error { refreshes++; return nil }))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want a redirect off the api to read as unauthorized", resp.StatusCode)
	}
	if refreshes != 1 {
		t.Errorf("refreshed %d times, want the refresh hook to fire for what is a 401", refreshes)
	}
	if stub.countRequests(http.MethodGet, "/login") != 0 {
		t.Error("the redirect was followed")
	}
}

func TestHTTPTransportKeepsARedirectThatStaysOnTheAPI(t *testing.T) {
	stub := newStub(t)
	stub.redirect(http.MethodGet, "/api/v2/dags", "/api/v2/dags/")
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := transport.Do(t.Context(), Request{Path: "/dags", Generation: Airflow3})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Errorf("status = %d, want the redirect reported as it came", resp.StatusCode)
	}
}

func TestDecodeNamesANonJSONAnswer(t *testing.T) {
	resp := Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
		Body:       []byte("<!DOCTYPE html><html><body>Sign In</body></html>"),
	}
	err := resp.Decode(&struct{}{})
	if err == nil {
		t.Fatal("decoded an HTML page as JSON")
	}
	if !strings.Contains(err.Error(), "text/html") {
		t.Errorf("err = %v, want it to name what came back instead", err)
	}
}

func TestBearerTokenTrimsAStoredScheme(t *testing.T) {
	// The CLI's stored context tokens carry the scheme in the value.
	for _, token := range []string{"abc123", "Bearer abc123"} {
		scheme, value, err := BearerToken(token)(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if scheme != bearerScheme || value != "abc123" {
			t.Errorf("BearerToken(%q) = %q %q, want a bare token", token, scheme, value)
		}
	}
}

func TestCallerAuthorizationHeaderWins(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{"total_entries":0}`)
	transport, err := NewHTTPTransport(stub.URL, WithCredentials(BearerToken("transport-token")))
	if err != nil {
		t.Fatal(err)
	}

	_, err = New(transport).Do(t.Context(), Request{
		Path:   "/dags",
		Header: http.Header{"Authorization": {"Bearer caller-token"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest().Header.Get("Authorization"); got != "Bearer caller-token" {
		t.Errorf("Authorization = %q, want the caller's header to win", got)
	}
}

func TestEscapedIDStaysEscapedOnTheWire(t *testing.T) {
	stub := newAF3Stub(t)
	stub.route(http.MethodGet, "/api/v2/dags/a/b", `{"dag_id":"a/b"}`)
	client := stub.client()

	if _, err := client.GetDAG(t.Context(), "a/b"); err != nil {
		// A 404 is fine here; the request line is what this checks.
		t.Log(err)
	}
	if got := stub.lastRequest().RequestURI; got != "/api/v2/dags/a%2Fb" {
		t.Errorf("request line = %q, want the id kept escaped", got)
	}
}

func TestHTTPTransportMergesCallerHeaders(t *testing.T) {
	stub := newStub(t)
	stub.route(http.MethodGet, "/api/v2/dags", `{}`)
	transport, err := NewHTTPTransport(stub.URL)
	if err != nil {
		t.Fatal(err)
	}

	_, err = transport.Do(t.Context(), Request{
		Path:       "/dags",
		Generation: Airflow3,
		Header:     http.Header{"Accept": {"text/plain"}, "X-Trace": {"abc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := stub.lastRequest()
	if got.Header.Get("Accept") != "text/plain" {
		t.Errorf("Accept = %q, want the caller's header to win", got.Header.Get("Accept"))
	}
	if got.Header.Get("X-Trace") != "abc" {
		t.Errorf("X-Trace = %q, want abc", got.Header.Get("X-Trace"))
	}
}

func TestPathfEscapesIDs(t *testing.T) {
	if got := pathf("/dags/%s", "a/b c"); got != "/dags/a%2Fb%20c" {
		t.Errorf("pathf escaped to %q", got)
	}
	if got := pathf("/dags/%s/dagRuns", anyID); got != "/dags/~/dagRuns" {
		t.Errorf("pathf mangled the wildcard: %q", got)
	}
}

func TestVersionBasePath(t *testing.T) {
	tests := []struct {
		version Generation
		want    string
	}{
		{Airflow2, "/api/v1"},
		{Airflow3, "/api/v2"},
		{GenerationNone, ""},
	}
	for _, tt := range tests {
		if got := tt.version.BasePath(); got != tt.want {
			t.Errorf("%v.BasePath() = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestResponseDecodeTreatsEmptyBodyAsNothing(t *testing.T) {
	run := DAGRun{DAGRunID: "kept"}
	if err := (Response{StatusCode: http.StatusNoContent}).Decode(&run); err != nil {
		t.Fatalf("decode of an empty body: %v", err)
	}
	if run.DAGRunID != "kept" {
		t.Errorf("decode of an empty body overwrote the value: %+v", run)
	}
}
