package instances

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Deps is what the network-touching half of resolution needs from the process.
// Every field is a seam, so the logic stays testable and the layer rules hold
// (docs/v2-architecture.md).
//
// The seams are not all the same kind of thing. Astro's session and the
// coordinate lookups live behind a seam because reading them touches config/
// and the cloud clients, which this layer may not import — the implementations
// sit in internal/astrosession and internal/instancelocate. The AWS and Google
// chains are different: their SDKs touch nothing this layer is barred from, so
// nil means "ask the SDK", and the seam exists only so a test can answer
// without an account.
//
// Those two are also the only methods that need a Provider before any of this
// is consulted: a build carrying neither refuses them by name instead. See
// providers.go.
type Deps struct {
	// Session hands back the current Astro login's bearer token for the astro
	// auth method, or an error naming why it cannot — logged out, expired,
	// offline. nil means no session is wired, which reads as logged out.
	Session func(ctx context.Context) (string, error)
	// LookupEnv reads an env var. nil uses the process environment.
	LookupEnv func(name string) (string, bool)
	// Locator resolves the base URL of a link whose coordinates must be looked
	// up — an astro deployment's web server, a Composer environment's Airflow
	// URI. nil means no lookup is wired, which only a coordinate link needs.
	Locator Locator
	// HTTPClient carries every request, including the local token mint. nil
	// uses the transport's own client.
	HTTPClient *http.Client
	// AWSConfig loads the AWS SDK's configuration for a region: the credential
	// chain, and the region itself when the manifest names none. nil uses the
	// SDK's own loader, which is the whole chain — environment, profile, SSO,
	// credential_process, instance role. It is a seam so the MWAA door can be
	// driven against a stub without an AWS account.
	AWSConfig func(ctx context.Context, region string) (aws.Config, error)
	// GoogleToken hands back an Application Default Credentials access token,
	// and GoogleAccount names the principal it speaks for — the fact that turns
	// a Composer 403 from a shrug into a fix. nil on either asks the Google
	// SDK's own chain, which is what a real run wants.
	GoogleToken   func(ctx context.Context) (string, error)
	GoogleAccount func(ctx context.Context) string
	// Providers is which auth methods this build can perform beyond the six
	// that need nothing. The zero value performs only those six, which is
	// right for a build that talks to neither platform; see providers.go for
	// why this is a field rather than a registry.
	Providers Providers
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

// baseHTTPClient is the client to build on when this package needs one of its
// own — the MWAA web session, which needs a cookie jar the airflowapi options
// cannot install.
func (d Deps) baseHTTPClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return airflowapi.DefaultHTTPClient()
}

// TargetString reads one string field of this instance's
// [tool.astro.targets.<target>] section. A section that is absent, or a field
// that is, reads as the empty string rather than an error: whether a backend
// can do without is the backend's own question — an MWAA region can also come
// from the AWS credential chain, while a Composer project cannot come from
// anywhere else.
//
// It is exported because the Composer lookup lives outside this package and
// reads the same section; two readers of one manifest table would otherwise
// give two different sentences for the same mistake.
func (i Instance) TargetString(field string) (string, error) {
	raw, ok := i.TargetConfig[field]
	if !ok {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("[tool.astro.targets.%s] %s must be a string, not %T", i.Link.Target, field, raw)
	}
	return value, nil
}

// Transport opens the door to an instance. It is the one step that can reach
// the network, and a command calls it once.
//
// The auth method picks the door, not just the credential: every method here
// speaks HTTP to an Airflow URL, but aws does not — MWAA's InvokeRestApi wraps
// the request in a signed AWS call with no URL in sight, so it is dispatched
// before anything looks a URL up.
func (i Instance) Transport(ctx context.Context, d Deps) (airflowapi.Transport, error) {
	method := i.authMethod()
	// Before anything else, including the coordinate lookup below. A build that
	// cannot perform this method should say so, not fail at whatever the
	// Locator reaches first — which for a Composer link is a network call, and
	// which is how an earlier draft of this made the refusal unreachable for
	// every real Composer deployment.
	if err := d.checkProvider(i.Name, method); err != nil {
		return nil, err
	}
	if p, ok := d.provider(method); ok && p.Transport != nil {
		return p.Transport(ctx, i, d)
	}
	// Everything else speaks HTTP to an Airflow URL, including the two
	// instances no manifest declares: a local Airflow and a --url target.
	return i.httpTransport(ctx, d)
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
	creds, refresh, err := credentials(ctx, i, baseURL, d)
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

// HTTPDoor is an Airflow reachable over plain HTTP: where it answers, and the
// Authorization header value that proves the caller to it. Authorization is
// empty when this instance sends no credential at all, which is a real answer —
// an open dev server.
type HTTPDoor struct {
	BaseURL       string
	Authorization string
}

// ErrNotHTTP reports an instance whose door is not HTTP to an Airflow URL.
// MWAA under InvokeRestApi is the case: the request travels inside a signed AWS
// API call, and there is no URL to point an HTTP client at. Reach it through
// Transport.
var ErrNotHTTP = errors.New("this deployment is not reached over HTTP: its requests travel inside a signed AWS API call, so there is no Airflow URL to address")

// HTTPDoorFor resolves an instance to a base URL and an Authorization header
// value, running the credential source once.
//
// It exists for one caller: `astro api airflow`, whose request machinery
// predates this package and builds its own HTTP requests — it generates curl
// commands, paginates by rewriting the query string, and traces the wire — so a
// Transport, which deliberately hides both halves, cannot serve it. New code
// takes a Transport. The credential is resolved once here rather than per
// request, which is the cost of handing it over as a string; one command run is
// short enough that a token minted at the start is still good at the end.
func (i Instance) HTTPDoorFor(ctx context.Context, d Deps) (HTTPDoor, error) {
	method := i.authMethod()
	if method == manifest.AuthAWS {
		return HTTPDoor{}, ErrNotHTTP
	}
	// Before the lookup, for the same reason Transport does it there.
	if err := d.checkProvider(i.Name, method); err != nil {
		return HTTPDoor{}, err
	}
	baseURL, err := i.baseURL(ctx, d)
	if err != nil {
		return HTTPDoor{}, err
	}
	creds, _, err := credentials(ctx, i, baseURL, d)
	if err != nil {
		return HTTPDoor{}, err
	}
	if creds == nil {
		return HTTPDoor{BaseURL: baseURL}, nil
	}
	scheme, value, err := creds(ctx)
	if err != nil {
		return HTTPDoor{}, err
	}
	if scheme == "" {
		return HTTPDoor{BaseURL: baseURL}, nil
	}
	return HTTPDoor{BaseURL: baseURL, Authorization: scheme + " " + value}, nil
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
		// Only reachable when a caller builds Deps by hand and leaves the
		// lookup out; the command tree wires it once, in its composition root.
		return "", fmt.Errorf("cannot reach %q: no lookup is wired for the Airflow URL of an %s link", i.Name, i.Kind)
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
