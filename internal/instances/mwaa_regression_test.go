package instances

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// TestMWAACarriesABodyWithATime: pkg/airflowapi's own trigger call puts a
// time.Time in the request body, and the smithy document type refuses one. The
// body goes through JSON on the way in so the encoder sees the shape the wire
// would have carried anyway.
func TestMWAACarriesABodyWithATime(t *testing.T) {
	stub := newAWSStub(t)
	transport := mwaaTransportFor(t, stub, Deps{})

	logical := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if _, err := transport.Do(context.Background(), airflowapi.Request{
		Generation: airflowapi.Airflow3,
		Method:     http.MethodPost,
		Path:       "/dags/orders/dagRuns",
		Body: map[string]any{
			"logical_date": logical,
			"dag_run_id":   "manual__2026-07-30",
			"conf":         map[string]any{"retries": 3},
		},
	}); err != nil {
		t.Fatalf("do: %v", err)
	}

	body, _ := stub.lastBody()["Body"].(map[string]any)
	if body["logical_date"] != logical.Format(time.RFC3339Nano) {
		t.Errorf("logical_date = %v, want the time marshaled as Airflow's API spells it", body["logical_date"])
	}
	if body["dag_run_id"] != "manual__2026-07-30" {
		t.Errorf("dag_run_id = %v", body["dag_run_id"])
	}
	nested, _ := body["conf"].(map[string]any)
	if nested["retries"] != float64(3) {
		t.Errorf("conf.retries = %v, want the nested value carried", nested["retries"])
	}
}

// TestMWAAExplainsAnExceptionWithNoBody: an exception with a status and no
// document leaves the caller a bare number. What the SDK did keep stands in, in
// the field a caller already reads explanations out of.
//
// It is not Airflow's own sentence. MWAA sends a "message" and the generated
// deserializer has no case for it, so the field is gone before this code sees
// the error; what survives is the operation, the AWS status, and the exception
// name, which at least says who refused.
func TestMWAAExplainsAnExceptionWithNoBody(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusBadRequest, "RestApiClientException",
			map[string]any{"message": "Airflow said: dag_id is required", "RestApiStatusCode": 400})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload struct {
		Detail string `json:"detail"`
	}
	if err := resp.Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(payload.Detail, "RestApiClientException") {
		t.Fatalf("detail = %q, want something to read beside the status", payload.Detail)
	}
}

// TestMWAADefaultsAnAbsentStatusPerCallSite: every MWAA shape carries the
// status as a pointer, and a refusal that reported none is still a refusal.
// Reading it as 200 would turn "denied" into "no results".
func TestMWAADefaultsAnAbsentStatusPerCallSite(t *testing.T) {
	cases := []struct {
		name   string
		answer func(w http.ResponseWriter)
		want   int
	}{
		{
			"success with no status",
			func(w http.ResponseWriter) {
				awsJSON(w, http.StatusOK, map[string]any{"RestApiResponse": map[string]any{}})
			},
			http.StatusOK,
		},
		{
			"client exception with no status",
			func(w http.ResponseWriter) {
				awsError(w, http.StatusBadRequest, "RestApiClientException", map[string]any{"message": "nope"})
			},
			http.StatusBadRequest,
		},
		{
			"server exception with no status",
			func(w http.ResponseWriter) {
				awsError(w, http.StatusBadGateway, "RestApiServerException", map[string]any{"message": "boom"})
			},
			http.StatusBadGateway,
		},
	}
	for _, tc := range cases {
		stub := newAWSStub(t)
		stub.invoke = func(w http.ResponseWriter, _ map[string]any) { tc.answer(w) }
		transport := mwaaTransportFor(t, stub, Deps{})
		resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.want)
		}
	}
}

// TestMWAAReadsANullDocumentAsNoPayload: a caller decoding a JSON null gets its
// zero value and no way to tell that from an answer of nothing, so nothing is
// the truer report.
func TestMWAAReadsANullDocumentAsNoPayload(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsJSON(w, http.StatusOK, map[string]any{"RestApiStatusCode": 204, "RestApiResponse": nil})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Method: http.MethodDelete, Path: "/dags/orders"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("body = %q, want nothing", resp.Body)
	}
	// And it decodes as a no-op rather than an error, which is what a 204 has
	// to do.
	var into struct{ Anything string }
	if err := resp.Decode(&into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// TestMWAAKeepsNumbersAndTextExact: the answer is re-encoded from a decoded
// document, so the round trip has to be a round trip.
func TestMWAAKeepsNumbersAndTextExact(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"RestApiStatusCode":200,"RestApiResponse":{"id":9007199254740993,"ratio":0.30000000000000004,"note":"héllo ☃"}}`))
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	var got struct {
		ID    json.Number `json:"id"`
		Ratio json.Number `json:"ratio"`
		Note  string      `json:"note"`
	}
	if err := resp.Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID.String() != "9007199254740993" || got.Ratio.String() != "0.30000000000000004" || got.Note != "héllo ☃" {
		t.Fatalf("the round trip changed the payload: %+v (%s)", got, resp.Body)
	}
}

// TestMWAARefusesRequestHeaders: there is nowhere to put them, and the header a
// caller is most likely to set is the one the contract promises will win.
// Sending the request as somebody else instead is worse than saying no.
func TestMWAARefusesRequestHeaders(t *testing.T) {
	stub := newAWSStub(t)
	transport := mwaaTransportFor(t, stub, Deps{})

	_, err := transport.Do(context.Background(), airflowapi.Request{
		Generation: airflowapi.Airflow3,
		Path:       "/dags",
		Header:     http.Header{"Authorization": {"Bearer someone-elses"}},
	})
	if err == nil {
		t.Fatal("a header was quietly dropped")
	}
	if !strings.Contains(err.Error(), "Authorization") || !strings.Contains(err.Error(), "orders-prod") {
		t.Fatalf("err = %v, want it to name the header and the environment", err)
	}
	if invokes, _ := stub.counts(); invokes != 0 {
		t.Error("the request went to AWS anyway")
	}
}

// TestMWAANamesAMissingEnvironment: the environment name and the region come
// from two different places in the manifest, and "not found" without saying
// which two is a hunt.
func TestMWAANamesAMissingEnvironment(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusNotFound, "ResourceNotFoundException", map[string]any{"message": "not found"})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	_, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
	if err == nil {
		t.Fatal("a missing environment resolved")
	}
	for _, want := range []string{"orders-prod", "us-west-2", "[tool.astro.targets.mwaa]", "environment"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %v", want, err)
		}
	}
	if _, tokens := stub.counts(); tokens != 0 {
		t.Error("a missing environment opened the web login")
	}
}

// slowCredentials stands in for a credential_process that prompts — aws-vault,
// 1Password, a Touch ID unlock, an SSO refresh that opens a browser.
type slowCredentials struct{ delay time.Duration }

func (s slowCredentials) Retrieve(ctx context.Context) (aws.Credentials, error) {
	select {
	case <-time.After(s.delay):
		return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "SECRET", Source: "prompt"}, nil
	case <-ctx.Done():
		return aws.Credentials{}, ctx.Err()
	}
}

// TestMWAATellsASlowChainFromAnEmptyOne: a helper still waiting on the user is
// working. Telling them to log in again would be advice to abandon the login
// they are in the middle of.
func TestMWAATellsASlowChainFromAnEmptyOne(t *testing.T) {
	i := link(t, mwaaLink)
	shortCredentialTimeout(t)
	slow := &slowPreflight{delay: 2 * awsCredentialTimeout}
	_, err := i.Transport(context.Background(), Deps{AWSConfig: slow.config, Providers: CloudProviders()})
	if err == nil {
		t.Fatal("a chain that never answered resolved")
	}
	if strings.Contains(err.Error(), "no AWS credentials found") {
		t.Fatalf("err = %v, want a slow chain reported as slow rather than as absent", err)
	}
	if !strings.Contains(err.Error(), "aws sso login") || !strings.Contains(err.Error(), "credential_process") {
		t.Errorf("message does not say what to finish: %v", err)
	}
}

// shortCredentialTimeout cuts the preflight bound down so a slow chain runs
// out in milliseconds: the deadline it produces is the same one the real bound
// would.
func shortCredentialTimeout(t *testing.T) {
	t.Helper()
	original := awsCredentialTimeout
	awsCredentialTimeout = 50 * time.Millisecond
	t.Cleanup(func() { awsCredentialTimeout = original })
}

// slowPreflight hands the SDK a chain that takes its time answering.
type slowPreflight struct{ delay time.Duration }

func (s *slowPreflight) config(ctx context.Context, _ string) (aws.Config, error) {
	return aws.Config{Region: "us-west-2", Credentials: slowCredentials{delay: s.delay}}, nil
}

// TestMWAAFallbackOpensOnceUnderConcurrentCallers: the web login costs a token
// and a round trip, and eight requests must not buy eight of them.
func TestMWAAFallbackOpensOnceUnderConcurrentCallers(t *testing.T) {
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mwaaLoginPath {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "sess", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":1}`))
	}))
	defer web.Close()

	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{"message": "denied"})
	}
	stub.webToken = func(w http.ResponseWriter) {
		awsJSON(w, http.StatusOK, map[string]any{"WebToken": "web-token", "WebServerHostname": strings.TrimPrefix(web.URL, "https://")})
	}
	transport := mwaaTransportFor(t, stub, Deps{HTTPClient: web.Client()})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, tokens := stub.counts(); tokens != 1 {
		t.Errorf("minted %d web tokens, want one for the run", tokens)
	}
}

// TestMWAARefusesASessionThatCannotReachTheAPI: a Set-Cookie with no Path is
// scoped to the directory it came from, so a session set at /aws_mwaa/login is
// good for /aws_mwaa and nothing else — the login looks like it worked and
// every call after it rides unauthenticated.
func TestMWAARefusesASessionThatCannotReachTheAPI(t *testing.T) {
	var sawCookie []string
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mwaaLoginPath {
			w.Header().Add("Set-Cookie", "session=sess-123; HttpOnly")
			w.WriteHeader(http.StatusFound)
			return
		}
		sawCookie = append(sawCookie, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":1}`))
	}))
	defer web.Close()

	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{"message": "no InvokeRestAPI"})
	}
	stub.webToken = func(w http.ResponseWriter) {
		awsJSON(w, http.StatusOK, map[string]any{"WebToken": "web-token", "WebServerHostname": strings.TrimPrefix(web.URL, "https://")})
	}
	transport := mwaaTransportFor(t, stub, Deps{HTTPClient: web.Client()})

	_, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"})
	if err == nil {
		t.Fatalf("a session that cannot reach the API was accepted; the API saw %q", sawCookie)
	}
	if !strings.Contains(err.Error(), mwaaLoginPath) {
		t.Errorf("err = %v, want it to name where the cookie is stuck", err)
	}
}

// TestMWAAWebSessionIsReMintedWhenItExpires: the session is short-lived and a
// long command outlives it, so the 401 that expiry looks like has to buy a new
// one rather than end the command.
func TestMWAAWebSessionIsReMintedWhenItExpires(t *testing.T) {
	var mu sync.Mutex
	live := "first"
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == mwaaLoginPath {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: live, Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		if cookieValue(r, "session") != live {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":1}`))
	}))
	defer web.Close()

	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{"message": "denied"})
	}
	stub.webToken = func(w http.ResponseWriter) {
		awsJSON(w, http.StatusOK, map[string]any{"WebToken": "web-token", "WebServerHostname": strings.TrimPrefix(web.URL, "https://")})
	}
	transport := mwaaTransportFor(t, stub, Deps{HTTPClient: web.Client()})

	if _, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// The session the CLI holds stops being the one the server accepts.
	mu.Lock()
	live = "second"
	mu.Unlock()

	resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"})
	if err != nil {
		t.Fatalf("after expiry: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the re-minted session to work", resp.StatusCode)
	}
	if _, tokens := stub.counts(); tokens != 2 {
		t.Errorf("minted %d web tokens, want a second one for the expired session", tokens)
	}
}
