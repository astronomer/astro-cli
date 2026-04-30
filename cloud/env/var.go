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
	ErrNotFound          = errors.New("environment variable not found")
)

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

// ListVars returns ENVIRONMENT_VARIABLE objects for the given scope.
// resolveLinked includes inherited workspace objects when listing at deployment scope.
// includeSecrets requests secret values from the server (subject to org policy).
func ListVars(scope Scope, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	limit := 1000
	params := buildListParams(scope, nil, resolveLinked, includeSecrets, limit)

	resp, err := coreClient.ListEnvironmentObjectsWithResponse(httpcontext.Background(), c.Organization, params)
	if err != nil {
		return nil, err
	}
	if err := normalizeListErr(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200.EnvironmentObjects, nil
}

// GetVar fetches a single env var by ID or key.
//
// The platform has no GET-by-key endpoint; for keys, we filter the list endpoint
// server-side via ObjectKey and force resolveLinked=false so the returned ID is
// addressable in subsequent CRUD calls.
func GetVar(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
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
	params := buildListParams(scope, &idOrKey, false, includeSecrets, 2)

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

// CreateVar creates a new ENVIRONMENT_VARIABLE object in the given scope.
// Returns a minimal EnvironmentObject populated from the create response and the inputs;
// the platform's create endpoint only returns the new ID, so callers wanting the full
// object should follow up with GetVar.
func CreateVar(scope Scope, key, value string, isSecret bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	scopeType := astrocore.CreateEnvironmentObjectRequestScopeWORKSPACE
	scopeEntityID := scope.WorkspaceID
	if scope.DeploymentID != "" {
		scopeType = astrocore.CreateEnvironmentObjectRequestScopeDEPLOYMENT
		scopeEntityID = scope.DeploymentID
	}

	body := astrocore.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrocore.CreateEnvironmentObjectRequestObjectTypeENVIRONMENTVARIABLE,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		EnvironmentVariable: &astrocore.CreateEnvironmentObjectEnvironmentVariableRequest{
			Value:    &value,
			IsSecret: &isSecret,
		},
	}

	resp, err := coreClient.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	id := resp.JSON200.Id
	return &astrocore.EnvironmentObject{
		Id:            &id,
		ObjectKey:     key,
		ObjectType:    astrocore.EnvironmentObjectObjectType(astrocore.ENVIRONMENTVARIABLE),
		Scope:         astrocore.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		EnvironmentVariable: &astrocore.EnvironmentObjectEnvironmentVariable{
			Value:    value,
			IsSecret: isSecret,
		},
	}, nil
}

// UpdateVar updates the value of an existing env var. The API does not allow toggling IsSecret;
// callers wanting to change secret status must delete and recreate.
func UpdateVar(idOrKey string, scope Scope, value string, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	id, err := resolveID(idOrKey, scope, coreClient)
	if err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrocore.UpdateEnvironmentObjectJSONRequestBody{
		EnvironmentVariable: &astrocore.UpdateEnvironmentObjectEnvironmentVariableRequest{
			Value: &value,
		},
	}
	resp, err := coreClient.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, id, body)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// DeleteVar deletes an env var by ID or key.
func DeleteVar(idOrKey string, scope Scope, coreClient astrocore.CoreClient) error {
	id, err := resolveID(idOrKey, scope, coreClient)
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
// otherwise looks up the ID by key. UpdateVar/DeleteVar use this so callers passing
// an ID don't pay for a redundant GET.
func resolveID(idOrKey string, scope Scope, coreClient astrocore.CoreClient) (string, error) {
	if organization.IsCUID(idOrKey) {
		return idOrKey, nil
	}
	existing, err := GetVar(idOrKey, scope, false, coreClient)
	if err != nil {
		return "", err
	}
	if existing.Id == nil || *existing.Id == "" {
		return "", fmt.Errorf("environment variable %q has no id", idOrKey)
	}
	return *existing.Id, nil
}

func buildListParams(scope Scope, objectKey *string, resolveLinked, includeSecrets bool, limit int) *astrocore.ListEnvironmentObjectsParams {
	objType := astrocore.ENVIRONMENTVARIABLE
	params := &astrocore.ListEnvironmentObjectsParams{
		ObjectType:    &objType,
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

// normalizeListErr wraps NormalizeAPIError to substitute the friendlier
// org-level secrets-fetching policy guidance from cloud/environment.
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
