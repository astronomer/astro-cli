package workerqueue

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/pkg/errors"
)

var (
	ErrWorkerQueueDefaultOptions = errors.New("failed to get worker-queue default options")
	ErrInvalidWorkerQueueOption  = errors.New("worker-queue option is invalid")
)

func Create(ws, deploymentID, name string, isDefaultWQueue bool, wQueueMin, wQueueMax, wQueueConcurrency int, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment       astro.Deployment
		deloymentToUpdateInput    *astro.DeploymentUpdateInput
		err                       error
		workerQueueToCreate       astro.WorkerQueue
		wQueueListToCreate        []astro.WorkerQueue
		workerQueueDefaultOptions astro.WorkerQueueDefaultOptions
	)
	// get defaults for min-count, max-count and concurrency from API
	workerQueueDefaultOptions, err = GetWorkerQueueDefaultOptions(client)
	if err != nil {
		return err
	}

	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, client)
	if err != nil {
		return err
	}

	workerQueueToCreate = astro.WorkerQueue{
		Name:       name,
		IsDefault:  isDefaultWQueue,
		NodePoolID: "", // TODO query clusters to get the nodepoolID
	}
	workerQueueToCreate = setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, workerQueueToCreate, workerQueueDefaultOptions)

	err = IsWorkerQueueInputValid(workerQueueToCreate, workerQueueDefaultOptions)
	if err != nil {
		return err
	}

	if len(requestedDeployment.WorkerQueues) > 0 {
		// worker-queues exist so we add a new one to the existing list
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

// setWorkerQueueValues sets values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency
// Default values are used if the user did not request any

func setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate astro.WorkerQueue, workerQueueDefaultOptions astro.WorkerQueueDefaultOptions) astro.WorkerQueue {
	if wQueueMin != 0 {
		// use the value from the user input
		workerQueueToCreate.MinWorkerCount = wQueueMin
	} else {
		// set default value as user input did not have it
		workerQueueToCreate.MinWorkerCount = workerQueueDefaultOptions.MinWorkerCount.Default
	}

	if wQueueMax != 0 {
		// use the value from the user input
		workerQueueToCreate.MaxWorkerCount = wQueueMax
	} else {
		// set default value as user input did not have it
		workerQueueToCreate.MaxWorkerCount = workerQueueDefaultOptions.MaxWorkerCount.Default
	}

	if wQueueConcurrency != 0 {
		workerQueueToCreate.WorkerConcurrency = wQueueConcurrency
	} else {
		workerQueueToCreate.WorkerConcurrency = workerQueueDefaultOptions.WorkerConcurrency.Default
	}
	return workerQueueToCreate
}

// GetWorkerQueueDefaultOptions calls the workerqueues query
// It returns WorkerQueueDefaultOptions if the query succeeds
// An error is returned if it fails

func GetWorkerQueueDefaultOptions(client astro.Client) (astro.WorkerQueueDefaultOptions, error) {
	var (
		workerQueueDefaultOptions astro.WorkerQueueDefaultOptions
		err                       error
	)
	workerQueueDefaultOptions, err = client.GetWorkerQueueOptions()
	if err != nil {
		return astro.WorkerQueueDefaultOptions{}, err
	}
	return workerQueueDefaultOptions, nil
}

// IsWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions
// if it adheres to them, it returns nil
// ErrInvalidWorkerQueueOption is returned if min, max or concurrency are out of range

func IsWorkerQueueInputValid(requestedWorkerQueue astro.WorkerQueue, defaultOptions astro.WorkerQueueDefaultOptions) error {
	var errorMessage string
	if !(requestedWorkerQueue.MinWorkerCount >= defaultOptions.MinWorkerCount.Floor) ||
		!(requestedWorkerQueue.MinWorkerCount <= defaultOptions.MinWorkerCount.Ceiling) {
		errorMessage = fmt.Sprintf("min worker count must be between %d and %d", defaultOptions.MinWorkerCount.Floor, defaultOptions.MinWorkerCount.Ceiling)
		return errors.Wrap(ErrInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedWorkerQueue.MaxWorkerCount >= defaultOptions.MaxWorkerCount.Floor) ||
		!(requestedWorkerQueue.MaxWorkerCount <= defaultOptions.MaxWorkerCount.Ceiling) {
		errorMessage = fmt.Sprintf("max worker count must be between %d and %d", defaultOptions.MaxWorkerCount.Floor, defaultOptions.MaxWorkerCount.Ceiling)
		return errors.Wrap(ErrInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedWorkerQueue.WorkerConcurrency >= defaultOptions.WorkerConcurrency.Floor) ||
		!(requestedWorkerQueue.WorkerConcurrency <= defaultOptions.WorkerConcurrency.Ceiling) {
		errorMessage = fmt.Sprintf("worker concurrency must be between %d and %d", defaultOptions.WorkerConcurrency.Floor, defaultOptions.WorkerConcurrency.Ceiling)
		return errors.Wrap(ErrInvalidWorkerQueueOption, errorMessage)
	}
	return nil
}
