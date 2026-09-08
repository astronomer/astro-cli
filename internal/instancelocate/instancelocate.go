// Package instancelocate answers the one question internal/instances cannot
// answer for itself: where does a link's Airflow actually live?
//
// A link carries coordinates, not an address. An astro link names a Deployment
// id and its web server URL comes from the control plane; a Composer link
// names an environment and its Airflow URI comes from the Composer API. Both
// are lookups against a cloud, and both need credentials the CLI holds
// elsewhere — the login session for one, Application Default Credentials for
// the other.
//
// It is its own package because of the layer rules (docs/v2-architecture.md):
// internal/instances and cmd/local may not import config/ or the cloud
// clients, and this does both. internal/astrosession and internal/emenv sit
// outside the same list for the same reason. The command tree wires this into
// instances.Deps at its composition root; internal/instances sees only the
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
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/internal/instances/googleauth"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
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

// lookupTimeout bounds one coordinate lookup. It is a single small GET against
// a cloud API, and a command should not sit on it.
const lookupTimeout = 30 * time.Second

// locator resolves a coordinate link's Airflow base URL. Every field is a
// seam, so the lookups can be driven against stubs with no Astro login and no
// Google account.
type locator struct {
	// deployments reads Astro Deployments. It authenticates from the current
	// login on every request, the same client internal/emenv reads Environment
	// Manager through.
	deployments Deployments
	// session reports the current login's bearer, or the named reason there is
	// none. It gates the deployment lookup so a logged-out machine is told what
	// happened rather than handed the API's own refusal.
	session func(ctx context.Context) (string, error)
	// organization is the org the session is scoped to. Deployments are read
	// under it, so a session without one cannot look anything up.
	organization func() (string, error)
	// httpClient carries the Composer lookup.
	httpClient *http.Client
	// googleToken hands back an Application Default Credentials access token
	// for the Composer lookup, and googleAccount names the principal those
	// credentials speak for — which is what makes Composer's
	// over-long-service-account refusal identifiable rather than guessed at.
	// Both are handed on to internal/instances as its own seams, so the lookup
	// and the Airflow calls after it are answered by the same chain.
	googleToken   func(ctx context.Context) (string, error)
	googleAccount func(ctx context.Context) string
	// composerEndpoint is the Composer API base URL.
	composerEndpoint string
}

// New builds the production locator over the v1 API client the process already
// holds. It does no I/O: every lookup happens when a command asks for one.
func New(deployments Deployments) instances.Locator {
	return &locator{
		deployments:      deployments,
		session:          astrosession.Bearer,
		organization:     currentOrganization,
		httpClient:       &http.Client{Timeout: lookupTimeout},
		googleToken:      googleauth.AccessToken,
		googleAccount:    googleauth.Account,
		composerEndpoint: composerAPI,
	}
}

// GoogleChain is implemented by a locator that resolves Application Default
// Credentials. The composition root asks for it so it can hand
// internal/instances the same chain the Composer lookup used, rather than
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

// BaseURL resolves an instance's Airflow base URL. It is the instances.Locator
// seam, and it is only ever called for an instance whose URL is not already
// known — an endpoint link and a local Airflow answer for themselves.
func (l *locator) BaseURL(ctx context.Context, i instances.Instance) (string, error) {
	switch i.Kind {
	case instances.KindAstro:
		return l.astroBaseURL(ctx, i)
	case instances.KindComposer:
		return l.composerBaseURL(ctx, i)
	case instances.KindMWAA:
		// The AWS API carries the request; there is no URL in that door.
		return "", fmt.Errorf("an MWAA environment has no Airflow URL to look up: it is reached through the AWS API")
	case instances.KindEndpoint, instances.KindLocal:
		return "", fmt.Errorf("%q already knows where its Airflow is; nothing to look up", i.Name)
	}
	return "", fmt.Errorf("cannot look up the Airflow URL of a %s deployment", i.Kind)
}

// currentOrganization reads the org out of the current login context. It is
// the one place this package touches config/, and it is the same read
// internal/emenv makes for the same reason.
// The organization is the piece ASTRO_API_TOKEN alone cannot supply: it lives
// in the login context, and a CI machine that never ran `astro login` has none.
// Reaching an astro link there needs a Deployment lookup, and the lookup needs
// an org, so the message says which half is missing rather than reporting the
// whole machine as logged out.
func currentOrganization() (string, error) {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return "", fmt.Errorf("no Astro organization on this machine, and looking up a Deployment's URL needs one: run `astro login`, or set ASTRO_DOMAIN and log in once so the organization is on disk")
	}
	if ctx.Organization == "" {
		return "", fmt.Errorf("your login is not scoped to an organization — pick one with `astro organization switch`")
	}
	return ctx.Organization, nil
}
