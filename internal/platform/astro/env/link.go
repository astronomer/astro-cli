package env

import (
	httpcontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/pkg/util"
)

// validateDeploymentID rejects empty or obviously-malformed deployment IDs at
// the CLI boundary so we surface a clean error instead of "Internal server
// error" from the platform.
func validateDeploymentID(depID string) error {
	if depID == "" {
		return errors.New("--deployment cannot be empty")
	}
	if !util.IsCUID(depID) {
		return fmt.Errorf("%q is not a valid deployment ID (expected a CUID)", depID)
	}
	return nil
}

// LinkKind names the kind of workspace object a link command manages. The
// platform links every object type the same way; what differs by kind is the
// shape of an override and of the value an update body has to echo.
type LinkKind string

const (
	LinkVariable        LinkKind = "variable"
	LinkConnection      LinkKind = "connection"
	LinkAirflowVariable LinkKind = "airflow-variable"
)

type linkKind struct {
	objectType astrov1.ListEnvironmentObjectsParamsObjectType
	// noun is how errors name the object.
	noun string
	// echoValue fills the update body's typed field from the current object
	// without changing it. The platform requires that field even when only
	// links change, and stores what it is given: see each kind's function.
	echoValue func(body *astrov1.UpdateEnvironmentObjectJSONRequestBody, current *astrov1.EnvironmentObject)
	// overrides renders a link's override as GET returned it in the PATCH
	// shape, nil when it has none. Secret fields are absent from GET, and the
	// platform keeps a secret a PATCH omits, so echoing what is visible
	// preserves the link as it is.
	overrides func(l *astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest
}

var linkKinds = map[LinkKind]linkKind{
	LinkVariable: {
		objectType: objectTypeVar,
		noun:       "environment variable",
		echoValue:  echoVarValue,
		overrides: func(l *astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest {
			if l.EnvironmentVariableOverrides == nil {
				return nil
			}
			return newOverrideRequest(l.EnvironmentVariableOverrides.Value)
		},
	},
	LinkAirflowVariable: {
		objectType: objectTypeAirflowVar,
		noun:       "Airflow variable",
		echoValue:  echoAirflowVarValue,
		overrides: func(l *astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest {
			if l.AirflowVariableOverrides == nil {
				return nil
			}
			v := l.AirflowVariableOverrides.Value
			return &astrov1.UpdateEnvironmentObjectOverridesRequest{
				AirflowVariable: &astrov1.UpdateEnvironmentObjectAirflowVariableOverridesRequest{Value: &v},
			}
		},
	},
	LinkConnection: {
		objectType: objectTypeConn,
		noun:       "connection",
		echoValue:  echoConnValue,
		overrides: func(l *astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest {
			o := l.ConnectionOverrides
			if o == nil {
				return nil
			}
			return &astrov1.UpdateEnvironmentObjectOverridesRequest{
				Connection: &astrov1.UpdateEnvironmentObjectConnectionOverridesRequest{
					Type: o.Type, Host: o.Host, Login: o.Login, Password: o.Password,
					Schema: o.Schema, Port: o.Port, Extra: o.Extra,
				},
			}
		},
	},
}

// The type-discriminator echo for each kind. Each one is safe against what
// GET hides, because the platform keeps a secret an update leaves empty: a
// secret variable's value, a connection's password and the extra keys that
// masking removed.

// echoVarValue sends the variable's value back. For a secret variable the
// fetched value is redacted to "", which the platform treats as "no change"
// for a secret — the stored value survives the PATCH (verified live).
func echoVarValue(body *astrov1.UpdateEnvironmentObjectJSONRequestBody, current *astrov1.EnvironmentObject) {
	if current.EnvironmentVariable != nil {
		v := current.EnvironmentVariable.Value
		body.EnvironmentVariable = &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{Value: &v}
	}
}

// echoAirflowVarValue is echoVarValue for an Airflow variable, which the
// platform retains the same way.
func echoAirflowVarValue(body *astrov1.UpdateEnvironmentObjectJSONRequestBody, current *astrov1.EnvironmentObject) {
	if current.AirflowVariable != nil {
		v := current.AirflowVariable.Value
		body.AirflowVariable = &astrov1.UpdateEnvironmentObjectAirflowVariableRequest{Value: &v}
	}
}

// echoConnValue sends every field of the connection GET returned, through
// patchConn with nothing changed: a connection update is not a patch, so a
// field left out would be cleared. See patchConn.
func echoConnValue(body *astrov1.UpdateEnvironmentObjectJSONRequestBody, current *astrov1.EnvironmentObject) {
	if c := current.Connection; c != nil {
		body.Connection = patchConn(ConnInput{Type: c.Type, Extra: c.Extra}, c)
	}
}

// LinkOverride is what a link gives one deployment in place of the workspace
// value. It describes the whole override: a field it leaves out is cleared
// from the link, not kept.
type LinkOverride struct {
	// Value is the override for a variable or an Airflow variable.
	Value *string
	// Connection is the override for a connection; a nil field is not set.
	Connection *ConnOverride
}

// ConnOverride is the part of a connection a link overrides.
type ConnOverride struct {
	Type     *string
	Host     *string
	Login    *string
	Password *string
	Schema   *string
	Port     *int
	Extra    *map[string]any
}

// request renders the override in the PATCH shape for kind, and names the
// fields it sets in the paths `unsetFields` and `setFields` use. A nil
// receiver, or one with nothing for this kind, sets nothing.
func (o *LinkOverride) request(kind LinkKind) (req *astrov1.UpdateEnvironmentObjectOverridesRequest, given []string) {
	if o == nil {
		return nil, nil
	}
	switch kind {
	case LinkVariable:
		if o.Value != nil {
			return newOverrideRequest(*o.Value), []string{overrideValueField}
		}
	case LinkAirflowVariable:
		if o.Value != nil {
			v := *o.Value
			return &astrov1.UpdateEnvironmentObjectOverridesRequest{
				AirflowVariable: &astrov1.UpdateEnvironmentObjectAirflowVariableOverridesRequest{Value: &v},
			}, []string{overrideValueField}
		}
	case LinkConnection:
		c := o.Connection
		if c == nil {
			return nil, nil
		}
		for name, set := range map[string]bool{
			"type": c.Type != nil, "host": c.Host != nil, "login": c.Login != nil,
			"password": c.Password != nil, "schema": c.Schema != nil, "port": c.Port != nil,
		} {
			if set {
				given = append(given, name)
			}
		}
		if c.Extra != nil {
			for k := range *c.Extra {
				given = append(given, "extra."+k)
			}
		}
		if len(given) == 0 {
			return nil, nil
		}
		sort.Strings(given)
		return &astrov1.UpdateEnvironmentObjectOverridesRequest{
			Connection: &astrov1.UpdateEnvironmentObjectConnectionOverridesRequest{
				Type: c.Type, Host: c.Host, Login: c.Login, Password: c.Password,
				Schema: c.Schema, Port: c.Port, Extra: c.Extra,
			},
		}, given
	}
	return nil, nil
}

// VarLinksReport is the consolidated view of how a workspace-scoped env var is
// attached (or excluded) across deployments. Returned by ListVarLinks.
type VarLinksReport struct {
	ObjectKey           string    `json:"object_key"`
	ObjectID            string    `json:"object_id"`
	WorkspaceValue      string    `json:"workspace_value"`
	IsSecret            bool      `json:"is_secret"`
	AutoLinkDeployments bool      `json:"auto_link_deployments"`
	Links               []VarLink `json:"links"`
	ExcludeLinks        []string  `json:"exclude_links"`
}

// VarLink describes one explicit Link entry on a workspace env var.
type VarLink struct {
	DeploymentID  string  `json:"deployment_id"`
	OverrideValue *string `json:"override_value,omitempty"`
}

// LinksReport is the link state of a workspace connection or Airflow
// variable. A connection override has several fields rather than one value,
// so the overrides are a map, and SetFields names the ones set even when
// they are secret and so absent from Overrides.
type LinksReport struct {
	ObjectKey           string       `json:"object_key"`
	ObjectID            string       `json:"object_id"`
	AutoLinkDeployments bool         `json:"auto_link_deployments"`
	Links               []ObjectLink `json:"links"`
	ExcludeLinks        []string     `json:"exclude_links"`
}

// ObjectLink is one explicit link in a LinksReport.
type ObjectLink struct {
	DeploymentID string         `json:"deployment_id"`
	Overrides    map[string]any `json:"overrides,omitempty"`
	SetFields    []string       `json:"set_fields,omitempty"`
}

// LinkVar sets a workspace-scoped env var's link to a deployment, and
// returns the variable's links as the change left them.
//
// Set semantics, matching `set` on the object nouns: the link is created when
// absent and updated when present, and overrideValue describes the whole
// override — nil means the link has none, so an existing one is cleared
// rather than left alone. That last part is the change from the old
// create-shaped behavior, where omitting the value preserved whatever was
// stored and removing an override meant deleting the link and re-creating it.
//
// noCreate refuses to create a link that is not there, the same guard the
// object nouns spell --no-create.
//
// Note the platform does NOT preserve the Links/ExcludeLinks arrays
// themselves when a PATCH omits them -- see echoPreservedFields.
func LinkVar(idOrKey string, scope Scope, depID string, overrideValue *string, noCreate bool, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	return varLinksAfter(link(LinkVariable, idOrKey, scope, depID, &LinkOverride{Value: overrideValue}, noCreate, astroV1Client))
}

// UnlinkVar removes an explicit deployment link from a workspace env var, and
// returns its links as the change left them.
func UnlinkVar(idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	return varLinksAfter(unlink(LinkVariable, idOrKey, scope, depID, astroV1Client))
}

// ExcludeVar adds a deployment to the workspace env var's excludeLinks list,
// and returns its links as the change left them. See exclude.
func ExcludeVar(idOrKey string, scope Scope, depID string, warn io.Writer, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	return varLinksAfter(exclude(LinkVariable, idOrKey, scope, depID, warn, astroV1Client))
}

// UnexcludeVar removes a deployment from a workspace env var's excludeLinks,
// and returns its links as the change left them.
func UnexcludeVar(idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	return varLinksAfter(unexclude(LinkVariable, idOrKey, scope, depID, astroV1Client))
}

// varLinksAfter reports a variable's links from the object a change left,
// secrets masked whatever the platform answered with (MaskSecrets).
func varLinksAfter(obj *astrov1.EnvironmentObject, err error) (*VarLinksReport, error) {
	if err != nil {
		return nil, err
	}
	return newVarLinksReport(MaskSecrets(obj)), nil
}

// linksAfter reports a connection's or an Airflow variable's links from the
// object a change left, secrets masked.
func linksAfter(k linkKind, obj *astrov1.EnvironmentObject, err error) (*LinksReport, error) {
	if err != nil {
		return nil, err
	}
	return newLinksReport(k, MaskSecrets(obj))
}

// Link, Unlink, Exclude and Unexclude change a workspace connection's or
// Airflow variable's links, and return them as the change left them: the
// report `link list` prints, built from the object the platform answered the
// change with, or for an exclude, read back after it.
//
// Link sets a workspace object's link to a deployment: created when absent,
// updated when present, with override describing the whole override. A
// field the link had that override does not give is cleared through
// `unsetFields`, so a nil override leaves the link with none.
func Link(kind LinkKind, idOrKey string, scope Scope, depID string, override *LinkOverride, noCreate bool, astroV1Client astrov1.APIClient) (*LinksReport, error) {
	obj, err := link(kind, idOrKey, scope, depID, override, noCreate, astroV1Client)
	return linksAfter(linkKinds[kind], obj, err)
}

// Unlink removes an explicit deployment link from a workspace object.
func Unlink(kind LinkKind, idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*LinksReport, error) {
	obj, err := unlink(kind, idOrKey, scope, depID, astroV1Client)
	return linksAfter(linkKinds[kind], obj, err)
}

// Exclude adds a deployment to a workspace object's excludeLinks list. See
// exclude.
func Exclude(kind LinkKind, idOrKey string, scope Scope, depID string, warn io.Writer, astroV1Client astrov1.APIClient) (*LinksReport, error) {
	obj, err := exclude(kind, idOrKey, scope, depID, warn, astroV1Client)
	return linksAfter(linkKinds[kind], obj, err)
}

// Unexclude removes a deployment from a workspace object's excludeLinks.
func Unexclude(kind LinkKind, idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*LinksReport, error) {
	obj, err := unexclude(kind, idOrKey, scope, depID, astroV1Client)
	return linksAfter(linkKinds[kind], obj, err)
}

// link sets the link, and returns the object the platform answered with.
func link(kind LinkKind, idOrKey string, scope Scope, depID string, override *LinkOverride, noCreate bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	k := linkKinds[kind]
	if err := validateDeploymentID(depID); err != nil {
		return nil, err
	}
	current, err := resolveWorkspaceObject(k, idOrKey, scope, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if excludeExists(current.ExcludeLinks, depID) {
		return nil, fmt.Errorf("%s %q has deployment %s in its exclude list; remove the exclude first", k.noun, current.ObjectKey, depID)
	}

	links, found := upsertLinkInUpdateList(kind, current.Links, depID, override)
	if !found && noCreate {
		return nil, fmt.Errorf("%s %q is not linked to deployment %s and --no-create was passed",
			k.noun, current.ObjectKey, depID)
	}
	return patchLinks(k, *current.Id, current, &links, nil, astroV1Client)
}

// unlink removes the link, and returns the object the platform answered with.
func unlink(kind LinkKind, idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	k := linkKinds[kind]
	if err := validateDeploymentID(depID); err != nil {
		return nil, err
	}
	current, err := resolveWorkspaceObject(k, idOrKey, scope, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if !linkExists(current.Links, depID) {
		return nil, fmt.Errorf("%s %q is not linked to deployment %s", k.noun, current.ObjectKey, depID)
	}
	links := buildUpdateLinksExcluding(k, current.Links, depID)
	return patchLinks(k, *current.Id, current, &links, nil, astroV1Client)
}

// exclude adds a deployment to a workspace object's excludeLinks list, using
// the platform's dedicated POST .../exclude-linking endpoint. Useful for
// auto-linked objects to opt out specific deployments. Idempotent: re-running
// against an already-excluded deployment is a no-op success.
//
// That endpoint answers with no body, so the object returned is read back
// after it. If that read fails, the exclude has still happened: it says so on
// warn and returns the object read before the change with the exclude added,
// what the endpoint did and the rest as it was.
func exclude(kind LinkKind, idOrKey string, scope Scope, depID string, warn io.Writer, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	k := linkKinds[kind]
	if err := validateDeploymentID(depID); err != nil {
		return nil, err
	}
	current, err := resolveWorkspaceObject(k, idOrKey, scope, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if linkExists(current.Links, depID) {
		return nil, fmt.Errorf("%s %q is explicitly linked to deployment %s; delete the link first to exclude", k.noun, current.ObjectKey, depID)
	}
	if excludeExists(current.ExcludeLinks, depID) {
		// Already excluded; desired state matches actual, no-op success.
		return current, nil
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrov1.ExcludeLinkingEnvironmentObjectJSONRequestBody{
		Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScopeDEPLOYMENT,
		ScopeEntityId: depID,
	}
	resp, err := astroV1Client.ExcludeLinkingEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	after, err := resolveWorkspaceObject(k, *current.Id, scope, false, astroV1Client)
	if err == nil {
		return after, nil
	}
	fmt.Fprintf(warn, "Excluded deployment %s, but reading %s %q back failed (%s); showing it as it was read before, with the exclude added.\n",
		depID, k.noun, current.ObjectKey, err)
	excludes := append(derefSlice(current.ExcludeLinks), astrov1.EnvironmentObjectExcludeLink{
		Scope:         astrov1.EnvironmentObjectExcludeLinkScopeDEPLOYMENT,
		ScopeEntityId: depID,
	})
	current.ExcludeLinks = &excludes
	return current, nil
}

// unexclude removes a deployment from a workspace object's excludeLinks.
// There's no dedicated endpoint for this, so it goes through PATCH.
func unexclude(kind LinkKind, idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	k := linkKinds[kind]
	if err := validateDeploymentID(depID); err != nil {
		return nil, err
	}
	current, err := resolveWorkspaceObject(k, idOrKey, scope, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if !excludeExists(current.ExcludeLinks, depID) {
		return nil, fmt.Errorf("%s %q does not have deployment %s in its exclude list", k.noun, current.ObjectKey, depID)
	}
	excludes := buildUpdateExcludesExcluding(current.ExcludeLinks, depID)
	return patchLinks(k, *current.Id, current, nil, &excludes, astroV1Client)
}

// ListVarLinks returns the consolidated link state of a workspace env var.
// includeSecrets is forwarded to the platform; without it, the workspace
// value and any per-link overrides for secret vars are redacted from the
// response.
func ListVarLinks(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	current, err := resolveWorkspaceObject(linkKinds[LinkVariable], idOrKey, scope, includeSecrets, astroV1Client)
	if err != nil {
		return nil, err
	}
	return newVarLinksReport(current), nil
}

// newVarLinksReport reports a workspace env var's links.
func newVarLinksReport(current *astrov1.EnvironmentObject) *VarLinksReport {
	// Links/ExcludeLinks start non-nil so an empty list marshals as [] rather
	// than null; scripted consumers iterate .links[] without null guards.
	report := &VarLinksReport{
		ObjectKey:    current.ObjectKey,
		Links:        []VarLink{},
		ExcludeLinks: excludeIDs(current.ExcludeLinks),
	}
	if current.Id != nil {
		report.ObjectID = *current.Id
	}
	if current.AutoLinkDeployments != nil {
		report.AutoLinkDeployments = *current.AutoLinkDeployments
	}
	if current.EnvironmentVariable != nil {
		report.WorkspaceValue = current.EnvironmentVariable.Value
		report.IsSecret = current.EnvironmentVariable.IsSecret
	}
	for _, l := range derefSlice(current.Links) {
		vl := VarLink{DeploymentID: l.ScopeEntityId}
		if l.EnvironmentVariableOverrides != nil {
			v := l.EnvironmentVariableOverrides.Value
			vl.OverrideValue = &v
		}
		report.Links = append(report.Links, vl)
	}
	return report
}

// ListLinks returns the link state of a workspace connection or Airflow
// variable. includeSecrets is forwarded to the platform, which otherwise
// leaves secret override fields out; SetFields still names them.
func ListLinks(kind LinkKind, idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*LinksReport, error) {
	k := linkKinds[kind]
	current, err := resolveWorkspaceObject(k, idOrKey, scope, includeSecrets, astroV1Client)
	if err != nil {
		return nil, err
	}
	return newLinksReport(k, current)
}

// newLinksReport reports a workspace connection's or Airflow variable's links.
func newLinksReport(k linkKind, current *astrov1.EnvironmentObject) (*LinksReport, error) {
	report := &LinksReport{
		ObjectKey:    current.ObjectKey,
		Links:        []ObjectLink{},
		ExcludeLinks: excludeIDs(current.ExcludeLinks),
	}
	if current.Id != nil {
		report.ObjectID = *current.Id
	}
	if current.AutoLinkDeployments != nil {
		report.AutoLinkDeployments = *current.AutoLinkDeployments
	}
	links := derefSlice(current.Links)
	for i := range links {
		ol := ObjectLink{DeploymentID: links[i].ScopeEntityId, SetFields: links[i].SetFields}
		overrides, err := overrideMap(k.overrides(&links[i]))
		if err != nil {
			return nil, err
		}
		ol.Overrides = overrides
		report.Links = append(report.Links, ol)
	}
	return report, nil
}

// overrideMap flattens a PATCH-shape override to its set fields, keyed by the
// names `setFields` uses, by way of the JSON tags the two share.
func overrideMap(o *astrov1.UpdateEnvironmentObjectOverridesRequest) (map[string]any, error) {
	if o == nil {
		return nil, nil
	}
	var typed any
	switch {
	case o.Connection != nil:
		typed = o.Connection
	case o.AirflowVariable != nil:
		typed = o.AirflowVariable
	case o.EnvironmentVariable != nil:
		typed = o.EnvironmentVariable
	default:
		return nil, nil
	}
	//astro:non-output-json — a round trip to a map, never written anywhere.
	b, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

// resolveWorkspaceObject fetches the workspace-scoped object by ID or key and
// validates that it is in fact workspace-scoped (linking is workspace-only;
// the platform refuses links on any other scope with a 400).
func resolveWorkspaceObject(k linkKind, idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if scope.WorkspaceID == "" {
		return nil, errors.New("linking commands require --workspace; deployment-scoped objects cannot be linked")
	}
	obj, err := getObject(idOrKey, scope, k.objectType, includeSecrets, astroV1Client)
	var outOfScope *outOfScopeError
	if errors.As(err, &outOfScope) && outOfScope.obj != nil && outOfScope.obj.Scope != astrov1.EnvironmentObjectScopeWORKSPACE {
		obj, err = outOfScope.obj, nil
	}
	if err != nil {
		return nil, err
	}
	if obj.Scope != astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE) {
		return nil, fmt.Errorf("%s %q is %s-scoped; only workspace-scoped objects can be linked", k.noun, obj.ObjectKey, obj.Scope)
	}
	if _, err := objectID(obj, idOrKey); err != nil {
		return nil, err
	}
	return obj, nil
}

// derefSlice unwraps the generated client's optional-array pointers; nil
// means "absent" and is treated as empty.
func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func excludeIDs(excludes *[]astrov1.EnvironmentObjectExcludeLink) []string {
	out := []string{}
	for _, e := range derefSlice(excludes) {
		out = append(out, e.ScopeEntityId)
	}
	return out
}

func linkExists(links *[]astrov1.EnvironmentObjectLink, depID string) bool {
	return slices.ContainsFunc(derefSlice(links), func(l astrov1.EnvironmentObjectLink) bool {
		return l.ScopeEntityId == depID
	})
}

func excludeExists(excludes *[]astrov1.EnvironmentObjectExcludeLink, depID string) bool {
	return slices.ContainsFunc(derefSlice(excludes), func(e astrov1.EnvironmentObjectExcludeLink) bool {
		return e.ScopeEntityId == depID
	})
}

// overrideValueField is the name `unsetFields` uses for a variable's or an
// Airflow variable's override value, matching the `value` json tag on both
// override requests.
const overrideValueField = "value"

// setOverrideFields names the fields a link's override has set: the
// platform's own setFields, which counts a secret field GET masks, together
// with what is visible, for a response that predates setFields.
func setOverrideFields(k linkKind, l *astrov1.EnvironmentObjectLink) []string {
	fields := slices.Clone(l.SetFields)
	visible, _ := overrideMap(k.overrides(l)) //nolint:errcheck // marshaling generated structs of strings, ints and maps cannot fail
	for name, v := range visible {
		if extra, ok := v.(map[string]any); ok && name == "extra" {
			for key := range extra {
				fields = append(fields, "extra."+key)
			}
			continue
		}
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return slices.Compact(fields)
}

// linkOverride renders the override for one link: what override sets, plus
// an `unsetFields` entry for every field the link had that override does not
// give.
//
// Clearing has to be said out loud. Omitting `overrides` leaves whatever is
// stored in place — which is why `link create` without --value was a no-op
// against an existing override, and why removing one used to mean deleting
// the link and re-creating it. `unsetFields` is the API's way to say "drop
// this and inherit the parent value".
//
// A field never set has nothing to unset — asking the API to unset it is a
// request for something that is not there — so a link with no override, new
// or re-linked, gets no `unsetFields` at all.
func linkOverride(kind LinkKind, override *LinkOverride, had []string) *astrov1.UpdateEnvironmentObjectOverridesRequest {
	req, given := override.request(kind)
	var unset []string
	for _, f := range had {
		if !slices.Contains(given, f) {
			unset = append(unset, f)
		}
	}
	if len(unset) == 0 {
		return req
	}
	if req == nil {
		req = &astrov1.UpdateEnvironmentObjectOverridesRequest{}
	}
	req.UnsetFields = &unset
	return req
}

func newOverrideRequest(value string) *astrov1.UpdateEnvironmentObjectOverridesRequest {
	return &astrov1.UpdateEnvironmentObjectOverridesRequest{
		EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: &value},
	}
}

// toUpdateLink converts one GET-shape link into the PATCH shape, copying any
// existing override so a partial update doesn't drop it.
func toUpdateLink(k linkKind, l *astrov1.EnvironmentObjectLink) astrov1.UpdateEnvironmentObjectLinkRequest {
	return astrov1.UpdateEnvironmentObjectLinkRequest{
		Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
		ScopeEntityId: l.ScopeEntityId,
		Overrides:     k.overrides(l),
	}
}

func toExcludeRequest(e astrov1.EnvironmentObjectExcludeLink) astrov1.ExcludeLinkEnvironmentObjectRequest {
	return astrov1.ExcludeLinkEnvironmentObjectRequest{
		Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScope(e.Scope),
		ScopeEntityId: e.ScopeEntityId,
	}
}

// upsertLinkInUpdateList builds the PATCH-shape Links list with the entry for
// depID created or updated. Other links round-trip with their existing
// overrides intact. The platform PATCH merges per-entry rather than fully
// replacing the array, so an entry's override is whatever override says:
// what it gives is set, and what the link had beyond that is cleared through
// unsetFields. Omitting `overrides` would preserve it, which is what the
// create-shaped behavior used to do.
func upsertLinkInUpdateList(kind LinkKind, current *[]astrov1.EnvironmentObjectLink, depID string, override *LinkOverride) (out []astrov1.UpdateEnvironmentObjectLinkRequest, found bool) {
	k := linkKinds[kind]
	links := derefSlice(current)
	out = make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(links)+1)
	for i := range links {
		req := toUpdateLink(k, &links[i])
		if links[i].ScopeEntityId == depID {
			found = true
			req.Overrides = linkOverride(kind, override, setOverrideFields(k, &links[i]))
		}
		out = append(out, req)
	}
	if !found {
		out = append(out, astrov1.UpdateEnvironmentObjectLinkRequest{
			Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT,
			ScopeEntityId: depID,
			Overrides:     linkOverride(kind, override, nil),
		})
	}
	return out, found
}

// buildUpdateLinks converts the GET-shape Links into the PATCH-shape, copying
// any existing overrides so a partial update doesn't drop them. Like the
// other build* helpers, it always returns a non-nil slice so an empty list
// marshals as [] rather than null.
func buildUpdateLinks(k linkKind, current *[]astrov1.EnvironmentObjectLink) []astrov1.UpdateEnvironmentObjectLinkRequest {
	return buildUpdateLinksExcluding(k, current, "")
}

func buildUpdateLinksExcluding(k linkKind, current *[]astrov1.EnvironmentObjectLink, depID string) []astrov1.UpdateEnvironmentObjectLinkRequest {
	links := derefSlice(current)
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(links))
	for i := range links {
		if depID != "" && links[i].ScopeEntityId == depID {
			continue
		}
		out = append(out, toUpdateLink(k, &links[i]))
	}
	return out
}

func buildUpdateExcludesExcluding(current *[]astrov1.EnvironmentObjectExcludeLink, depID string) []astrov1.ExcludeLinkEnvironmentObjectRequest {
	excludes := derefSlice(current)
	out := make([]astrov1.ExcludeLinkEnvironmentObjectRequest, 0, len(excludes))
	for _, e := range excludes {
		if e.ScopeEntityId == depID {
			continue
		}
		out = append(out, toExcludeRequest(e))
	}
	return out
}

func buildExistingExcludes(current *[]astrov1.EnvironmentObjectExcludeLink) []astrov1.ExcludeLinkEnvironmentObjectRequest {
	return buildUpdateExcludesExcluding(current, "")
}

// echoPreservedFields fills in the fields of an update body the caller
// didn't set, echoing the object's current state. The platform drops state
// omitted from a partial update:
//
//   - Links/ExcludeLinks arrays omitted from a PATCH are treated as "remove
//     them all", and an entry sent without `overrides` keeps only its secret
//     fields, so every entry carries its visible override back and every update body must carry the existing arrays even
//     when the caller only means to change the value -- otherwise the object
//     is silently unlinked from every deployment. Empty arrays marshal as []
//     rather than null.
//   - autoLinkDeployments is cleared when omitted (AINF-1792), so when the
//     caller isn't explicitly setting it, the current flag is echoed back.
//
// current is safe to fetch without secrets (so this works regardless of the
// org's secrets-fetching policy): the platform omits per-link override values
// for secret objects rather than blanking them, so the echoed entries omit
// overrides and the platform's per-entry merge preserves the real ones
// (verified live against the platform).
func echoPreservedFields(body *astrov1.UpdateEnvironmentObjectJSONRequestBody, current *astrov1.EnvironmentObject) {
	if body.AutoLinkDeployments == nil {
		body.AutoLinkDeployments = current.AutoLinkDeployments
	}
	if body.Links == nil {
		links := buildUpdateLinks(kindOf(current), current.Links)
		body.Links = &links
	}
	if body.ExcludeLinks == nil {
		excludes := buildExistingExcludes(current.ExcludeLinks)
		body.ExcludeLinks = &excludes
	}
}

// kindOf picks the link kind matching an object's type, so the links an
// update echoes keep their overrides whatever the object is. Before this, only
// an env var's did, so updating a linked connection, Airflow variable or
// metrics export cleared every non-secret override on its links.
func kindOf(obj *astrov1.EnvironmentObject) linkKind {
	for _, k := range linkKinds {
		if string(k.objectType) == string(obj.ObjectType) {
			return k
		}
	}
	if string(obj.ObjectType) == string(objectTypeMetrics) {
		return metricsExportLinks
	}
	return linkKind{overrides: func(*astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest { return nil }}
}

// metricsExportLinks copies a metrics export's link overrides on an update.
// It is not in linkKinds because a metrics export has no link commands, so
// nothing needs to echo its value through it.
//
// One field it cannot keep: the platform masks a password override on GET,
// and unlike every other secret it restores a stored one only when the
// update's is set (the platform's link type
// its restore rule tests != nil), so a password override is lost
// on update whatever the CLI sends. The basic token and Datadog key are
// restored as absent, as other secrets are.
var metricsExportLinks = linkKind{
	objectType: objectTypeMetrics,
	noun:       "metrics export",
	overrides: func(l *astrov1.EnvironmentObjectLink) *astrov1.UpdateEnvironmentObjectOverridesRequest {
		o := l.MetricsExportOverrides
		if o == nil {
			return nil
		}
		req := &astrov1.UpdateEnvironmentObjectMetricsExportOverridesRequest{
			BasicToken:     o.BasicToken,
			Endpoint:       o.Endpoint,
			Headers:        o.Headers,
			Labels:         o.Labels,
			Password:       o.Password,
			SigV4AssumeArn: o.SigV4AssumeArn,
			SigV4StsRegion: o.SigV4StsRegion,
			Username:       o.Username,
		}
		if o.AuthType != nil {
			v := astrov1.UpdateEnvironmentObjectMetricsExportOverridesRequestAuthType(*o.AuthType)
			req.AuthType = &v
		}
		if o.ExporterType != nil {
			v := astrov1.UpdateEnvironmentObjectMetricsExportOverridesRequestExporterType(*o.ExporterType)
			req.ExporterType = &v
		}
		return &astrov1.UpdateEnvironmentObjectOverridesRequest{MetricsExport: req}
	},
}

// patchLinks PATCHes the env-object with new Links and/or ExcludeLinks,
// echoing its value through the kind and the rest of its state via
// echoPreservedFields, and returns the object the platform answered with.
func patchLinks(
	k linkKind,
	id string,
	current *astrov1.EnvironmentObject,
	links *[]astrov1.UpdateEnvironmentObjectLinkRequest,
	excludes *[]astrov1.ExcludeLinkEnvironmentObjectRequest,
	astroV1Client astrov1.APIClient,
) (*astrov1.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	var body astrov1.UpdateEnvironmentObjectJSONRequestBody
	k.echoValue(&body, current)
	body.Links = links
	body.ExcludeLinks = excludes
	echoPreservedFields(&body, current)
	resp, err := astroV1Client.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, errors.New("update returned empty response body")
	}
	return resp.JSON200, nil
}
