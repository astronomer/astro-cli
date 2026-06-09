package env

import (
	httpcontext "context"
	"errors"
	"fmt"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
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
// override (the platform's PATCH preserves omitted fields). To remove an
// existing override, delete the link then re-create it without --value.
func LinkVar(idOrKey string, scope Scope, depID string, overrideValue *string, coreClient astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q has deployment %s in its exclude list; remove the exclude first", current.ObjectKey, depID)
	}

	links := upsertLinkInUpdateList(current.Links, depID, overrideValue)
	return patchVarLinks(*current.Id, current, &links, nil, coreClient)
}

// UnlinkVar removes an explicit deployment link from a workspace env var.
func UnlinkVar(idOrKey string, scope Scope, depID string, coreClient astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if !linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is not linked to deployment %s", current.ObjectKey, depID)
	}
	links := buildUpdateLinksExcluding(current.Links, depID)
	return patchVarLinks(*current.Id, current, &links, nil, coreClient)
}

// ExcludeVar adds a deployment to the workspace env var's excludeLinks list,
// using the platform's dedicated POST .../exclude-linking endpoint. Useful for
// auto-linked objects to opt out specific deployments. Idempotent: re-running
// against an already-excluded deployment is a no-op success.
func ExcludeVar(idOrKey string, scope Scope, depID string, coreClient astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is explicitly linked to deployment %s; delete the link first to exclude", current.ObjectKey, depID)
	}
	if excludeExists(current.ExcludeLinks, depID) {
		// Already excluded — desired state matches actual, no-op success.
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
	resp, err := coreClient.ExcludeLinkingEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// UnexcludeVar removes a deployment from a workspace env var's excludeLinks.
// There's no dedicated endpoint for this, so it goes through PATCH.
func UnexcludeVar(idOrKey string, scope Scope, depID string, coreClient astrov1.APIClient) error {
	if err := validateDeploymentID(depID); err != nil {
		return err
	}
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if !excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q does not have deployment %s in its exclude list", current.ObjectKey, depID)
	}
	excludes := buildUpdateExcludesExcluding(current.ExcludeLinks, depID)
	return patchVarLinks(*current.Id, current, nil, &excludes, coreClient)
}

// ListVarLinks returns the consolidated link state of a workspace env var.
// includeSecrets is forwarded to the platform; without it, the workspace
// value and any per-link overrides for secret vars are redacted from the
// response.
func ListVarLinks(idOrKey string, scope Scope, includeSecrets bool, coreClient astrov1.APIClient) (*VarLinksReport, error) {
	current, err := resolveWorkspaceVarWithSecrets(idOrKey, scope, includeSecrets, coreClient)
	if err != nil {
		return nil, err
	}
	report := &VarLinksReport{
		ObjectKey: current.ObjectKey,
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
	if current.Links != nil {
		for _, l := range *current.Links {
			vl := VarLink{DeploymentID: l.ScopeEntityId}
			if l.EnvironmentVariableOverrides != nil {
				v := l.EnvironmentVariableOverrides.Value
				vl.OverrideValue = &v
			}
			report.Links = append(report.Links, vl)
		}
	}
	if current.ExcludeLinks != nil {
		for _, e := range *current.ExcludeLinks {
			report.ExcludeLinks = append(report.ExcludeLinks, e.ScopeEntityId)
		}
	}
	return report, nil
}

// resolveWorkspaceVar fetches the workspace-scoped env var by ID or key and
// validates that it is in fact workspace-scoped (linking is workspace-only).
func resolveWorkspaceVar(idOrKey string, scope Scope, coreClient astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return resolveWorkspaceVarWithSecrets(idOrKey, scope, false, coreClient)
}

func resolveWorkspaceVarWithSecrets(idOrKey string, scope Scope, includeSecrets bool, coreClient astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if scope.WorkspaceID == "" {
		return nil, errors.New("linking commands require --workspace-id; deployment-scoped objects cannot be linked")
	}
	obj, err := getObject(idOrKey, scope, objectTypeVar, includeSecrets, coreClient)
	if err != nil {
		return nil, err
	}
	if obj.Scope != astrov1.EnvironmentObjectScope(astrov1.CreateEnvironmentObjectRequestScopeWORKSPACE) {
		return nil, fmt.Errorf("environment variable %q is %s-scoped; only workspace-scoped objects can be linked", obj.ObjectKey, obj.Scope)
	}
	if obj.Id == nil || *obj.Id == "" {
		return nil, fmt.Errorf("environment variable %q has no addressable ID", obj.ObjectKey)
	}
	return obj, nil
}

func linkExists(links *[]astrov1.EnvironmentObjectLink, depID string) bool {
	if links == nil {
		return false
	}
	for _, l := range *links {
		if l.ScopeEntityId == depID {
			return true
		}
	}
	return false
}

func excludeExists(excludes *[]astrov1.EnvironmentObjectExcludeLink, depID string) bool {
	if excludes == nil {
		return false
	}
	for _, e := range *excludes {
		if e.ScopeEntityId == depID {
			return true
		}
	}
	return false
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
		newOverride = &astrov1.UpdateEnvironmentObjectOverridesRequest{
			EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: overrideValue},
		}
	}
	out := []astrov1.UpdateEnvironmentObjectLinkRequest{}
	found := false
	if current != nil {
		for _, l := range *current {
			if l.ScopeEntityId == depID {
				found = true
				out = append(out, astrov1.UpdateEnvironmentObjectLinkRequest{
					Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
					ScopeEntityId: l.ScopeEntityId,
					Overrides:     newOverride,
				})
				continue
			}
			req := astrov1.UpdateEnvironmentObjectLinkRequest{
				Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
				ScopeEntityId: l.ScopeEntityId,
			}
			if l.EnvironmentVariableOverrides != nil {
				v := l.EnvironmentVariableOverrides.Value
				req.Overrides = &astrov1.UpdateEnvironmentObjectOverridesRequest{
					EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: &v},
				}
			}
			out = append(out, req)
		}
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
// any existing overrides so a partial update doesn't drop them.
func buildUpdateLinks(current *[]astrov1.EnvironmentObjectLink) []astrov1.UpdateEnvironmentObjectLinkRequest {
	if current == nil {
		return nil
	}
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(*current))
	for _, l := range *current {
		req := astrov1.UpdateEnvironmentObjectLinkRequest{
			Scope:         astrov1.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
			ScopeEntityId: l.ScopeEntityId,
		}
		if l.EnvironmentVariableOverrides != nil {
			v := l.EnvironmentVariableOverrides.Value
			req.Overrides = &astrov1.UpdateEnvironmentObjectOverridesRequest{
				EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: &v},
			}
		}
		out = append(out, req)
	}
	return out
}

func buildUpdateLinksExcluding(current *[]astrov1.EnvironmentObjectLink, depID string) []astrov1.UpdateEnvironmentObjectLinkRequest {
	all := buildUpdateLinks(current)
	out := make([]astrov1.UpdateEnvironmentObjectLinkRequest, 0, len(all))
	for _, l := range all {
		if l.ScopeEntityId == depID {
			continue
		}
		out = append(out, l)
	}
	return out
}

func buildUpdateExcludesExcluding(current *[]astrov1.EnvironmentObjectExcludeLink, depID string) []astrov1.ExcludeLinkEnvironmentObjectRequest {
	if current == nil {
		return nil
	}
	out := make([]astrov1.ExcludeLinkEnvironmentObjectRequest, 0, len(*current))
	for _, e := range *current {
		if e.ScopeEntityId == depID {
			continue
		}
		out = append(out, astrov1.ExcludeLinkEnvironmentObjectRequest{
			Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScope(e.Scope),
			ScopeEntityId: e.ScopeEntityId,
		})
	}
	return out
}

// patchVarLinks PATCHes the env-object with new Links and/or ExcludeLinks,
// preserving AutoLinkDeployments to dodge the AINF-1792 server bug where an
// omitted autoLinkDeployments is silently cleared.
func patchVarLinks(
	id string,
	current *astrov1.EnvironmentObject,
	links *[]astrov1.UpdateEnvironmentObjectLinkRequest,
	excludes *[]astrov1.ExcludeLinkEnvironmentObjectRequest,
	coreClient astrov1.APIClient,
) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	body := astrov1.UpdateEnvironmentObjectJSONRequestBody{
		AutoLinkDeployments: current.AutoLinkDeployments,
	}
	// The platform requires the typed body field even when only mutating
	// links/excludes. Echo the existing value back to satisfy the
	// type-discriminator check without changing the workspace value.
	if current.EnvironmentVariable != nil {
		v := current.EnvironmentVariable.Value
		body.EnvironmentVariable = &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{
			Value: &v,
		}
	}
	if links != nil {
		body.Links = links
	} else {
		// Caller didn't touch Links; round-trip the existing ones so the server
		// doesn't drop them on a partial PATCH.
		existing := buildUpdateLinks(current.Links)
		body.Links = &existing
	}
	if excludes != nil {
		body.ExcludeLinks = excludes
	} else {
		existing := buildExistingExcludes(current.ExcludeLinks)
		body.ExcludeLinks = &existing
	}
	resp, err := coreClient.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id, body)
	if err != nil {
		return err
	}
	return astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

func buildExistingExcludes(current *[]astrov1.EnvironmentObjectExcludeLink) []astrov1.ExcludeLinkEnvironmentObjectRequest {
	if current == nil {
		return nil
	}
	out := make([]astrov1.ExcludeLinkEnvironmentObjectRequest, 0, len(*current))
	for _, e := range *current {
		out = append(out, astrov1.ExcludeLinkEnvironmentObjectRequest{
			Scope:         astrov1.ExcludeLinkEnvironmentObjectRequestScope(e.Scope),
			ScopeEntityId: e.ScopeEntityId,
		})
	}
	return out
}
