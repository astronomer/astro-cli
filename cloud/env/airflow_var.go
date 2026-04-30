//nolint:dupl // Mirror of var.go for AIRFLOW_VARIABLE; see comment there.
package env

import (
	httpcontext "context"
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeAirflowVar = astrocore.AIRFLOWVARIABLE

// ListAirflowVars returns AIRFLOW_VARIABLE objects for the given scope.
func ListAirflowVars(scope Scope, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	return listObjects(scope, objectTypeAirflowVar, resolveLinked, includeSecrets, coreClient)
}

// GetAirflowVar fetches a single Airflow variable by ID or key.
func GetAirflowVar(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeAirflowVar, includeSecrets, coreClient)
}

// CreateAirflowVar creates a new AIRFLOW_VARIABLE object.
func CreateAirflowVar(scope Scope, key, value string, isSecret bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
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
		ObjectType:    astrocore.CreateEnvironmentObjectRequestObjectTypeAIRFLOWVARIABLE,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		AirflowVariable: &astrocore.CreateEnvironmentObjectAirflowVariableRequest{
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
		ObjectType:    astrocore.EnvironmentObjectObjectType(astrocore.AIRFLOWVARIABLE),
		Scope:         astrocore.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		AirflowVariable: &astrocore.EnvironmentObjectAirflowVariable{
			Value:    value,
			IsSecret: isSecret,
		},
	}, nil
}

// UpdateAirflowVar updates the value of an existing Airflow variable. The API
// does not allow toggling IsSecret.
func UpdateAirflowVar(idOrKey string, scope Scope, value string, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	id, err := resolveID(idOrKey, scope, objectTypeAirflowVar, coreClient)
	if err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrocore.UpdateEnvironmentObjectJSONRequestBody{
		AirflowVariable: &astrocore.UpdateEnvironmentObjectAirflowVariableRequest{
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

// DeleteAirflowVar deletes an Airflow variable by ID or key.
func DeleteAirflowVar(idOrKey string, scope Scope, coreClient astrocore.CoreClient) error {
	return deleteObject(idOrKey, scope, objectTypeAirflowVar, coreClient)
}
