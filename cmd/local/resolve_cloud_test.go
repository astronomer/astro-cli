package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// cloudManifest is the inventory a team on three clouds commits: an Astro
// Deployment, an MWAA environment, a Composer environment, and one hand-rolled
// Airflow behind a token. Not one credential is in the file.
const cloudManifest = `
[tool.astro.deployments.prod]
deployment = 'clm2xk9dq000108l7a2b3c4d5'
default = true

[tool.astro.deployments.prod-mwaa]
target = 'mwaa'
environment = 'orders-prod'

[tool.astro.deployments.prod-composer]
target = 'composer'
environment = 'orders-composer'

[tool.astro.deployments.staging]
url = 'https://airflow.staging.corp.dev'
auth = { method = 'token', token-env = 'STAGING_AIRFLOW_TOKEN' }

[tool.astro.targets.mwaa]
region = 'us-east-1'

[tool.astro.targets.composer]
project = 'acme-data'
location = 'us-central1'
`

// TestUseListsEveryCloudLink: the inventory renders whole, with each kind's own
// coordinate and the auth method that kind defaults to, and not one network
// call is made to build it.
func TestUseListsEveryCloudLink(t *testing.T) {
	dir := instanceProject(t, cloudManifest)
	d, out, _ := instanceDeps(t, dir)
	d.Locator = anyDomain(failingLocator{t})
	if err := execute(t, d, "use", "--output", "json"); err != nil {
		t.Fatal(err)
	}

	var report struct {
		Deployments []map[string]any `json:"deployments"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	byName := map[string]map[string]any{}
	for _, row := range report.Deployments {
		byName[row["name"].(string)] = row
	}
	want := map[string][2]string{
		"prod":          {"astro", "astro"},
		"prod-mwaa":     {"mwaa", "aws"},
		"prod-composer": {"composer", "google"},
		"staging":       {"endpoint", "token"},
	}
	for name, kindAndAuth := range want {
		row, ok := byName[name]
		if !ok {
			t.Fatalf("%s is missing from the inventory: %v", name, byName)
		}
		if row["kind"] != kindAndAuth[0] || row["auth_method"] != kindAndAuth[1] {
			t.Errorf("%s = kind %v, auth %v; want %v", name, row["kind"], row["auth_method"], kindAndAuth)
		}
	}
	// A coordinate link shows as the manifest writes it, with no url filled
	// in: nothing was looked up to render this.
	if byName["prod-composer"]["where"] != "orders-composer" {
		t.Errorf("where = %v, want the coordinate as written", byName["prod-composer"]["where"])
	}
	if _, resolved := byName["prod-mwaa"]["url"]; resolved {
		t.Error("listing resolved an MWAA environment's URL")
	}
}

// TestUseAnnouncesEachCloudKind: the one stderr line every resolving command
// prints has to say what a name points at, whichever cloud it is on.
func TestUseAnnouncesEachCloudKind(t *testing.T) {
	for name, want := range map[string]string{
		"prod":          "→ prod (astro deployment clm2xk9dq000108l7a2b3c4d5)",
		"prod-mwaa":     "→ prod-mwaa (mwaa environment orders-prod)",
		"prod-composer": "→ prod-composer (composer environment orders-composer)",
		"staging":       "→ staging (endpoint https://airflow.staging.corp.dev)",
	} {
		dir := instanceProject(t, cloudManifest)
		d, _, errOut := instanceDeps(t, dir)
		d.Locator = anyDomain(failingLocator{t})
		if err := execute(t, d, "use", name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("%s: stderr = %q, want %q", name, errOut, want)
		}
	}
}

// TestAstroLinkTalksToTheAirflowTheLookupFound is the wiring this PR closes:
// the pinned Deployment's URL comes from the locator the command tree wires in,
// and the request lands there under the session's bearer.
func TestAstroLinkTalksToTheAirflowTheLookupFound(t *testing.T) {
	var landed, auth string
	airflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		landed, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":0}`))
	}))
	defer airflow.Close()

	dir := instanceProject(t, cloudManifest)
	d, _, _ := instanceDeps(t, dir)
	d.Session = func(context.Context, string) (string, error) { return "Bearer session-token", nil }
	asked := ""
	d.Locator = anyDomain(locatorFunc(func(_ context.Context, i instances.Instance) (string, error) {
		asked = i.Link.Deployment
		return airflow.URL, nil
	}))

	c := &cli{d: d}
	client, err := c.deploymentClient(context.Background(), deploymentFlags{deployment: "prod"})
	if err != nil {
		t.Fatalf("deploymentClient: %v", err)
	}
	if _, err := client.Do(context.Background(), airflowapi.Request{Path: "/dags"}); err != nil {
		t.Fatalf("do: %v", err)
	}
	if asked != "clm2xk9dq000108l7a2b3c4d5" {
		t.Fatalf("the lookup was asked about %q", asked)
	}
	if !strings.HasSuffix(landed, "/dags") || auth != "Bearer session-token" {
		t.Fatalf("request landed at %q with %q", landed, auth)
	}
}

// TestComposerLinkCarriesTheLookupsFailure: a lookup that cannot answer ends
// the command with its own named cause, rather than a URL-less request.
func TestComposerLinkCarriesTheLookupsFailure(t *testing.T) {
	dir := instanceProject(t, cloudManifest)
	d, _, _ := instanceDeps(t, dir)
	d.Locator = anyDomain(locatorFunc(func(context.Context, instances.Instance) (string, error) {
		return "", googleauth.ErrNoCredentials
	}))

	c := &cli{d: d}
	_, err := c.deploymentClient(context.Background(), deploymentFlags{deployment: "prod-composer"})
	if err == nil || !strings.Contains(err.Error(), "gcloud auth application-default login") {
		t.Fatalf("err = %v, want the lookup's own cause", err)
	}
}

// TestAstroLinkUsesTheProjectsDomain: a project that names its Astro host has
// its astro links looked up and proven with that host's login, and one that
// names none leaves the choice to the current context.
func TestAstroLinkUsesTheProjectsDomain(t *testing.T) {
	airflow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_entries":0}`))
	}))
	defer airflow.Close()

	for _, tc := range []struct{ name, manifest, env, want string }{
		{"manifest", "domain = 'https://cloud.astronomer-dev.io'\n" + cloudManifest, "astronomer-stage.io", "astronomer-dev.io"},
		{"ASTRO_DOMAIN", cloudManifest, "astronomer-stage.io", "astronomer-stage.io"},
		{"neither", cloudManifest, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ASTRO_DOMAIN", tc.env)
			dir := instanceProject(t, tc.manifest)
			d, _, _ := instanceDeps(t, dir)
			sessionDomain, locatorDomain := "unset", "unset"
			d.Session = func(_ context.Context, domain string) (string, error) {
				sessionDomain = domain
				return "Bearer t", nil
			}
			d.Locator = func(domain string) instances.Locator {
				locatorDomain = domain
				return locatorFunc(func(context.Context, instances.Instance) (string, error) { return airflow.URL, nil })
			}

			c := &cli{d: d}
			client, err := c.deploymentClient(context.Background(), deploymentFlags{deployment: "prod"})
			if err != nil {
				t.Fatalf("deploymentClient: %v", err)
			}
			if _, err := client.Do(context.Background(), airflowapi.Request{Path: "/dags"}); err != nil {
				t.Fatalf("do: %v", err)
			}
			if sessionDomain != tc.want || locatorDomain != tc.want {
				t.Fatalf("session asked for %q, lookup for %q; want %q", sessionDomain, locatorDomain, tc.want)
			}
		})
	}
}

// anyDomain is a lookup seam that answers with l whatever the project's host.
func anyDomain(l instances.Locator) func(string) instances.Locator {
	return func(string) instances.Locator { return l }
}

// locatorFunc adapts a function to the lookup seam.
type locatorFunc func(ctx context.Context, i instances.Instance) (string, error)

func (f locatorFunc) BaseURL(ctx context.Context, i instances.Instance) (string, error) {
	return f(ctx, i)
}

// failingLocator fails the test if it is called: the display commands promise
// to touch no network, and the way to hold them to it is to make a lookup an
// error.
type failingLocator struct{ t *testing.T }

func (l failingLocator) BaseURL(context.Context, instances.Instance) (string, error) {
	l.t.Error("a display command looked up a coordinate over the network")
	return "", nil
}
