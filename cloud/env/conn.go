package env

import (
	httpcontext "context"
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeConn = astrocore.CONNECTION

// ConnInput is the user-supplied data needed to create or update a connection.
// Type is required on create. All other fields are optional and overlaid as a
// partial update.
type ConnInput struct {
	Type     string
	Host     *string
	Login    *string
	Password *string
	Schema   *string
	Port     *int
	Extra    *map[string]any
}

// ListConns returns CONNECTION objects for the given scope.
func ListConns(scope Scope, resolveLinked, includeSecrets bool, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	return listObjects(scope, objectTypeConn, resolveLinked, includeSecrets, coreClient)
}

// GetConn fetches a single connection by ID or key.
func GetConn(idOrKey string, scope Scope, includeSecrets bool, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeConn, includeSecrets, coreClient)
}

// CreateConn creates a new CONNECTION object in the given scope.
func CreateConn(scope Scope, key string, in ConnInput, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if in.Type == "" {
		return nil, errors.New("connection type is required (e.g. --type postgres)")
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	scopeType, scopeEntityID := scopeRequest(scope)
	body := astrocore.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrocore.CreateEnvironmentObjectRequestObjectTypeCONNECTION,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		Connection: &astrocore.CreateEnvironmentObjectConnectionRequest{
			Type:     in.Type,
			Host:     in.Host,
			Login:    in.Login,
			Password: in.Password,
			Schema:   in.Schema,
			Port:     in.Port,
			Extra:    in.Extra,
		},
	}

	resp, err := coreClient.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	return followCreate(resp.JSON200.Id, coreClient)
}

// UpdateConn updates an existing connection. Type is required by the API.
func UpdateConn(idOrKey string, scope Scope, in ConnInput, coreClient astrocore.CoreClient) (*astrocore.EnvironmentObject, error) {
	if in.Type == "" {
		return nil, errors.New("connection type is required (e.g. --type postgres)")
	}
	id, err := resolveID(idOrKey, scope, objectTypeConn, coreClient)
	if err != nil {
		return nil, err
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	body := astrocore.UpdateEnvironmentObjectJSONRequestBody{
		Connection: &astrocore.UpdateEnvironmentObjectConnectionRequest{
			Type:     in.Type,
			Host:     in.Host,
			Login:    in.Login,
			Password: in.Password,
			Schema:   in.Schema,
			Port:     in.Port,
			Extra:    in.Extra,
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

// DeleteConn deletes a connection by ID or key.
func DeleteConn(idOrKey string, scope Scope, coreClient astrocore.CoreClient) error {
	return deleteObject(idOrKey, scope, objectTypeConn, coreClient)
}
