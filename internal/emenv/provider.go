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
// Scope: env-var-keyed objects only — plain env vars and Airflow variables,
// whose objectKey is already the Airflow env-var key the local chain reads.
// Re-encoding native structured CONNECTION objects into an AIRFLOW_CONN_<id>
// value is out of scope here.
package emenv

import (
	httpcontext "context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	astroenv "github.com/astronomer/astro-cli/internal/platform/astro/env"
)

// sourceLabel is the source name a workspace-resolved value reports.
const sourceLabel = "workspace"

const (
	// listLimit is the page size for the Environment Manager list call.
	listLimit = 1000
	// maxPages bounds one fetch against a server that mis-reports TotalCount.
	maxPages = 100
)

// provider resolves declared env values for one workspace from Environment
// Manager. It fetches lazily on the first Lookup and caches the result for the
// run.
type provider struct {
	workspaceID string
	client      astrov1.APIClient
	reveal      bool // ask for secret values (start/get); false is presence-only (list)

	once    sync.Once
	objects map[string]objectValue
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
	if obj, ok := p.objects[key]; ok && obj.isSecret && obj.value == "" {
		return `it lives in Environment Manager but your org disables secret fetching — ask an org admin to enable "Environment Secrets Fetching"`
	}
	return "Environment Manager holds no value for it in this workspace"
}

// load fetches the workspace's objects once, recording a whole-provider outage
// on any failure so Lookup stays silent and Label/Diagnose explain.
func (p *provider) load() {
	p.once.Do(func() {
		// The current context picks the login and the org; a logged-out user
		// has none, and the provider is simply absent. What counts as logged
		// out comes from internal/astrosession, so this and the query commands
		// never disagree about whether there is a session.
		ctx, err := config.GetCurrentContext()
		if err != nil || astrosession.Credential(ctx.Token) == "" {
			p.down = &outage{short: "logged out", cause: "you are not logged in — log in with 'astro login'"}
			return
		}
		objs, err := p.fetch(ctx.Organization, p.reveal)
		if errors.Is(err, errSecretsDisabled) {
			// The org disallows reading secrets. Non-secret values still
			// resolve, so retry without the secret request; a workspace secret
			// then reads as a miss whose cause names the org toggle.
			objs, err = p.fetch(ctx.Organization, false)
		}
		if err != nil {
			p.down = classify(err)
			return
		}
		p.objects = objs
	})
}

// envVarKeyedTypes are the object types whose objectKey is already the Airflow
// env-var key: plain env vars and Airflow variables. The list endpoint filters
// by one type per call (a request with no type is rejected server-side), so a
// fetch is one call per type — still shared across every name in a run. Native
// connections and metrics exports are out of scope.
var envVarKeyedTypes = []astrov1.ListEnvironmentObjectsParamsObjectType{
	astrov1.ENVIRONMENTVARIABLE,
	astrov1.AIRFLOWVARIABLE,
}

// fetch reads the workspace's env-var-keyed objects and indexes them by
// objectKey. It reads at workspace scope only (no resolveLinked, no deployment
// id): workspace objects are the team-shared tier meant for local dev, and a
// deployment's runtime config stays off the laptop.
//
// It drives the generated client directly rather than calling cloud/env's
// ListVars/ListAirflowVars because those flatten the HTTP status into a bare
// message (through NormalizeAPIError), and classify needs the 401/403/404 to
// name the failure. The paging mirrors cloud/env.listObjects deliberately.
func (p *provider) fetch(org string, showSecrets bool) (map[string]objectValue, error) {
	out := map[string]objectValue{}
	for _, objectType := range envVarKeyedTypes {
		if err := p.fetchType(out, org, objectType, showSecrets); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fetchType pages one object type into out.
func (p *provider) fetchType(out map[string]objectValue, org string, objectType astrov1.ListEnvironmentObjectsParamsObjectType, showSecrets bool) error {
	resolveLinked := false
	limit := listLimit
	seen := 0
	for page := 0; page < maxPages; page++ {
		offset := seen
		params := &astrov1.ListEnvironmentObjectsParams{
			ObjectType:    &objectType,
			ResolveLinked: &resolveLinked,
			ShowSecrets:   &showSecrets,
			Limit:         &limit,
			Offset:        &offset,
			WorkspaceId:   &p.workspaceID,
		}
		resp, err := p.client.ListEnvironmentObjectsWithResponse(httpcontext.Background(), org, params)
		if err != nil {
			// A request that never reached an HTTP response — offline. classify
			// turns any non-*httpError into the offline outage.
			return err
		}
		if resp.JSON200 == nil {
			return statusError(resp)
		}
		for i := range resp.JSON200.EnvironmentObjects {
			indexObject(out, &resp.JSON200.EnvironmentObjects[i])
		}
		seen += len(resp.JSON200.EnvironmentObjects)
		if len(resp.JSON200.EnvironmentObjects) < limit || seen >= resp.JSON200.TotalCount {
			return nil
		}
	}
	return fmt.Errorf("aborted reading the workspace after %d pages", maxPages)
}

// indexObject adds one env-var-keyed object to the index. Native connections
// (structured CONNECTION objects) and metrics exports are skipped: a
// connection stored env-keyed as an ENVIRONMENT_VARIABLE still lands here.
func indexObject(out map[string]objectValue, obj *astrov1.EnvironmentObject) {
	switch obj.ObjectType {
	case astrov1.EnvironmentObjectObjectTypeENVIRONMENTVARIABLE:
		if obj.EnvironmentVariable != nil {
			out[obj.ObjectKey] = objectValue{value: obj.EnvironmentVariable.Value, isSecret: obj.EnvironmentVariable.IsSecret}
		}
	case astrov1.EnvironmentObjectObjectTypeAIRFLOWVARIABLE:
		if obj.AirflowVariable != nil {
			out[obj.ObjectKey] = objectValue{value: obj.AirflowVariable.Value, isSecret: obj.AirflowVariable.IsSecret}
		}
	case astrov1.EnvironmentObjectObjectTypeCONNECTION, astrov1.EnvironmentObjectObjectTypeMETRICSEXPORT:
		// Native structured connections need re-encoding into an
		// AIRFLOW_CONN_<id> value, out of scope here; metrics exports are a
		// deployment telemetry concern, also out of scope. Skip both. (fetch
		// asks for neither type, so this is a defensive guard.)
	}
}

// errSecretsDisabled marks the org-level "secrets fetching not allowed"
// refusal, which load retries without the secret request.
var errSecretsDisabled = errors.New("environment secrets fetching disabled")

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
func statusError(resp *astrov1.ListEnvironmentObjectsResponse) error {
	if resp.HTTPResponse == nil {
		return &httpError{err: errors.New("empty response from Environment Manager")}
	}
	apiErr := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if astroenv.IsSecretsFetchingNotAllowedError(apiErr) {
		return errSecretsDisabled
	}
	return &httpError{code: resp.HTTPResponse.StatusCode, err: apiErr}
}

// classify turns a fetch error into the outage a user sees: each named failure
// mode gets its own short label and remediation cause.
func classify(err error) *outage {
	var he *httpError
	if errors.As(err, &he) {
		switch he.code {
		case http.StatusUnauthorized:
			return &outage{short: "session expired", cause: "your session expired — log in again with 'astro login'"}
		case http.StatusForbidden:
			return &outage{short: "access lost", cause: "you no longer have access to this workspace — ask an org admin to restore it"}
		case http.StatusNotFound:
			return &outage{short: "workspace not found", cause: "this workspace no longer exists — check the `workspace` in your manifest"}
		}
		return &outage{short: "unreachable", cause: fmt.Sprintf("Environment Manager returned an error: %v", he.err)}
	}
	return &outage{short: "offline", cause: "could not reach Astro to read the workspace — check your connection"}
}
