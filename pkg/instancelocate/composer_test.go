package instancelocate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/instances/instancestest"
)

// composerInstance is the link and target section a Composer team commits, read
// through the real parser rather than hand-built, so a test sees exactly what a
// user's pyproject.toml produces — kinds derived and target section attached.
func composerInstance(t *testing.T) instances.Instance {
	t.Helper()
	set := instances.Build(instancestest.Manifest(t, `
[tool.astro.targets.composer]
project = 'acme-data'
location = 'us-central1'

[tool.astro.deployments.prod]
environment = 'orders-prod'
target = 'composer'
`))
	return instancestest.OneLink(t, set.All(), set.Names())
}

// composerStub stands in for the Composer API, recording the request it was
// asked with.
func composerStub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}

// composerOptions is the seam set a test resolves through: its own endpoint
// and a chain that answers without a Google account.
func composerOptions(endpoint string) Options {
	return Options{
		Endpoint: endpoint,
		Google: googleauth.Options{
			Token:   func(context.Context) (string, error) { return "ya29.token", nil },
			Account: func(context.Context) string { return "" },
		},
	}
}

// lookup is the call under test, so a case reads as the one thing it varies.
func lookup(o Options, i instances.Instance) (string, error) {
	return ComposerBaseURL(context.Background(), i, o)
}

func TestComposerLinkResolvesItsAirflowURI(t *testing.T) {
	var path, auth string
	server := composerStub(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"config":{"airflowUri":"https://abc123-dot-us-central1.composer.googleusercontent.com"}}`))
	})

	url, err := lookup(composerOptions(server.URL), composerInstance(t))
	if err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if url != "https://abc123-dot-us-central1.composer.googleusercontent.com" {
		t.Fatalf("url = %q", url)
	}
	if path != "/v1/projects/acme-data/locations/us-central1/environments/orders-prod" {
		t.Fatalf("asked %s, want the environment's own resource path", path)
	}
	// The lookup carries the same ADC token the Airflow calls will, so a
	// machine whose credentials are wrong hears it here rather than as an
	// unexplained 403 from Airflow.
	if auth != "Bearer ya29.token" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestComposerLinkNeedsItsProjectAndLocation(t *testing.T) {
	for _, missing := range []string{"project", "location"} {
		i := composerInstance(t)
		delete(i.TargetConfig, missing)
		_, err := lookup(composerOptions("http://unused"), i)
		if err == nil {
			t.Fatalf("%s missing and the lookup went ahead", missing)
		}
		for _, want := range []string{missing, "[tool.astro.targets.composer]"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: message does not name %q: %v", missing, want, err)
			}
		}
	}
}

func TestComposerLinkNamesTheMissingGoogleChain(t *testing.T) {
	o := composerOptions("http://unused")
	o.Google.Token = func(context.Context) (string, error) { return "", googleauth.ErrNoCredentials }
	_, err := lookup(o, composerInstance(t))
	if err == nil || !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Fatalf("err = %v, want the ADC message", err)
	}
}

func TestComposerLinkNamesEveryOutage(t *testing.T) {
	cases := []struct {
		status int
		want   []string
	}{
		{http.StatusUnauthorized, []string{"rejected the credentials", "gcloud auth application-default login"}},
		{http.StatusForbidden, []string{"roles/composer.user", "orders-prod", "acme-data"}},
		{http.StatusNotFound, []string{"does not exist", "us-central1", "pyproject.toml"}},
		{http.StatusBadGateway, []string{"502", "the sky fell"}},
	}
	for _, tc := range cases {
		server := composerStub(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"error":{"message":"the sky fell"}}`))
		})
		_, err := lookup(composerOptions(server.URL), composerInstance(t))
		if err == nil {
			t.Fatalf("%d resolved", tc.status)
		}
		for _, want := range tc.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%d: message does not name %q: %v", tc.status, want, err)
			}
		}
	}
}

// TestComposerAddsTheLongServiceAccountFixToA403 covers Composer's one
// genuinely surprising refusal. It is named only when the credentials in hand
// are actually a long service account, so the common 403 — a missing role —
// is not buried under advice about a problem the reader does not have.
func TestComposerAddsTheLongServiceAccountFixToA403(t *testing.T) {
	server := composerStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	long := strings.Repeat("o", 45) + "@acme-data.iam.gserviceaccount.com"

	o := composerOptions(server.URL)
	o.Google.Account = func(context.Context) string { return long }
	_, err := lookup(o, composerInstance(t))
	for _, want := range []string{long, "numeric account id", "pre-register"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want the pre-registration fix naming %q", err, want)
		}
	}

	// A short service account, and a user login, which has no account at all.
	for _, fine := range []string{"short@acme-data.iam.gserviceaccount.com", ""} {
		o.Google.Account = func(context.Context) string { return fine }
		_, err = lookup(o, composerInstance(t))
		if err == nil || strings.Contains(err.Error(), "numeric account id") {
			t.Fatalf("%q: err = %v, want no advice about a length that is fine", fine, err)
		}
	}
}

// The HTTPClient seam is actually used, which nothing asserted: the endpoint
// and the token seams are both observable through a stub server, so a client
// that was quietly ignored still produced a passing test. A consumer handing in
// an instrumented or proxy-bound transport has to know it carries the request.
func TestTheSuppliedHTTPClientCarriesTheLookup(t *testing.T) {
	var used int
	o := composerOptions("http://composer.invalid")
	o.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"config":{"airflowUri":"https://from-the-supplied-client"}}`)),
			Request:    r,
		}, nil
	})}

	url, err := lookup(o, composerInstance(t))
	if err != nil {
		t.Fatalf("ComposerBaseURL: %v", err)
	}
	if used != 1 {
		t.Errorf("the supplied client carried %d requests, want 1: the lookup used a client of its own", used)
	}
	if url != "https://from-the-supplied-client" {
		t.Errorf("url = %q, want the answer the supplied client gave", url)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A kind this lookup cannot address is refused by name. Without the guard an
// astro link earns advice about setting `environment` and about a
// [tool.astro.targets.composer] section it does not have, which reads as the
// user's mistake rather than the caller's.
func TestANonComposerKindIsRefusedByName(t *testing.T) {
	astro := instances.Instance{Name: "prod", Kind: instances.KindAstro}
	_, err := lookup(composerOptions("http://composer.invalid"), astro)
	if err == nil {
		t.Fatal("an astro instance was accepted by the Composer lookup")
	}
	for _, want := range []string{"prod", "astro", "Composer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "tool.astro.targets.composer") {
		t.Errorf("err = %v: a caller's routing bug must not read as advice to edit the manifest", err)
	}
}

func TestComposerReportsAnEnvironmentWithNoURIYet(t *testing.T) {
	server := composerStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"config":{}}`))
	})
	_, err := lookup(composerOptions(server.URL), composerInstance(t))
	if err == nil || !strings.Contains(err.Error(), "still be starting") {
		t.Fatalf("err = %v, want the not-ready cause", err)
	}
}

func TestComposerReportsBeingOffline(t *testing.T) {
	// A port nothing listens on stands in for a machine with no route out.
	_, err := lookup(composerOptions("http://127.0.0.1:1"), composerInstance(t))
	if err == nil || !strings.Contains(err.Error(), "check your connection") {
		t.Fatalf("err = %v, want the offline cause", err)
	}
}
