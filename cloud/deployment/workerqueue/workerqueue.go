package workerqueue

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/ansi"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	createAction     = "create"
	updateAction     = "update"
	defaultQueueName = "default"
)

var (
	errWorkerQueueDefaultOptions = errors.New("failed to get worker queue default options")
	errInvalidWorkerQueueOption  = errors.New("worker queue option is invalid")
	errCannotUpdateExistingQueue = errors.New("worker queue already exists")
	errCannotCreateNewQueue      = errors.New("worker queue does not exist")
	errInvalidNodePool           = errors.New("node pool selection failed")
	errQueueDoesNotExist         = errors.New("worker queue does not exist")
	errInvalidQueue              = errors.New("worker queue selection failed")
	errCannotDeleteDefaultQueue  = errors.New("default queue can not be deleted")
	ErrNotSupported              = errors.New("KubernetesExecutor does not support")
)

func CreateOrUpdate(ws, deploymentID, deploymentName, name, action, workerType string, wQueueMin, wQueueMax, wQueueConcurrency int, force bool, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment                  astro.Deployment
		err                                  error
		errHelp, succeededAction, nodePoolID string
		queueToCreateOrUpdate                *astro.WorkerQueue
		listToCreate                         []astro.WorkerQueue
		defaultOptions                       astro.WorkerQueueDefaultOptions
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// get the node poolID to use
	nodePoolID, err = selectNodePool(workerType, requestedDeployment.Cluster.NodePools, out)
	if err != nil {
		return err
	}

	queueToCreateOrUpdate = &astro.WorkerQueue{
		Name:       name,
		IsDefault:  false, // cannot create a default queue
		NodePoolID: nodePoolID,
	}

	if name == "" {
		queueToCreateOrUpdate.Name, err = getQueueName(name, action, &requestedDeployment, out)
		if err != nil {
			return err
		}
	}
	switch requestedDeployment.DeploymentSpec.Executor {
	case deployment.CeleryExecutor:
		// get defaults for min-count, max-count and concurrency from API
		defaultOptions, err = GetWorkerQueueDefaultOptions(client)
		if err != nil {
			return err
		}

		queueToCreateOrUpdate = SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdate, defaultOptions)

		err = IsCeleryWorkerQueueInputValid(queueToCreateOrUpdate, defaultOptions)
		if err != nil {
			return err
		}
	case deployment.KubeExecutor:
		err = IsKubernetesWorkerQueueInputValid(queueToCreateOrUpdate)
		if err != nil {
			return err
		}
	}

	switch action {
	case createAction:
		if QueueExists(requestedDeployment.WorkerQueues, queueToCreateOrUpdate) {
			// create does not allow updating existing queues
			errHelp = fmt.Sprintf("use worker queue update %s instead", queueToCreateOrUpdate.Name)
			return fmt.Errorf("%w: %s", errCannotUpdateExistingQueue, errHelp)
		}
		// queueToCreateOrUpdate does not exist
		// user requested create, so we add queueToCreateOrUpdate to the list
		listToCreate = append(requestedDeployment.WorkerQueues, *queueToCreateOrUpdate) //nolint
	case updateAction:
		if QueueExists(requestedDeployment.WorkerQueues, queueToCreateOrUpdate) {
			if !force {
				i, _ := input.Confirm(
					fmt.Sprintf("\nAre you sure you want to %s the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", action, ansi.Bold(queueToCreateOrUpdate.Name)))

				if !i {
					fmt.Fprintf(out, "Canceling worker queue %s\n", action)
					return nil
				}
			}
			// user requested an update and queueToCreateOrUpdate exists
			listToCreate = updateQueueList(requestedDeployment.WorkerQueues, queueToCreateOrUpdate, requestedDeployment.DeploymentSpec.Executor)
		} else {
			// update does not allow creating new queues
			errHelp = fmt.Sprintf("use worker queue create %s instead", queueToCreateOrUpdate.Name)
			return fmt.Errorf("%w: %s", errCannotCreateNewQueue, errHelp)
		}
	}

	// update the deployment with the new list of worker queues
	err = deployment.Update(requestedDeployment.ID, "", ws, "", "", "", "", 0, 0, listToCreate, true, client)
	if err != nil {
		return err
	}
	// change action to past tense
	succeededAction = fmt.Sprintf("%sd", action)

	fmt.Fprintf(out, "worker queue %s for %s in %s workspace %s\n", queueToCreateOrUpdate.Name, requestedDeployment.Label, ws, succeededAction)
	return nil
}

// SetWorkerQueueValues sets values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency
// Default values are used if the user did not request any
func SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate *astro.WorkerQueue, workerQueueDefaultOptions astro.WorkerQueueDefaultOptions) *astro.WorkerQueue {
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
		return astro.WorkerQueueDefaultOptions{}, fmt.Errorf("%w: %s", errWorkerQueueDefaultOptions, err.Error())
	}
	return workerQueueDefaultOptions, nil
}

// IsCeleryWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
func IsCeleryWorkerQueueInputValid(requestedWorkerQueue *astro.WorkerQueue, defaultOptions astro.WorkerQueueDefaultOptions) error {
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

// IsKubernetesWorkerQueueInputValid checks if the requestedQueue has all the necessary properties
// required to create a worker queue for the KubernetesExecutor.
// errNotSupported is returned for any invalid properties.
func IsKubernetesWorkerQueueInputValid(requestedWorkerQueue *astro.WorkerQueue) error {
	var errorMessage string
	if requestedWorkerQueue.Name != defaultQueueName {
		errorMessage = "a non default worker queue in the request. Rename the queue to default"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
	}
	if requestedWorkerQueue.PodCPU != "" {
		errorMessage = "pod_cpu in the request. It will be calculated based on the requested worker_type"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
	}
	if requestedWorkerQueue.PodRAM != "" {
		errorMessage = "pod_ram in the request. It will be calculated based on the requested worker_type"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
	}
	if requestedWorkerQueue.MinWorkerCount != 0 {
		errorMessage = "min_worker_count in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
	}
	if requestedWorkerQueue.MaxWorkerCount != 0 {
		errorMessage = "max_worker_count in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
	}
	if requestedWorkerQueue.WorkerConcurrency != 0 {
		errorMessage = "worker_concurrency in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%w %s", ErrNotSupported, errorMessage)
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

// Delete deletes the specified worker queue from the deployment
// user gets prompted if no deployment was specified
// user gets prompted if no name for the queue to delete was specified
// An errQueueDoesNotExist is returned if queue to delete does not exist
// An errCannotDeleteDefaultQueue is returned if a user chooses the default queue
func Delete(ws, deploymentID, deploymentName, name string, force bool, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment astro.Deployment
		err                 error
		queueToDelete       *astro.WorkerQueue
		listToDelete        []astro.WorkerQueue
		queue               astro.WorkerQueue
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// prompt for queue name if one was not provided
	if name == "" {
		name, err = selectQueue(requestedDeployment.WorkerQueues, out)
		if err != nil {
			return err
		}
	}
	// check if default queue is being deleted
	if name == defaultQueueName {
		return errCannotDeleteDefaultQueue
	}
	queueToDelete = &astro.WorkerQueue{
		Name:      name,
		IsDefault: false, // cannot delete a default queue
	}

	if QueueExists(requestedDeployment.WorkerQueues, queueToDelete) {
		if !force {
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to delete the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", ansi.Bold(queueToDelete.Name)))

			if !i {
				fmt.Fprintf(out, "Canceling worker queue deletion\n")
				return nil
			}
		}

		// create a new listToDelete without queueToDelete in it
		for _, queue = range requestedDeployment.WorkerQueues {
			if queue.Name != queueToDelete.Name {
				listToDelete = append(listToDelete, queue)
			}
		}
		// update the deployment with the new list
		err = deployment.Update(requestedDeployment.ID, "", ws, "", "", "", "", 0, 0, listToDelete, true, client)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "worker queue %s for %s in %s workspace deleted\n", queueToDelete.Name, requestedDeployment.Label, ws)
		return nil
	}
	// can not delete a queue that does not exist
	return fmt.Errorf("%w: %s", errQueueDoesNotExist, queueToDelete.Name)
}

// selectQueue takes []WorkerQueue and io.Writer as arguments
// user can select a queue to delete from the list and the name of the selected queue is returned
// An errInvalidQueue is returned if a user chooses a queue not on the list
func selectQueue(queueList []astro.WorkerQueue, out io.Writer) (string, error) {
	var (
		errToReturn        error
		queueName, message string
		queueToDelete      astro.WorkerQueue
	)

	tab := printutil.Table{
		Padding:        []int{5, 30, 20, 50},
		DynamicPadding: true,
		Header:         []string{"#", "WORKER QUEUE", "ISDEFAULT", "ID"},
	}

	fmt.Println(message)

	sort.Slice(queueList, func(i, j int) bool {
		return queueList[i].Name < queueList[j].Name
	})

	queueMap := map[string]astro.WorkerQueue{}
	for i := range queueList {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), queueList[i].Name, strconv.FormatBool(queueList[i].IsDefault), queueList[i].ID}, false)

		queueMap[strconv.Itoa(index)] = queueList[i]
	}

	tab.Print(out)
	choice := input.Text("\n> ")
	queueToDelete, ok := queueMap[choice]
	if !ok {
		// returning an error as choice was not in queueMap
		errToReturn = fmt.Errorf("%w: invalid worker queue: %s selected", errInvalidQueue, choice)
		return queueName, errToReturn
	}
	return queueToDelete.Name, nil
}

func updateQueueList(existingQueues []astro.WorkerQueue, queueToUpdate *astro.WorkerQueue, executor string) []astro.WorkerQueue {
	for i, queue := range existingQueues {
		if queue.Name != queueToUpdate.Name {
			continue
		}

		queue.ID = existingQueues[i].ID               // we need IDs to update existing queues
		queue.IsDefault = existingQueues[i].IsDefault // users can not change this
		if executor == deployment.CeleryExecutor {
			queue.WorkerConcurrency = queueToUpdate.WorkerConcurrency
			queue.MinWorkerCount = queueToUpdate.MinWorkerCount
			queue.MaxWorkerCount = queueToUpdate.MaxWorkerCount
		} else if executor == deployment.KubeExecutor {
			// KubernetesExecutor calculates resources automatically based on the worker type
			queue.WorkerConcurrency = 0
			queue.MinWorkerCount = 0
			queue.MaxWorkerCount = 0
			queue.PodRAM = ""
			queue.PodCPU = ""
		}
		queue.NodePoolID = queueToUpdate.NodePoolID
		existingQueues[i] = queue
		return existingQueues
	}
	return existingQueues
}

// getQueueName returns the name for a worker-queue. If action is to create, it prompts the user for a name to use.
// If action is to update, it makes the user select a queue from a list of existing ones.
// It returns errInvalidQueue if a user chooses a queue not on the list
func getQueueName(name, action string, requestedDeployment *astro.Deployment, out io.Writer) (string, error) {
	var (
		queueName string
		err       error
	)
	if name == "" {
		switch action {
		case createAction:
			// prompt for name if one was not provided
			queueName = input.Text("Enter a name for the worker queue\n> ")
		case updateAction:
			// user selects a queue as no name was provided
			queueName, err = selectQueue(requestedDeployment.WorkerQueues, out)
			if err != nil {
				return "", err
			}
		}
	}
	return queueName, nil
}
