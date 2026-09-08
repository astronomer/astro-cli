package instances

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
)

// decodeJSON reads a request body into v, for the stub Airflows the tests run.
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// locatorFunc adapts a function to the Locator seam.
type locatorFunc func(ctx context.Context, i Instance) (string, error)

func (f locatorFunc) BaseURL(ctx context.Context, i Instance) (string, error) { return f(ctx, i) }

func TestTransportCarriesTheCredentialToTheEndpoint(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	m := parseManifest(t, "\n[tool.astro.deployments.staging]\nurl = '"+server.URL+"'\nauth = { method = 'token', token-env = 'AF_TOKEN' }\n")
	set := Build(m)
	sel, err := set.Select(Request{})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.Instance.Name != "staging" || sel.From != LayerDefault {
		t.Fatalf("selected %s from %s, want staging from the default link", sel.Instance.Name, sel.From)
	}
	transport, err := sel.Instance.Transport(context.Background(),
		Deps{LookupEnv: env(map[string]string{"AF_TOKEN": "s3cr3t"}), HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if _, err := transport.Do(context.Background(), airflowapi.Request{Path: "/dags"}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if seen != "Bearer s3cr3t" {
		t.Fatalf("Authorization = %q", seen)
	}
}

func TestTransportAsksTheLocatorForACoordinateLink(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod]\ndeployment = 'clm2xk9dq000108l7a2b3c4d5'\n")

	// Nothing wired: the message names the missing lookup rather than failing
	// obscurely.
	_, err := i.Transport(context.Background(), Deps{LookupEnv: env(nil)})
	if err == nil || !strings.Contains(err.Error(), "no lookup is wired") {
		t.Fatalf("err = %v, want the unwired-lookup message", err)
	}

	// Wired: the locator says where the deployment's Airflow is, and that is
	// where the request lands, carrying the session's bearer.
	var landed, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		landed, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	asked := ""
	transport, err := i.Transport(context.Background(), Deps{
		Providers:  CloudProviders(),
		LookupEnv:  env(map[string]string{EnvAPIToken: "ci-token"}),
		HTTPClient: server.Client(),
		Locator: locatorFunc(func(_ context.Context, in Instance) (string, error) {
			asked = in.Link.Deployment
			return server.URL, nil
		}),
	})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if asked != "clm2xk9dq000108l7a2b3c4d5" {
		t.Fatalf("locator asked about %q", asked)
	}
	if _, err := transport.Do(context.Background(), airflowapi.Request{Generation: airflowapi.Airflow3, Path: "/dags"}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if landed != "/api/v2/dags" || auth != "Bearer ci-token" {
		t.Fatalf("request landed at %q with %q", landed, auth)
	}
}

// TestTransportDispatchesOnTheAuthMethod: MWAA is not an Airflow URL behind a
// credential, it is a different door entirely, so the method picks the
// transport before any URL lookup is attempted.
func TestTransportDispatchesOnTheAuthMethod(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod-mwaa]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n")
	// A locator is wired and still never asked: there is no URL in this door.
	asked := false
	transport, err := i.Transport(context.Background(), Deps{
		Providers: CloudProviders(),
		LookupEnv: env(nil),
		AWSConfig: stubAWSConfig(t, "https://unused"),
		Locator: locatorFunc(func(context.Context, Instance) (string, error) {
			asked = true
			return "https://never", nil
		}),
	})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if _, ok := transport.(*awsTransport); !ok {
		t.Fatalf("transport = %T, want the AWS door", transport)
	}
	if asked {
		t.Error("the URL locator was asked about an MWAA environment")
	}
}

func TestTransportRefusesALocalRecordWithNoPort(t *testing.T) {
	i := Instance{Name: LocalName, Kind: KindLocal, Source: SourceRunning, Project: "/tmp/orders"}
	_, err := i.Transport(context.Background(), Deps{})
	if err == nil || !strings.Contains(err.Error(), "astro local restart") {
		t.Fatalf("err = %v, want the restart hint", err)
	}
}

// TestHTTPDoorForHandsBackTheURLAndTheCredential: `astro api airflow` builds its
// own requests, so it needs the two things a Transport hides.
func TestHTTPDoorForHandsBackTheURLAndTheCredential(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.staging]\nurl = 'https://airflow.staging.corp.dev'\nauth = { method = 'token', token-env = 'STAGING_TOKEN' }\n")
	door, err := i.HTTPDoorFor(context.Background(), Deps{LookupEnv: env(map[string]string{"STAGING_TOKEN": "abc123"})})
	if err != nil {
		t.Fatalf("door: %v", err)
	}
	if door.BaseURL != "https://airflow.staging.corp.dev" {
		t.Errorf("base url = %q", door.BaseURL)
	}
	if door.Authorization != "Bearer abc123" {
		t.Errorf("authorization = %q", door.Authorization)
	}
}

// An Airflow that wants no credential is a real answer, not a missing one.
func TestHTTPDoorForSendsNothingWhenNothingIsNeeded(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.open]\nurl = 'https://airflow.corp.dev'\nauth = { method = 'none' }\n")
	door, err := i.HTTPDoorFor(context.Background(), Deps{LookupEnv: env(nil)})
	if err != nil {
		t.Fatalf("door: %v", err)
	}
	if door.Authorization != "" {
		t.Errorf("authorization = %q, want none", door.Authorization)
	}
}

// The MWAA door has no URL at all, and saying so is the whole point: a caller
// that can only speak HTTP has to hear it rather than get an empty string.
func TestHTTPDoorForRefusesTheAWSDoor(t *testing.T) {
	i := link(t, "\n[tool.astro.deployments.prod-mwaa]\ntarget = 'mwaa'\nenvironment = 'orders-prod'\n")
	if _, err := i.HTTPDoorFor(context.Background(), Deps{LookupEnv: env(nil)}); !errors.Is(err, ErrNotHTTP) {
		t.Fatalf("err = %v, want ErrNotHTTP", err)
	}
}
