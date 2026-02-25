package deployment

import (
	"context"
	"fmt"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
)

// PackagesGet fetches the deployment and prints the current system packages.
func PackagesGet(ws, deploymentID, deploymentName string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
	if deploymentID == "" {
		d, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, coreClient)
		if err != nil {
			return err
		}
		deploymentID = d.Id
	}

	dep, err := coreGetDeployment("", deploymentID, coreClient)
	if err != nil {
		return err
	}

	if !isApiDriven(dep) {
		fmt.Fprintln(out, "This deployment is not API-driven. Packages are only supported for API-driven deployments.")
		return nil
	}

	pkgs := ""
	if dep.ApiDriven.Packages != nil {
		pkgs = *dep.ApiDriven.Packages
	}
	if pkgs == "" {
		fmt.Fprintln(out, "No packages set for this deployment.")
		return nil
	}

	fmt.Fprint(out, pkgs)
	if pkgs[len(pkgs)-1] != '\n' {
		fmt.Fprintln(out)
	}
	return nil
}

// PackagesSet updates the deployment's system packages by calling the update deployment endpoint.
func PackagesSet(ws, deploymentID, deploymentName, packagesContent string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
	if deploymentID == "" {
		d, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, coreClient)
		if err != nil {
			return err
		}
		deploymentID = d.Id
	}

	dep, err := coreGetDeployment("", deploymentID, coreClient)
	if err != nil {
		return err
	}

	if !isApiDriven(dep) {
		return fmt.Errorf("this deployment is not API-driven; packages are only supported for API-driven deployments")
	}

	deploymentType := ""
	if dep.Type != nil {
		deploymentType = string(*dep.Type)
	}

	var updateReq astrocore.UpdateDeploymentJSONRequestBody

	switch deploymentType {
	case "HYBRID":
		hybridReq := buildHybridUpdateRequest(dep, nil, &packagesContent)
		err = marshalUpdateRequest(&updateReq, hybridReq)
	default:
		hostedReq := buildHostedUpdateRequest(dep, nil, &packagesContent)
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

	fmt.Fprintf(out, "Successfully updated packages for deployment %s\n", dep.Name)
	return nil
}
