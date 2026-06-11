//nolint:dupl // Each env-object type has its own typed Create/Update body shape; keeping the per-type service files parallel makes intent obvious and avoids the indirection a generic helper would require.
package env

import (
	httpcontext "context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeVar = astrov1.ENVIRONMENTVARIABLE

// ListVars returns ENVIRONMENT_VARIABLE objects for the given scope.
func ListVars(scope Scope, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) ([]astrov1.EnvironmentObject, error) {
	return listObjects(scope, objectTypeVar, resolveLinked, includeSecrets, astroV1Client)
}

// GetVar fetches a single env var by ID or key.
func GetVar(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeVar, includeSecrets, astroV1Client)
}

// CreateVar creates a new ENVIRONMENT_VARIABLE object in the given scope.
// autoLink, when non-nil, sets the object's "auto-link to all deployments"
// flag (workspace scope only).
//
// Returns a minimal EnvironmentObject populated from inputs and the create
// response ID; the platform's create endpoint only returns the ID, so the
// full server-side object is not fetched.
func CreateVar(scope Scope, key, value string, isSecret bool, autoLink *bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := validateAutoLink(scope, autoLink); err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	scopeType, scopeEntityID := scopeRequest(scope)

	body := astrov1.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrov1.CreateEnvironmentObjectRequestObjectTypeENVIRONMENTVARIABLE,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		EnvironmentVariable: &astrov1.CreateEnvironmentObjectEnvironmentVariableRequest{
			Value:    &value,
			IsSecret: &isSecret,
		},
		AutoLinkDeployments: autoLink,
	}

	resp, err := astroV1Client.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	id := resp.JSON200.Id
	return &astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     key,
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.ENVIRONMENTVARIABLE),
		Scope:         astrov1.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		EnvironmentVariable: &astrov1.EnvironmentObjectEnvironmentVariable{
			Value:    value,
			IsSecret: isSecret,
		},
	}, nil
}

// UpdateVar updates the value of an existing env var. The API does not allow
// toggling IsSecret; callers wanting to change secret status must delete and
// recreate. autoLink, when non-nil, toggles the "auto-link to all deployments"
// flag (workspace scope only).
func UpdateVar(idOrKey string, scope Scope, value string, autoLink *bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	// Fetch the full object (not just the ID): the update body must round-trip
	// the existing Links/ExcludeLinks and auto-link flag or the platform drops
	// them. See echoPreservedFields.
	current, err := getObject(idOrKey, scope, objectTypeVar, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	if current.Id == nil || *current.Id == "" {
		return nil, fmt.Errorf("environment object %q has no id", idOrKey)
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrov1.UpdateEnvironmentObjectJSONRequestBody{
		EnvironmentVariable: &astrov1.UpdateEnvironmentObjectEnvironmentVariableRequest{
			Value: &value,
		},
		AutoLinkDeployments: autoLink,
	}
	echoPreservedFields(&body, current)
	resp, err := astroV1Client.UpdateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, *current.Id, body)
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

// DeleteVar deletes an env var by ID or key.
func DeleteVar(idOrKey string, scope Scope, astroV1Client astrov1.APIClient) error {
	return deleteObject(idOrKey, scope, objectTypeVar, astroV1Client)
}
