package workerqueue

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
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
	errNoWorkerQueues            = errors.New("no worker queues found for this deployment")
)

// CreateOrUpdate creates a new worker queue or updates an existing worker queue for a deployment.
func CreateOrUpdate(ws, deploymentID, deploymentName, name, action, workerType string, wQueueMin, wQueueMax, wQueueConcurrency int, force bool, astroV1Client astrov1.APIClient, out io.Writer) error { //nolint
	var (
		requestedDeployment                  astrov1.Deployment
		err                                  error
		errHelp, succeededAction, nodePoolID string
		workerMachine                        astrov1.WorkerMachine
		queueToCreateOrUpdate                astrov1.WorkerQueueRequest
		queueToCreateOrUpdateHybrid          astrov1.HybridWorkerQueueRequest
		listToCreate                         []astrov1.WorkerQueueRequest
		existingQueues                       []astrov1.WorkerQueue
		hybridListToCreate                   []astrov1.HybridWorkerQueueRequest
		defaultOptions                       astrov1.WorkerQueueOptions
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, true, nil, astroV1Client)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("%s %s\n", deployment.NoDeploymentInWSMsg, ansi.Bold(ws))
		return nil
	}

	getDeploymentOptions := astrov1.GetDeploymentOptionsParams{
		DeploymentId: &requestedDeployment.Id,
	}
	deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", getDeploymentOptions, astroV1Client)
	if err != nil {
		return err
	}
	defaultOptions = deploymentOptions.WorkerQueues

	if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
		// create listToCreate
		if requestedDeployment.WorkerQueues != nil {
			queues := *requestedDeployment.WorkerQueues
			for i := range *requestedDeployment.WorkerQueues {
				existingQueueRequest := astrov1.WorkerQueueRequest{
					Name:              queues[i].Name,
					Id:                &queues[i].Id,
					IsDefault:         queues[i].IsDefault,
					MaxWorkerCount:    queues[i].MaxWorkerCount,
					MinWorkerCount:    queues[i].MinWorkerCount,
					WorkerConcurrency: queues[i].WorkerConcurrency,
					AstroMachine:      astrov1.WorkerQueueRequestAstroMachine(*queues[i].AstroMachine),
				}
				listToCreate = append(listToCreate, existingQueueRequest)
			}
		}
		if name == "" {
			name, err = getQueueName(name, action, &requestedDeployment, out)
			if err != nil {
				return err
			}
		}
		if action == updateAction && workerType == "" {
			// get workerType
			for i := range listToCreate {
				if name == listToCreate[i].Name {
					workerType = string(listToCreate[i].AstroMachine)
				}
			}
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
		queueToCreateOrUpdate = astrov1.WorkerQueueRequest{
			Name:              name,
			IsDefault:         false, // cannot create a default queue
			AstroMachine:      astrov1.WorkerQueueRequestAstroMachine(workerMachine.Name),
			MinWorkerCount:    wQueueMin,         // use the value from the user input
			MaxWorkerCount:    wQueueMax,         // use the value from the user input
			WorkerConcurrency: wQueueConcurrency, // use the value from the user input
		}
		queueToCreateOrUpdate = SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdate, defaultOptions, &workerMachine)
	} else {
		// get the node poolID to use
		cluster, err := deployment.GetClusterByID("", *requestedDeployment.ClusterId, astroV1Client)
		if err != nil {
			return err
		}
		nodePoolID, err = selectNodePool(workerType, *cluster.NodePools, out)
		if err != nil {
			return err
		}
		queueToCreateOrUpdateHybrid = astrov1.HybridWorkerQueueRequest{
			Name:              name,
			IsDefault:         false, // cannot create a default queue
			NodePoolId:        nodePoolID,
			MinWorkerCount:    wQueueMin,         // use the value from the user input
			MaxWorkerCount:    wQueueMax,         // use the value from the user input
			WorkerConcurrency: wQueueConcurrency, // use the value from the user input
		}
		// create hybridListToCreate
		queues := *requestedDeployment.WorkerQueues
		for i := range *requestedDeployment.WorkerQueues {
			existingHybridQueueRequest := astrov1.HybridWorkerQueueRequest{
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
	case astrov1.DeploymentExecutorCELERY, astrov1.DeploymentExecutorASTRO:
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			err = IsHostedWorkerQueueInputValid(queueToCreateOrUpdate, defaultOptions, &workerMachine)
			if err != nil {
				return err
			}
		} else {
			err = IsWorkerQueueInputValid(queueToCreateOrUpdateHybrid, defaultOptions)
			if err != nil {
				return err
			}
		}
	case astrov1.DeploymentExecutorKUBERNETES:
		// worker queues are only used with the kubernetes execuor for hybrid deployments
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			return errNoUseWorkerQueues
		}
		// -1 is the CLI default to allow users to request wQueueMin=0. Here we set it to default because MinWorkerCount is not used in Kubernetes Deployments
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
		// add the new queue to the list of worker queues
		listToCreate = append(listToCreate, queueToCreateOrUpdate) //nolint
		hybridListToCreate = append(hybridListToCreate, queueToCreateOrUpdateHybrid)
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
			listToCreate = updateQueueList(listToCreate, queueToCreateOrUpdate, requestedDeployment.Executor, wQueueMin, wQueueMax, wQueueConcurrency)
			hybridListToCreate = updateHybridQueueList(hybridListToCreate, queueToCreateOrUpdateHybrid, requestedDeployment.Executor, wQueueMin, wQueueMax, wQueueConcurrency)
		} else {
			// update does not allow creating new queues
			if !reflect.DeepEqual(queueToCreateOrUpdate, astrov1.WorkerQueueRequest{}) {
				errHelp = fmt.Sprintf("use worker queue create %s instead", queueToCreateOrUpdate.Name)
			}
			if !reflect.DeepEqual(queueToCreateOrUpdateHybrid, astrov1.HybridWorkerQueueRequest{}) {
				errHelp = fmt.Sprintf("use worker queue create %s instead", queueToCreateOrUpdateHybrid.Name)
			}
			return fmt.Errorf("%w: %s", errCannotCreateNewQueue, errHelp)
		}
	}
	// update the deployment with the new list of worker queues
	err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, listToCreate, hybridListToCreate, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client)
	if err != nil {
		return err
	}
	// change action to past tense
	succeededAction = fmt.Sprintf("%sd", action)

	fmt.Fprintf(out, "worker queue %s for %s in %s workspace %s\n", name, requestedDeployment.Name, ws, succeededAction)
	return nil
}

// SetWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func SetWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate astrov1.WorkerQueueRequest, workerQueueDefaultOptions astrov1.WorkerQueueOptions, machineOptions *astrov1.WorkerMachine) astrov1.WorkerQueueRequest {
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
		workerQueueToCreate.WorkerConcurrency = int(machineOptions.Concurrency.Default)
	}
	return workerQueueToCreate
}

// SetWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func SetWorkerQueueValuesHybrid(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate astrov1.HybridWorkerQueueRequest, workerQueueDefaultOptions astrov1.WorkerQueueOptions) astrov1.HybridWorkerQueueRequest {
	// -1 is the CLI default to allow users to request wQueueMin=default
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

// IsWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// ErrNotSupported is returned if PodCPU or PodRAM are requested.
func IsWorkerQueueInputValid(requestedHybridWorkerQueue astrov1.HybridWorkerQueueRequest, defaultOptions astrov1.WorkerQueueOptions) error {
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

// IsHostedWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions and machineOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// ErrNotSupported is returned if PodCPU or PodRAM are requested.
func IsHostedWorkerQueueInputValid(requestedWorkerQueue astrov1.WorkerQueueRequest, defaultOptions astrov1.WorkerQueueOptions, machineOptions *astrov1.WorkerMachine) error {
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
		errorMessage = fmt.Sprintf("worker concurrency must be between %d and %d", workerConcurrenyFloor, int(machineOptions.Concurrency.Ceiling))
		return fmt.Errorf("%w: %s", errInvalidWorkerQueueOption, errorMessage)
	}
	return nil
}

// IsKubernetesWorkerQueueInputValid checks if the requestedQueue has all the necessary properties
// required to create a worker queue for the KubernetesExecutor.
// errNotSupported is returned for any invalid properties.
func IsKubernetesWorkerQueueInputValid(queueToCreateOrUpdateHybrid astrov1.HybridWorkerQueueRequest) error {
	var errorMessage string

	if queueToCreateOrUpdateHybrid.Name != defaultQueueName {
		errorMessage = "a non default worker queue in the request. Rename the queue to default"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, ErrNotSupported, errorMessage)
	}
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

// QueueExists takes a []existingQueues and a queueToCreateOrUpdate as arguments
// It returns true if queueToCreateOrUpdate exists in []existingQueues
// It returns false if queueToCreateOrUpdate does not exist in []existingQueues
func QueueExists(existingQueues []astrov1.WorkerQueue, queueToCreateOrUpdate astrov1.WorkerQueueRequest, queueToCreateOrUpdateHybrid astrov1.HybridWorkerQueueRequest) bool {
	for _, queue := range existingQueues { //nolint
		if queue.Name == queueToCreateOrUpdateHybrid.Name {
			// queueToCreateOrUpdate exists
			return true
		}
		if queueToCreateOrUpdateHybrid.Id != nil {
			if queue.Id == *queueToCreateOrUpdateHybrid.Id {
				// queueToCreateOrUpdate exists
				return true
			}
		}
		if queue.Name == queueToCreateOrUpdate.Name {
			// queueToCreateOrUpdate exists
			return true
		}
		if queueToCreateOrUpdate.Id != nil {
			if queue.Id == *queueToCreateOrUpdate.Id {
				// queueToCreateOrUpdate exists
				return true
			}
		}
	}
	return false
}

func selectWorkerMachine(workerType string, workerMachines []astrov1.WorkerMachine, out io.Writer) (astrov1.WorkerMachine, error) {
	var (
		workerMachine astrov1.WorkerMachine
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

		machineMap := map[string]astrov1.WorkerMachine{}

		for i := range workerMachines {
			index := i + 1
			tab.AddRow([]string{strconv.Itoa(index), string(workerMachines[i].Name), workerMachines[i].Spec.Cpu + " vCPU", workerMachines[i].Spec.Memory}, false)

			machineMap[strconv.Itoa(index)] = workerMachines[i]
		}

		tab.Print(out)
		choice := input.Text("\n> ")
		selectedPool, ok := machineMap[choice]
		if !ok {
			// returning an error as choice was not in nodePoolMap
			errToReturn = fmt.Errorf("%w: invalid worker type: %s selected", errInvalidAstroMachine, choice)
			return astrov1.WorkerMachine{}, errToReturn
		}
		return selectedPool, nil
	default:
		for _, workerMachine = range workerMachines {
			if strings.EqualFold(string(workerMachine.Name), workerType) {
				return workerMachine, nil
			}
		}
		// did not find a matching workerType in any node pool
		errToReturn = fmt.Errorf("%w: workerType %s is not available for this deployment", errInvalidAstroMachine, workerType)
		return astrov1.WorkerMachine{}, errToReturn
	}
}

// selectNodePool takes workerType and []NodePool as arguments
// If user requested a workerType, then the matching nodePoolID is returned
// If user did not request a workerType, then it prompts the user to pick one
// An errInvalidNodePool is returned if a user chooses an option not on the list
func selectNodePool(workerType string, nodePools []astrov1.NodePool, out io.Writer) (string, error) {
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

		nodePoolMap := map[string]astrov1.NodePool{}
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
func Delete(ws, deploymentID, deploymentName, name string, force bool, astroV1Client astrov1.APIClient, out io.Writer) error { //nolint:gocognit
	var (
		requestedDeployment      astrov1.Deployment
		err                      error
		queueToDelete            astrov1.WorkerQueueRequest
		queueToDeleteHybrid      astrov1.HybridWorkerQueueRequest
		existingQueues           []astrov1.WorkerQueue
		workerQueuesToKeep       []astrov1.WorkerQueueRequest
		hybridWorkerQueuesToKeep []astrov1.HybridWorkerQueueRequest
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, true, nil, astroV1Client)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("%s %s\n", deployment.NoDeploymentInWSMsg, ansi.Bold(ws))
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
	queueToDelete = astrov1.WorkerQueueRequest{
		Name:      name,
		IsDefault: false, // cannot delete a default queue
	}
	queueToDeleteHybrid = astrov1.HybridWorkerQueueRequest{
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
			// create a new workerQueuesToKeep without queueToDelete in it
			for i := range existingQueues { //nolint
				if existingQueues[i].Name != queueToDelete.Name {
					existingQueueRequest := astrov1.WorkerQueueRequest{
						Name:              existingQueues[i].Name,
						Id:                &existingQueues[i].Id,
						IsDefault:         existingQueues[i].IsDefault,
						MaxWorkerCount:    existingQueues[i].MaxWorkerCount,
						MinWorkerCount:    existingQueues[i].MinWorkerCount,
						WorkerConcurrency: existingQueues[i].WorkerConcurrency,
						AstroMachine:      astrov1.WorkerQueueRequestAstroMachine(*existingQueues[i].AstroMachine),
					}
					workerQueuesToKeep = append(workerQueuesToKeep, existingQueueRequest)
				}
			}
			// update the deployment with the new list
			err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueuesToKeep, hybridWorkerQueuesToKeep, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "worker queue %s for %s in %s workspace deleted\n", queueToDelete.Name, requestedDeployment.Name, ws)
		} else {
			// create a new listToDeleteHybrid without queueToDeleteHybrid in it
			for i := range existingQueues { //nolint
				if existingQueues[i].Name != queueToDeleteHybrid.Name {
					existingQueueRequest := astrov1.HybridWorkerQueueRequest{
						Name:              existingQueues[i].Name,
						Id:                &existingQueues[i].Id,
						IsDefault:         existingQueues[i].IsDefault,
						MaxWorkerCount:    existingQueues[i].MaxWorkerCount,
						MinWorkerCount:    existingQueues[i].MinWorkerCount,
						WorkerConcurrency: existingQueues[i].WorkerConcurrency,
						NodePoolId:        *existingQueues[i].NodePoolId,
					}
					hybridWorkerQueuesToKeep = append(hybridWorkerQueuesToKeep, existingQueueRequest)
				}
			}
			// update the deployment with the new list
			err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueuesToKeep, hybridWorkerQueuesToKeep, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client)
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
func selectQueue(queueListIndex *[]astrov1.WorkerQueue, out io.Writer) (string, error) {
	var (
		errToReturn        error
		queueName, message string
		queueToDelete      astrov1.WorkerQueue
		queueList          []astrov1.WorkerQueue
	)
	if queueListIndex != nil {
		queueList = *queueListIndex
	} else {
		return "", errNoWorkerQueues
	}

	tab := printutil.Table{
		Padding:        []int{5, 30, 20, 50},
		DynamicPadding: true,
		Header:         []string{"#", "WORKER QUEUE", "ISDEFAULT", "ID"},
	}

	fmt.Println(message)

	sort.Slice(queueList, func(i, j int) bool {
		return queueList[i].Name < queueList[j].Name
	})

	queueMap := map[string]astrov1.WorkerQueue{}
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
// sets the resources for CeleryExecutor and AstroExecutor and removes all resources for KubernetesExecutor as they get calculated based
// on the worker type.
//
//nolint:dupl
func updateQueueList(existingQueues []astrov1.WorkerQueueRequest, queueToUpdate astrov1.WorkerQueueRequest, executor *astrov1.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astrov1.WorkerQueueRequest {
	for i, queue := range existingQueues { //nolint
		if queue.Name != queueToUpdate.Name {
			continue
		}

		queue.Id = existingQueues[i].Id               // we need IDs to update existing queues
		queue.IsDefault = existingQueues[i].IsDefault // users can not change this
		switch *executor {
		case astrov1.DeploymentExecutorCELERY, astrov1.DeploymentExecutorASTRO:
			if wQueueMin != -1 {
				queue.MinWorkerCount = queueToUpdate.MinWorkerCount
			}
			if wQueueMax != 0 {
				queue.MaxWorkerCount = queueToUpdate.MaxWorkerCount
			}
			if wQueueConcurrency != 0 {
				queue.WorkerConcurrency = queueToUpdate.WorkerConcurrency
			}
		case astrov1.DeploymentExecutorKUBERNETES:
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
func updateHybridQueueList(existingQueues []astrov1.HybridWorkerQueueRequest, queueToUpdate astrov1.HybridWorkerQueueRequest, executor *astrov1.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astrov1.HybridWorkerQueueRequest {
	for i, queue := range existingQueues { //nolint
		if queue.Name != queueToUpdate.Name {
			continue
		}

		queue.Id = existingQueues[i].Id               // we need IDs to update existing queues
		queue.IsDefault = existingQueues[i].IsDefault // users can not change this
		if *executor == astrov1.DeploymentExecutorCELERY {
			if wQueueMin != -1 {
				queue.MinWorkerCount = queueToUpdate.MinWorkerCount
			}
			if wQueueMax != 0 {
				queue.MaxWorkerCount = queueToUpdate.MaxWorkerCount
			}
			if wQueueConcurrency != 0 {
				queue.WorkerConcurrency = queueToUpdate.WorkerConcurrency
			}
		} else if *executor == astrov1.DeploymentExecutorKUBERNETES {
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
func getQueueName(name, action string, requestedDeployment *astrov1.Deployment, out io.Writer) (string, error) {
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
func sanitizeExistingQueues(existingQueues []astrov1.WorkerQueue, executor astrov1.DeploymentExecutor) []astrov1.WorkerQueue {
	// sort queues by name
	sort.Slice(existingQueues, func(i, j int) bool {
		return existingQueues[i].Name < existingQueues[j].Name
	})
	for i := range existingQueues {
		if executor == astrov1.DeploymentExecutorCELERY {
			existingQueues[i].PodMemory = ""
			existingQueues[i].PodCpu = ""
		} else if executor == astrov1.DeploymentExecutorKUBERNETES {
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
