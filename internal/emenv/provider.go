// Package emenv is the Environment Manager read-through provider for local
// env resolution. A declared name with `source = "workspace"` in the manifest
// resolves from the project's workspace Environment Manager objects — the
// team-shared tier — when the user is logged in; offline or logged out, the
// provider is silently absent and the name falls back to the local files, or
// is reported missing with a message that names the cause.
//
// The provider implements envresolve.Provider (Lookup/Label) plus the optional
// envresolve.Diagnoser (the cause behind a miss). One provider serves a run:
// its sync.Once fetch is shared across every workspace-source name, and the
// result is held in memory only — Environment Manager values are never written
// to disk.
//
// Scope: plain env vars, Airflow variables and connections, each indexed under
// the Airflow env-var key the local chain reads. A connection is re-encoded
// from its structured fields into the AIRFLOW_CONN_<id> JSON the local tiers
// store, through the same pkg/airflowenv codec Astro Desktop uses, so a
// workspace connection resolves to the same value whichever app starts
// Airflow. Metrics exports are a deployment telemetry concern and stay out.
package emenv

import (
	httpcontext "context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/airflowenv"
	"github.com/astronomer/astro-cli/pkg/connmodel"
	"github.com/astronomer/astro-cli/pkg/emfetch"
)

// sourceLabel is the source name a workspace-resolved value reports.
const sourceLabel = "workspace"

// login reads the stored login for a domain, refreshing a stale one. A var so
// a test can stand in for the refresh.
var login = astrosession.Login

// provider resolves declared env values for one workspace from Environment
// Manager. It fetches lazily on the first Lookup and caches the result for the
// run.
type provider struct {
	workspaceID string
	// domain is the Astro host the workspace lives on, from the manifest. It
	// picks the stored login the read uses, whatever host the CLI's current
	// context names — docs/v2-workspace-link.md.
	domain    string
	clientFor ClientFactory
	client    astrov1.APIClient // built in load, from the domain's login
	reveal    bool              // ask for secret values (start/get); false is presence-only (list)

	once    sync.Once
	objects map[string]objectValue
	// secretsIncluded records whether the fetch that filled objects asked for
	// secret values and was not refused, which is what separates "the org
	// withheld this" from "the platform holds no value for it".
	secretsIncluded bool
	// down, when set, is the whole-provider failure: a short reason for the
	// list label and a longer cause for the missing-value message.
	down *outage
}

// objectValue is one env-var-keyed Environment Manager object, indexed by its
// objectKey (already the Airflow env-var key). value is empty for a secret the
// fetch did not or could not reveal.
type objectValue struct {
	value    string
	isSecret bool
}

// outage is a whole-provider failure.
type outage struct {
	short string // list label suffix, e.g. "logged out"
	cause string // the missing-value message cause
}

// Lookup reports the value under an Airflow env-var key, and whether the
// workspace holds one to resolve with.
func (p *provider) Lookup(key string) (string, bool) {
	p.load()
	if p.down != nil {
		return "", false
	}
	obj, ok := p.objects[key]
	if !ok {
		return "", false
	}
	if obj.isSecret && obj.value == "" {
		// A secret with no value in hand. In reveal mode (start/get) the org
		// withholds it — a hard miss whose cause Diagnose names. In presence
		// mode (list) the object still exists, so report it as resolving from
		// here; list is value-free and never needed the secret itself.
		return "", !p.reveal
	}
	return obj.value, true
}

// Label is "workspace", or "workspace (unavailable: <reason>)" when the
// workspace could not be read.
func (p *provider) Label() string {
	p.load()
	if p.down != nil {
		return sourceLabel + " (unavailable: " + p.down.short + ")"
	}
	return sourceLabel
}

// Diagnose explains why key did not resolve, for the missing-value message.
func (p *provider) Diagnose(key string) string {
	p.load()
	if p.down != nil {
		return p.down.cause
	}
	if obj, ok := p.objects[key]; ok && emfetch.Withheld(emfetch.Object{Secret: obj.isSecret, Value: obj.value}, p.secretsIncluded) {
		return p.cause(emfetch.CauseSecretsWithheld)
	}
	return p.cause(emfetch.CauseNoValue)
}

// cause is c's text for this provider's domain and workspace, worded as
// docs/v2-workspace-link.md words it. The words are pkg/emfetch's, so Astro
// Desktop shows the same text and a failure reads the same from either app.
func (p *provider) cause(c emfetch.Cause) string {
	return c.Text(p.domain, p.workspaceID)
}

// load fetches the workspace's objects once, recording a whole-provider outage
// on any failure so Lookup stays silent and Label/Diagnose explain.
func (p *provider) load() {
	p.once.Do(func() {
		// The manifest's domain picks the login and the org, not the current
		// context: a production-linked project keeps reading production while
		// the CLI is switched to dev. No login for that domain and the provider
		// is simply absent. What counts as logged in comes from
		// internal/astrosession, so this and the query commands never disagree
		// about whether there is a session.
		ctx, err := login(p.domain)
		if errors.Is(err, astrosession.ErrSessionExpired) {
			p.down = p.classify(&httpError{code: http.StatusUnauthorized})
			return
		}
		if err != nil || astrosession.Credential(ctx.Token) == "" {
			p.down = &outage{
				short: "not logged in to " + p.domain,
				cause: p.cause(emfetch.CauseNotLoggedIn),
			}
			return
		}
		p.client = p.clientFor(Login{Domain: ctx.Domain, Token: ctx.Token, APIURL: ctx.GetPublicRESTAPIURL("v1")})
		// When the org disallows reading secrets, non-secret values still
		// resolve, so the fallback re-reads without the secret request and a
		// workspace secret then reads as a miss whose cause names the org
		// toggle. secretsIncluded is what Diagnose uses to tell that apart from
		// a secret the platform simply holds no value for.
		objs, secretsIncluded, err := emfetch.WithSecretsFallback(httpcontext.Background(), p.reveal,
			func(reqCtx httpcontext.Context, showSecrets bool) (map[string]objectValue, error) {
				return p.fetch(reqCtx, ctx.Organization, showSecrets)
			})
		if err != nil {
			p.down = p.classify(err)
			return
		}
		p.objects = objs
		p.secretsIncluded = secretsIncluded
	})
}

// fetchedTypes are the object types local Airflow reads. The list endpoint
// filters by one type per call (a request with no type is rejected
// server-side), so a fetch is one call per type — still shared across every
// name in a run. Metrics exports are out of scope.
//
// CONNECTION comes after ENVIRONMENT_VARIABLE on purpose: a connection stored
// both ways — natively and as an AIRFLOW_CONN_* env var — resolves to the
// native one, since indexObject lets the later write win. Astro Desktop layers
// the two in the same order.
var fetchedTypes = []astrov1.ListEnvironmentObjectsParamsObjectType{
	astrov1.ENVIRONMENTVARIABLE,
	astrov1.AIRFLOWVARIABLE,
	astrov1.CONNECTION,
}

// fetch reads the workspace's objects and indexes them by Airflow env-var key. It reads at workspace scope only (no resolveLinked, no deployment
// id): workspace objects are the team-shared tier meant for local dev, and a
// deployment's runtime config stays off the laptop.
//
// It drives the generated client directly rather than calling cloud/env's
// ListVars/ListAirflowVars because those flatten the HTTP status into a bare
// message (through NormalizeAPIError), and classify needs the 401/403/404 to
// name the failure. The paging itself is pkg/emfetch's, shared with every other
// reader of the endpoint.
func (p *provider) fetch(ctx httpcontext.Context, org string, showSecrets bool) (map[string]objectValue, error) {
	out := map[string]objectValue{}
	for _, objectType := range fetchedTypes {
		rows, err := p.listType(ctx, org, objectType, showSecrets)
		if err != nil {
			return nil, err
		}
		for i := range rows {
			indexObject(out, &rows[i], showSecrets)
		}
	}
	return out, nil
}

// listType pages one object type.
func (p *provider) listType(ctx httpcontext.Context, org string, objectType astrov1.ListEnvironmentObjectsParamsObjectType, showSecrets bool) ([]astrov1.EnvironmentObject, error) {
	resolveLinked := false
	return emfetch.Paginate(ctx, func(reqCtx httpcontext.Context, offset, limit int) ([]astrov1.EnvironmentObject, int, error) {
		params := &astrov1.ListEnvironmentObjectsParams{
			ObjectType:    &objectType,
			ResolveLinked: &resolveLinked,
			ShowSecrets:   &showSecrets,
			Limit:         &limit,
			Offset:        &offset,
			WorkspaceId:   &p.workspaceID,
		}
		resp, err := p.client.ListEnvironmentObjectsWithResponse(reqCtx, org, params)
		if err != nil {
			// A request that never reached an HTTP response — offline. classify
			// turns any non-*httpError into the offline outage.
			return nil, 0, err
		}
		if resp.JSON200 == nil {
			return nil, 0, statusError(showSecrets, resp)
		}
		return resp.JSON200.EnvironmentObjects, resp.JSON200.TotalCount, nil
	})
}

// indexObject adds one object to the index under the Airflow env-var key that
// satisfies it. An object whose key cannot be an env var is skipped, since no
// local declaration could name it. showSecrets is whether this read asked for
// secret values, which decides what a connection can resolve to.
func indexObject(out map[string]objectValue, obj *astrov1.EnvironmentObject, showSecrets bool) {
	switch obj.ObjectType {
	case astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE:
		if obj.EnvironmentVariable != nil {
			out[obj.ObjectKey] = objectValue{value: obj.EnvironmentVariable.Value, isSecret: obj.EnvironmentVariable.IsSecret}
		}
	case astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE:
		if obj.AirflowVariable != nil {
			out[airflowenv.EnvKeyForStoredVarKey(obj.ObjectKey)] = objectValue{value: obj.AirflowVariable.Value, isSecret: obj.AirflowVariable.IsSecret}
		}
	case astrov1.EnvironmentObjectObjectTypeCONNECTION:
		if obj.Connection == nil {
			return
		}
		key, value, ok := airflowenv.EncodeConnEnv(connFromObject(obj.ObjectKey, obj.Connection))
		if !ok {
			return
		}
		if emfetch.Withheld(emfetch.Object{Connection: true}, showSecrets) {
			// Read without secrets, the password and the values in extra arrive
			// blank, and nothing in the object says which were blanked: "no
			// password" and "password withheld" look the same. Encoding what is
			// left would hand Airflow a connection that starts and then fails to
			// authenticate. So every native connection read this way is a
			// withheld secret — a hard miss in reveal mode whose cause names the
			// org toggle, and still present for list, which never needs the value.
			// Astro Desktop skips these connections the same way.
			out[key] = objectValue{isSecret: true}
			return
		}
		out[key] = objectValue{value: value}
	case astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT:
		// A deployment telemetry concern, out of scope. fetch does not ask for
		// the type, so this is a defensive guard.
	}
}

// connFromObject is a CONNECTION object's structured fields as the connection
// the shared codec encodes.
func connFromObject(objectKey string, c *astrov1.EnvironmentObjectConnection) connmodel.Connection {
	out := connmodel.Connection{ConnID: airflowenv.ConnIDForStoredConnKey(objectKey), ConnType: c.Type}
	if c.Host != nil {
		out.ConnHost = *c.Host
	}
	if c.Login != nil {
		out.ConnLogin = *c.Login
	}
	if c.Password != nil {
		out.ConnPassword = *c.Password
	}
	if c.Schema != nil {
		out.ConnSchema = *c.Schema
	}
	if c.Port != nil {
		out.ConnPort = *c.Port
	}
	if c.Extra != nil {
		out.ConnExtra = *c.Extra
	}
	return out
}

// httpError carries the HTTP status of a failed read, which NormalizeAPIError
// otherwise flattens into a bare message.
type httpError struct {
	code int
	err  error
}

func (e *httpError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("status %d", e.code)
}

// statusError classifies a non-200 list response: the org secrets refusal, or
// an HTTP status carried for classify to turn into a named cause.
//
// The refusal is read from the response rather than from the error
// NormalizeAPIError builds out of it. That error is only the message field of a
// JSON envelope, so a refusal arriving as a plain body, an HTML page, or an
// envelope keyed on anything else loses the words that identify it — and losing
// them here means the fallback never runs and the outage is reported as
// unreachable.
func statusError(wantSecrets bool, resp *astrov1.ListEnvironmentObjectsResponse) error {
	if resp.HTTPResponse == nil {
		return &httpError{err: errors.New("empty response from Environment Manager")}
	}
	if refusal := emfetch.RefusalFor(wantSecrets, resp.HTTPResponse.StatusCode, resp.Body); refusal != nil {
		return refusal
	}
	return &httpError{code: resp.HTTPResponse.StatusCode, err: astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)}
}

// classify turns a fetch error into the outage a user sees: each named failure
// mode gets its own short label, and the remediation cause pkg/emfetch words
// as docs/v2-workspace-link.md does. Each names the domain, because the
// commonest wrong answer — a production workspace asked of a dev host — is
// fixed by the login, not by the manifest.
func (p *provider) classify(err error) *outage {
	var he *httpError
	if errors.As(err, &he) {
		if he.code == http.StatusUnauthorized {
			// astrosession.Rejected is emfetch's session-expired cause, unless
			// ASTRO_API_TOKEN is set: then that token is the one refused, and
			// `astro login` cannot fix it, so the variable is named instead.
			return &outage{short: shortLabels[emfetch.CauseSessionExpired], cause: astrosession.Rejected(p.domain).Error()}
		}
		short := "unreachable"
		if c, named := emfetch.StatusCause(he.code); named {
			short = shortLabels[c]
		}
		return &outage{short: short, cause: emfetch.StatusText(he.code, p.domain, p.workspaceID, he.err)}
	}
	return &outage{short: "offline", cause: p.cause(emfetch.CauseOffline)}
}

// shortLabels is the `list` label suffix for each cause a platform status
// names. The labels are the CLI's own; the causes they abbreviate are shared.
var shortLabels = map[emfetch.Cause]string{
	emfetch.CauseSessionExpired: "session expired",
	emfetch.CauseNoAccess:       "no access",
	emfetch.CauseNotFound:       "workspace not found",
}
