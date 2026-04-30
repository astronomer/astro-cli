//nolint:dupl // Each env-object type has its own typed Create/Update body shape; keeping the per-type service files parallel makes intent obvious and avoids the indirection a generic helper would require.
package env

import (
	httpcontext "context"
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeVar = astrocore.ENVIRONMENTVARIABLE

// ListVars returns ENVIRONMENT_VARIABLE objects for the given scope.
func ListVars(scope Scope, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	return listObjects(scope, objectTypeVar, resolveLinked, includeSecrets, coreClient)
}

// GetVar fetches a single env var by ID or key.
func GetVar(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeVar, includeSecrets, coreClient)
}

// CreateVar creates a new ENVIRONMENT_VARIABLE object in the given scope.
// Returns a minimal EnvironmentObject populated from inputs and the create
// response ID; the platform's create endpoint only returns the ID, so the
// full server-side object is not fetched.
func CreateVar(scope Scope, key, value string, isSecret bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	scopeType, scopeEntityID := scopeRequest(scope)

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

// UpdateVar updates the value of an existing env var. The API does not allow
// toggling IsSecret; callers wanting to change secret status must delete and
// recreate.
func UpdateVar(idOrKey string, scope Scope, value string, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	id, err := resolveID(idOrKey, scope, objectTypeVar, coreClient)
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
	if resp.JSON200 == nil {
		return nil, errors.New("update returned empty response body")
	}
	return resp.JSON200, nil
}

// DeleteVar deletes an env var by ID or key.
func DeleteVar(idOrKey string, scope Scope, coreClient astrocore.CoreClient) error {
	return deleteObject(idOrKey, scope, objectTypeVar, coreClient)
}
