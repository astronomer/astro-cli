package workerqueue

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	errWorkerQueueDefaultOptions = errors.New("failed to get worker queue default options")
	errInvalidWorkerQueueOption  = errors.New("worker queue option is invalid")
	errCannotUpdateExistingQueue = errors.New("worker queue already exists")
	errInvalidNodePool           = errors.New("node pool selection failed")
)

func Create(ws, deploymentID, deploymentName, name, workerType string, wQueueMin, wQueueMax, wQueueConcurrency int, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment astro.Deployment
		err                 error
		errHelp             string
		queueToCreate       *astro.WorkerQueue
		listToCreate        []astro.WorkerQueue
		defaultOptions      astro.WorkerQueueDefaultOptions
		nodePoolID          string
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// get defaults for min-count, max-count and concurrency from API
	defaultOptions, err = GetWorkerQueueDefaultOptions(client)
	if err != nil {
		return fmt.Errorf("%w: %s", errWorkerQueueDefaultOptions, err.Error())
	}

	// get the node poolID to use
	nodePoolID, err = selectNodePool(workerType, requestedDeployment.Cluster.NodePools, out)
	if err != nil {
		return err
	}

	// prompt for name if one was not provided
	if name == "" {
		name = input.Text("Enter a name for the worker queue\n> ")
	}

	queueToCreate = &astro.WorkerQueue{
		Name:       name,
		IsDefault:  false, // cannot create a default queue
		NodePoolID: nodePoolID,
	}
	queueToCreate = setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreate, defaultOptions)

	err = IsWorkerQueueInputValid(queueToCreate, defaultOptions)
	if err != nil {
		return err
	}

	if QueueExists(requestedDeployment.WorkerQueues, queueToCreate) {
		// create does not allow updating existing queues
		errHelp = fmt.Sprintf("use worker queue update %s instead", queueToCreate.Name)
		return fmt.Errorf("%w: %s", errCannotUpdateExistingQueue, errHelp)
	}

	// queueToCreate does not exist so we add it
	listToCreate = append(requestedDeployment.WorkerQueues, *queueToCreate) //nolint

	// update the deployment with the new list of worker queues
	err = deployment.Update(requestedDeployment.ID, "", ws, "", "", 0, 0, 0, listToCreate, true, client)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "worker queue %s for %s in %s workspace created\n", queueToCreate.Name, requestedDeployment.Label, ws)
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
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range
func IsWorkerQueueInputValid(requestedWorkerQueue *astro.WorkerQueue, defaultOptions astro.WorkerQueueDefaultOptions) error {
	var errorMessage string
	if !(requestedWorkerQueue.MinWorkerCount >= defaultOptions.MinWorkerCount.Floor) ||
		!(requestedWorkerQueue.MinWorkerCount <= defaultOptions.MinWorkerCount.Ceiling) {
		errorMessage = fmt.Sprintf("min worker count must be between %d and %d", defaultOptions.MinWorkerCount.Floor, defaultOptions.MinWorkerCount.Ceiling)
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedWorkerQueue.MaxWorkerCount >= defaultOptions.MaxWorkerCount.Floor) ||
		!(requestedWorkerQueue.MaxWorkerCount <= defaultOptions.MaxWorkerCount.Ceiling) {
		errorMessage = fmt.Sprintf("max worker count must be between %d and %d", defaultOptions.MaxWorkerCount.Floor, defaultOptions.MaxWorkerCount.Ceiling)
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedWorkerQueue.WorkerConcurrency >= defaultOptions.WorkerConcurrency.Floor) ||
		!(requestedWorkerQueue.WorkerConcurrency <= defaultOptions.WorkerConcurrency.Ceiling) {
		errorMessage = fmt.Sprintf("worker concurrency must be between %d and %d", defaultOptions.WorkerConcurrency.Floor, defaultOptions.WorkerConcurrency.Ceiling)
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
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

// selectNodePool takes workerType and []NodePool as arguments
// If user requested a workerType, then the matching nodePoolID is returned
// If user did not request a workerType, then it prompts the user to pick one
// An errInvalidNodePool is returned if a user chooses an option not on the list
func selectNodePool(workerType string, nodePools []astro.NodePool, out io.Writer) (string, error) {
	var (
		nodePoolID, message string
		pool                astro.NodePool
		errToReturn         error
	)

	message = "No worker type was specified. Select the worker type to use"
	switch workerType {
	case "":
		tab := printutil.Table{
			Padding:        []int{5, 30, 20, 50},
			DynamicPadding: true,
			Header:         []string{"#", "WORKER TYPE", "ISDEFAULT", "ID"},
		}

		fmt.Println(message)

		sort.Slice(nodePools, func(i, j int) bool {
			return nodePools[i].CreatedAt.Before(nodePools[j].CreatedAt)
		})

		nodePoolMap := map[string]astro.NodePool{}
		for i := range nodePools {
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), nodePools[i].NodeInstanceType, strconv.FormatBool(nodePools[i].IsDefault), nodePools[i].ID}, false)

			nodePoolMap[strconv.Itoa(index)] = nodePools[i]
		}

		tab.Print(out)
		choice := input.Text("\n> ")
		selectedPool, ok := nodePoolMap[choice]
		if !ok {
			// returning an error as choice was not in nodePoolMap
			errToReturn = fmt.Errorf("%w: invalid worker type: %s selected", errInvalidNodePool, choice)
			return nodePoolID, errToReturn
		}
		return selectedPool.ID, nil
	default:
		// Get the nodePoolID for pool that matches workerType
		for _, pool = range nodePools {
			if pool.NodeInstanceType == workerType {
				nodePoolID = pool.ID
				return nodePoolID, errToReturn
			}
		}
		// did not find a matching workerType in any node pool
		errToReturn = fmt.Errorf("%w: workerType %s is not available for this deployment", errInvalidNodePool, workerType)
		return nodePoolID, errToReturn
	}
}
