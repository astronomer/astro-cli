package workerqueue

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
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
	errNotSupported              = errors.New("does not support")
	errNoUseWorkerQueues         = errors.New("don't use 'worker_queues' to update default queue with KubernetesExecutor, use 'default_task_pod_cpu' and 'default_task_pod_memory' instead")
	errNoWorkerQueues            = errors.New("no worker queues found for this deployment")
	errNoWorkerTypes             = errors.New("no worker types are available for this deployment")
)

// CreateOrUpdate creates a new worker queue or updates an existing worker queue for a deployment.
func CreateOrUpdate(ws, deploymentID, deploymentName, name, action, workerType string, wQueueMin, wQueueMax, wQueueConcurrency int, force bool, astroV1Client astrov1.APIClient, out io.Writer) (*Result, error) { //nolint:gocognit,gocyclo // v1 complexity, refactor tracked separately
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
		nodePools                            []astrov1.NodePool
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(ws, deploymentID, deploymentName, true, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	if requestedDeployment.Id == "" {
		return nil, errNoDeployment(ws)
	}

	getDeploymentOptions := astrov1.GetDeploymentOptionsParams{
		DeploymentId: &requestedDeployment.Id,
	}
	deploymentOptions, err := deployment.GetPlatformDeploymentOptions("", getDeploymentOptions, astroV1Client)
	if err != nil {
		return nil, err
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
				return nil, err
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
			return nil, err
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
		queueToCreateOrUpdate = setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdate, defaultOptions, &workerMachine)
	} else {
		// get the node poolID to use
		cluster, err := deployment.GetClusterByID("", *requestedDeployment.ClusterId, astroV1Client)
		if err != nil {
			return nil, err
		}
		nodePools = *cluster.NodePools
		nodePoolID, err = selectNodePool(workerType, *cluster.NodePools, out)
		if err != nil {
			return nil, err
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
				return nil, err
			}
			name = queueToCreateOrUpdateHybrid.Name
		}
		queueToCreateOrUpdateHybrid = setWorkerQueueValuesHybrid(wQueueMin, wQueueMax, wQueueConcurrency, queueToCreateOrUpdateHybrid, defaultOptions)
	}
	switch *requestedDeployment.Executor {
	case astrov1.DeploymentExecutorCELERY, astrov1.DeploymentExecutorASTRO:
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			err = isHostedWorkerQueueInputValid(queueToCreateOrUpdate, defaultOptions, &workerMachine)
			if err != nil {
				return nil, err
			}
		} else {
			err = isWorkerQueueInputValid(queueToCreateOrUpdateHybrid, defaultOptions)
			if err != nil {
				return nil, err
			}
		}
	case astrov1.DeploymentExecutorKUBERNETES:
		// worker queues are only used with the kubernetes execuor for hybrid deployments
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			return nil, errNoUseWorkerQueues
		}
		// -1 is the CLI default to allow users to request wQueueMin=0. Here we set it to default because MinWorkerCount is not used in Kubernetes Deployments
		queueToCreateOrUpdateHybrid.MinWorkerCount = -1
		err = isKubernetesWorkerQueueInputValid(queueToCreateOrUpdateHybrid)
		if err != nil {
			return nil, err
		}
	}

	// sanitize all the existing queues based on executor
	existingQueues = sanitizeExistingQueues(*requestedDeployment.WorkerQueues, *requestedDeployment.Executor)
	// create listToCreate
	switch action {
	case createAction:
		if queueExists(existingQueues, queueToCreateOrUpdate, queueToCreateOrUpdateHybrid) {
			// create does not allow updating existing queues
			errHelp = fmt.Sprintf("use worker queue update %s instead", name)
			return nil, fmt.Errorf("%w: %s", errCannotUpdateExistingQueue, errHelp)
		}
		// add the new queue to the list of worker queues
		listToCreate = append(listToCreate, queueToCreateOrUpdate)
		hybridListToCreate = append(hybridListToCreate, queueToCreateOrUpdateHybrid)
	case updateAction:
		if queueExists(existingQueues, queueToCreateOrUpdate, queueToCreateOrUpdateHybrid) {
			if !force {
				i, err := input.Confirm(
					fmt.Sprintf("\nAre you sure you want to %s the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", action, ansi.Bold(name)), input.AnsweredBy("--yes"))
				if err != nil {
					return nil, err
				}

				if !i {
					fmt.Fprintf(out, "Canceling worker queue %s\n", action)
					return nil, nil
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
			return nil, fmt.Errorf("%w: %s", errCannotCreateNewQueue, errHelp)
		}
	}
	// update the deployment with the new list of worker queues
	_, err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, listToCreate, hybridListToCreate, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client, nil)
	if err != nil {
		return nil, err
	}
	// change action to past tense
	succeededAction = fmt.Sprintf("%sd", action)

	res := &Result{
		DeploymentID:   requestedDeployment.Id,
		DeploymentName: requestedDeployment.Name,
		WorkspaceID:    requestedDeployment.WorkspaceId,
		Action:         succeededAction,
	}
	for i := range listToCreate {
		if q := listToCreate[i]; q.Name == name && (deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type)) {
			res.WorkerQueue = Queue{Name: q.Name, IsDefault: q.IsDefault, WorkerType: string(q.AstroMachine), MinWorkerCount: q.MinWorkerCount, MaxWorkerCount: q.MaxWorkerCount, WorkerConcurrency: q.WorkerConcurrency}
		}
	}
	for i := range hybridListToCreate {
		if q := hybridListToCreate[i]; q.Name == name && !(deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type)) {
			res.WorkerQueue = Queue{Name: q.Name, IsDefault: q.IsDefault, WorkerType: nodePoolInstanceType(q.NodePoolId, nodePools), MinWorkerCount: q.MinWorkerCount, MaxWorkerCount: q.MaxWorkerCount, WorkerConcurrency: q.WorkerConcurrency}
			if *requestedDeployment.Executor == astrov1.DeploymentExecutorKUBERNETES {
				// A KubernetesExecutor queue has no counts of its own: its
				// pods are sized by the worker type. The request carries the
				// CLI's -1 "unset" for the minimum, which is not a count, so
				// the result reports what an update does (updateHybridQueueList):
				// 0 for all three.
				res.WorkerQueue.MinWorkerCount, res.WorkerQueue.MaxWorkerCount, res.WorkerQueue.WorkerConcurrency = 0, 0, 0
			}
		}
	}
	return res, nil
}

// setWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func setWorkerQueueValues(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate astrov1.WorkerQueueRequest, workerQueueDefaultOptions astrov1.WorkerQueueOptions, machineOptions *astrov1.WorkerMachine) astrov1.WorkerQueueRequest { //nolint:gocritic // WorkerQueueRequest is a large generated API type; passed by value intentionally
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

// setWorkerQueueValues sets default values for MinWorkerCount, MaxWorkerCount and WorkerConcurrency if none were requested.
func setWorkerQueueValuesHybrid(wQueueMin, wQueueMax, wQueueConcurrency int, workerQueueToCreate astrov1.HybridWorkerQueueRequest, workerQueueDefaultOptions astrov1.WorkerQueueOptions) astrov1.HybridWorkerQueueRequest {
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

// isWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// errNotSupported is returned if PodCPU or PodRAM are requested.
func isWorkerQueueInputValid(requestedHybridWorkerQueue astrov1.HybridWorkerQueueRequest, defaultOptions astrov1.WorkerQueueOptions) error {
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

// isHostedWorkerQueueInputValid checks if the requestedWorkerQueue adheres to the floor and ceiling set in the defaultOptions and machineOptions.
// if it adheres to them, it returns nil.
// errInvalidWorkerQueueOption is returned if min, max or concurrency are out of range.
// errNotSupported is returned if PodCPU or PodRAM are requested.
func isHostedWorkerQueueInputValid(requestedWorkerQueue astrov1.WorkerQueueRequest, defaultOptions astrov1.WorkerQueueOptions, machineOptions *astrov1.WorkerMachine) error { //nolint:gocritic // WorkerQueueRequest is a large generated API type; passed by value intentionally
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

// isKubernetesWorkerQueueInputValid checks if the requestedQueue has all the necessary properties
// required to create a worker queue for the KubernetesExecutor.
// errNotSupported is returned for any invalid properties.
func isKubernetesWorkerQueueInputValid(queueToCreateOrUpdateHybrid astrov1.HybridWorkerQueueRequest) error {
	var errorMessage string

	if queueToCreateOrUpdateHybrid.Name != defaultQueueName {
		errorMessage = "a non default worker queue in the request. Rename the queue to default"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, errNotSupported, errorMessage)
	}
	if queueToCreateOrUpdateHybrid.MaxWorkerCount != 0 {
		errorMessage = "maximum worker count in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, errNotSupported, errorMessage)
	}
	if queueToCreateOrUpdateHybrid.WorkerConcurrency != 0 {
		errorMessage = "worker concurrency in the request. It can only be used with CeleryExecutor"
		return fmt.Errorf("%s %w %s", deployment.KubeExecutor, errNotSupported, errorMessage)
	}

	return nil
}

// queueExists takes a []existingQueues and a queueToCreateOrUpdate as arguments
// It returns true if queueToCreateOrUpdate exists in []existingQueues
// It returns false if queueToCreateOrUpdate does not exist in []existingQueues
func queueExists(existingQueues []astrov1.WorkerQueue, queueToCreateOrUpdate astrov1.WorkerQueueRequest, queueToCreateOrUpdateHybrid astrov1.HybridWorkerQueueRequest) bool { //nolint:gocritic // WorkerQueueRequest is a large generated API type; passed by value intentionally
	for _, queue := range existingQueues {
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
		list := picker.List{
			Title:  "No worker type was specified. Select the worker type to use",
			Header: []string{"WORKER TYPE", "CPU", "Memory"},
			Ask:    []input.Option{input.About("a worker type"), input.AnsweredBy("--worker-type")},
			InvalidAnswer: func(choice string) error {
				return fmt.Errorf("%w: invalid worker type: %s selected", errInvalidAstroMachine, choice)
			},
			Empty: errNoWorkerTypes,
		}
		for i := range workerMachines {
			list.AddRow(false, string(workerMachines[i].Name), workerMachines[i].Spec.Cpu+" vCPU", workerMachines[i].Spec.Memory)
		}
		i, err := list.Pick(out, os.Stdin)
		if err != nil {
			return astrov1.WorkerMachine{}, err
		}
		return workerMachines[i], nil
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
		sort.Slice(nodePools, func(i, j int) bool {
			return nodePools[i].CreatedAt.Before(nodePools[j].CreatedAt)
		})

		list := picker.List{
			Title:  message,
			Header: []string{"WORKER TYPE", "ISDEFAULT", "ID"},
			Ask:    []input.Option{input.About("a worker type"), input.AnsweredBy("--worker-type")},
			InvalidAnswer: func(choice string) error {
				return fmt.Errorf("%w: invalid worker type: %s selected", errInvalidNodePool, choice)
			},
			Empty: errNoWorkerTypes,
		}
		for i := range nodePools {
			list.AddRow(false, nodePools[i].NodeInstanceType, strconv.FormatBool(nodePools[i].IsDefault), nodePools[i].Id)
		}
		i, err := list.Pick(out, os.Stdin)
		if err != nil {
			return nodePoolID, err
		}
		return nodePools[i].Id, nil
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
func Delete(ws, deploymentID, deploymentName, name string, force bool, astroV1Client astrov1.APIClient, out io.Writer) (*Result, error) { //nolint:gocognit // v1 complexity, refactor tracked separately
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
		return nil, err
	}

	if requestedDeployment.Id == "" {
		return nil, errNoDeployment(ws)
	}

	// prompt for queue name if one was not provided
	if name == "" {
		name, err = selectQueue(requestedDeployment.WorkerQueues, out)
		if err != nil {
			return nil, err
		}
	}
	// check if default queue is being deleted
	if name == defaultQueueName {
		return nil, errCannotDeleteDefaultQueue
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

	if queueExists(existingQueues, queueToDelete, queueToDeleteHybrid) {
		if !force {
			i, err := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to delete the %s worker queue? If there are any tasks in your DAGs assigned to this worker queue, the tasks might get stuck in a queued state and fail to execute", ansi.Bold(queueToDelete.Name)), input.AnsweredBy("--yes"))
			if err != nil {
				return nil, err
			}

			if !i {
				fmt.Fprintf(out, "Canceling worker queue deletion\n")
				return nil, nil
			}
		}
		if deployment.IsDeploymentStandard(*requestedDeployment.Type) || deployment.IsDeploymentDedicated(*requestedDeployment.Type) {
			// create a new workerQueuesToKeep without queueToDelete in it
			for i := range existingQueues {
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
			_, err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueuesToKeep, hybridWorkerQueuesToKeep, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client, nil)
			if err != nil {
				return nil, err
			}
		} else {
			// create a new listToDeleteHybrid without queueToDeleteHybrid in it
			for i := range existingQueues {
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
			_, err = deployment.Update(requestedDeployment.Id, "", ws, "", "", "", "", "", "", "", "", "", "", "", "", "", 0, 0, workerQueuesToKeep, hybridWorkerQueuesToKeep, []astrov1.DeploymentEnvironmentVariableRequest{}, nil, nil, nil, true, astroV1Client, nil)
			if err != nil {
				return nil, err
			}
		}
		res := &Result{
			DeploymentID:   requestedDeployment.Id,
			DeploymentName: requestedDeployment.Name,
			WorkspaceID:    requestedDeployment.WorkspaceId,
			Action:         actionDeleted,
		}
		for i := range existingQueues {
			if q := existingQueues[i]; q.Name == name {
				res.WorkerQueue = Queue{Name: q.Name, IsDefault: q.IsDefault, MinWorkerCount: q.MinWorkerCount, MaxWorkerCount: q.MaxWorkerCount, WorkerConcurrency: q.WorkerConcurrency}
				if q.AstroMachine != nil {
					res.WorkerQueue.WorkerType = *q.AstroMachine
				}
			}
		}
		return res, nil
	}
	// can not delete a queue that does not exist
	return nil, fmt.Errorf("%w: %s", errQueueDoesNotExist, queueToDelete.Name)
}

// selectQueue takes []WorkerQueue and io.Writer as arguments
// user can select a queue to delete from the list and the name of the selected queue is returned
// An errInvalidQueue is returned if a user chooses a queue not on the list
func selectQueue(queueListIndex *[]astrov1.WorkerQueue, out io.Writer) (string, error) {
	if queueListIndex == nil {
		return "", errNoWorkerQueues
	}
	queueList := *queueListIndex

	sort.Slice(queueList, func(i, j int) bool {
		return queueList[i].Name < queueList[j].Name
	})

	list := picker.List{
		Header: []string{"WORKER QUEUE", "ISDEFAULT", "ID"},
		Ask:    []input.Option{input.About("a worker queue"), input.AnsweredBy("--name")},
		InvalidAnswer: func(choice string) error {
			return fmt.Errorf("%w: invalid worker queue: %s selected", errInvalidQueue, choice)
		},
	}
	for i := range queueList {
		list.AddRow(false, queueList[i].Name, strconv.FormatBool(queueList[i].IsDefault), queueList[i].Id)
	}
	i, err := list.Pick(out, os.Stdin)
	if err != nil {
		return "", err
	}
	return queueList[i].Name, nil
}

// updateQueueList is used to merge existingQueues with the queueToUpdate. Based on the executor for the deployment, it
// sets the resources for CeleryExecutor and AstroExecutor and removes all resources for KubernetesExecutor as they get calculated based
// on the worker type.
func updateQueueList(existingQueues []astrov1.WorkerQueueRequest, queueToUpdate astrov1.WorkerQueueRequest, executor *astrov1.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astrov1.WorkerQueueRequest { //nolint:gocritic // WorkerQueueRequest is a large generated API type; passed by value intentionally
	for i, queue := range existingQueues {
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

func updateHybridQueueList(existingQueues []astrov1.HybridWorkerQueueRequest, queueToUpdate astrov1.HybridWorkerQueueRequest, executor *astrov1.DeploymentExecutor, wQueueMin, wQueueMax, wQueueConcurrency int) []astrov1.HybridWorkerQueueRequest {
	for i, queue := range existingQueues {
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
			queueName, err = input.Text("Enter a name for the worker queue\n> ", input.AnsweredBy("--name"))
			if err != nil {
				return "", err
			}
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
