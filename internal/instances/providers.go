package instances

import (
	"context"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Two of the eight auth methods cost far more than the other six.
//
// aws reaches MWAA through the AWS SDK's signed API and the whole credential
// chain behind LoadDefaultConfig; google reaches Composer through the
// application-default chain. Measured on their own, the two add about 7.7MB and
// seventeen modules to a binary. The other six are a bearer token, a username
// and password, a token exchange, a command to run, the Astro session, and
// nothing at all — stdlib and this repo.
//
// A second consumer is coming that talks to neither: Astro Desktop stores mwaa
// and composer links but never calls them, and ships a signed, notarized app to
// people who mostly have no AWS or GCP account. Go links what is imported, so a
// switch naming those two implementations decides that for everybody.
//
// So which methods a build can perform is a property of the build, and it
// arrives the way every other optional dependency in this tree arrives: on
// Deps, wired at the composition root beside Session, Locator and HTTPClient.
// Not a package-level registry with init() side effects — that would be the
// only non-test init() in the v2 source, it makes a forgotten blank import a
// run-time failure rather than a compile error, and a process-global map that
// resolution reads is a data race waiting for the first caller that registers
// outside init().
//
// # This is the dispatch, not yet the saving
//
// The implementations still live in this package, so both SDK chains are
// compiled and linked here today and leaving a provider unwired drops nothing.
// Two things still hold the dependency in: Deps.AWSConfig names aws.Config, so
// constructing a Deps at all pulls the SDK in; and google.go exports the ADC
// helpers that internal/instancelocate wires into the Composer lookup, which is
// a second door into the same chain. Moving those is the next change. What this
// one buys is that the decision has somewhere to live, and that a build without
// a provider refuses by name instead of behaving as though it had one.

// A CredentialProvider builds the credential source for one auth method, plus
// the refresh hook to install alongside it (nil when the credential cannot go
// stale mid-run).
//
// It takes a context because a credential can cost a bounded call — a keychain
// read, a metadata probe, the credential-chain walk the MWAA door already
// preflights under a timeout — and a provider with no way to observe
// cancellation would be the wrong shape to publish.
type CredentialProvider func(ctx context.Context, i Instance, baseURL string, d Deps) (airflowapi.CredentialSource, func(context.Context) error, error)

// A TransportProvider builds the round tripper for an auth method whose door is
// not an HTTP credential at all.
type TransportProvider func(ctx context.Context, i Instance, d Deps) (airflowapi.Transport, error)

// A Provider is how one auth method is performed: on the request, or by
// replacing the door.
//
// Exactly one of the two is set. Six methods prove themselves on an HTTP
// request and supply a credential; aws does not speak to an Airflow URL at all
// and supplies the whole transport, which is why the two cannot share a shape.
type Provider struct {
	Credentials CredentialProvider
	Transport   TransportProvider
}

// Providers maps an auth method to the implementation a build carries. The zero
// value performs only the six methods that need nothing, which is every build
// that talks to neither platform.
type Providers map[manifest.AuthMethod]Provider

// needsProvider lists the methods no build performs on its own.
//
// Consulted before a base URL is resolved, so a build without the provider says
// so rather than failing at whatever the coordinate lookup happens to hit
// first. That ordering is the whole difference between a legible refusal and a
// network error from an unrelated layer.
var needsProvider = map[manifest.AuthMethod]bool{
	manifest.AuthAWS:    true,
	manifest.AuthGoogle: true,
}

// provider returns the implementation for a method, and whether this build has
// one at all.
func (d Deps) provider(m manifest.AuthMethod) (Provider, bool) {
	p, ok := d.Providers[m]
	return p, ok
}

// checkProvider refuses a method this build cannot perform.
func (d Deps) checkProvider(name string, m manifest.AuthMethod) error {
	if !needsProvider[m] {
		return nil
	}
	p, ok := d.provider(m)
	if !ok || (p.Credentials == nil && p.Transport == nil) {
		return errNoProvider(name, m)
	}
	return nil
}

// viaProvider builds a credential through this build's provider for a method,
// refusing rather than passing on an empty one.
//
// The empty case matters as much as the missing one and is easier to write by
// accident: a provider that returns (nil, nil, nil) when the machine has no
// credential to offer — a natural reading of "nothing to add" — sends the
// request with no Authorization header at all. That arrives as a bare 401 from
// Airflow, which is precisely the "a credential the user got wrong" reading the
// refusal below exists to prevent.
func (d Deps) viaProvider(ctx context.Context, i Instance, baseURL string, m manifest.AuthMethod) (airflowapi.CredentialSource, func(context.Context) error, error) {
	p, ok := d.provider(m)
	if !ok || p.Credentials == nil {
		return nil, nil, errNoProvider(i.Name, m)
	}
	source, refresh, err := p.Credentials(ctx, i, baseURL, d)
	if err != nil {
		return nil, nil, err
	}
	if source == nil {
		return nil, nil, fmt.Errorf("the %s provider for %q returned no credential", m, i.Name)
	}
	return source, refresh, nil
}

// errNoProvider reports an auth method this build cannot perform, and names the
// way to reach the deployment anyway.
//
// A reachable state rather than a defensive one: a link can name any method the
// manifest's vocabulary has, whatever the program reading it can do. Naming the
// method and the way out both matter — this package's own rule is that a
// failure travels with its cause and its fix, and "not supported here" with
// neither leaves someone with a correctly configured deployment and nothing to
// try.
func errNoProvider(name string, m manifest.AuthMethod) error {
	return fmt.Errorf(
		"cannot reach %q here: it authenticates with %s, which this build does not carry — use the Astro CLI for it (`astro use %s`)",
		name, m, name,
	)
}
