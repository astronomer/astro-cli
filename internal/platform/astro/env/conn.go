package env

import (
	httpcontext "context"
	"errors"

	"github.com/astronomer/astro-cli/config"
	astrov1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
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
	// the existing Links/ExcludeLinks and auto-link flag or the platform drops
	// them. See echoPreservedFields.
	current, err := getObject(idOrKey, scope, objectTypeConn, false, astroV1Client)
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
		Connection:          patchConn(in, current.Connection),
		AutoLinkDeployments: in.AutoLinkDeployments,
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

// patchConn builds the update body's connection: what in sets, and the
// current value for each field it leaves unset.
//
// The platform does not patch a connection. It stores host, login, schema,
// port and auth type as sent, so a field left out of the body is cleared —
// `set db --type postgres --host new` blanked the login, schema and port it
// was never given. Only the password and extra keys are kept when absent,
// which is why those two pass through as given: GET masks them, and echoing
// the masked form back would be the thing that cleared them.
//
// The auth type describes one connection type, so it is kept only while the
// type stays the same.
func patchConn(in ConnInput, current *astrov1.EnvironmentObjectConnection) *astrov1.UpdateEnvironmentObjectConnectionRequest {
	req := &astrov1.UpdateEnvironmentObjectConnectionRequest{
		Type:     in.Type,
		Host:     in.Host,
		Login:    in.Login,
		Password: in.Password,
		Schema:   in.Schema,
		Port:     in.Port,
		Extra:    in.Extra,
	}
	if current == nil {
		return req
	}
	if req.Host == nil {
		req.Host = current.Host
	}
	if req.Login == nil {
		req.Login = current.Login
	}
	if req.Schema == nil {
		req.Schema = current.Schema
	}
	if req.Port == nil {
		req.Port = current.Port
	}
	if a := current.ConnectionAuthType; a != nil && a.Id != "" && a.AirflowType == in.Type {
		id := a.Id
		req.AuthTypeId = &id
	}
	return req
}

// DeleteConn deletes a connection by ID or key, and returns it as it was.
func DeleteConn(idOrKey string, scope Scope, astroV1Client astrov1.APIClient) (*astrov1.EnvironmentObject, error) {
	return deleteObject(idOrKey, scope, objectTypeConn, astroV1Client)
}
