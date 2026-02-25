package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/config"
)

// coreGetDeployment gets a deployment using the core (v1alpha1) API client.
var coreGetDeployment = func(orgID, deploymentID string, coreClient astrocore.CoreClient) (*astrocore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return nil, err
		}
		orgID = c.Organization
	}
	resp, err := coreClient.GetDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return nil, err
	}
	return resp.JSON200, nil
}

// isApiDriven checks whether a deployment is API-driven.
func isApiDriven(dep *astrocore.Deployment) bool {
	return dep.ApiDriven != nil && dep.ApiDriven.IsApiDriven != nil && *dep.ApiDriven.IsApiDriven
}

// RequirementsGet fetches the deployment and prints the current requirements.
func RequirementsGet(ws, deploymentID, deploymentName string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
	// Resolve deployment ID using the platform client if needed
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
		fmt.Fprintln(out, "This deployment is not API-driven. Requirements are only supported for API-driven deployments.")
		return nil
	}

	reqs := ""
	if dep.ApiDriven.Requirements != nil {
		reqs = *dep.ApiDriven.Requirements
	}
	if reqs == "" {
		fmt.Fprintln(out, "No requirements set for this deployment.")
		return nil
	}

	fmt.Fprint(out, reqs)
	if reqs[len(reqs)-1] != '\n' {
		fmt.Fprintln(out)
	}
	return nil
}

// RequirementsSet updates the deployment's requirements by calling the update deployment endpoint.
func RequirementsSet(ws, deploymentID, deploymentName, requirementsContent string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
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
		return fmt.Errorf("this deployment is not API-driven; requirements are only supported for API-driven deployments")
	}

	// Build the update request based on deployment type
	deploymentType := ""
	if dep.Type != nil {
		deploymentType = string(*dep.Type)
	}

	var updateReq astrocore.UpdateDeploymentJSONRequestBody

	switch deploymentType {
	case "HYBRID":
		hybridReq := buildHybridUpdateRequest(dep, &requirementsContent, nil)
		err = marshalUpdateRequest(&updateReq, hybridReq)
	default:
		hostedReq := buildHostedUpdateRequest(dep, &requirementsContent, nil)
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

	fmt.Fprintf(out, "Successfully updated requirements for deployment %s\n", dep.Name)
	return nil
}

func buildHostedUpdateRequest(dep *astrocore.Deployment, requirements, packages *string) astrocore.UpdateHostedDeploymentRequest {
	executor := astrocore.UpdateHostedDeploymentRequestExecutor("")
	if dep.Executor != nil {
		executor = astrocore.UpdateHostedDeploymentRequestExecutor(*dep.Executor)
	}

	schedulerSize := astrocore.UpdateHostedDeploymentRequestSchedulerSize("")
	if dep.SchedulerSize != nil {
		schedulerSize = astrocore.UpdateHostedDeploymentRequestSchedulerSize(*dep.SchedulerSize)
	}

	depType := astrocore.UpdateHostedDeploymentRequestType("")
	if dep.Type != nil {
		depType = astrocore.UpdateHostedDeploymentRequestType(*dep.Type)
	}

	envVars := buildEnvVarsRequest(dep)

	isHA := false
	if dep.IsHighAvailability != nil {
		isHA = *dep.IsHighAvailability
	}

	req := astrocore.UpdateHostedDeploymentRequest{
		Name:                           dep.Name,
		Description:                    dep.Description,
		WorkspaceId:                    dep.WorkspaceId,
		Executor:                       executor,
		IsCicdEnforced:                 dep.IsCicdEnforced,
		IsDagDeployEnabled:             dep.IsDagDeployEnabled,
		SchedulerSize:                  schedulerSize,
		IsHighAvailability:             isHA,
		Type:                           depType,
		EnvironmentVariables:           envVars,
		Requirements:                   requirements,
		Packages:                       packages,
		WorkloadIdentity:               dep.WorkloadIdentity,
		ResourceQuotaCpu:               dep.ResourceQuotaCpu,
		ResourceQuotaMemory:            dep.ResourceQuotaMemory,
		DefaultTaskPodCpu:              dep.DefaultTaskPodCpu,
		DefaultTaskPodMemory:           dep.DefaultTaskPodMemory,
		DefaultTaskPodEphemeralStorage: dep.DefaultTaskPodEphemeralStorage,
		WorkerQueues:                   buildWorkerQueuesRequest(dep),
	}

	if dep.RemoteExecution != nil {
		req.RemoteExecution = &astrocore.DeploymentRemoteExecutionRequest{
			Enabled:       dep.RemoteExecution.Enabled,
			TaskLogBucket: dep.RemoteExecution.TaskLogBucket,
		}
	}

	return req
}

func buildHybridUpdateRequest(dep *astrocore.Deployment, requirements, packages *string) astrocore.UpdateHybridDeploymentRequest {
	executor := astrocore.UpdateHybridDeploymentRequestExecutor("")
	if dep.Executor != nil {
		executor = astrocore.UpdateHybridDeploymentRequestExecutor(*dep.Executor)
	}

	depType := astrocore.UpdateHybridDeploymentRequestType("")
	if dep.Type != nil {
		depType = astrocore.UpdateHybridDeploymentRequestType(*dep.Type)
	}

	envVars := buildEnvVarsRequest(dep)

	au := 5
	if dep.SchedulerAu != nil {
		au = *dep.SchedulerAu
	}
	replicas := dep.SchedulerReplicas

	return astrocore.UpdateHybridDeploymentRequest{
		Name:                 dep.Name,
		Description:          dep.Description,
		WorkspaceId:          dep.WorkspaceId,
		Executor:             executor,
		IsCicdEnforced:       dep.IsCicdEnforced,
		IsDagDeployEnabled:   dep.IsDagDeployEnabled,
		Type:                 depType,
		EnvironmentVariables: envVars,
		Requirements:         requirements,
		Packages:             packages,
		WorkloadIdentity:     dep.WorkloadIdentity,
		Scheduler: astrocore.DeploymentInstanceSpecRequest{
			Au:       au,
			Replicas: replicas,
		},
	}
}

// marshalUpdateRequest directly marshals the request body into the union type,
// bypassing the generated From* methods which hardcode discriminator defaults.
func marshalUpdateRequest(req *astrocore.UpdateDeploymentJSONRequestBody, body interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return req.UnmarshalJSON(b)
}

func buildWorkerQueuesRequest(dep *astrocore.Deployment) *[]astrocore.MutateWorkerQueueRequest {
	if dep.WorkerQueues == nil {
		return nil
	}
	queues := make([]astrocore.MutateWorkerQueueRequest, 0, len(*dep.WorkerQueues))
	for _, wq := range *dep.WorkerQueues {
		id := wq.Id
		var machine *astrocore.MutateWorkerQueueRequestAstroMachine
		if wq.AstroMachine != nil {
			m := astrocore.MutateWorkerQueueRequestAstroMachine(*wq.AstroMachine)
			machine = &m
		}
		queues = append(queues, astrocore.MutateWorkerQueueRequest{
			Id:                  &id,
			Name:                wq.Name,
			IsDefault:           wq.IsDefault,
			MaxWorkerCount:      wq.MaxWorkerCount,
			MinWorkerCount:      wq.MinWorkerCount,
			WorkerConcurrency:   wq.WorkerConcurrency,
			AstroMachine:        machine,
			NodePoolId:          wq.NodePoolId,
			PodEphemeralStorage: wq.PodEphemeralStorage,
		})
	}
	return &queues
}

func buildEnvVarsRequest(dep *astrocore.Deployment) []astrocore.DeploymentEnvironmentVariableRequest {
	envVars := make([]astrocore.DeploymentEnvironmentVariableRequest, 0)
	if dep.EnvironmentVariables != nil {
		for _, v := range *dep.EnvironmentVariables {
			envVars = append(envVars, astrocore.DeploymentEnvironmentVariableRequest{
				Key:      v.Key,
				Value:    v.Value,
				IsSecret: v.IsSecret,
			})
		}
	}
	return envVars
}
