package env

import (
	httpcontext "context"
	"errors"
	"fmt"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

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

// LinkVar attaches a workspace-scoped env var to a specific deployment with
// optional override value. Errors if the link already exists.
func LinkVar(idOrKey string, scope Scope, depID string, overrideValue *string, coreClient astrocore.CoreClient) error {
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is already linked to deployment %s", current.ObjectKey, depID)
	}
	if excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q has deployment %s in its exclude list; remove the exclude first", current.ObjectKey, depID)
	}

	links := buildUpdateLinks(current.Links)
	newLink := astrocore.UpdateEnvironmentObjectLinkRequest{
		Scope:         astrocore.UpdateEnvironmentObjectLinkRequestScopeDEPLOYMENT,
		ScopeEntityId: depID,
	}
	if overrideValue != nil {
		newLink.Overrides = &astrocore.UpdateEnvironmentObjectOverridesRequest{
			EnvironmentVariable: &astrocore.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{
				Value: overrideValue,
			},
		}
	}
	links = append(links, newLink)

	return patchVarLinks(*current.Id, current, &links, nil, coreClient)
}

// UnlinkVar removes an explicit deployment link from a workspace env var.
func UnlinkVar(idOrKey string, scope Scope, depID string, coreClient astrocore.CoreClient) error {
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
// auto-linked objects to opt out specific deployments.
func ExcludeVar(idOrKey string, scope Scope, depID string, coreClient astrocore.CoreClient) error {
	current, err := resolveWorkspaceVar(idOrKey, scope, coreClient)
	if err != nil {
		return err
	}
	if linkExists(current.Links, depID) {
		return fmt.Errorf("environment variable %q is explicitly linked to deployment %s; unlink first to exclude", current.ObjectKey, depID)
	}
	if excludeExists(current.ExcludeLinks, depID) {
		return fmt.Errorf("environment variable %q already excludes deployment %s", current.ObjectKey, depID)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	body := astrocore.ExcludeLinkingEnvironmentObjectJSONRequestBody{
		Scope:         astrocore.ExcludeLinkEnvironmentObjectRequestScopeDEPLOYMENT,
		ScopeEntityId: depID,
	}
	resp, err := coreClient.ExcludeLinkingEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
	if err != nil {
		return err
	}
	return astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// UnexcludeVar removes a deployment from a workspace env var's excludeLinks.
// There's no dedicated endpoint for this, so it goes through PATCH.
func UnexcludeVar(idOrKey string, scope Scope, depID string, coreClient astrocore.CoreClient) error {
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
func ListVarLinks(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*VarLinksReport, error) {
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
func resolveWorkspaceVar(idOrKey string, scope Scope, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	return resolveWorkspaceVarWithSecrets(idOrKey, scope, false, coreClient)
}

func resolveWorkspaceVarWithSecrets(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if scope.WorkspaceID == "" {
		return nil, errors.New("linking commands require --workspace-id; deployment-scoped objects cannot be linked")
	}
	obj, err := getObject(idOrKey, scope, objectTypeVar, includeSecrets, coreClient)
	if err != nil {
		return nil, err
	}
	if obj.Scope != astrocore.EnvironmentObjectScope(astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE) {
		return nil, fmt.Errorf("environment variable %q is %s-scoped; only workspace-scoped objects can be linked", obj.ObjectKey, obj.Scope)
	}
	if obj.Id == nil || *obj.Id == "" {
		return nil, fmt.Errorf("environment variable %q has no addressable ID", obj.ObjectKey)
	}
	return obj, nil
}

func linkExists(links *[]astrocore.EnvironmentObjectLink, depID string) bool {
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

func excludeExists(excludes *[]astrocore.EnvironmentObjectExcludeLink, depID string) bool {
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

// buildUpdateLinks converts the GET-shape Links into the PATCH-shape, copying
// any existing overrides so a partial update doesn't drop them.
func buildUpdateLinks(current *[]astrocore.EnvironmentObjectLink) []astrocore.UpdateEnvironmentObjectLinkRequest {
	if current == nil {
		return nil
	}
	out := make([]astrocore.UpdateEnvironmentObjectLinkRequest, 0, len(*current))
	for _, l := range *current {
		req := astrocore.UpdateEnvironmentObjectLinkRequest{
			Scope:         astrocore.UpdateEnvironmentObjectLinkRequestScope(l.Scope),
			ScopeEntityId: l.ScopeEntityId,
		}
		if l.EnvironmentVariableOverrides != nil {
			v := l.EnvironmentVariableOverrides.Value
			req.Overrides = &astrocore.UpdateEnvironmentObjectOverridesRequest{
				EnvironmentVariable: &astrocore.UpdateEnvironmentObjectEnvironmentVariableOverridesRequest{Value: &v},
			}
		}
		out = append(out, req)
	}
	return out
}

func buildUpdateLinksExcluding(current *[]astrocore.EnvironmentObjectLink, depID string) []astrocore.UpdateEnvironmentObjectLinkRequest {
	all := buildUpdateLinks(current)
	out := make([]astrocore.UpdateEnvironmentObjectLinkRequest, 0, len(all))
	for _, l := range all {
		if l.ScopeEntityId == depID {
			continue
		}
		out = append(out, l)
	}
	return out
}

func buildUpdateExcludesExcluding(current *[]astrocore.EnvironmentObjectExcludeLink, depID string) []astrocore.ExcludeLinkEnvironmentObjectRequest {
	if current == nil {
		return nil
	}
	out := make([]astrocore.ExcludeLinkEnvironmentObjectRequest, 0, len(*current))
	for _, e := range *current {
		if e.ScopeEntityId == depID {
			continue
		}
		out = append(out, astrocore.ExcludeLinkEnvironmentObjectRequest{
			Scope:         astrocore.ExcludeLinkEnvironmentObjectRequestScope(e.Scope),
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
	current *astrocore.EnvironmentObject,
	links *[]astrocore.UpdateEnvironmentObjectLinkRequest,
	excludes *[]astrocore.ExcludeLinkEnvironmentObjectRequest,
	coreClient astrocore.CoreClient,
) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	body := astrocore.UpdateEnvironmentObjectJSONRequestBody{
		AutoLinkDeployments: current.AutoLinkDeployments,
	}
	// The platform requires the typed body field even when only mutating
	// links/excludes. Echo the existing value back to satisfy the
	// type-discriminator check without changing the workspace value.
	if current.EnvironmentVariable != nil {
		v := current.EnvironmentVariable.Value
		body.EnvironmentVariable = &astrocore.UpdateEnvironmentObjectEnvironmentVariableRequest{
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
	return astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

func buildExistingExcludes(current *[]astrocore.EnvironmentObjectExcludeLink) []astrocore.ExcludeLinkEnvironmentObjectRequest {
	if current == nil {
		return nil
	}
	out := make([]astrocore.ExcludeLinkEnvironmentObjectRequest, 0, len(*current))
	for _, e := range *current {
		out = append(out, astrocore.ExcludeLinkEnvironmentObjectRequest{
			Scope:         astrocore.ExcludeLinkEnvironmentObjectRequestScope(e.Scope),
			ScopeEntityId: e.ScopeEntityId,
		})
	}
	return out
}
