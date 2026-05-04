package env

import (
	httpcontext "context"
	"errors"
	"fmt"
	"net/http"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/config"
)

var (
	ErrScopeNotSpecified = errors.New("--workspace-id or --deployment-id must be specified")
	ErrScopeAmbiguous    = errors.New("--workspace-id and --deployment-id are mutually exclusive")
	ErrNotFound          = errors.New("environment object not found")
)

const (
	// defaultListLimit is the page size requested from list endpoints.
	defaultListLimit = 1000
	// maxListPages caps a single list call at maxListPages*defaultListLimit
	// rows as a safety bound against a server that mis-reports TotalCount.
	maxListPages = 100
)

// Scope captures the target of an env-object operation. Exactly one of
// WorkspaceID / DeploymentID is set.
type Scope struct {
	WorkspaceID  string
	DeploymentID string
}

func (s Scope) Validate() error {
	switch {
	case s.WorkspaceID == "" && s.DeploymentID == "":
		return ErrScopeNotSpecified
	case s.WorkspaceID != "" && s.DeploymentID != "":
		return ErrScopeAmbiguous
	}
	return nil
}

// ScopeFromIDs builds a Scope from raw workspace/deployment IDs, applying the
// "deployment wins when both are set" precedence used by the local-dev path.
// Callers that want strict mutual-exclusion should construct Scope directly
// and call Validate.
func ScopeFromIDs(workspaceID, deploymentID string) Scope {
	if deploymentID != "" {
		return Scope{DeploymentID: deploymentID}
	}
	return Scope{WorkspaceID: workspaceID}
}

// listObjects returns env-objects of the given type within the scope.
// resolveLinked includes inherited workspace objects when listing at deployment scope.
// includeSecrets requests secret values from the server (subject to org policy).
//
// Pages through the list endpoint when the server reports more rows than fit
// in a single response. Stops when the accumulated count reaches TotalCount,
// when a short page is returned, or when maxListPages is hit (safety bound).
func listObjects(scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	var (
		all    []astrocore.EnvironmentObject
		offset int
	)
	for page := 0; page < maxListPages; page++ {
		params := buildListParams(scope, objectType, nil, resolveLinked, includeSecrets, defaultListLimit)
		params.Offset = &offset

		resp, err := coreClient.ListEnvironmentObjectsWithResponse(httpcontext.Background(), c.Organization, params)
		if err != nil {
			return nil, err
		}
		if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		batch := resp.JSON200.EnvironmentObjects
		all = append(all, batch...)
		if len(batch) < defaultListLimit || len(all) >= resp.JSON200.TotalCount {
			return all, nil
		}
		offset = len(all)
	}
	return all, fmt.Errorf("aborted listing %s objects after %d pages", objectType, maxListPages)
}

// getObject fetches a single env-object by ID or key.
//
// The platform has no GET-by-key endpoint; for keys, we filter the list endpoint
// server-side via ObjectKey and force resolveLinked=false so the returned ID is
// addressable in subsequent CRUD calls.
func getObject(idOrKey string, scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	if organization.IsCUID(idOrKey) {
		resp, err := coreClient.GetEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, idOrKey)
		if err != nil {
			return nil, err
		}
		if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, err
		}
		return resp.JSON200, nil
	}

	if err := scope.Validate(); err != nil {
		return nil, err
	}
	// ObjectKey filtering returns at most one row per (scope, objectType);
	// limit=2 catches server-side anomalies without forcing pagination.
	params := buildListParams(scope, objectType, &idOrKey, false, includeSecrets, 2)

	resp, err := coreClient.ListEnvironmentObjectsWithResponse(httpcontext.Background(), c.Organization, params)
	if err != nil {
		return nil, err
	}
	if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	for i := range resp.JSON200.EnvironmentObjects {
		if resp.JSON200.EnvironmentObjects[i].ObjectKey == idOrKey {
			return &resp.JSON200.EnvironmentObjects[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, idOrKey)
}

// deleteObject deletes an env-object by ID or key.
func deleteObject(idOrKey string, scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, coreClient astrocore.CoreClient) error {
	id, err := resolveID(idOrKey, scope, objectType, coreClient)
	if err != nil {
		return err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	resp, err := coreClient.DeleteEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id)
	if err != nil {
		return err
	}
	return astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
}

// resolveID returns idOrKey unchanged when it's already a CUID (no network call),
// otherwise looks up the ID by key. Used by Update / Delete paths so callers
// passing an ID don't pay for a redundant GET.
func resolveID(idOrKey string, scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, coreClient astrocore.CoreClient) (string, error) {
	if organization.IsCUID(idOrKey) {
		return idOrKey, nil
	}
	existing, err := getObject(idOrKey, scope, objectType, false, coreClient)
	if err != nil {
		return "", err
	}
	if existing.Id == nil || *existing.Id == "" {
		return "", fmt.Errorf("environment object %q has no id", idOrKey)
	}
	return *existing.Id, nil
}

// scopeRequest converts a Scope into the create-request scope enum + entity ID.
func scopeRequest(scope Scope) (scopeType astrocore.CreateEnvironmentObjectRequestScope, scopeEntityID string) {
	if scope.DeploymentID != "" {
		return astrocore.CreateEnvironmentObjectRequestScopeDEPLOYMENT, scope.DeploymentID
	}
	return astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE, scope.WorkspaceID
}

// ErrAutoLinkRequiresWorkspace is returned when a caller asks to auto-link an
// object to all deployments while creating it at deployment scope. Auto-link
// is a workspace-scope concept; deployment-scope objects are already pinned
// to a single deployment.
var ErrAutoLinkRequiresWorkspace = errors.New("--auto-link applies only to workspace-scoped objects")

// validateAutoLink rejects auto-link=true on a deployment-scoped object. nil
// (flag unset) and false are no-ops.
func validateAutoLink(scope Scope, autoLink *bool) error {
	if autoLink != nil && *autoLink && scope.DeploymentID != "" {
		return ErrAutoLinkRequiresWorkspace
	}
	return nil
}

func buildListParams(scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, objectKey *string, resolveLinked, includeSecrets bool, limit int) *astrocore.ListEnvironmentObjectsParams {
	params := &astrocore.ListEnvironmentObjectsParams{
		ObjectType:    &objectType,
		ObjectKey:     objectKey,
		ShowSecrets:   &includeSecrets,
		ResolveLinked: &resolveLinked,
		Limit:         &limit,
	}
	if scope.WorkspaceID != "" {
		params.WorkspaceId = &scope.WorkspaceID
	} else if scope.DeploymentID != "" {
		params.DeploymentId = &scope.DeploymentID
	}
	return params
}

// normalizeListErr substitutes the friendlier org-level secrets-fetching
// guidance when applicable.
func normalizeListErr(httpResp *http.Response, body []byte) error {
	err := astrocore.NormalizeAPIError(httpResp, body)
	if err == nil {
		return nil
	}
	if IsSecretsFetchingNotAllowedError(err) {
		return errors.New(SecretsFetchingNotAllowedErrMsg)
	}
	return err
}
