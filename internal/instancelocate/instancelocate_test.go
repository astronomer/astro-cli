package instancelocate

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/instances"
	"github.com/astronomer/astro-cli/pkg/manifest"
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
