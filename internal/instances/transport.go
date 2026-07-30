package instances

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Deps is what the network-touching half of resolution needs from the process.
// Every field is a seam: this package reads no config, no session file, and no
// SDK credential chain itself, so its logic stays testable and the layer rules
// hold (docs/v2-architecture.md).
type Deps struct {
	// Session hands back the current Astro login's bearer token for the astro
	// auth method, or an error naming why it cannot — logged out, expired,
	// offline. nil means no session is wired, which reads as logged out.
	Session func(ctx context.Context) (string, error)
	// LookupEnv reads an env var. nil uses the process environment.
	LookupEnv func(name string) (string, bool)
	// Locator resolves the base URL of a link whose coordinates must be looked
	// up. nil until the query commands wire it.
	Locator Locator
	// HTTPClient carries every request, including the local token mint. nil
	// uses the transport's own client.
	HTTPClient *http.Client
}

// Locator turns a link's coordinates into an Airflow base URL — an astro
// deployment's web server, a Composer environment's Airflow URI. It is an
// interface because each lookup speaks to its own control plane, and because
// resolution must stay testable without any of them.
type Locator interface {
	// BaseURL returns the Airflow base URL for an instance whose URL field is
	// empty.
	BaseURL(ctx context.Context, i Instance) (string, error)
}

func (d Deps) lookupEnv(name string) (string, bool) {
	if d.LookupEnv != nil {
		return d.LookupEnv(name)
	}
	return os.LookupEnv(name)
}

// httpOptions carries the process's HTTP client into an airflowapi call, when
// one was handed in. Every call this package makes — the transport and the
// local token mint — goes through the same client.
func (d Deps) httpOptions() []airflowapi.HTTPOption {
	if d.HTTPClient == nil {
		return nil
	}
	return []airflowapi.HTTPOption{airflowapi.WithHTTPClient(d.HTTPClient)}
}

// Transport opens the door to an instance. It is the one step that can reach
// the network, and a command calls it once.
//
// The auth method picks the door, not just the credential: every method here
// speaks HTTP to an Airflow URL, but aws does not — MWAA's InvokeRestApi wraps
// the request in a signed AWS call with no URL in sight. Dispatching on the
// method now means that door arrives as another case rather than a rewrite.
func (i Instance) Transport(ctx context.Context, d Deps) (airflowapi.Transport, error) {
	switch i.authMethod() {
	case manifest.AuthAWS:
		return nil, &NotImplementedError{What: "AWS API door an MWAA environment is reached through", Issue: authIssue}
	case manifest.AuthAstro, manifest.AuthGoogle, manifest.AuthBasic, manifest.AuthToken,
		manifest.AuthAirflowToken, manifest.AuthExec, manifest.AuthNone:
		return i.httpTransport(ctx, d)
	default:
		// The empty method: a local Airflow or a --url target, neither of which
		// a manifest declares. Both speak HTTP.
		return i.httpTransport(ctx, d)
	}
}

// authMethod is how this instance proves itself: the link's method, or — for
// the two instances no manifest declares — what that kind does. A local
// Airflow mints against itself; a --url target reads the environment.
func (i Instance) authMethod() manifest.AuthMethod {
	if i.Source == SourceManifest {
		return i.Link.Auth.Method
	}
	return ""
}

// httpTransport is the common door: a base URL plus a credential source that
// runs per request and stores nothing.
func (i Instance) httpTransport(ctx context.Context, d Deps) (airflowapi.Transport, error) {
	baseURL, err := i.baseURL(ctx, d)
	if err != nil {
		return nil, err
	}
	creds, refresh, err := credentials(i, baseURL, d)
	if err != nil {
		return nil, err
	}
	opts := d.httpOptions()
	if creds != nil {
		opts = append(opts, airflowapi.WithCredentials(creds))
	}
	if refresh != nil {
		opts = append(opts, airflowapi.WithRefresh(refresh))
	}
	return airflowapi.NewHTTPTransport(baseURL, opts...)
}

// baseURL is where the instance's Airflow answers. An endpoint link and a
// local Airflow already know; a coordinate link has to ask its control plane,
// which is the Locator's job.
func (i Instance) baseURL(ctx context.Context, d Deps) (string, error) {
	switch {
	case i.URL != "":
		return i.URL, nil
	case i.Kind == KindLocal:
		// A local instance with no URL means the record carried no port, which
		// only happens if the runtime record was written by a build that did
		// not record one.
		return "", fmt.Errorf("the local Airflow for %s records no port; restart it with `astro local restart`", i.Project)
	}
	if d.Locator == nil {
		return "", &NotImplementedError{What: fmt.Sprintf("lookup of an %s link's Airflow URL (needed by %q)", i.Kind, i.Name), Issue: authIssue}
	}
	url, err := d.Locator.BaseURL(ctx, i)
	if err != nil {
		return "", err
	}
	if url == "" {
		return "", fmt.Errorf("cannot reach %q: its %s has no Airflow URL", i.Name, i.Where)
	}
	return url, nil
}
