package deployment

import (
	"context"
	"fmt"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
)

// RuntimeVersionSet updates the deployment's runtime version by calling the update deployment endpoint.
func RuntimeVersionSet(ws, deploymentID, deploymentName, runtimeVersion string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
	// Resolve deployment ID using the platform client if needed
	if deploymentID == "" {
		d, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, coreClient)
		if err != nil {
			return err
		}
		deploymentID = d.Id
	}

	// Get the current deployment from core API to have all required fields
	dep, err := coreGetDeployment("", deploymentID, coreClient)
	if err != nil {
		return err
	}

	if !isApiDriven(dep) {
		return fmt.Errorf("this deployment is not API-driven; runtime version upgrades are only supported for API-driven deployments")
	}

	// Build the update request based on deployment type
	deploymentType := ""
	if dep.Type != nil {
		deploymentType = string(*dep.Type)
	}

	var updateReq astrocore.UpdateDeploymentJSONRequestBody

	switch deploymentType {
	case "HYBRID":
		hybridReq := buildHybridUpdateRequest(dep, nil, nil)
		hybridReq.AstroRuntimeVersion = &runtimeVersion
		err = marshalUpdateRequest(&updateReq, hybridReq)
	default:
		hostedReq := buildHostedUpdateRequest(dep, nil, nil)
		hostedReq.AstroRuntimeVersion = &runtimeVersion
		err = marshalUpdateRequest(&updateReq, hostedReq)
	}
	if err != nil {
		return fmt.Errorf("failed to build update request: %w", err)
	}

	resp, err := coreClient.UpdateDeploymentWithResponse(
		context.Background(),
		dep.OrganizationId,
		dep.Id,
		updateReq,
	)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Successfully updated runtime version for deployment %s to %s\n", dep.Name, runtimeVersion)
	return nil
}
