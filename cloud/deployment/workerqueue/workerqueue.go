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
	ErrCannotUpdateExistingQueue = errors.New("worker-queue exists")
)

func Create(ws, deploymentID, name string, isDefaultWQueue bool, wQueueMin, wQueueMax, wQueueConcurrency int, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment       astro.Deployment
		err                       error
		errHelp                   string
		workerQueueToCreate       *astro.WorkerQueue
		wQueueListToCreate        []astro.WorkerQueue
		workerQueueDefaultOptions astro.WorkerQueueDefaultOptions
		pool                      astro.NodePool
		nodePoolIDForWorkerQueue  string
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, client)
	if err != nil {
		return err
	}

	// TODO should we only get defaults once via init()
	// get defaults for min-count, max-count and concurrency from API
	workerQueueDefaultOptions, err = GetWorkerQueueDefaultOptions(client)
	if err != nil {
		return err
	}

	// Get the default nodepool ID to use
	for _, pool = range requestedDeployment.Cluster.NodePools {
		if pool.IsDefault {
			nodePoolIDForWorkerQueue = pool.ID
		}
	}

	// TODO user selects nodePoolID for creating a queue on non-default nodepools

	workerQueueToCreate = &astro.WorkerQueue{
		Name:       name,
		IsDefault:  isDefaultWQueue,
		NodePoolID: nodePoolIDForWorkerQueue,
	}
	workerQueueToCreate = setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, workerQueueToCreate, workerQueueDefaultOptions)

	err = IsWorkerQueueInputValid(workerQueueToCreate, workerQueueDefaultOptions)
	if err != nil {
		return err
	}

	if QueueExists(requestedDeployment.WorkerQueues, workerQueueToCreate) {
		// create does not allow updating existing queues
		errHelp = fmt.Sprintf("use worker-queue update %s instead", workerQueueToCreate.Name)
		return errors.Wrap(ErrCannotUpdateExistingQueue, errHelp)
	}

	// workerQueueToCreate does not exist so we add it
	wQueueListToCreate = append(requestedDeployment.WorkerQueues, *workerQueueToCreate) //nolint

	// update the deployment with the new list of worker-queues
	err = deployment.Update(requestedDeployment.ID, "", ws, "", 0, 0, 0, wQueueListToCreate, true, client)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "worker-queue %s for %s in %s workspace created\n", workerQueueToCreate.Name, requestedDeployment.ID, ws)
	return nil
}

// setWorkerQueueValues sets values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency
// Default values are used if the user did not request any

func setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate *astro.WorkerQueue, workerQueueDefaultOptions astro.WorkerQueueDefaultOptions) *astro.WorkerQueue {
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

func IsWorkerQueueInputValid(requestedWorkerQueue *astro.WorkerQueue, defaultOptions astro.WorkerQueueDefaultOptions) error {
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

// QueueExists takes a []existingQueues and a queueToCreate as arguments
// It returns true if queueToCreate exists in []existingQueues
// It returns false if queueToCreate does not exist in []existingQueues

func QueueExists(existingQueues []astro.WorkerQueue, queueToCreate *astro.WorkerQueue) bool {
	for _, queue := range existingQueues {
		if queue.ID == queueToCreate.ID {
			// queueToCreate exists
			return true
		}
		if queue.Name == queueToCreate.Name {
			// queueToCreate exists
			return true
		}
	}
	return false
}
