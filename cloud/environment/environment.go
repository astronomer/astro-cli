package environment

import (
	http_context "context"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

func ListConnections(workspaceID, deploymentID string, coreClient astrocore.CoreClient) map[string]astrocore.EnvironmentObjectConnection {
	envObjs := listEnvironmentObjects(workspaceID, deploymentID, astrocore.ListEnvironmentObjectsParamsObjectTypeCONNECTION, coreClient)
	connections := make(map[string]astrocore.EnvironmentObjectConnection)
	for _, envObj := range envObjs {
		connections[envObj.ObjectKey] = *envObj.Connection
	}

	return connections
}

func listEnvironmentObjects(workspaceID, deploymentID string, objectType astrocore.ListEnvironmentObjectsParamsObjectType, coreClient astrocore.CoreClient) []astrocore.EnvironmentObject {
	c, err := config.GetCurrentContext()
	if err != nil {
		return []astrocore.EnvironmentObject{}
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
	case c.Workspace != "":
		// or, if the workspace is specified as part of the context, use that
		listParams.WorkspaceId = &c.Workspace
	default:
		// otherwise, we don't have an entity to list for, so we return an empty list
		return []astrocore.EnvironmentObject{}
	}

	resp, err := coreClient.ListEnvironmentObjectsWithResponse(http_context.Background(), c.Organization, listParams)
	if err != nil {
		return []astrocore.EnvironmentObject{}
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrocore.EnvironmentObject{}
	}
	envObjsPaginated := *resp.JSON200
	envObjs := envObjsPaginated.EnvironmentObjects

	return envObjs
}
