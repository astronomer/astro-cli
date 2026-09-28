package instancelocate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

// deploymentsFunc adapts a function to the one call this package makes on the
// v1 client, so a test needs neither a mock nor a login.
type deploymentsFunc func(ctx context.Context, org, deploymentID string) (*astrov1.GetDeploymentResponse, error)

func (f deploymentsFunc) GetDeploymentWithResponse(ctx context.Context, org, deploymentID string, _ ...astrov1.RequestEditorFn) (*astrov1.GetDeploymentResponse, error) {
	return f(ctx, org, deploymentID)
}

// astroLocator builds a locator with a session and an org already in hand, so
// each test says only what it is about.
func astroLocator(deployments deploymentsFunc) *locator {
	return &locator{
		deployments:  deployments,
		session:      func(context.Context) (string, error) { return "Bearer session", nil },
		organization: func() (string, error) { return "cl-org", nil },
	}
}

func astroInstance() instances.Instance {
	return instances.Instance{
		Name:   "prod",
		Kind:   instances.KindAstro,
		Source: instances.SourceManifest,
		Where:  "deployment clm2xk9dq000108l7a2b3c4d5",
		Link:   manifest.Link{Target: "astro", Deployment: "clm2xk9dq000108l7a2b3c4d5"},
	}
}

func deploymentResponse(status int, url string) *astrov1.GetDeploymentResponse {
	resp := &astrov1.GetDeploymentResponse{HTTPResponse: &http.Response{StatusCode: status}}
	if status == http.StatusOK {
		resp.JSON200 = &astrov1.Deployment{WebServerAirflowApiUrl: url}
	}
	return resp
}

func TestAstroLinkResolvesItsWebServerURL(t *testing.T) {
	var askedOrg, askedDeployment string
	l := astroLocator(func(_ context.Context, org, deploymentID string) (*astrov1.GetDeploymentResponse, error) {
		askedOrg, askedDeployment = org, deploymentID
		return deploymentResponse(http.StatusOK, "https://orders.astronomer.run/abc123"), nil
	})

	url, err := l.BaseURL(context.Background(), astroInstance())
	if err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if url != "https://orders.astronomer.run/abc123" {
		t.Fatalf("url = %q", url)
	}
	if askedOrg != "cl-org" || askedDeployment != "clm2xk9dq000108l7a2b3c4d5" {
		t.Fatalf("asked about %s/%s", askedOrg, askedDeployment)
	}
}

// TestAstroLinkNamesEveryOutage: each failure has its own fix, and a reader
// should not have to guess which one they are looking at.
func TestAstroLinkNamesEveryOutage(t *testing.T) {
	cases := []struct {
		status int
		want   []string
	}{
		{http.StatusUnauthorized, []string{"session expired", "astro login"}},
		{http.StatusForbidden, []string{"do not have access", "workspace admin"}},
		{http.StatusNotFound, []string{"does not exist", "pyproject.toml"}},
	}
	for _, tc := range cases {
		l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
			return deploymentResponse(tc.status, ""), nil
		})
		_, err := l.BaseURL(context.Background(), astroInstance())
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

// A Deployment id typed on the command line has no link to check, so its 404
// does not point at pyproject.toml.
func TestAstroDeploymentIDNotFoundDoesNotBlameALink(t *testing.T) {
	l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		return deploymentResponse(http.StatusNotFound, ""), nil
	})
	i := astroInstance()
	i.Source = instances.SourceDeploymentID
	_, err := l.BaseURL(context.Background(), i)
	if err == nil || !strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "pyproject.toml") {
		t.Fatalf("err = %v, want a not-found error that names no link", err)
	}
}

// A project that names its Astro host gets that host named when the control
// plane refuses the session, with the login that fixes it.
func TestAstroLinkNamesTheProjectsDomainWhenTheSessionIsRefused(t *testing.T) {
	l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		return deploymentResponse(http.StatusUnauthorized, ""), nil
	})
	l.domain = "astronomer-dev.io"
	_, err := l.BaseURL(context.Background(), astroInstance())
	want := "your astronomer-dev.io session expired. Log in again with `astro login astronomer-dev.io` (looking up \"prod\")"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// The organization comes from the login for the project's host, not the
// current context, and a host with no login is named.
func TestOrganizationReadsTheDomainsLogin(t *testing.T) {
	testUtil.InitTestConfig(testUtil.LocalPlatform)
	prod := config.Context{Domain: "astronomer.io"}
	if err := prod.SetContextKey("organization", "cl-prod-org"); err != nil {
		t.Fatal(err)
	}
	if org, err := Organization("astronomer.io"); err != nil || org != "cl-prod-org" {
		t.Fatalf("org, err = %q, %v; want the astronomer.io login's org", org, err)
	}
	if _, err := Organization("astronomer-stage.io"); err == nil || !strings.Contains(err.Error(), "astro login astronomer-stage.io") {
		t.Fatalf("err = %v, want the missing host's login named", err)
	}
}

// TestAstroLinkPutsTheResolvedIdentityOnTheRequest: ASTRO_API_TOKEN outranks a
// stale login context on the credential path, and the lookup has to agree —
// a run that proves itself to Airflow as one identity and to the control plane
// as another is two different answers to one question.
func TestAstroLinkPutsTheResolvedIdentityOnTheRequest(t *testing.T) {
	var sent string
	l := astroLocator(func(ctx context.Context, _, _ string) (*astrov1.GetDeploymentResponse, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", http.NoBody)
		req.Header.Set("Authorization", "Bearer from-the-login-context")
		if err := bearer("ci-token")(ctx, req); err != nil {
			return nil, err
		}
		sent = req.Header.Get("Authorization")
		if got := req.Header.Values("Authorization"); len(got) != 1 {
			t.Errorf("Authorization set %d times, want the login context replaced", len(got))
		}
		return deploymentResponse(http.StatusOK, "https://orders.astronomer.run/abc"), nil
	})
	if _, err := l.BaseURL(context.Background(), astroInstance()); err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if sent != "Bearer ci-token" {
		t.Fatalf("Authorization = %q, want the identity the session resolved", sent)
	}
}

func TestAstroLinkNeedsASession(t *testing.T) {
	l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		t.Fatal("the control plane was called without a session")
		return nil, nil
	})
	l.session = func(context.Context) (string, error) {
		return "", errors.New("your session expired — log in with `astro login`")
	}
	_, err := l.BaseURL(context.Background(), astroInstance())
	if err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("err = %v, want the session's own cause", err)
	}

	// No session wired at all reads as logged out, the same answer
	// internal/emenv gives for the same machine.
	l.session = nil
	if _, err := l.BaseURL(context.Background(), astroInstance()); !errors.Is(err, astrosession.ErrLoggedOut) {
		t.Fatalf("err = %v, want the logged-out message", err)
	}
}

func TestAstroLinkReportsBeingOffline(t *testing.T) {
	l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		return nil, errors.New("dial tcp: no such host")
	})
	_, err := l.BaseURL(context.Background(), astroInstance())
	if err == nil || !strings.Contains(err.Error(), "check your connection") {
		t.Fatalf("err = %v, want the offline cause", err)
	}
}

func TestAstroLinkReportsADeploymentWithNoURLYet(t *testing.T) {
	l := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		return deploymentResponse(http.StatusOK, ""), nil
	})
	_, err := l.BaseURL(context.Background(), astroInstance())
	if err == nil || !strings.Contains(err.Error(), "still be starting") {
		t.Fatalf("err = %v, want the not-ready cause", err)
	}
}

// A Composer link reaches pkg/instancelocate, carrying this locator's seams.
//
// The lookup itself moved to that module and its tests went with it, which
// left this switch's Composer case covered by nothing: delete the case and
// BaseURL falls through to "cannot look up a composer deployment" with no test
// objecting. This asserts the routing and that the chain and the endpoint are
// handed on, which is the part that has to stay true — the lookup and the
// Airflow calls after it must be answered by one set of credentials.
func TestComposerLinkReachesThePromotedLookup(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"config":{"airflowUri":"https://composer.example.com"}}`))
	}))
	defer server.Close()

	l := &locator{
		composerEndpoint: server.URL,
		httpClient:       server.Client(),
		googleToken:      func(context.Context) (string, error) { return "ya29.from-this-locator", nil },
		googleAccount:    func(context.Context) string { return "" },
	}
	url, err := l.BaseURL(context.Background(), instances.Instance{
		Name:         "gcp",
		Kind:         instances.KindComposer,
		Source:       instances.SourceManifest,
		Link:         manifest.Link{Target: "composer", Environment: "orders-prod"},
		TargetConfig: map[string]any{"project": "acme-data", "location": "us-central1"},
	})
	if err != nil {
		t.Fatalf("BaseURL: %v", err)
	}
	if url != "https://composer.example.com" {
		t.Errorf("url = %q, want the Airflow URI the Composer API reported", url)
	}
	// Compared, never printed. If the seam stops being handed on, the module
	// falls back to the machine's real ADC chain, and echoing gotAuth would
	// write a live access token into the test log.
	if gotAuth != "Bearer ya29.from-this-locator" {
		t.Errorf("authorization (%d bytes) is not the chain this locator holds: the lookup and the "+
			"Airflow calls after it have to be answered by the same credentials", len(gotAuth))
	}
	if !strings.Contains(gotPath, "orders-prod") {
		t.Errorf("path = %q, want the environment the link names", gotPath)
	}
}

// TestKindsWithNothingToLookUp: the locator is only ever asked about a link
// whose URL is not already known, so the other kinds say plainly that there is
// nothing here to find rather than returning an empty string.
func TestKindsWithNothingToLookUp(t *testing.T) {
	l := astroLocator(nil)
	for kind, want := range map[instances.Kind]string{
		instances.KindMWAA:     "reached through the AWS API",
		instances.KindEndpoint: "already knows",
		instances.KindLocal:    "already knows",
	} {
		_, err := l.BaseURL(context.Background(), instances.Instance{Name: "x", Kind: kind})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want one naming %q", kind, err, want)
		}
	}
}

func deploymentIn(status astrov1.DeploymentStatus) deploymentsFunc {
	return func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		resp := deploymentResponse(http.StatusOK, "https://orders.astronomer.run/abc123")
		resp.JSON200.Status = status
		return resp, nil
	}
}

func TestWhyUnavailableNamesTheStateAstroReports(t *testing.T) {
	cases := []struct {
		status astrov1.DeploymentStatus
		is     error
		want   string
	}{
		{astrov1.DeploymentStatusHIBERNATING, ErrDeploymentHibernating, "astro deployment wake-up clm2xk9dq000108l7a2b3c4d5"},
		{astrov1.DeploymentStatusDEPLOYING, ErrDeploymentDeploying, "still deploying"},
		{astrov1.DeploymentStatusCREATING, ErrDeploymentDeploying, "still deploying"},
		{astrov1.DeploymentStatusUNHEALTHY, ErrDeploymentUnhealthy, "astro deployment inspect clm2xk9dq000108l7a2b3c4d5"},
		{astrov1.DeploymentStatusHEALTHY, ErrAirflowUnavailable, "reports healthy, but its Airflow is not answering yet"},
	}
	for _, tc := range cases {
		err := astroLocator(deploymentIn(tc.status)).WhyUnavailable(context.Background(), astroInstance())
		if !errors.Is(err, tc.is) {
			t.Errorf("%s: err = %v, want %v", tc.status, err, tc.is)
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to name %q", tc.status, err, tc.want)
		}
	}
}

func TestWhyUnavailableHasNothingToAddForAnUnknownStatusOrAFailedLookup(t *testing.T) {
	if err := astroLocator(deploymentIn(astrov1.DeploymentStatusUNKNOWN)).WhyUnavailable(context.Background(), astroInstance()); err != nil {
		t.Errorf("unknown: err = %v, want nil", err)
	}
	offline := astroLocator(func(context.Context, string, string) (*astrov1.GetDeploymentResponse, error) {
		return nil, errors.New("dial tcp: no such host")
	})
	if err := offline.WhyUnavailable(context.Background(), astroInstance()); err != nil {
		t.Errorf("offline: err = %v, want nil", err)
	}
}
