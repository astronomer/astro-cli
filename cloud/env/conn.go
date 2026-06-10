package env

import (
	httpcontext "context"
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/config"
)

const objectTypeConn = astrov1.CONNECTION

var errConnTypeRequired = errors.New("connection type is required (e.g. --type postgres)")

// ConnInput is the user-supplied data needed to create or update a connection.
// Type is required on create. All other fields are optional and overlaid as a
// partial update. AutoLinkDeployments, when non-nil, sets the workspace
// object's "auto-link to all deployments" flag.
type ConnInput struct {
	Type                string
	Host                *string
	Login               *string
	Password            *string
	Schema              *string
	Port                *int
	Extra               *map[string]any
	AutoLinkDeployments *bool
}

// ListConns returns CONNECTION objects for the given scope.
func ListConns(scope Scope, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) ([]astrov1.EnvironmentObject, error) {
	return listObjects(scope, objectTypeConn, resolveLinked, includeSecrets, astroV1Client)
}

// ListConnsByKey returns CONNECTION objects keyed by ObjectKey. Convenience
// for callers that want O(1) key lookup (e.g. injecting connections into a
// local Airflow container).
func ListConnsByKey(scope Scope, resolveLinked, includeSecrets bool, astroV1Client astrov1.APIClient) (map[string]astrov1.EnvironmentObjectConnection, error) {
	objs, err := ListConns(scope, resolveLinked, includeSecrets, astroV1Client)
	if err != nil {
		return nil, err
	}
	out := make(map[string]astrov1.EnvironmentObjectConnection, len(objs))
	for i := range objs {
		if objs[i].Connection != nil {
			out[objs[i].ObjectKey] = *objs[i].Connection
		}
	}
	return out, nil
}

// GetConn fetches a single connection by ID or key.
func GetConn(idOrKey string, scope Scope, includeSecrets bool, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return getObject(idOrKey, scope, objectTypeConn, includeSecrets, astroV1Client)
}

// CreateConn creates a new CONNECTION object in the given scope.
func CreateConn(scope Scope, key string, in ConnInput, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := validateAutoLink(scope, in.AutoLinkDeployments); err != nil {
		return nil, err
	}
	if in.Type == "" {
		return nil, errConnTypeRequired
	}
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	scopeType, scopeEntityID := scopeRequest(scope)
	body := astrov1.CreateEnvironmentObjectJSONRequestBody{
		ObjectKey:     key,
		ObjectType:    astrov1.CreateEnvironmentObjectRequestObjectTypeCONNECTION,
		Scope:         scopeType,
		ScopeEntityId: scopeEntityID,
		Connection: &astrov1.CreateEnvironmentObjectConnectionRequest{
			Type:     in.Type,
			Host:     in.Host,
			Login:    in.Login,
			Password: in.Password,
			Schema:   in.Schema,
			Port:     in.Port,
			Extra:    in.Extra,
		},
		AutoLinkDeployments: in.AutoLinkDeployments,
	}

	resp, err := astroV1Client.CreateEnvironmentObjectWithResponse(httpcontext.Background(), c.Organization, body)
	if err != nil {
		return nil, err
	}
	if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
		return nil, err
	}
	id := resp.JSON200.Id
	connection := &astrov1.EnvironmentObjectConnection{
		Type:     in.Type,
		Host:     in.Host,
		Login:    in.Login,
		Password: in.Password,
		Schema:   in.Schema,
		Port:     in.Port,
		Extra:    in.Extra,
	}
	return &astrov1.EnvironmentObject{
		Id:            &id,
		ObjectKey:     key,
		ObjectType:    astrov1.EnvironmentObjectObjectType(astrov1.CONNECTION),
		Scope:         astrov1.EnvironmentObjectScope(scopeType),
		ScopeEntityId: scopeEntityID,
		Connection:    connection,
	}, nil
}

// UpdateConn updates an existing connection. Type is required by the API.
func UpdateConn(idOrKey string, scope Scope, in ConnInput, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	if in.Type == "" {
		return nil, errConnTypeRequired
	}
	if err := validateAutoLink(scope, in.AutoLinkDeployments); err != nil {
		return nil, err
	}
	// Fetch the full object (not just the ID): the update body must round-trip
	// the existing Links/ExcludeLinks or the platform drops them. See
	// echoLinksAndExcludes.
	current, err := getObject(idOrKey, scope, objectTypeConn, false, astroV1Client)
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
		Connection: &astrov1.UpdateEnvironmentObjectConnectionRequest{
			Type:     in.Type,
			Host:     in.Host,
			Login:    in.Login,
			Password: in.Password,
			Schema:   in.Schema,
			Port:     in.Port,
			Extra:    in.Extra,
		},
		AutoLinkDeployments: in.AutoLinkDeployments,
	}
	echoLinksAndExcludes(&body, current)
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

// DeleteConn deletes a connection by ID or key.
func DeleteConn(idOrKey string, scope Scope, astroV1Client astrov1.APIClient) error {
	return deleteObject(idOrKey, scope, objectTypeConn, astroV1Client)
}
