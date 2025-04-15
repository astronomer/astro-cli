package environment

import (
	http_context "context"
	"errors"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

var ErrorEntityIDNotSpecified = errors.New("workspace or deployment ID must be specified")

func ListConnections(workspaceID, deploymentID string, coreClient astrocore.CoreClient) (map[string]astrocore.EnvironmentObjectConnection, error) {
	envObjs, err := listEnvironmentObjects(workspaceID, deploymentID, astrocore.CONNECTION, coreClient)
	if err != nil {
		return nil, err
	}
	connections := make(map[string]astrocore.EnvironmentObjectConnection)
	for i := range envObjs {
		connections[envObjs[i].ObjectKey] = *envObjs[i].Connection
	}

	return connections, nil
}

func listEnvironmentObjects(workspaceID, deploymentID string, objectType astrocore.ListEnvironmentObjectsParamsObjectType, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	showSecrets := true
	resolvedLinked := true
	limit := 1000
	listParams := &astrocore.ListEnvironmentObjectsParams{
		ObjectType:    &objectType,
		ShowSecrets:   &showSecrets,
		ResolveLinked: &resolvedLinked,
		Limit:         &limit,
	}

	switch {
	case deploymentID != "":
		// if the deployment is specified during the command, use that as the entity
		// that environment objects will be listed for
		listParams.DeploymentId = &deploymentID
	case workspaceID != "":
		// or, if the workspace is specified during the command, use that
		listParams.WorkspaceId = &workspaceID
	default:
		// otherwise, we don't have an entity to list for, so we return an empty list
		return nil, ErrorEntityIDNotSpecified
	}

	resp, err := coreClient.ListEnvironmentObjectsWithResponse(http_context.Background(), c.Organization, listParams)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	envObjsPaginated := *resp.JSON200
	envObjs := envObjsPaginated.EnvironmentObjects

	return envObjs, nil
}
