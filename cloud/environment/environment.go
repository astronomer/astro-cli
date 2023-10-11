package environment

import (
	http_context "context"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
)

func ListConnections(deploymentId string, coreClient astrocore.CoreClient) (map[string]astrocore.EnvironmentObjectConnection, error) {
	envObjs, err := listEnvironmentObjects(deploymentId, astrocore.ListEnvironmentObjectsParamsObjectTypeCONNECTION, coreClient)
	if err != nil {
		return nil, err
	}
	connections := make(map[string]astrocore.EnvironmentObjectConnection)
	for _, envObj := range envObjs {
		connections[envObj.ObjectKey] = *envObj.Connection
	}

	return connections, nil
}

func listEnvironmentObjects(deploymentId string, objectType astrocore.ListEnvironmentObjectsParamsObjectType, coreClient astrocore.CoreClient) ([]astrocore.EnvironmentObject, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return []astrocore.EnvironmentObject{}, nil
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
	if deploymentId != "" {
		// if the deployment is specified during the command, use that as the entity
		// that environment objects will be listed for
		listParams.DeploymentId = &deploymentId
	} else if c.Workspace != "" {
		// or, if the workspace is specified as part of the context, use that
		listParams.WorkspaceId = &c.Workspace
	} else {
		// otherwise, we don't have an entity to list for, so we return an empty list
		return []astrocore.EnvironmentObject{}, nil
	}
	resp, err := coreClient.ListEnvironmentObjectsWithResponse(http_context.Background(), c.OrganizationShortName, listParams)
	if err != nil {
		return []astrocore.EnvironmentObject{}, nil
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrocore.EnvironmentObject{}, nil
	}
	envObjsPaginated := *resp.JSON200
	envObjs := envObjsPaginated.EnvironmentObjects

	return envObjs, nil
}
