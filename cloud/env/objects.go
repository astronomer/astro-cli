package env

import (
	httpcontext "context"
	"errors"
	"fmt"
	"net/http"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/environment"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/config"
)

var (
	ErrScopeNotSpecified = errors.New("--workspace-id or --deployment-id must be specified")
	ErrScopeAmbiguous    = errors.New("--workspace-id and --deployment-id are mutually exclusive")
	ErrNotFound          = errors.New("environment object not found")
)

// defaultListLimit is the page size we request from list endpoints. Org-wide
// counts are typically well under this, so we don't paginate today.
const defaultListLimit = 1000

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

// listObjects returns env-objects of the given type within the scope.
// resolveLinked includes inherited workspace objects when listing at deployment scope.
// includeSecrets requests secret values from the server (subject to org policy).
func listObjects(scope Scope, objectType astrocore.ListEnvironmentObjectsParamsObjectType, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	params := buildListParams(scope, objectType, nil, resolveLinked, includeSecrets, defaultListLimit)

	resp, err := coreClient.ListEnvironmentObjectsWithResponse(httpcontext.Background(), c.Organization, params)
	if err != nil {
		return nil, err
	}
	if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200.EnvironmentObjects, nil
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

// followCreate fetches a created object via its returned ID. Used when the
// caller wants the full object back from the platform rather than the
// id-only synthesized one.
func followCreate(id string, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	resp, err := coreClient.GetEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
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
// guidance from cloud/environment when applicable.
func normalizeListErr(httpResp *http.Response, body []byte) error {
	err := astrocore.NormalizeAPIError(httpResp, body)
	if err == nil {
		return nil
	}
	if environment.IsSecretsFetchingNotAllowedError(err) {
		return errors.New(environment.SecretsFetchingNotAllowedErrMsg)
	}
	return err
}
