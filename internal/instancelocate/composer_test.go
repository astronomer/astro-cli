package instancelocate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// composerInstance is the link and target section a Composer team commits.
func composerInstance() instances.Instance {
	return instances.Instance{
		Name:         "prod",
		Kind:         instances.KindComposer,
		Source:       instances.SourceManifest,
		Where:        "environment orders-prod",
		Link:         manifest.Link{Target: "composer", Environment: "orders-prod"},
		TargetConfig: map[string]any{"project": "acme-data", "location": "us-central1"},
	}
}

// composerStub stands in for the Composer API, recording the request it was
// asked with.
func composerStub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	return server
}

func composerLocator(endpoint string) *locator {
	return &locator{
		composerEndpoint: endpoint,
		httpClient:       &http.Client{Timeout: lookupTimeout},
		googleToken:      func(context.Context) (string, error) { return "ya29.token", nil },
		googleAccount:    func(context.Context) string { return "" },
	}
}

func TestComposerLinkResolvesItsAirflowURI(t *testing.T) {
	var path, auth string
	server := composerStub(t, func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"config":{"airflowUri":"https://abc123-dot-us-central1.composer.googleusercontent.com"}}`))
	})

	url, err := composerLocator(server.URL).BaseURL(context.Background(), composerInstance())
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
		i := composerInstance()
		delete(i.TargetConfig, missing)
		_, err := composerLocator("http://unused").BaseURL(context.Background(), i)
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
	l := composerLocator("http://unused")
	l.googleToken = func(context.Context) (string, error) {
		return "", googleauth.ErrNoCredentials
	}
	_, err := l.BaseURL(context.Background(), composerInstance())
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
		_, err := composerLocator(server.URL).BaseURL(context.Background(), composerInstance())
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

	l := composerLocator(server.URL)
	l.googleAccount = func(context.Context) string { return long }
	_, err := l.BaseURL(context.Background(), composerInstance())
	for _, want := range []string{long, "numeric account id", "pre-register"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want the pre-registration fix naming %q", err, want)
		}
	}

	// A short service account, and a user login, which has no account at all.
	for _, fine := range []string{"short@acme-data.iam.gserviceaccount.com", ""} {
		l.googleAccount = func(context.Context) string { return fine }
		_, err = l.BaseURL(context.Background(), composerInstance())
		if err == nil || strings.Contains(err.Error(), "numeric account id") {
			t.Fatalf("%q: err = %v, want no advice about a length that is fine", fine, err)
		}
	}
}

func TestComposerReportsAnEnvironmentWithNoURIYet(t *testing.T) {
	server := composerStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"config":{}}`))
	})
	_, err := composerLocator(server.URL).BaseURL(context.Background(), composerInstance())
	if err == nil || !strings.Contains(err.Error(), "still be starting") {
		t.Fatalf("err = %v, want the not-ready cause", err)
	}
}

func TestComposerReportsBeingOffline(t *testing.T) {
	// A port nothing listens on stands in for a machine with no route out.
	_, err := composerLocator("http://127.0.0.1:1").BaseURL(context.Background(), composerInstance())
	if err == nil || !strings.Contains(err.Error(), "check your connection") {
		t.Fatalf("err = %v, want the offline cause", err)
	}
}
