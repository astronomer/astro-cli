package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
	astrocontext "github.com/astronomer/astro-cli/context"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/instancelocate"
	"github.com/astronomer/astro-cli/internal/instances"
	"github.com/astronomer/astro-cli/pkg/airflowapi"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// This file is `astro api airflow`'s targeting: which Airflow one run talks to.
//
// The command used to have its own answer — a localhost default, --api-url, and
// an Astro deployment lookup behind --deployment-id — and its own hand-rolled
// version probe and token mint. It now shares the one the query commands use
// (docs/v2-instances.md): -d/--deployment names a deployment link the project's
// manifest declares, --url reaches an Airflow no project declares, and both
// resolve through internal/instances, so an MWAA, Composer, or token-minting
// Airflow is reachable here for free. The old flags stay as deprecated aliases
// for one release.
//
// The localhost default stays for a bare `astro api airflow /dags` — it is this
// command's documented behavior — but it goes through the same primitives as
// everything else: pkg/airflowapi's TokenMinter for the credential, and the
// client's own generation detection for the version.

// airflowTarget is the Airflow one run acts on: how to reach it, and what to
// call it in a message.
type airflowTarget struct {
	// name is the target as a reader would say it: a link name, a URL, a
	// deployment id.
	name string
	// hostRoot is the Airflow's base URL below any /api/vN prefix. It is empty
	// for a door that is not HTTP — MWAA under InvokeRestApi — which is the one
	// case this command's own request machinery cannot address, because there is
	// no URL to build a request or a curl command against.
	hostRoot string
	// authorization is the Authorization header value, scheme included. Empty
	// sends no credential, which is a real answer for an open dev server.
	authorization string
	// transport is the door itself. Every target has one; for an HTTP door it is
	// built from the two fields above, and it is what the version probe and the
	// non-HTTP passthrough go through.
	transport airflowapi.Transport
	// named says a deployment was asked for by name, rather than an Airflow
	// addressed by URL. See the note on the named helper below.
	named bool
	// airflow is the client over the transport, built once by client().
	airflow *airflowapi.Client
}

// isHTTP reports whether this target can be addressed by URL, which is what the
// request machinery below needs.
func (t *airflowTarget) isHTTP() bool { return t.hostRoot != "" }

// isNamedDeployment reports whether a deployment was named. See the note on
// named.
func (t *airflowTarget) isNamedDeployment() bool { return t.named }

// apiBase is the URL a request is built against: the host root with the
// generation's prefix on it. Empty for a target that is not addressed by URL.
func (t *airflowTarget) apiBase(version string) string {
	if t.hostRoot == "" {
		return ""
	}
	return t.hostRoot + apiPrefixForVersion(version)
}

// client wraps the target's door in the generation-adaptive client, which is
// where the version probe this command used to hand-roll now lives. One client
// per run, because it caches the generation it detected — a second one would
// probe the instance all over again.
func (t *airflowTarget) client() *airflowapi.Client {
	if t.airflow == nil {
		t.airflow = airflowapi.New(t.transport)
	}
	return t.airflow
}

// resolveAirflowTarget settles which Airflow this run talks to, in the order the
// flags rank: --url (or its --api-url alias), then -d/--deployment (or its
// --deployment-id alias), then the localhost default.
//
// Unlike the query commands there is no ambient layer here — no pin, no
// ASTRO_DEPLOYMENT. This command's documented default is localhost, and a raw
// API escape hatch that silently redirected itself at a deployment because a
// pin was set would be a worse surprise than typing -d.
func resolveAirflowTarget(ctx context.Context, opts *AirflowOptions) (*airflowTarget, error) {
	url, err := opts.targetURL()
	if err != nil {
		return nil, err
	}
	name, err := opts.targetDeployment()
	if err != nil {
		return nil, err
	}
	if url != "" && name != "" {
		// Name the flags the user actually typed. Half of these four spellings
		// are deprecated aliases, and telling someone who wrote --api-url and
		// --deployment-id to stop combining --url and --deployment sends them
		// looking for flags they never used.
		return nil, fmt.Errorf("%s and %s cannot be used together: %s names a deployment this project links, %s targets an Airflow with no name at all",
			opts.urlFlag(), opts.deploymentFlag(), opts.deploymentFlag(), opts.urlFlag())
	}

	switch {
	case name != "":
		return named(deploymentTarget(ctx, opts, name))
	case url != "":
		return opts.urlTarget(ctx, url, url)
	default:
		return opts.urlTarget(ctx, airflowLocalhost, "the Airflow on localhost")
	}
}

// named marks a deployment the user asked for by name. A deployment resolves
// through a control plane that says it exists, so it should be reachable; a URL
// — the localhost default included — is a guess someone typed, and may simply
// have nothing running behind it.
func named(t *airflowTarget, err error) (*airflowTarget, error) {
	if err != nil {
		return nil, err
	}
	t.named = true
	return t, nil
}

// airflowLocalhost is the bare command's target, below the API prefix — this
// command's documented default since it shipped.
const airflowLocalhost = "http://localhost:8080"

// targetURL folds --api-url into --url. The two mean the same thing, so passing
// both is a contradiction rather than a precedence question.
func (o *AirflowOptions) targetURL() (string, error) {
	if o.URL != "" && o.APIURL != "" && o.URL != o.APIURL {
		return "", errors.New("--url and --api-url are the same flag under two names (--api-url is deprecated): pass one")
	}
	// The alias carried an /api/vN suffix, because it named the API base rather
	// than the server. Strip it: the generation is detected, not typed.
	return airflowHostRoot(firstNonEmpty(o.URL, o.APIURL)), nil
}

// targetDeployment folds --deployment-id into --deployment. --deployment takes a
// link name and falls through to an id, so the deprecated flag's values keep
// working under the new name.
func (o *AirflowOptions) targetDeployment() (string, error) {
	if o.Deployment != "" && o.DeploymentID != "" && o.Deployment != o.DeploymentID {
		return "", errors.New("--deployment and --deployment-id are the same flag under two names (--deployment-id is deprecated): pass one")
	}
	return firstNonEmpty(o.Deployment, o.DeploymentID), nil
}

// urlFlag and deploymentFlag name the spelling this run actually used, so a
// message about two flags naming two targets names the two the reader typed.
func (o *AirflowOptions) urlFlag() string {
	if o.URL == "" && o.APIURL != "" {
		return "--api-url"
	}
	return "--url"
}

func (o *AirflowOptions) deploymentFlag() string {
	if o.Deployment == "" && o.DeploymentID != "" {
		return "--deployment-id"
	}
	return "--deployment"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// deploymentTarget resolves -d. The name is a deployment link the manifest
// declares; a name no link declares is an Astro Deployment id, which is what
// --deployment-id always meant and what CI already passes.
func deploymentTarget(ctx context.Context, opts *AirflowOptions, name string) (*airflowTarget, error) {
	var known []string
	if m, err := manifest.Load(filepath.Join(config.WorkingPath, manifest.Marker)); err == nil {
		set := instances.Build(m)
		if instance, ok := set.Lookup(name); ok {
			return linkTarget(ctx, opts, &instance)
		}
		known = set.Names()
	}
	baseURL, token, err := resolveDeploymentAirflowURL(opts, name)
	if err != nil {
		// A name that matched no link is far more often a typo than an id, so
		// the names it could have been travel with the failure.
		if len(known) > 0 {
			return nil, fmt.Errorf("%w\nDeployments this project links: %s", err, strings.Join(known, ", "))
		}
		return nil, err
	}
	return opts.httpTarget(name, airflowHostRoot(baseURL), bearerHeader(token))
}

// bearerHeader makes a whole Authorization header value out of the session
// token, which the config stores with its scheme on some machines and without
// it on others. Astro takes a bearer either way — this is the same normalizing
// internal/instances does for the same token.
func bearerHeader(token string) string {
	if token == "" || strings.Contains(token, " ") {
		return token
	}
	return "Bearer " + token
}

// linkTarget opens the door a manifest link describes, through the same
// resolution the query commands use. An MWAA link comes back with no URL at
// all: its requests travel inside a signed AWS call, so the target carries the
// transport and nothing else.
func linkTarget(ctx context.Context, opts *AirflowOptions, instance *instances.Instance) (*airflowTarget, error) {
	deps := opts.instanceDeps()
	door, err := instance.HTTPDoorFor(ctx, deps)
	if err == nil {
		// Strip any /api/vN the link's own url carries, the same as the --url and
		// id paths do. Every one of the three feeds hostRoot, and the generation
		// prefix is put back on top of it — a link written
		// `url = 'https://airflow.corp.dev/api/v1'`, or a control plane whose
		// WebServerAirflowApiUrl already ends in /api/v2, would otherwise be
		// addressed at /api/v1/api/v2/dags.
		return opts.httpTarget(instance.Name, airflowHostRoot(door.BaseURL), door.Authorization)
	}
	if !errors.Is(err, instances.ErrNotHTTP) {
		return nil, err
	}
	transport, err := instance.Transport(ctx, deps)
	if err != nil {
		return nil, err
	}
	return &airflowTarget{name: instance.Name, transport: transport}, nil
}

// instanceDeps hands resolution what it needs from the process: the login and
// the coordinate lookups. cmd/api is a v1 package, so it wires the two
// implementations directly rather than through a seam the way cmd/local has to.
//
// The Google chain rides along when the lookup exposes one, so a Composer link
// resolves its URL and proves itself to the Airflow behind it through the same
// credentials.
func (o *AirflowOptions) instanceDeps() instances.Deps {
	locator := instancelocate.New(astrov1.NewV1Client(httputil.NewHTTPClient()))
	deps := instances.Deps{
		Session:    astrosession.Bearer,
		Locator:    locator,
		HTTPClient: o.GetHTTPClient(),
	}
	if chain, ok := locator.(instancelocate.GoogleChain); ok {
		deps.GoogleToken, deps.GoogleAccount = chain.Google()
	}
	return deps
}

// urlTarget opens a bare Airflow URL: the localhost default and --url. Neither
// is declared anywhere, so the credential is minted from the username and
// password this command has always taken — through pkg/airflowapi's TokenMinter,
// which asks the instance's own /auth/token and falls back to basic auth on an
// Airflow that serves none.
func (o *AirflowOptions) urlTarget(ctx context.Context, hostRoot, name string) (*airflowTarget, error) {
	// A caller who set their own Authorization header has said how to prove
	// themselves; minting a second credential would only overwrite it.
	if o.hasAuthorizationHeader() {
		return o.httpTarget(name, hostRoot, "")
	}
	token, err := o.mintToken(ctx, hostRoot)
	if err != nil {
		// An explicit --username or --password is a claim about how to log in,
		// so failing to log in is this command's failure.
		if o.CredentialsExplicit {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
		// Otherwise carry on unauthenticated: plenty of Airflows need no
		// credential, and the request itself reports a refusal far better than a
		// guess here would. A connection failure says nothing about auth, and the
		// request is about to report it properly, so it is not worth a warning.
		if !isConnectionError(err) {
			fmt.Fprintf(o.GetErrOut(), "Warning: could not fetch auth token (%v), continuing without authentication\n", err)
		}
		return o.httpTarget(name, hostRoot, "")
	}
	return o.httpTarget(name, hostRoot, token)
}

// mintToken exchanges the command's username and password for whatever this
// Airflow accepts: a JWT from its own /auth/token (Airflow 3), or basic auth on
// an Airflow that does not serve that endpoint (Airflow 2).
func (o *AirflowOptions) mintToken(ctx context.Context, hostRoot string) (string, error) {
	minter, err := airflowapi.NewTokenMinter(hostRoot, o.Username, o.Password,
		airflowapi.WithHTTPClient(o.GetHTTPClient()))
	if err != nil {
		return "", err
	}
	scheme, value, err := minter.Credentials(ctx)
	if err != nil || scheme == "" {
		return "", err
	}
	return scheme + " " + value, nil
}

func (o *AirflowOptions) hasAuthorizationHeader() bool {
	for _, h := range o.RequestHeaders {
		if strings.HasPrefix(strings.ToLower(h), "authorization:") {
			return true
		}
	}
	return false
}

// httpTarget builds the common target: a base URL, a credential, and the
// transport over both.
func (o *AirflowOptions) httpTarget(name, hostRoot, authorization string) (*airflowTarget, error) {
	opts := []airflowapi.HTTPOption{airflowapi.WithHTTPClient(o.GetHTTPClient())}
	if authorization != "" {
		scheme, value, _ := strings.Cut(authorization, " ")
		opts = append(opts, airflowapi.WithCredentials(
			func(context.Context) (string, string, error) { return scheme, value, nil }))
	}
	transport, err := airflowapi.NewHTTPTransport(hostRoot, opts...)
	if err != nil {
		return nil, err
	}
	// The transport's own URL, not the one handed in. An Astro Deployment's
	// WebServerAirflowApiUrl arrives with no scheme, and this command builds its
	// own requests rather than sending them through the transport — so taking
	// the raw string here produced "unsupported protocol scheme" on every
	// deployment, and a --generate curl nobody could paste.
	return &airflowTarget{name: name, hostRoot: transport.BaseURL(), authorization: authorization, transport: transport}, nil
}

// resolveDeploymentAirflowURL fetches the Airflow API URL from an Astro
// Deployment, for a -d value that names no link in the manifest.
func resolveDeploymentAirflowURL(opts *AirflowOptions, deploymentID string) (baseURL, authToken string, err error) {
	if !astrocontext.IsCloudContext() {
		return "", "", fmt.Errorf("no deployment link %q, and reaching a Deployment by id requires cloud context. Run 'astro login' to connect to Astro Cloud", deploymentID)
	}

	ctx, err := astrocontext.GetCurrentContext()
	if err != nil {
		return "", "", fmt.Errorf("getting current context: %w", err)
	}
	if ctx.Token == "" {
		return "", "", errors.New("not authenticated. Run 'astro login' to authenticate")
	}

	orgID := opts.OrganizationID
	if orgID == "" {
		orgID = ctx.Organization
	}
	if orgID == "" {
		return "", "", errors.New("organization ID not set. Use --organization-id or run 'astro organization switch'")
	}

	dep, err := deployment.GetDeploymentByID(orgID, deploymentID, astrov1.NewV1Client(httputil.NewHTTPClient()))
	if err != nil {
		return "", "", fmt.Errorf("fetching deployment: %w", err)
	}
	if dep.WebServerAirflowApiUrl == "" {
		return "", "", fmt.Errorf("deployment %s does not have an Airflow API URL configured", deploymentID)
	}

	airflowURL := dep.WebServerAirflowApiUrl
	if !strings.HasPrefix(airflowURL, "http://") && !strings.HasPrefix(airflowURL, "https://") {
		airflowURL = "https://" + airflowURL
	}
	return airflowURL, ctx.Token, nil
}
