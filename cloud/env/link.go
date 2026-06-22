package env

import (
	httpcontext "context"
	"errors"
	"fmt"
	"slices"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/pkg/astro-client-v1"
	"github.com/astronomer/astro-cli/pkg/util"
)

// validateDeploymentID rejects empty or obviously-malformed deployment IDs at
// the CLI boundary so we surface a clean error instead of "Internal server
// error" from the platform.
func validateDeploymentID(depID string) error {
	if depID == "" {
		return errors.New("--deployment-id cannot be empty")
	}
	if !util.IsCUID(depID) {
		return fmt.Errorf("%q is not a valid deployment ID (expected a CUID)", depID)
	}
	return nil
}

// VarLinksReport is the consolidated view of how a workspace-scoped env var is
// attached (or excluded) across deployments. Returned by ListVarLinks.
type VarLinksReport struct {
	ObjectKey           string    `json:"objectKey" yaml:"objectKey"`
	ObjectID            string    `json:"objectId" yaml:"objectId"`
	WorkspaceValue      string    `json:"workspaceValue" yaml:"workspaceValue"`
	IsSecret            bool      `json:"isSecret" yaml:"isSecret"`
	AutoLinkDeployments bool      `json:"autoLinkDeployments" yaml:"autoLinkDeployments"`
	Links               []VarLink `json:"links" yaml:"links"`
	ExcludeLinks        []string  `json:"excludeLinks" yaml:"excludeLinks"`
}

// VarLink describes one explicit Link entry on a workspace env var.
type VarLink struct {
	DeploymentID  string  `json:"deploymentId" yaml:"deploymentId"`
	OverrideValue *string `json:"overrideValue,omitempty" yaml:"overrideValue,omitempty"`
}

// LinkVar attaches a workspace-scoped env var to a specific deployment.
// Upsert semantics: if the link doesn't exist it's created; if it does, only
// the override field is touched, and only when overrideValue is non-nil.
// Calling with overrideValue=nil on an existing link is a no-op for the
// override (the platform preserves fields omitted from a link entry; note it
// does NOT preserve the Links/ExcludeLinks arrays themselves when a PATCH
// omits them -- see echoPreservedFields). To remove an existing override,
// delete the link then re-create it without --value.
func LinkVar(idOrKey string, scope Scope, depID string, overrideValue *string, astroV1Client astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, false, astroV1Client)
	if err != nil {
		return err
	}
	if excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q has deployment %s in its exclude list; remove the exclude first", current.ObjectKey, depID)
	}

	links := upsertLinkInUpdateList(current.Links, depID, overrideValue)
	return patchVarLinks(*current.Id, current, &links, nil, astroV1Client)
}

// UnlinkVar removes an explicit deployment link from a workspace env var.
func UnlinkVar(idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, false, astroV1Client)
	if err != nil {
		return err
	}
	if !linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is not linked to deployment %s", current.ObjectKey, depID)
	}
	links := buildUpdateLinksExcluding(current.Links, depID)
	return patchVarLinks(*current.Id, current, &links, nil, astroV1Client)
}

// ExcludeVar adds a deployment to the workspace env var's excludeLinks list,
// using the platform's dedicated POST .../exclude-linking endpoint. Useful for
// auto-linked objects to opt out specific deployments. Idempotent: re-running
// against an already-excluded deployment is a no-op success.
func ExcludeVar(idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, false, astroV1Client)
	if err != nil {
		return err
	}
	if linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is explicitly linked to deployment %s; delete the link first to exclude", current.ObjectKey, depID)
	}
	if excludeExists(current.ExcludeLinks, depID) {
		// Already excluded; desired state matches actual, no-op success.
		return nil
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	body := astrov1.ExcludeLinkingEnvironmentObjectJSONRequestBody{
		Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScopeDEPLOYMENT,
		ScopeEntityId: depID,
	}
	resp, err := astroV1Client.ExcludeLinkingEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// UnexcludeVar removes a deployment from a workspace env var's excludeLinks.
// There's no dedicated endpoint for this, so it goes through PATCH.
func UnexcludeVar(idOrKey string, scope Scope, depID string, astroV1Client astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, false, astroV1Client)
	if err != nil {
		return err
	}
	if !excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q does not have deployment %s in its exclude list", current.ObjectKey, depID)
	}
	excludes := buildUpdateExcludesExcluding(current.ExcludeLinks, depID)
	return patchVarLinks(*current.Id, current, nil, &excludes, astroV1Client)
}

// ListVarLinks returns the consolidated link state of a workspace env var.
// includeSecrets is forwarded to the platform; without it, the workspace
// value and any per-link overrides for secret vars are redacted from the
// response.
func ListVarLinks(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*VarLinksReport, error) {
	current, err := resolveWorkspaceVar(idOrKey, scope, includeSecrets, astroV1Client)
	if err != nil {
		return nil, err
	}
	// Links/ExcludeLinks start non-nil so an empty list marshals as [] rather
	// than null; scripted consumers iterate .links[] without null guards.
	report := &VarLinksReport{
		ObjectKey:    current.ObjectKey,
		Links:        []VarLink{},
		ExcludeLinks: []string{},
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
	for _, e := range derefSlice(current.ExcludeLinks) {
		report.ExcludeLinks = append(report.ExcludeLinks, e.ScopeEntityId)
	}
	return report, nil
}

// resolveWorkspaceVar fetches the workspace-scoped env var by ID or key and
// validates that it is in fact workspace-scoped (linking is workspace-only).
func resolveWorkspaceVar(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if scope.WorkspaceID == "" {
		return nil, errors.New("linking commands require --workspace-id; deployment-scoped objects cannot be linked")
	}
	obj, err := getObject(idOrKey, scope, objectTypeVar, includeSecrets, astroV1Client)
	if err != nil {
		return nil, err
	}
	if obj.Scope != astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE) {
		return nil, fmt.Errorf("environment variable %q is %s-scoped; only workspace-scoped objects can be linked", obj.ObjectKey, obj.Scope)
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

// newOverrideRequest wraps an override value in the PATCH body's nested
// override shape.
func newOverrideRequest(value string) *astrov1.UpdateEnvironmentObjectOverridesRequest {
	return &astrov1.UpdateEnvironmentObjectOverridesRequest{
		EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: &value},
	}
}

// toUpdateLink converts one GET-shape link into the PATCH shape, copying any
// existing override so a partial update doesn't drop it.
func toUpdateLink(l astrov1.EnvironmentObjectLink) astrov1.UpdateEnvironmentObjectLinkRequest {
	req := astrov1.UpdateEnvironmentObjectLinkRequest{
		Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
		ScopeEntityId: l.ScopeEntityId,
	}
	if l.EnvironmentVariableOverrides != nil {
		req.Overrides = newOverrideRequest(l.EnvironmentVariableOverrides.Value)
	}
	return req
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
// replacing the array, so sending `overrides: nil` on an existing entry is a
// no-op for the override (it's preserved). To clear an override, the caller
// must delete the link first.
func upsertLinkInUpdateList(current *[]astrov1.EnvironmentObjectLink, depID string, overrideValue *string) []astrov1.UpdateEnvironmentObjectLinkRequest {
	var newOverride *astrov1.UpdateEnvironmentObjectOverridesRequest
	if overrideValue != nil {
		newOverride = newOverrideRequest(*overrideValue)
	}
	links := derefSlice(current)
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(links)+1)
	found := false
	for _, l := range links {
		req := toUpdateLink(l)
		if l.ScopeEntityId == depID {
			found = true
			req.Overrides = newOverride
		}
		out = append(out, req)
	}
	if !found {
		out = append(out, astrov1.UpdateEnvironmentObjectLinkRequest{
			Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT,
			ScopeEntityId: depID,
			Overrides:     newOverride,
		})
	}
	return out
}

// buildUpdateLinks converts the GET-shape Links into the PATCH-shape, copying
// any existing overrides so a partial update doesn't drop them. Like the
// other build* helpers, it always returns a non-nil slice so an empty list
// marshals as [] rather than null.
func buildUpdateLinks(current *[]astrov1.EnvironmentObjectLink) []astrov1.UpdateEnvironmentObjectLinkRequest {
	links := derefSlice(current)
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(links))
	for _, l := range links {
		out = append(out, toUpdateLink(l))
	}
	return out
}

func buildUpdateLinksExcluding(current *[]astrov1.EnvironmentObjectLink, depID string) []astrov1.UpdateEnvironmentObjectLinkRequest {
	links := derefSlice(current)
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(links))
	for _, l := range links {
		if l.ScopeEntityId == depID {
			continue
		}
		out = append(out, toUpdateLink(l))
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
	excludes := derefSlice(current)
	out := make([]astrov1.ExcludeLinkEnvironmentObjectRequest, 0, len(excludes))
	for _, e := range excludes {
		out = append(out, toExcludeRequest(e))
	}
	return out
}

// echoPreservedFields fills in the fields of an update body the caller
// didn't set, echoing the object's current state. The platform drops state
// omitted from a partial update:
//
//   - Links/ExcludeLinks arrays omitted from a PATCH are treated as "remove
//     them all" (while omitted fields *within* a present link entry are
//     preserved), so every update body must carry the existing arrays even
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
		links := buildUpdateLinks(current.Links)
		body.Links = &links
	}
	if body.ExcludeLinks == nil {
		excludes := buildExistingExcludes(current.ExcludeLinks)
		body.ExcludeLinks = &excludes
	}
}

// patchVarLinks PATCHes the env-object with new Links and/or ExcludeLinks,
// echoing the rest of the object's state via echoPreservedFields.
func patchVarLinks(
	id string,
	current *astrov1.EnvironmentObject,
	links *[]astrov1.UpdateEnvironmentObjectLinkRequest,
	excludes *[]astrov1.ExcludeLinkEnvironmentObjectRequest,
	astroV1Client astrov1.APIClient,
) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	var body astrov1.UpdateEnvironmentObjectJSONRequestBody
	// The platform requires the typed body field even when only mutating
	// links/excludes. Echo the existing value back to satisfy the
	// type-discriminator check without changing the workspace value. For
	// secret vars the fetched value is redacted to "", which the platform
	// treats as "no change" for a secret -- the stored value survives the
	// PATCH (verified live).
	if current.EnvironmentVariable != nil {
		v := current.EnvironmentVariable.Value
		body.EnvironmentVariable = &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{
			Value: &v,
		}
	}
	body.Links = links
	body.ExcludeLinks = excludes
	echoPreservedFields(&body, current)
	resp, err := astroV1Client.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id, body)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}
