// Package instancelocate answers the one question pkg/instances cannot
// answer for itself: where does a link's Airflow actually live?
//
// A link carries coordinates, not an address. An astro link names a Deployment
// id and its web server URL comes from the control plane; a Composer link
// names an environment and its Airflow URI comes from the Composer API. Both
// are lookups against a cloud, and both need credentials the CLI holds
// elsewhere — the login session for one, Application Default Credentials for
// the other.
//
// The Composer lookup itself lives in pkg/instancelocate now, because
// Astro Desktop needs that half and has its own Deployment lookup for the
// other. What stays here is the astro half, which reads the control plane
// through the generated client and the login context — both under internal/,
// so neither can cross a module boundary — and the switch that routes a kind
// to its lookup.
//
// It is its own package because of the layer rules (docs/v2-architecture.md):
// pkg/instances and cmd/local may not import config/ or the cloud
// clients, and this does both. internal/astrosession and internal/emenv sit
// outside the same list for the same reason. The command tree wires this into
// instances.Deps at its composition root; pkg/instances sees only the
// Locator interface.
//
// Nothing here prints. Every failure is a named outage — logged out, expired,
// no access, gone, offline — with the fix in the sentence, the posture
// internal/emenv holds for the same session.
package instancelocate

import (
	"context"
	"fmt"
	"net/http"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/googleauth"
	"github.com/astronomer/astro-cli/pkg/httputil"
	pkglocate "github.com/astronomer/astro-cli/pkg/instancelocate"
	"github.com/astronomer/astro-cli/pkg/instances"
)

// Deployments is the one control-plane call this package makes.
//
// It is named here rather than taken as the whole generated client because a
// consumer defines the interface it needs, and because stubbing one call
// should not mean implementing every endpoint Astro has. cloud/deployment's
// GetDeploymentByID would be the shorter route and is the wrong one: it runs
// the answer through NormalizeAPIError, which flattens away the very status
// the outage messages below branch on, and it hardcodes its own context.
type Deployments interface {
	GetDeploymentWithResponse(ctx context.Context, organizationID, deploymentID string, reqEditors ...astrov1.RequestEditorFn) (*astrov1.GetDeploymentResponse, error)
}

// locator resolves a coordinate link's Airflow base URL. Every field is a
// seam, so the lookups can be driven against stubs with no Astro login and no
// Google account.
type locator struct {
	// domain is the Astro host the project's login lives on, or empty for the
	// current context's. The lookup's own outages name it.
	domain string
	// deployments reads Astro Deployments on domain's control plane.
	deployments Deployments
	// session reports the login's bearer, or the named reason there is none.
	// It gates the deployment lookup so a logged-out machine is told what
	// happened rather than handed the API's own refusal.
	session func(ctx context.Context) (string, error)
	// organization is the org the session is scoped to. Deployments are read
	// under it, so a session without one cannot look anything up.
	organization func() (string, error)
	// httpClient carries the Composer lookup. nil in production, where
	// pkg/instancelocate supplies one with its own timeout; a test hands in
	// its stub server's.
	httpClient *http.Client
	// googleToken hands back an Application Default Credentials access token
	// for the Composer lookup, and googleAccount names the principal those
	// credentials speak for — which is what makes Composer's
	// over-long-service-account refusal identifiable rather than guessed at.
	// Both are handed on to pkg/instances as its own seams, so the lookup
	// and the Airflow calls after it are answered by the same chain.
	googleToken   func(ctx context.Context) (string, error)
	googleAccount func(ctx context.Context) string
	// composerEndpoint points the Composer lookup at a test's own server.
	// Always empty in production, where pkg/instancelocate supplies the public
	// API and the timeout that goes with it.
	composerEndpoint string
}

// New builds the production locator for a project whose Astro host is domain:
// its Deployment lookups go to that host's control plane under that host's
// login and organization, whatever the current context names. An empty domain
// is the current context's host. It does no I/O: every lookup happens when a
// command asks for one.
func New(domain string) instances.Locator {
	return &locator{
		domain:      domain,
		deployments: deploymentsOn(domain),
		session: func(ctx context.Context) (string, error) {
			return astrosession.BearerFor(ctx, domain)
		},
		organization:  func() (string, error) { return organization(domain) },
		googleToken:   googleauth.AccessToken,
		googleAccount: googleauth.Account,
	}
}

// deploymentsOn is the v1 client for domain's control plane. Its token is left
// empty because astroDeployment puts the session's bearer on every request.
func deploymentsOn(domain string) Deployments {
	if domain == "" {
		return astrov1.NewV1Client(httputil.NewHTTPClient())
	}
	c := config.Context{Domain: domain}
	return astrov1.NewV1ClientForLogin(httputil.NewHTTPClient(), "", c.GetPublicRESTAPIURL("v1"))
}

// GoogleChain is implemented by a locator that resolves Application Default
// Credentials. The composition root asks for it so it can hand
// pkg/instances the same chain the Composer lookup used, rather than
// letting the two halves of one command ask two different ones — a run that
// finds an environment it then cannot talk to is the failure that would cause.
//
// It is an optional interface rather than a field on Locator because a test
// that wires a bare lookup function has no chain to offer and should not have
// to invent one.
type GoogleChain interface {
	Google() (token func(ctx context.Context) (string, error), account func(ctx context.Context) string)
}

func (l *locator) Google() (token func(ctx context.Context) (string, error), account func(ctx context.Context) string) {
	return l.googleToken, l.googleAccount
}

// Diagnoser is implemented by a locator that can ask a control plane why a
// link's Airflow is not answering. The composition root asks for it only after
// a command has failed on such an answer, so a working command never pays for
// the extra lookup. It is optional for the reason GoogleChain is.
type Diagnoser interface {
	WhyUnavailable(ctx context.Context, i instances.Instance) error
}

// BaseURL resolves an instance's Airflow base URL. It is the instances.Locator
// seam, and it is only ever called for an instance whose URL is not already
// known — an endpoint link and a local Airflow answer for themselves.
func (l *locator) BaseURL(ctx context.Context, i instances.Instance) (string, error) {
	switch i.Kind {
	case instances.KindAstro:
		return l.astroBaseURL(ctx, i)
	case instances.KindComposer:
		// The same chain this hands pkg/instances, so the lookup and the
		// Airflow calls after it are answered by one set of credentials.
		return pkglocate.ComposerBaseURL(ctx, i, pkglocate.Options{
			Google: googleauth.Options{
				Token:   l.googleToken,
				Account: l.googleAccount,
			},
			HTTPClient: l.httpClient,
			Endpoint:   l.composerEndpoint,
		})
	case instances.KindMWAA:
		// The AWS API carries the request; there is no URL in that door.
		return "", fmt.Errorf("an MWAA environment has no Airflow URL to look up: it is reached through the AWS API")
	case instances.KindEndpoint, instances.KindLocal:
		return "", fmt.Errorf("%q already knows where its Airflow is; nothing to look up", i.Name)
	}
	return "", fmt.Errorf("cannot look up the Airflow URL of a %s deployment", i.Kind)
}

// organization reads the org out of the login for domain, or out of the
// current login context when domain is empty. It is the same read
// internal/emenv makes for the same reason.
// The organization is the piece ASTRO_API_TOKEN alone cannot supply: it lives
// in the login context, and a CI machine that never ran `astro login` has none.
// Reaching an astro link there needs a Deployment lookup, and the lookup needs
// an org, so the message says which half is missing rather than reporting the
// whole machine as logged out.
func organization(domain string) (string, error) {
	if domain == "" {
		ctx, err := config.GetCurrentContext()
		if err != nil {
			return "", fmt.Errorf("no Astro organization on this machine, and looking up a Deployment's URL needs one: run `astro login`, or set ASTRO_DOMAIN and log in once so the organization is on disk")
		}
		if ctx.Organization == "" {
			return "", fmt.Errorf("your login is not scoped to an organization — pick one with `astro organization switch`")
		}
		return ctx.Organization, nil
	}
	c := config.Context{Domain: domain}
	ctx, err := c.GetContext()
	if err != nil {
		return "", fmt.Errorf("no Astro organization for %s on this machine, and looking up a Deployment's URL needs one: run `astro login %s` once so the organization is on disk", domain, domain)
	}
	if ctx.Organization == "" {
		return "", fmt.Errorf("your %s login is not scoped to an organization — log in with `astro login %s` and pick one", domain, domain)
	}
	return ctx.Organization, nil
}
