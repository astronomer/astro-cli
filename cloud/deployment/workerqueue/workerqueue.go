package workerqueue

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
)

func Create(ws, deploymentID, name string, isDefaultWQueue bool, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment    astro.Deployment
		deloymentToUpdateInput *astro.DeploymentUpdateInput
		err                    error
		workerQueueToCreate    astro.WorkerQueue
		wQueueListToCreate     []astro.WorkerQueue
	)
	// TODO get defaults for min-count, max-count and concurrency from API

	// TODO get the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, client)
	if err != nil {
		return err
	}

	workerQueueToCreate = astro.WorkerQueue{
		Name:              name,
		IsDefault:         isDefaultWQueue,
		MaxWorkerCount:    0,
		MinWorkerCount:    0,
		WorkerConcurrency: 0,
		NodePoolID:        "", // TODO query clusters to get the nodepoolID
	}
	// TODO is this the correct way to get worker queues?
	if len(requestedDeployment.WorkerQueues) > 0 {
		wQueueListToCreate = append(requestedDeployment.WorkerQueues, workerQueueToCreate) //nolint
		// TODO if worker-queue exists, update it with requested values
		// TODO should workerqueue.Create() do this?
	}

	// no worker-queues exist so create a new list of one worker queue
	wQueueListToCreate = []astro.WorkerQueue{workerQueueToCreate}
	deloymentToUpdateInput = &astro.DeploymentUpdateInput{
		ID:             deploymentID,
		OrchestratorID: requestedDeployment.Orchestrator.ID, // TODO do we need to provide this?
		Label:          requestedDeployment.Label,           // TODO do we need to provide this?
		Description:    requestedDeployment.Description,     // TODO do we need to provide this?
		DeploymentSpec: astro.DeploymentCreateSpec{},        // TODO do we need to provide this?
		WorkerQueues:   wQueueListToCreate,
	}
	_, err = client.UpdateDeployment(deloymentToUpdateInput)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "worker-queue %s for %s in %s workspace created\n", workerQueueToCreate.Name, deploymentID, ws)
	return nil
}
