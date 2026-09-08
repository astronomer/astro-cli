package instances

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// The MWAA tests drive the real SDK client against a stub of the AWS API, so
// the request MWAA would receive and the answer it would send both travel
// through the SDK's own serialization. Only two things are faked: where the
// requests go, and the credentials that sign them.

// awsStub is a stand-in for the MWAA control-plane API. It records what it was
// asked and answers whatever the test set.
//
// Everything it records is under a mutex: the concurrency cases drive it from
// several goroutines, and an httptest server serves each connection on its own.
type awsStub struct {
	*httptest.Server
	// invoke answers POST /restapi/{name}, and webToken POST /webtoken/{name}.
	// Both are set before the server is driven and read-only after.
	invoke   func(w http.ResponseWriter, body map[string]any)
	webToken func(w http.ResponseWriter)

	mu sync.Mutex
	// invoked is the last InvokeRestApi body, decoded.
	invoked map[string]any
	// invokes and webTokens count what each way in was asked for.
	invokes   int
	webTokens int
}

func newAWSStub(t *testing.T) *awsStub {
	t.Helper()
	stub := &awsStub{}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/restapi/"):
			body := map[string]any{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
				t.Errorf("decode InvokeRestApi body: %v", err)
			}
			stub.mu.Lock()
			stub.invokes++
			stub.invoked = body
			stub.mu.Unlock()
			if stub.invoke == nil {
				awsJSON(w, http.StatusOK, map[string]any{"RestApiStatusCode": 200, "RestApiResponse": map[string]any{}})
				return
			}
			stub.invoke(w, body)
		case strings.HasPrefix(r.URL.Path, "/webtoken/"):
			stub.mu.Lock()
			stub.webTokens++
			stub.mu.Unlock()
			if stub.webToken == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			stub.webToken(w)
		default:
			t.Errorf("unexpected AWS call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(stub.Close)
	return stub
}

// counts reports what the stub was asked, safely.
func (s *awsStub) counts() (invokes, webTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invokes, s.webTokens
}

// lastBody is the last InvokeRestApi body the stub decoded.
func (s *awsStub) lastBody() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invoked
}

func awsJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// awsError answers with the restjson shape the SDK reads an exception out of.
func awsError(w http.ResponseWriter, status int, errType string, body map[string]any) {
	w.Header().Set("X-Amzn-ErrorType", errType)
	awsJSON(w, status, body)
}

// stubAWSConfig points the SDK at a stub. The endpoint would be enough on its
// own if the MWAA operations did not prepend "env." to whatever host they are
// given, so the rewrite happens in the HTTP client instead — which also leaves
// the signed request exactly as the SDK built it.
func stubAWSConfig(t *testing.T, endpoint string) func(context.Context, string) (aws.Config, error) {
	t.Helper()
	return func(_ context.Context, region string) (aws.Config, error) {
		if region == "" {
			region = "us-east-1"
		}
		target, err := url.Parse(endpoint)
		if err != nil {
			return aws.Config{}, err
		}
		return aws.Config{
			Region:      region,
			Credentials: awscreds.NewStaticCredentialsProvider("AKID", "SECRET", ""),
			HTTPClient:  &hostRewriter{target: target, inner: http.DefaultClient},
			// One attempt: the stub answers 5xx on purpose in several cases,
			// and the SDK's default three tries with backoff spend seconds
			// re-asking a stub whose next answer is already known.
			RetryMaxAttempts: 1,
		}, nil
	}
}

// hostRewriter sends every AWS request to the stub, whatever host the SDK
// resolved.
type hostRewriter struct {
	target *url.URL
	inner  *http.Client
}

func (h *hostRewriter) Do(req *http.Request) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = h.target.Scheme, h.target.Host
	req.Host = ""
	return h.inner.Do(req)
}

// mwaaLink is the manifest a team with an MWAA environment commits.
const mwaaLink = "\n[tool.astro.deployments.prod]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n\n[tool.astro.targets.mwaa]\nregion = 'us-west-2'\nbucket = 's3://acme-airflow-orders'\n"

func mwaaTransportFor(t *testing.T, stub *awsStub, d Deps) airflowapi.Transport {
	t.Helper()
	i := link(t, mwaaLink)
	if d.AWSConfig == nil {
		d.AWSConfig = stubAWSConfig(t, stub.URL)
	}
	// These tests are about the door itself, so they carry the build that has
	// it. A Deps with no providers is refused before the door is reached, which
	// is the behavior TestAnUnsupportedMethodIsRefusedBeforeAnyLookup covers.
	if d.Providers == nil {
		d.Providers = CloudProviders()
	}
	transport, err := i.Transport(context.Background(), d)
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	return transport
}

// TestMWAACarriesTheRequestThroughInvokeRestApi is the ordinary path: the
// Airflow call rides inside the AWS API, unprefixed, and the answer comes back
// as an ordinary Response.
func TestMWAACarriesTheRequestThroughInvokeRestApi(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsJSON(w, http.StatusOK, map[string]any{
			"RestApiStatusCode": 200,
			"RestApiResponse":   map[string]any{"total_entries": 3},
		})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	resp, err := transport.Do(context.Background(), airflowapi.Request{
		Generation: airflowapi.Airflow3,
		Path:       "/dags",
		Query:      url.Values{"limit": {"100"}},
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var payload struct {
		TotalEntries int `json:"total_entries"`
	}
	if err := resp.Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.TotalEntries != 3 {
		t.Fatalf("total_entries = %d, want the document round-tripped", payload.TotalEntries)
	}

	// The path goes over without a version prefix: this door picks the
	// generation itself, which is the case the Transport contract describes.
	invoked := stub.lastBody()
	if got := invoked["Path"]; got != "/dags" {
		t.Errorf("Path = %v, want the unprefixed path", got)
	}
	if got := invoked["Method"]; got != "GET" {
		t.Errorf("Method = %v, want GET for a request that named none", got)
	}
	query, _ := invoked["QueryParameters"].(map[string]any)
	if query["limit"] != "100" {
		t.Errorf("QueryParameters = %v, want the query carried", invoked["QueryParameters"])
	}
}

// TestMWAAReportsAirflowsOwnStatusAsAStatus: MWAA models a non-2xx from
// Airflow as an AWS exception. The Transport contract says a status is never
// an error, so it has to come back out as the answer it started as.
func TestMWAAReportsAirflowsOwnStatusAsAStatus(t *testing.T) {
	cases := []struct {
		errType string
		status  int
	}{
		{"RestApiClientException", http.StatusNotFound},
		{"RestApiClientException", http.StatusForbidden},
		{"RestApiServerException", http.StatusBadGateway},
	}
	for _, tc := range cases {
		stub := newAWSStub(t)
		stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
			awsError(w, http.StatusBadRequest, tc.errType, map[string]any{
				"message":           "airflow said no",
				"RestApiStatusCode": tc.status,
				"RestApiResponse":   map[string]any{"detail": "DAG not found"},
			})
		}
		transport := mwaaTransportFor(t, stub, Deps{})

		resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags/gone"})
		if err != nil {
			t.Fatalf("%s: do returned an error for a status: %v", tc.errType, err)
		}
		if resp.StatusCode != tc.status {
			t.Errorf("%s: status = %d, want %d", tc.errType, resp.StatusCode, tc.status)
		}
		if !strings.Contains(string(resp.Body), "DAG not found") {
			t.Errorf("%s: body = %q, want Airflow's own answer", resp.Body, tc.errType)
		}
	}
}

// TestMWAAAnswersAServerRootRequestWith404: there is no server root behind the
// AWS API, and the Transport contract asks a door with none to say so, so
// health probing falls back instead of failing.
func TestMWAAAnswersAServerRootRequestWith404(t *testing.T) {
	stub := newAWSStub(t)
	transport := mwaaTransportFor(t, stub, Deps{})

	resp, err := transport.Do(context.Background(), airflowapi.Request{Path: "/health"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a request below the API", resp.StatusCode)
	}
	if invokes, _ := stub.counts(); invokes != 0 {
		t.Error("a server-root request reached the AWS API")
	}
}

// TestMWAAFallsBackToTheWebLoginDoor is the other half of the story: an IAM
// policy without airflow:InvokeRestAPI, a web login token traded for a session
// cookie, and ordinary HTTP from there on.
func TestMWAAFallsBackToTheWebLoginDoor(t *testing.T) {
	// The Airflow web server. TLS because MWAA hands back a hostname and the
	// exchange is always https.
	var carried string
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mwaaLoginPath {
			if err := r.ParseForm(); err != nil || r.Form.Get("token") != "web-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "sess-123", Path: "/"})
			w.WriteHeader(http.StatusFound)
			return
		}
		carried = r.URL.Path + "|" + cookieValue(r, "session")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":7}`))
	}))
	defer web.Close()

	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{
			"message": "User is not authorized to perform: airflow:InvokeRestAPI on the Airflow role",
		})
	}
	stub.webToken = func(w http.ResponseWriter) {
		awsJSON(w, http.StatusOK, map[string]any{
			"WebToken":          "web-token",
			"WebServerHostname": strings.TrimPrefix(web.URL, "https://"),
		})
	}
	transport := mwaaTransportFor(t, stub, Deps{HTTPClient: web.Client()})

	resp, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	// The fallback is a plain HTTP transport, so the generation prefix applies
	// again — the AWS door was the exception, not the rule.
	if carried != "/api/v1/dags|sess-123" {
		t.Fatalf("the web server saw %q, want the prefixed path under the session cookie", carried)
	}

	// The door does not close and reopen: every later request goes straight
	// through the session, with no second refusal from the AWS API.
	if _, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow2, Path: "/dags"}); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if invokes, tokens := stub.counts(); invokes != 1 || tokens != 1 {
		t.Errorf("AWS calls = %d invoke, %d web token; want one refusal and one token", invokes, tokens)
	}
}

// TestMWAAReportsBothDoorsWhenBothAreShut: hiding the first refusal behind the
// second would leave the reader fixing the wrong permission.
func TestMWAAReportsBothDoorsWhenBothAreShut(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{"message": "no airflow:InvokeRestAPI"})
	}
	stub.webToken = func(w http.ResponseWriter) {
		awsError(w, http.StatusForbidden, "AccessDeniedException", map[string]any{"message": "no airflow:CreateWebLoginToken"})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	_, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
	if err == nil {
		t.Fatal("both doors shut and the call succeeded")
	}
	for _, want := range []string{"orders-prod", "us-west-2", "InvokeRestAPI", "CreateWebLoginToken"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %v", want, err)
		}
	}
}

// TestMWAAPassesOnAnErrorThatIsNotARefusal: only AccessDenied opens the second
// door. A throttle or an outage is reported, not worked around.
func TestMWAAPassesOnAnErrorThatIsNotARefusal(t *testing.T) {
	stub := newAWSStub(t)
	stub.invoke = func(w http.ResponseWriter, _ map[string]any) {
		awsError(w, http.StatusInternalServerError, "InternalServerException", map[string]any{"message": "boom"})
	}
	transport := mwaaTransportFor(t, stub, Deps{})

	_, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"})
	if err == nil {
		t.Fatal("an internal error was swallowed")
	}
	if _, tokens := stub.counts(); tokens != 0 {
		t.Error("an internal error opened the web login")
	}
	if !strings.Contains(err.Error(), "orders-prod") {
		t.Errorf("message does not name the environment: %v", err)
	}
}

func TestMWAANamesAMissingCredentialChain(t *testing.T) {
	i := link(t, mwaaLink)
	_, err := i.Transport(context.Background(), Deps{
		Providers: CloudProviders(),
		AWSConfig: func(context.Context, string) (aws.Config, error) {
			return aws.Config{
				Region:      "us-west-2",
				Credentials: failingCredentials{},
			}, nil
		},
	})
	if err == nil {
		t.Fatal("a machine with no AWS credentials resolved")
	}
	if !errors.Is(err, errNoAWSCredentials) {
		t.Fatalf("err = %v, want it to read as the missing chain", err)
	}
	for _, want := range []string{"aws sso login", "AWS_PROFILE", "orders-prod", "us-west-2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %s: %v", want, err)
		}
	}
}

type failingCredentials struct{}

func (failingCredentials) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{}, errors.New("no EC2 IMDS role found")
}

func TestMWAANamesAMissingRegion(t *testing.T) {
	// The link declares no [tool.astro.targets.mwaa], so the region can only
	// come from the AWS chain — and here it does not.
	i := link(t, "\n[tool.astro.deployments.prod]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n")
	_, err := i.Transport(context.Background(), Deps{
		Providers: CloudProviders(),
		AWSConfig: func(context.Context, string) (aws.Config, error) { return aws.Config{}, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "AWS_REGION") {
		t.Fatalf("err = %v, want the region message", err)
	}
	if !strings.Contains(err.Error(), "[tool.astro.targets.mwaa]") {
		t.Errorf("message does not name the manifest section: %v", err)
	}
}

// TestMWAATakesTheRegionFromTheManifest: the link names the environment and
// the target section says where it lives, so both halves have to arrive.
func TestMWAATakesTheRegionFromTheManifest(t *testing.T) {
	asked := ""
	i := link(t, mwaaLink)
	if _, err := i.Transport(context.Background(), Deps{
		Providers: CloudProviders(),
		AWSConfig: func(_ context.Context, region string) (aws.Config, error) {
			asked = region
			return aws.Config{Region: region, Credentials: awscreds.NewStaticCredentialsProvider("A", "B", "")}, nil
		},
	}); err != nil {
		t.Fatalf("transport: %v", err)
	}
	if asked != "us-west-2" {
		t.Fatalf("loaded the AWS config for %q, want the manifest's region", asked)
	}
}

func TestMWAARefusesAMethodTheDoorCannotCarry(t *testing.T) {
	stub := newAWSStub(t)
	transport := mwaaTransportFor(t, stub, Deps{})
	_, err := transport.Do(context.Background(), airflowapi.Request{
		Generation: airflowapi.Airflow3, Method: http.MethodHead, Path: "/dags",
	})
	if err == nil || !strings.Contains(err.Error(), "HEAD") {
		t.Fatalf("err = %v, want the method named", err)
	}
}

func TestQueryDocumentKeepsRepeatedParameters(t *testing.T) {
	got := queryDocument(url.Values{"one": {"a"}, "many": {"a", "b"}})
	if got["one"] != "a" {
		t.Errorf("one = %v, want the single value unwrapped", got["one"])
	}
	if fmt.Sprint(got["many"]) != "[a b]" {
		t.Errorf("many = %v, want the list kept", got["many"])
	}
	if queryDocument(nil) != nil {
		t.Error("an empty query produced a document")
	}
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
