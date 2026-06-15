//nolint:dupl // Mirror of var.go for AIRFLOW_VARIABLE; see comment there.
package env

import (
	httpcontext "context"
	"errors"

	astrov1 "github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeAirflowVar = astrov1.AIRFLOWVARIABLE

// ListAirflowVars returns AIRFLOW_VARIABLE objects for the given scope.
func ListAirflowVars(scope Scope, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) ([]astrov1.EnvironmentObject, error) {
	return listObjects(scope, objectTypeAirflowVar, resolveLinked, includeSecrets, astroV1Client)
}

// GetAirflowVar fetches a single Airflow variable by ID or key.
func GetAirflowVar(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeAirflowVar, includeSecrets, astroV1Client)
}

// CreateAirflowVar creates a new AIRFLOW_VARIABLE object. autoLink, when
// non-nil, sets the "auto-link to all deployments" flag (workspace scope only).
func CreateAirflowVar(scope Scope, key, value string, isSecret bool, autoLink *bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
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
		ObjectType:    astrov1.CreateEnvironmentObjectRequestObjectTypeAIRFLOWVARIABLE,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		AirflowVariable: &astrov1.CreateEnvironmentObjectAirflowVariableRequest{
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
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.AIRFLOWVARIABLE),
		Scope:         astrov1.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		AirflowVariable: &astrov1.EnvironmentObjectAirflowVariable{
			Value:    value,
			IsSecret: isSecret,
		},
	}, nil
}

// UpdateAirflowVar updates the value of an existing Airflow variable. The API
// does not allow toggling IsSecret. autoLink, when non-nil, toggles the
// "auto-link to all deployments" flag (workspace scope only).
func UpdateAirflowVar(idOrKey string, scope Scope, value string, autoLink *bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	// Fetch the full object (not just the ID): the update body must round-trip
	// the existing Links/ExcludeLinks and auto-link flag or the platform drops
	// them. See echoPreservedFields.
	current, err := getObject(idOrKey, scope, objectTypeAirflowVar, false, astroV1Client)
	if err != nil {
		return nil, err
	}
	id, err := objectID(current, idOrKey)
	if err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrov1.UpdateEnvironmentObjectJSONRequestBody{
		AirflowVariable: &astrov1.UpdateEnvironmentObjectAirflowVariableRequest{
			Value: &value,
		},
		AutoLinkDeployments: autoLink,
	}
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

// DeleteAirflowVar deletes an Airflow variable by ID or key.
func DeleteAirflowVar(idOrKey string, scope Scope, astroV1Client astrov1.APIClient) error {
	return deleteObject(idOrKey, scope, objectTypeAirflowVar, astroV1Client)
}
