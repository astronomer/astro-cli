package workerqueue

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/astronomer/astro-cli/pkg/ansi"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	createAction       = "create"
	updateAction       = "update"
	defaultQueueName   = "default"
	podCPUErrorMessage = "pod_cpu in the request. It can only be used with KubernetesExecutor"
	podRAMErrorMessage = "pod_ram in the request. It can only be used with KubernetesExecutor"
)

var (
	errWorkerQueueDefaultOptions = errors.New("failed to get worker queue default options")
	errInvalidWorkerQueueOption  = errors.New("worker queue option is invalid")
	errCannotUpdateExistingQueue = errors.New("worker queue already exists")
	errCannotCreateNewQueue      = errors.New("worker queue does not exist")
	errInvalidNodePool           = errors.New("node pool selection failed")
	errInvalidAstroMachine       = errors.New("invalid astro machine selection failed")
	errQueueDoesNotExist         = errors.New("worker queue does not exist")
	errInvalidQueue              = errors.New("worker queue selection failed")
	errCannotDeleteDefaultQueue  = errors.New("default queue can not be deleted")
	ErrNotSupported              = errors.New("does not support")
	errNoUseWorkerQueues         = errors.New("don't use 'worker_queues' to update default queue with KubernetesExecutor, use 'default_task_pod_cpu' and 'default_task_pod_memory' instead")
)

// CreateOrUpdate creates a new worker queue or updates an existing worker queue for a deployment.
func CreateOrUpdate(ws, deploymentID, deploymentName, name, action, workerType string, wQueueMin, wQueueMax, wQueueConcurrency int, force bool, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error { //nolint
	var (
		requestedDeployment                  astroplatformcore.Deployment
		err                                  error
		errHelp, succeededAction, nodePoolID string
		workerMachine                        astroplatformcore.WorkerMachine
		queueToCreateOrUpdate                *astroplatformcore.WorkerQueueRequest
		queueToCreateOrUpdateHybrid          *astroplatformcore.HybridWorkerQueueRequest
		listToCreate                         []astroplatformcore.WorkerQueueRequest
		existingQueues                       []astroplatformcore.WorkerQueue
		hybridListToCreate                   []astroplatformcore.HybridWorkerQueueRequest
		defaultOptions                       astroplatformcore.WorkerQueueOptions
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, true, platformCoreClient, coreClient)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("No Deployments found in workspace %s\n", ansi.Bold(ws))
		return nil
	}

	if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
		getDeploymentOptions := astroplatformcore.GetDeploymentOptionsParams{
			DeploymentId: &requestedDeployment.Id,
		}
		deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", getDeploymentOptions, platformCoreClient)
		if err != nil {
			return err
		}
		WorkerMachines := deploymentOptions.WorkerMachines
		// get the machine to use
		workerMachine, err = selectWorkerMachine(workerType, WorkerMachines, out)
		if err != nil {
			return err
		}

		if wQueueConcurrency == 0 && action == createAction {
			wQueueConcurrency = int(workerMachine.Concurrency.Default) // This is set based on the machine type the user chooses if not explicitly passed by the user
		}
		queueToCreateOrUpdate = &astroplatformcore.WorkerQueueRequest{
			Name:              name,
			IsDefault:         false, // cannot create a default queue
			AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(workerMachine.Name),
			MinWorkerCount:    wQueueMin,         // use the value from the user input
			MaxWorkerCount:    wQueueMax,         // use the value from the user input
			WorkerConcurrency: wQueueConcurrency, // use the value from the user input
		}

		// create listToCreate
		if requestedDeployment.WorkerQueues != nil {
			for i := range *requestedDeployment.WorkerQueues {
				queues := *requestedDeployment.WorkerQueues
				existingQueueRequest := astroplatformcore.WorkerQueueRequest{
					Name:              queues[i].Name,
					Id:                &queues[i].Id,
					IsDefault:         queues[i].IsDefault,
					MaxWorkerCount:    queues[i].MaxWorkerCount,
					MinWorkerCount:    queues[i].MinWorkerCount,
					WorkerConcurrency: queues[i].WorkerConcurrency,
					AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(*queues[i].AstroMachine),
				}
				listToCreate = append(listToCreate, existingQueueRequest)
			}
		}
		if name == "" {
			queueToCreateOrUpdate.Name, err = getQueueName(name, action, &requestedDeployment, out)
			if err != nil {
				return err
			}
			name = queueToCreateOrUpdate.Name
		}
		queueToCreateOrUpdate = SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdate, defaultOptions)
	} else {
		// get the node poolID to use
		cluster, err := deployment.CoreGetCluster("", *requestedDeployment.ClusterId, platformCoreClient)
		if err != nil {
			return err
		}
		nodePoolID, err = selectNodePool(workerType, *cluster.NodePools, out)
		if err != nil {
			return err
		}
		queueToCreateOrUpdateHybrid = &astroplatformcore.HybridWorkerQueueRequest{
			Name:              name,
			IsDefault:         false, // cannot create a default queue
			NodePoolId:        nodePoolID,
			MinWorkerCount:    wQueueMin,         // use the value from the user input
			MaxWorkerCount:    wQueueMax,         // use the value from the user input
			WorkerConcurrency: wQueueConcurrency, // use the value from the user input
		}
		// create hybridListToCreate
		for i := range *requestedDeployment.WorkerQueues {
			queues := *requestedDeployment.WorkerQueues
			existingHybridQueueRequest := astroplatformcore.HybridWorkerQueueRequest{
				Name:              queues[i].Name,
				Id:                &queues[i].Id,
				IsDefault:         queues[i].IsDefault,
				MaxWorkerCount:    queues[i].MaxWorkerCount,
				MinWorkerCount:    queues[i].MinWorkerCount,
				WorkerConcurrency: queues[i].WorkerConcurrency,
				NodePoolId:        *queues[i].NodePoolId,
			}
			hybridListToCreate = append(hybridListToCreate, existingHybridQueueRequest)
		}
		if name == "" {
			queueToCreateOrUpdateHybrid.Name, err = getQueueName(name, action, &requestedDeployment, out)
			if err != nil {
				return err
			}
			name = queueToCreateOrUpdateHybrid.Name
		}
		queueToCreateOrUpdateHybrid = SetWorkerQueueValuesHybrid(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdateHybrid, defaultOptions)
	}
	switch *requestedDeployment.Executor {
	case astroplatformcore.DeploymentExecutorCELERY:
		deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", astroplatformcore.GetDeploymentOptionsParams{}, platformCoreClient)
		if err != nil {
			return err
		}

		defaultOptions = deploymentOptions.WorkerQueues

		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			err = IsHostedCeleryWorkerQueueInputValid(queueToCreateOrUpdate, defaultOptions, &workerMachine)
			if err != nil {
				return err
			}
		} else {
			err = IsCeleryWorkerQueueInputValid(queueToCreateOrUpdateHybrid, defaultOptions)
			if err != nil {
				return err
			}
		}
	case astroplatformcore.DeploymentExecutorKUBERNETES:
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			return errNoUseWorkerQueues
		}
		queueToCreateOrUpdateHybrid.MinWorkerCount = -1
		err = IsKubernetesWorkerQueueInputValid(queueToCreateOrUpdateHybrid)
		if err != nil {
			return err
		}
	}

	// sanitize all the existing queues based on executor
	existingQueues = sanitizeExistingQueues(*requestedDeployment.WorkerQueues, *requestedDeployment.Executor)
	// create listToCreate
	switch action {
	case createAction:
		if QueueExists(existingQueues, queueToCreateOrUpdate, queueToCreateOrUpdateHybrid) {
			// create does not allow updating existing queues
			errHelp = fmt.Sprintf("use worker queue update %s instead", name)
			return fmt.Errorf("%w: %s", errCannotUpdateExistingQueue, errHelp)
		}
		// queueToCreateOrUpdate does not exist
		// user requested create, so we add queueToCreateOrUpdate to the list
		if queueToCreateOrUpdate != nil {
			listToCreate = append(listToCreate, *queueToCreateOrUpdate) //nolint
		}
		if queueToCreateOrUpdateHybrid != nil {
			hybridListToCreate = append(hybridListToCreate, *queueToCreateOrUpdateHybrid)
		}
	case updateAction:
		if QueueExists(existingQueues, queueToCreateOrUpdate, queueToCreateOrUpdateHybrid) {
			if !force {
				i, _ := input.Confirm(
					fmt.Sprintf("\nAre you sure you want to %s the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", action, ansi.Bold(name)))

				if !i {
					fmt.Fprintf(out, "Canceling worker queue %s\n", action)
					return nil
				}
			}
			// user requested an update and queueToCreateOrUpdate exists
			if queueToCreateOrUpdate != nil {
				listToCreate = updateQueueList(listToCreate, queueToCreateOrUpdate, requestedDeployment.Executor, wQueueMin, wQueueMax, wQueueConcurrency)
			}
			if queueToCreateOrUpdateHybrid != nil {
				hybridListToCreate = updateHybridQueueList(hybridListToCreate, queueToCreateOrUpdateHybrid, requestedDeployment.Executor, wQueueMin, wQueueMax, wQueueConcurrency)
			}
		} else {
			// update does not allow creating new queues
			if queueToCreateOrUpdate != nil {
				errHelp = fmt.Sprintf("use worker queue create %s instead", queueToCreateOrUpdate.Name)
			}
			if queueToCreateOrUpdateHybrid != nil {
				errHelp = fmt.Sprintf("use worker queue create %s instead", queueToCreateOrUpdateHybrid.Name)
			}
			return fmt.Errorf("%w: %s", errCannotCreateNewQueue, errHelp)
		}
	}
	// update the deployment with the new list of worker queues
	err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", 0, 0, listToCreate, hybridListToCreate, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, true, coreClient, platformCoreClient)
	if err != nil {
		return err
	}
	// change action to past tense
	succeededAction = fmt.Sprintf("%sd", action)

	fmt.Fprintf(out, "worker queue %s for %s in %s workspace %s\n", name, requestedDeployment.Name, ws, succeededAction)
	return nil
}

// SetWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate *astroplatformcore.WorkerQueueRequest, workerQueueDefaultOptions astroplatformcore.WorkerQueueOptions) *astroplatformcore.WorkerQueueRequest {
	// -1 is the CLI default to allow users to request wQueueMin=0
	if wQueueMin == -1 {
		// set default value as user input did not have it
		workerQueueToCreate.MinWorkerCount = int(workerQueueDefaultOptions.MinWorkers.Default)
	}

	if wQueueMax == 0 {
		// set default value as user input did not have it
		workerQueueToCreate.MaxWorkerCount = int(workerQueueDefaultOptions.MaxWorkers.Default)
	}

	if wQueueConcurrency == 0 {
		// set default value as user input did not have it
		workerQueueToCreate.WorkerConcurrency = int(workerQueueDefaultOptions.WorkerConcurrency.Default)
	}
	return workerQueueToCreate
}

// SetWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func SetWorkerQueueValuesHybrid(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate *astroplatformcore.HybridWorkerQueueRequest, workerQueueDefaultOptions astroplatformcore.WorkerQueueOptions) *astroplatformcore.HybridWorkerQueueRequest {
	// -1 is the CLI default to allow users to request wQueueMin=0
	if wQueueMin == -1 {
		// set default value as user input did not have it
		workerQueueToCreate.MinWorkerCount = int(workerQueueDefaultOptions.MinWorkers.Default)
	}

	if wQueueMax == 0 {
		// set default value as user input did not have it
		workerQueueToCreate.MaxWorkerCount = int(workerQueueDefaultOptions.MaxWorkers.Default)
	}

	if wQueueConcurrency == 0 {
		// set default value as user input did not have it
		workerQueueToCreate.WorkerConcurrency = int(workerQueueDefaultOptions.WorkerConcurrency.Default)
	}
	return workerQueueToCreate
}

// IsCeleryWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// ErrNotSupported is returned if PodCPU or PodRAM are requested.
func IsCeleryWorkerQueueInputValid(requestedHybridWorkerQueue *astroplatformcore.HybridWorkerQueueRequest, defaultOptions astroplatformcore.WorkerQueueOptions) error {
	var errorMessage string
	if !(requestedHybridWorkerQueue.MinWorkerCount >= int(defaultOptions.MinWorkers.Floor)) ||
		!(requestedHybridWorkerQueue.MinWorkerCount <= int(defaultOptions.MinWorkers.Ceiling)) {
		errorMessage = fmt.Sprintf("min worker count must be between %d and %d", int(defaultOptions.MinWorkers.Floor), int(defaultOptions.MinWorkers.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedHybridWorkerQueue.MaxWorkerCount >= int(defaultOptions.MaxWorkers.Floor)) ||
		!(requestedHybridWorkerQueue.MaxWorkerCount <= int(defaultOptions.MaxWorkers.Ceiling)) {
		errorMessage = fmt.Sprintf("max worker count must be between %d and %d", int(defaultOptions.MaxWorkers.Floor), int(defaultOptions.MaxWorkers.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedHybridWorkerQueue.WorkerConcurrency >= int(defaultOptions.WorkerConcurrency.Floor)) ||
		!(requestedHybridWorkerQueue.WorkerConcurrency <= int(defaultOptions.WorkerConcurrency.Ceiling)) {
		errorMessage = fmt.Sprintf("worker concurrency must be between %d and %d", int(defaultOptions.WorkerConcurrency.Floor), int(defaultOptions.WorkerConcurrency.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	return nil
}

// IsHostedCeleryWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions and machineOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// ErrNotSupported is returned if PodCPU or PodRAM are requested.
func IsHostedCeleryWorkerQueueInputValid(requestedWorkerQueue *astroplatformcore.WorkerQueueRequest, defaultOptions astroplatformcore.WorkerQueueOptions, machineOptions *astroplatformcore.WorkerMachine) error {
	var errorMessage string
	if !(requestedWorkerQueue.MinWorkerCount >= int(defaultOptions.MinWorkers.Floor)) ||
		!(requestedWorkerQueue.MinWorkerCount <= int(defaultOptions.MinWorkers.Ceiling)) {
		errorMessage = fmt.Sprintf("min worker count must be between %d and %d", int(defaultOptions.MinWorkers.Floor), int(defaultOptions.MinWorkers.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	if !(requestedWorkerQueue.MaxWorkerCount >= int(defaultOptions.MaxWorkers.Floor)) ||
		!(requestedWorkerQueue.MaxWorkerCount <= int(defaultOptions.MaxWorkers.Ceiling)) {
		errorMessage = fmt.Sprintf("max worker count must be between %d and %d", int(defaultOptions.MaxWorkers.Floor), int(defaultOptions.MaxWorkers.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	// The floor for worker concurrency for hosted deployments is always 1 for all astro machines
	workerConcurrenyFloor := 1
	if !(requestedWorkerQueue.WorkerConcurrency >= workerConcurrenyFloor) ||
		!(requestedWorkerQueue.WorkerConcurrency <= int(machineOptions.Concurrency.Ceiling)) {
		fmt.Println("hosted function called")
		errorMessage = fmt.Sprintf("worker concurrency must be between %d and %d", workerConcurrenyFloor, int(machineOptions.Concurrency.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	return nil
}

// IsKubernetesWorkerQueueInputValid checks if the requestedQueue has all the necessary properties
// required to create a worker queue for the KubernetesExecutor.
// errNotSupported is returned for any invalid properties.
func IsKubernetesWorkerQueueInputValid(queueToCreateOrUpdateHybrid *astroplatformcore.HybridWorkerQueueRequest) error {
	var errorMessage string

	if queueToCreateOrUpdateHybrid.Name != defaultQueueName {
		errorMessage = "a non default worker queue in the request. Rename the queue to default"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, ErrNotSupported, errorMessage)
	}
	if queueToCreateOrUpdateHybrid.MinWorkerCount != -1 {
		errorMessage = "minimum worker count in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, ErrNotSupported, errorMessage)
	}
	fmt.Println(queueToCreateOrUpdateHybrid.MaxWorkerCount)
	if queueToCreateOrUpdateHybrid.MaxWorkerCount != 0 {
		errorMessage = "maximum worker count in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, ErrNotSupported, errorMessage)
	}
	if queueToCreateOrUpdateHybrid.WorkerConcurrency != 0 {
		errorMessage = "worker concurrency in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, ErrNotSupported, errorMessage)
	}

	return nil
}

// QueueExists takes a []existingQueues and a queueToCreate as arguments
// It returns true if queueToCreate exists in []existingQueues
// It returns false if queueToCreate does not exist in []existingQueues
func QueueExists(existingQueues []astroplatformcore.WorkerQueue, queueToCreate *astroplatformcore.WorkerQueueRequest, queueToCreateOrUpdateHybrid *astroplatformcore.HybridWorkerQueueRequest) bool {
	for _, queue := range existingQueues { //nolint
		if queueToCreateOrUpdateHybrid != nil {
			if queue.Name == queueToCreateOrUpdateHybrid.Name {
				// queueToCreate exists
				return true
			}
			if queueToCreateOrUpdateHybrid.Id != nil {
				if queue.Id == *queueToCreateOrUpdateHybrid.Id {
					// queueToCreate exists
					return true
				}
			}
		}
		if queueToCreate != nil {
			if queue.Name == queueToCreate.Name {
				// queueToCreate exists
				return true
			}
			if queueToCreate.Id != nil {
				if queue.Id == *queueToCreate.Id {
					// queueToCreate exists
					return true
				}
			}
		}
	}
	return false
}

func selectWorkerMachine(workerType string, workerMachines []astroplatformcore.WorkerMachine, out io.Writer) (astroplatformcore.WorkerMachine, error) {
	var (
		workerMachine astroplatformcore.WorkerMachine
		errToReturn   error
	)

	switch workerType {
	case "":
		tab := printutil.Table{
			Padding:        []int{5, 30, 20, 50},
			DynamicPadding: true,
			Header:         []string{"#", "WORKER TYPE", "CPU", "Memory"},
		}

		fmt.Println("No worker type was specified. Select the worker type to use")

		machineMap := map[string]astroplatformcore.WorkerMachine{}

		for i := range workerMachines {
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), workerMachines[i].Name, workerMachines[i].Spec.Cpu + " vCPU", workerMachines[i].Spec.Memory}, false)

			machineMap[strconv.Itoa(index)] = workerMachines[i]
		}

		tab.Print(out)
		choice := input.Text("\n> ")
		selectedPool, ok := machineMap[choice]
		if !ok {
			// returning an error as choice was not in nodePoolMap
			errToReturn = fmt.Errorf("%w: invalid worker type: %s selected", errInvalidAstroMachine, choice)
			return astroplatformcore.WorkerMachine{}, errToReturn
		}
		return selectedPool, nil
	default:
		for _, workerMachine = range workerMachines {
			if workerMachine.Name == workerType {
				return workerMachine, nil
			}
		}
		// did not find a matching workerType in any node pool
		errToReturn = fmt.Errorf("%w: workerType %s is not available for this deployment", errInvalidAstroMachine, workerType)
		return astroplatformcore.WorkerMachine{}, errToReturn
	}
}

// selectNodePool takes workerType and []NodePool as arguments
// If user requested a workerType, then the matching nodePoolID is returned
// If user did not request a workerType, then it prompts the user to pick one
// An errInvalidNodePool is returned if a user chooses an option not on the list
func selectNodePool(workerType string, nodePools []astroplatformcore.NodePool, out io.Writer) (string, error) {
	var (
		nodePoolID, message string
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

		nodePoolMap := map[string]astroplatformcore.NodePool{}
		for i := range nodePools {
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), nodePools[i].NodeInstanceType, strconv.FormatBool(nodePools[i].IsDefault), nodePools[i].Id}, false)

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
		return selectedPool.Id, nil
	default:
		// Get the nodePoolID for pool that matches workerType
		for i := range nodePools {
			if nodePools[i].NodeInstanceType == workerType {
				nodePoolID = nodePools[i].Id
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
func Delete(ws, deploymentID, deploymentName, name string, force bool, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer) error {
	var (
		requestedDeployment astroplatformcore.Deployment
		err                 error
		queueToDelete       *astroplatformcore.WorkerQueueRequest
		queueToDeleteHybrid *astroplatformcore.HybridWorkerQueueRequest
		existingQueues      []astroplatformcore.WorkerQueue
		listToDelete        []astroplatformcore.WorkerQueueRequest
		hybridListToDelete  []astroplatformcore.HybridWorkerQueueRequest
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, true, platformCoreClient, coreClient)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("No Deployments found in workspace %s\n", ansi.Bold(ws))
		return nil
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
	queueToDelete = &astroplatformcore.WorkerQueueRequest{
		Name:      name,
		IsDefault: false, // cannot delete a default queue
	}
	queueToDeleteHybrid = &astroplatformcore.HybridWorkerQueueRequest{
		Name:      name,
		IsDefault: false, // cannot delete a default queue
	}

	// sanitize all the existing queues based on executor
	existingQueues = sanitizeExistingQueues(*requestedDeployment.WorkerQueues, *requestedDeployment.Executor)

	if QueueExists(existingQueues, queueToDelete, queueToDeleteHybrid) {
		if !force {
			i, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to delete the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", ansi.Bold(queueToDelete.Name)))

			if !i {
				fmt.Fprintf(out, "Canceling worker queue deletion\n")
				return nil
			}
		}
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			// create a new listToDelete without queueToDelete in it
			for i := range existingQueues { //nolint
				if existingQueues[i].Name != queueToDelete.Name {
					existingQueueRequest := astroplatformcore.WorkerQueueRequest{
						Name:              existingQueues[i].Name,
						Id:                &existingQueues[i].Id,
						IsDefault:         existingQueues[i].IsDefault,
						MaxWorkerCount:    existingQueues[i].MaxWorkerCount,
						MinWorkerCount:    existingQueues[i].MinWorkerCount,
						WorkerConcurrency: existingQueues[i].WorkerConcurrency,
						AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(*existingQueues[i].AstroMachine),
					}
					listToDelete = append(listToDelete, existingQueueRequest)
				}
			}
			// update the deployment with the new list
			err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", 0, 0, listToDelete, hybridListToDelete, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, true, coreClient, platformCoreClient)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "worker queue %s for %s in %s workspace deleted\n", queueToDelete.Name, requestedDeployment.Name, ws)
		} else {
			// create a new listToDeleteHybrid without queueToDeleteHybrid in it
			for i := range existingQueues { //nolint
				if existingQueues[i].Name != queueToDeleteHybrid.Name {
					existingQueueRequest := astroplatformcore.HybridWorkerQueueRequest{
						Name:              existingQueues[i].Name,
						Id:                &existingQueues[i].Id,
						IsDefault:         existingQueues[i].IsDefault,
						MaxWorkerCount:    existingQueues[i].MaxWorkerCount,
						MinWorkerCount:    existingQueues[i].MinWorkerCount,
						WorkerConcurrency: existingQueues[i].WorkerConcurrency,
						NodePoolId:        *existingQueues[i].NodePoolId,
					}
					hybridListToDelete = append(hybridListToDelete, existingQueueRequest)
				}
			}
			// update the deployment with the new list
			err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", 0, 0, listToDelete, hybridListToDelete, []astroplatformcore.DeploymentEnvironmentVariableRequest{}, true, coreClient, platformCoreClient)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "worker queue %s for %s in %s workspace deleted\n", queueToDelete.Name, requestedDeployment.Name, ws)
		}
		return nil
	}
	// can not delete a queue that does not exist
	return fmt.Errorf("%w: %s", errQueueDoesNotExist, queueToDelete.Name)
}

// selectQueue takes []WorkerQueue and io.Writer as arguments
// user can select a queue to delete from the list and the name of the selected queue is returned
// An errInvalidQueue is returned if a user chooses a queue not on the list
func selectQueue(queueListIndex *[]astroplatformcore.WorkerQueue, out io.Writer) (string, error) {
	var (
		errToReturn        error
		queueName, message string
		queueToDelete      astroplatformcore.WorkerQueue
	)

	queueList := *queueListIndex

	tab := printutil.Table{
		Padding:        []int{5, 30, 20, 50},
		DynamicPadding: true,
		Header:         []string{"#", "WORKER QUEUE", "ISDEFAULT", "ID"},
	}

	fmt.Println(message)

	sort.Slice(queueList, func(i, j int) bool {
		return queueList[i].Name < queueList[j].Name
	})

	queueMap := map[string]astroplatformcore.WorkerQueue{}
	for i := range queueList {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), queueList[i].Name, strconv.FormatBool(queueList[i].IsDefault), queueList[i].Id}, false)

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

// updateQueueList is used to merge existingQueues with the queueToUpdate. Based on the executor for the deployment, it
// sets the resources for CeleryExecutor and removes all resources for KubernetesExecutor as they get calculated based
// on the worker type.
//
//nolint:dupl
func updateQueueList(existingQueues []astroplatformcore.WorkerQueueRequest, queueToUpdate *astroplatformcore.WorkerQueueRequest, executor *astroplatformcore.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astroplatformcore.WorkerQueueRequest {
	for i, queue := range existingQueues { //nolint
		if queue.Name != queueToUpdate.Name {
			continue
		}

		queue.Id = existingQueues[i].Id               // we need IDs to update existing queues
		queue.IsDefault = existingQueues[i].IsDefault // users can not change this
		if *executor == astroplatformcore.DeploymentExecutorCELERY {
			if wQueueMin != -1 {
				queue.MinWorkerCount = queueToUpdate.MinWorkerCount
			}
			if wQueueMax != 0 {
				queue.MaxWorkerCount = queueToUpdate.MaxWorkerCount
			}
			if wQueueConcurrency != 0 {
				queue.WorkerConcurrency = queueToUpdate.WorkerConcurrency
			}
		} else if *executor == astroplatformcore.DeploymentExecutorKUBERNETES {
			// KubernetesExecutor calculates resources automatically based on the worker type
			queue.WorkerConcurrency = 0
			queue.MinWorkerCount = 0
			queue.MaxWorkerCount = 0
		}
		queue.AstroMachine = queueToUpdate.AstroMachine
		existingQueues[i] = queue
		return existingQueues
	}
	return existingQueues
}

//nolint:dupl
func updateHybridQueueList(existingQueues []astroplatformcore.HybridWorkerQueueRequest, queueToUpdate *astroplatformcore.HybridWorkerQueueRequest, executor *astroplatformcore.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astroplatformcore.HybridWorkerQueueRequest {
	for i, queue := range existingQueues { //nolint
		if queue.Name != queueToUpdate.Name {
			continue
		}

		queue.Id = existingQueues[i].Id               // we need IDs to update existing queues
		queue.IsDefault = existingQueues[i].IsDefault // users can not change this
		if *executor == astroplatformcore.DeploymentExecutorCELERY {
			if wQueueMin != -1 {
				queue.MinWorkerCount = queueToUpdate.MinWorkerCount
			}
			if wQueueMax != 0 {
				queue.MaxWorkerCount = queueToUpdate.MaxWorkerCount
			}
			if wQueueConcurrency != 0 {
				queue.WorkerConcurrency = queueToUpdate.WorkerConcurrency
			}
		} else if *executor == astroplatformcore.DeploymentExecutorKUBERNETES {
			// KubernetesExecutor calculates resources automatically based on the worker type
			queue.WorkerConcurrency = 0
			queue.MinWorkerCount = 0
			queue.MaxWorkerCount = 0
		}
		queue.NodePoolId = queueToUpdate.NodePoolId
		existingQueues[i] = queue
		return existingQueues
	}
	return existingQueues
}

// getQueueName returns the name for a worker-queue. If action is to create, it prompts the user for a name to use.
// If action is to update, it makes the user select a queue from a list of existing ones.
// It returns errInvalidQueue if a user chooses a queue not on the list
func getQueueName(name, action string, requestedDeployment *astroplatformcore.Deployment, out io.Writer) (string, error) {
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

// sanitizeExistingQueues takes a list of existing worker queues and removes fields that are not needed for queues based
// on the executor. For deployments with CeleryExecutor it returns a list of queues without PodCPU and PodRam.  For
// deployments with KubernetesExecutor it returns a list of queues with no resources as they get calculated
// based on the worker type.
func sanitizeExistingQueues(existingQueues []astroplatformcore.WorkerQueue, executor astroplatformcore.DeploymentExecutor) []astroplatformcore.WorkerQueue {
	// sort queues by name
	sort.Slice(existingQueues, func(i, j int) bool {
		return existingQueues[i].Name < existingQueues[j].Name
	})
	for i := range existingQueues {
		if executor == astroplatformcore.DeploymentExecutorCELERY {
			existingQueues[i].PodMemory = ""
			existingQueues[i].PodCpu = ""
		} else if executor == astroplatformcore.DeploymentExecutorKUBERNETES {
			// KubernetesExecutor calculates resources automatically based on the worker type
			existingQueues[i].WorkerConcurrency = 0
			existingQueues[i].MinWorkerCount = 0
			existingQueues[i].MaxWorkerCount = 0
			existingQueues[i].PodMemory = ""
			existingQueues[i].PodCpu = ""
		}
	}
	return existingQueues
}
