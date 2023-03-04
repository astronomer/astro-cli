package deployment

import (
	"bytes"
	"context"
	"fmt"
	astroContext "github.com/astronomer/astro-cli/context"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/util"
	"github.com/pkg/errors"
)

var (
	errInvalidDeployment    = errors.New("the Deployment specified was not found in this workspace. Your account or API Key may not have access to the deployment specified")
	ErrInvalidDeploymentKey = errors.New("invalid Deployment selected")
	errTimedOut             = errors.New("timed out waiting for the deployment to become healthy")
	// Monkey patched to write unit tests
	createDeployment = Create
)

const (
	noWorkspaceMsg = "no workspaces with id (%s) found"
	KubeExecutor   = "KubernetesExecutor"
	CeleryExecutor = "CeleryExecutor"
)

// TODO: get these values from the Astrohub API
var (
	SchedulerAuMin       = 5
	SchedulerReplicasMin = 1
	schedulerAuMax       = 30
	schedulerReplicasMax = 4
	sleepTime            = 5
	tickNum              = 10
	timeoutNum           = 180
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "NAMESPACE", "CLUSTER", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED"},
	}
}

func newTableOutAll() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE", "NAMESPACE", "CLUSTER", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED"},
	}
}

// List all airflow deployments
func List(ws string, all bool, client astro.Client, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	tab := newTableOut()
	if all {
		ws = ""
		tab = newTableOutAll()
	}
	deployments, err := client.ListDeployments(c.Organization, ws)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })

	// Build rows
	for i := range deployments {
		d := deployments[i]
		runtimeVersionText := d.RuntimeRelease.Version + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"
		if all {
			tab.AddRow([]string{d.Label, d.Workspace.Label, d.ReleaseName, d.Cluster.Name, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled)}, false)
		} else {
			tab.AddRow([]string{d.Label, d.ReleaseName, d.Cluster.Name, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled)}, false)
		}
	}

	return tab.Print(out)
}

func Logs(deploymentID, ws, deploymentName string, warnLogs, errorLogs, infoLogs bool, logCount int, client astro.Client) error {
	logLevels := []string{}

	// log level
	if warnLogs {
		logLevels = append(logLevels, "WARN")
	}
	if errorLogs {
		logLevels = append(logLevels, "ERROR")
	}
	if infoLogs {
		logLevels = append(logLevels, "INFO")
	}
	if len(logLevels) == 0 {
		logLevels = []string{"WARN", "ERROR", "INFO"}
	}

	// get deployment
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	deploymentID = deployment.ID

	// deployment logs request
	vars := map[string]interface{}{
		"deploymentId":  deploymentID,
		"logCountLimit": logCount,
		"start":         "-24hrs",
		"logLevels":     logLevels,
	}

	deploymentHistoryResp, err := client.GetDeploymentHistory(vars)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	Logs := deploymentHistoryResp.SchedulerLogs

	if len(Logs) == 0 {
		fmt.Println("No matching logs have been recorded in the past 24 hours for Deployment " + deployment.Label)
		return nil
	}

	fmt.Println(Logs)

	return nil
}

func Create(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool) error {
	var organizationID string
	var currentWorkspace astro.Workspace
	var dagDeployEnabled bool

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	// validate resources requests
	resourcesValid := validateResources(schedulerAU, schedulerReplicas)
	if !resourcesValid {
		return nil
	}

	versionValid, err := validateRuntimeVersion(runtimeVersion, client)
	if err != nil {
		return err
	}

	if !versionValid {
		return nil
	}

	// validate workspace
	ws, err := client.ListWorkspaces(c.Organization)
	if err != nil {
		return err
	}

	for i := range ws {
		if workspaceID == ws[i].ID {
			organizationID = ws[i].OrganizationID
			currentWorkspace = ws[i]
		}
	}

	if organizationID == "" {
		return fmt.Errorf(noWorkspaceMsg, workspaceID) //nolint:goerr113
	}
	fmt.Printf("Current Workspace: %s\n\n", currentWorkspace.Label)

	// label input
	if label == "" {
		fmt.Println("Please specify a name for your Deployment")
		label = input.Text(ansi.Bold("\nDeployment name: "))
		if label == "" {
			return errors.New("you must give your Deployment a name")
		}
		deployments, err := GetDeployments(workspaceID, client)
		if err != nil {
			return errors.Wrap(err, errInvalidDeployment.Error())
		}

		for i := range deployments {
			if deployments[i].Label == label {
				return errors.New("A Deployment with that name already exists")
			}
		}
	}

	// select and validate cluster
	clusterID, err = selectCluster(clusterID, organizationID, client)
	if err != nil {
		return err
	}

	scheduler := astro.Scheduler{
		AU:       schedulerAU,
		Replicas: schedulerReplicas,
	}

	spec := astro.DeploymentCreateSpec{
		Executor:  executor,
		Scheduler: scheduler,
	}

	if dagDeploy == "enable" { //nolint: goconst
		dagDeployEnabled = true
	} else {
		dagDeployEnabled = false
	}

	createInput := &astro.CreateDeploymentInput{
		WorkspaceID:           workspaceID,
		ClusterID:             clusterID,
		Label:                 label,
		Description:           description,
		DagDeployEnabled:      dagDeployEnabled,
		RuntimeReleaseVersion: runtimeVersion,
		DeploymentSpec:        spec,
	}

	// Create request
	d, err := client.CreateDeployment(createInput)
	if err != nil {
		return err
	}

	if waitForStatus {
		err = healthPoll(d.ID, workspaceID, client, coreClient)
		if err != nil {
			errOutput := createOutput(workspaceID, &d)
			if errOutput != nil {
				return errOutput
			}
			return err
		}
	}

	err = createOutput(workspaceID, &d)
	if err != nil {
		return err
	}

	return nil
}

func createOutput(workspaceID string, d *astro.Deployment) error {
	tab := newTableOut()

	runtimeVersionText := d.RuntimeRelease.Version + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"

	tab.AddRow([]string{d.Label, d.ReleaseName, d.Cluster.Name, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled)}, false)

	deploymentURL, err := GetDeploymentURL(d.ID, workspaceID)
	if err != nil {
		return err
	}
	tab.SuccessMsg = fmt.Sprintf("\n Successfully created Deployment: %s", ansi.Bold(d.Label)) +
		"\n Deployment can be accessed at the following URLs \n" +
		fmt.Sprintf("\n Deployment Dashboard: %s", ansi.Bold(deploymentURL)) +
		fmt.Sprintf("\n Airflow Dashboard: %s", ansi.Bold(d.DeploymentSpec.Webserver.URL))

	tab.Print(os.Stdout)

	return nil
}

func validateResources(schedulerAU, schedulerReplicas int) bool {
	if schedulerAU > schedulerAuMax || schedulerAU < SchedulerAuMin {
		fmt.Printf("\nScheduler AUs must be between a min of %d and a max of %d AUs", SchedulerAuMin, schedulerAuMax)
		return false
	}
	if schedulerReplicas > schedulerReplicasMax || schedulerReplicas < SchedulerReplicasMin {
		fmt.Printf("\nScheduler Replicas must between a min of %d and a max of %d Replicas", SchedulerReplicasMin, schedulerReplicasMax)
		return false
	}
	return true
}

func validateRuntimeVersion(runtimeVersion string, client astro.Client) (bool, error) {
	runtimeReleases, err := GetRuntimeReleases(client)
	if err != nil {
		return false, err
	}
	if !util.Contains(runtimeReleases, runtimeVersion) {
		fmt.Printf("\nRuntime version not valid. Must be one of the following: %v\n", runtimeReleases)
		return false, nil
	}
	return true, nil
}

func GetRuntimeReleases(client astro.Client) ([]string, error) {
	// get deployment config options
	runtimeReleases := []string{}

	ConfigOptions, err := client.GetDeploymentConfig()
	if err != nil {
		return runtimeReleases, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	for i := range ConfigOptions.RuntimeReleases {
		runtimeReleases = append(runtimeReleases, ConfigOptions.RuntimeReleases[i].Version)
	}

	return runtimeReleases, nil
}

func selectCluster(clusterID, organizationID string, client astro.Client) (newClusterID string, err error) {
	clusterTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
	}
	// cluster request
	cs, err := client.ListClusters(organizationID)
	if err != nil {
		return "", errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	// select cluster
	if clusterID == "" {
		fmt.Println("\nPlease select a Cluster for your Deployment:")

		clusterMap := map[string]astro.Cluster{}
		for i := range cs {
			index := i + 1
			clusterTab.AddRow([]string{strconv.Itoa(index), cs[i].Name, cs[i].CloudProvider, cs[i].ID}, false)

			clusterMap[strconv.Itoa(index)] = cs[i]
		}

		clusterTab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := clusterMap[choice]
		if !ok {
			return "", ErrInvalidDeploymentKey
		}

		clusterID = selected.ID
	}

	// validate cluster
	csID := ""
	for i := range cs {
		if clusterID == cs[i].ID {
			csID = cs[i].ID
		}
	}
	if csID == "" {
		return "", errors.New("unable to find specified Cluster")
	}
	return clusterID, nil
}

func healthPoll(deploymentID, ws string, client astro.Client, coreClient astrocore.CoreClient) error {
	fmt.Printf("Waiting for the deployment to become healthy…\n\nThis may take a few minutes\n")
	time.Sleep(time.Duration(sleepTime) * time.Second)
	buf := new(bytes.Buffer)
	timeout := time.After(time.Duration(timeoutNum) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Second)
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return errTimedOut
		// Got a tick, we should check if deployment is healthy
		case <-ticker.C:
			buf.Reset()
			limit := 100
			params := &astrocore.ListDeploymentsParams{
				DeploymentIds: &deploymentID,
				WorkspaceIds:  nil,
				Offset:        nil,
				Limit:         &limit,
				Sorts:         nil,
			}
			ctx, err := astroContext.GetCurrentContext()
			if err != nil {
				return err
			}
			orgShortID := ctx.OrganizationShortName
			// TODO This is the query we should use
			// getDeplymentStatusParams := astrocore.GetDeploymentsStatusesParams{
			//	DeploymentIds: &deploymentID,
			//	WorkspaceIds:  &ws,
			//	Offset:        nil,
			//	Limit:         &limit,
			//	Sorts:         nil,
			// }
			// resp, err := coreClient.GetDeploymentsStatusesWithResponse(context.Background(), orgShortID, &getDeplymentStatusParams)
			resp, err := coreClient.ListDeploymentsWithResponse(context.Background(), orgShortID, params)
			if err != nil {
				return err
			}
			for _, deployment := range resp.JSON200.Deployments {
				if deployment.Id == deploymentID {
					fmt.Printf("Status ----> %s \n\n", deployment.Status)
					if deployment.Status == "HEALTHY" {
						fmt.Printf("Deployment %s is now healthy\n", deployment.Label)
						return nil
					}
				}
			}
			continue
		}
	}
}

func Update(deploymentID, label, ws, description, deploymentName, dagDeploy, executor string, schedulerAU, schedulerReplicas int, wQueueList []astro.WorkerQueue, forceDeploy bool, client astro.Client) error {
	var queueCreateUpdate, confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// build query input
	scheduler := astro.Scheduler{}

	if schedulerAU != 0 {
		scheduler.AU = schedulerAU
	} else {
		schedulerAU = currentDeployment.DeploymentSpec.Scheduler.AU
		scheduler.AU = currentDeployment.DeploymentSpec.Scheduler.AU
	}

	if schedulerReplicas != 0 {
		scheduler.Replicas = schedulerReplicas
	} else {
		schedulerReplicas = currentDeployment.DeploymentSpec.Scheduler.Replicas
		scheduler.Replicas = currentDeployment.DeploymentSpec.Scheduler.Replicas
	}

	spec := astro.DeploymentCreateSpec{
		Scheduler: scheduler,
		Executor:  currentDeployment.DeploymentSpec.Executor,
	}

	// change the executor if requested
	confirmWithUser, spec = mutateExecutor(executor, spec, len(currentDeployment.WorkerQueues))

	deploymentUpdate := &astro.UpdateDeploymentInput{
		ID:             currentDeployment.ID,
		ClusterID:      currentDeployment.Cluster.ID,
		DeploymentSpec: spec,
	}
	if label != "" {
		deploymentUpdate.Label = label
	} else {
		deploymentUpdate.Label = currentDeployment.Label
	}
	if description != "" {
		deploymentUpdate.Description = description
	} else {
		deploymentUpdate.Description = currentDeployment.Description
	}

	if dagDeploy == "enable" {
		if currentDeployment.DagDeployEnabled {
			fmt.Println("\nDAG deploys are already enabled for this Deployment. Your DAGs will continue to run as scheduled.")
			return nil
		}

		fmt.Printf("\nYou enabled DAG-only deploys for this Deployment. Running tasks are not interrupted but new tasks will not be scheduled." +
			"\nRun `astro deploy --dags` to complete enabling this feature and resume your DAGs. It may take a few minutes for the Airflow UI to update..\n\n")
		deploymentUpdate.DagDeployEnabled = true
	} else if dagDeploy == "disable" {
		if !currentDeployment.DagDeployEnabled {
			fmt.Println("\nDAG-only deploys is already disabled for this deployment.")
			return nil
		}
		if config.CFG.ShowWarnings.GetBool() {
			fmt.Printf("\nWarning: This command will disable DAG-only deploys for this Deployment. Running tasks will not be interrupted, but new tasks will not be scheduled" +
				"\nRun `astro deploy` after this command to restart your DAGs. It may take a few minutes for the Airflow UI to update.")
			confirmWithUser = true
		}
		deploymentUpdate.DagDeployEnabled = false
	}

	// if we have worker queues add them to the input
	if len(wQueueList) > 0 {
		queueCreateUpdate = true
		deploymentUpdate.WorkerQueues = wQueueList
	}
	// validate resources requests
	resourcesValid := validateResources(schedulerAU, schedulerReplicas)
	if !resourcesValid {
		return nil
	}

	// confirm changes with user only if force=false
	if !forceDeploy {
		if confirmWithUser {
			y, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to update the %s Deployment?", ansi.Bold(currentDeployment.Label)))

			if !y {
				fmt.Println("Canceling Deployment update")
				return nil
			}
		}
	}
	// update deployment
	d, err := client.UpdateDeployment(deploymentUpdate)
	if err != nil {
		return err
	}

	if d.ID == "" {
		fmt.Printf("Something went wrong. Deployment %s was not updated", currentDeployment.Label)
	}

	// do not print table if worker queue create or update was used
	if !queueCreateUpdate {
		tabDeployment := newTableOut()

		runtimeVersionText := d.RuntimeRelease.Version + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"

		tabDeployment.AddRow([]string{d.Label, d.ReleaseName, d.Cluster.Name, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled)}, false)
		tabDeployment.SuccessMsg = "\n Successfully updated Deployment"
		tabDeployment.Print(os.Stdout)
	}
	return nil
}

func Delete(deploymentID, ws, deploymentName string, forceDelete bool, client astro.Client) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// prompt user
	if !forceDelete {
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to delete the %s Deployment?", ansi.Bold(currentDeployment.Label)))

		if !i {
			fmt.Println("Canceling deployment deletion")
			return nil
		}
	}

	// delete deployment
	deploymentInput := astro.DeleteDeploymentInput{
		ID: currentDeployment.ID,
	}
	deploymentDeleteResp, err := client.DeleteDeployment(deploymentInput)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	if deploymentDeleteResp.ID == "" {
		fmt.Printf("Something went wrong. Deployment %s not deleted", currentDeployment.Label)
	}

	fmt.Println("\nSuccessfully deleted deployment " + ansi.Bold(currentDeployment.Label))

	return nil
}

var GetDeployments = func(ws string, client astro.Client) ([]astro.Deployment, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return []astro.Deployment{}, err
	}

	deployments, err := client.ListDeployments(c.Organization, ws)
	if err != nil {
		return deployments, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	return deployments, nil
}

var SelectDeployment = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
	// select deployment
	if len(deployments) == 0 {
		return astro.Deployment{}, nil
	}

	if len(deployments) == 1 {
		fmt.Println("Only one Deployment was found. Using the following Deployment by default: \n" +
			fmt.Sprintf("\n Deployment Name: %s", ansi.Bold(deployments[0].Label)) +
			fmt.Sprintf("\n Deployment ID: %s\n", ansi.Bold(deployments[0].ID)))

		return deployments[0], nil
	}

	tab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID"},
	}

	fmt.Println(message)

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})

	deployMap := map[string]astro.Deployment{}
	for i := range deployments {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Label, deployments[i].ReleaseName, deployments[i].ID}, false)

		deployMap[strconv.Itoa(index)] = deployments[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return astro.Deployment{}, ErrInvalidDeploymentKey
	}
	return selected, nil
}

func GetDeployment(ws, deploymentID, deploymentName string, client astro.Client) (astro.Deployment, error) {
	deployments, err := GetDeployments(ws, client)
	if err != nil {
		return astro.Deployment{}, errors.Wrap(err, errInvalidDeployment.Error())
	}

	if deploymentID != "" && deploymentName != "" {
		fmt.Printf("Both a Deployment ID and Deployment name have been supplied. The Deployment ID %s will be used\n", deploymentID)
	}
	// find deployment by name
	if deploymentID == "" && deploymentName != "" {
		var stageDeployments []astro.Deployment
		for i := range deployments {
			if deployments[i].Label == deploymentName {
				stageDeployments = append(stageDeployments, deployments[i])
			}
		}
		if len(stageDeployments) > 1 {
			fmt.Printf("More than one Deployment with the name %s was found\n", deploymentName)
		}
		if len(stageDeployments) == 1 {
			return stageDeployments[0], nil
		}
		if len(stageDeployments) < 1 {
			fmt.Printf("No Deployment with the name %s was found\n", deploymentName)
			return astro.Deployment{}, errInvalidDeployment
		}
	}

	var currentDeployment astro.Deployment

	// select deployment if deploymentID is empty
	if deploymentID == "" {
		return deploymentSelectionProcess(ws, deployments, client)
	}
	// find deployment by ID
	for i := range deployments {
		if deployments[i].ID == deploymentID {
			currentDeployment = deployments[i]
		}
	}
	if currentDeployment.ID == "" {
		return astro.Deployment{}, errInvalidDeployment
	}
	return currentDeployment, nil
}

func deploymentSelectionProcess(ws string, deployments []astro.Deployment, client astro.Client) (astro.Deployment, error) {
	currentDeployment, err := SelectDeployment(deployments, "Select a Deployment")
	if err != nil {
		return astro.Deployment{}, err
	}
	if currentDeployment.ID == "" {
		// get latest runtime version
		airflowVersionClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
		runtimeVersion, err := airflowversions.GetDefaultImageTag(airflowVersionClient, "")
		if err != nil {
			return astro.Deployment{}, err
		}

		// walk user through creating a deployment
		err = createDeployment("", ws, "", "", runtimeVersion, "disable", CeleryExecutor, SchedulerAuMin, SchedulerReplicasMin, client, nil, false)
		if err != nil {
			return astro.Deployment{}, err
		}

		// get a new deployment list
		deployments, err = GetDeployments(ws, client)
		if err != nil {
			return astro.Deployment{}, err
		}
		currentDeployment, err = SelectDeployment(deployments, "Select which Deployment you want to update")
		if err != nil {
			return astro.Deployment{}, err
		}
	}
	return currentDeployment, nil
}

// GetDeploymentURL takes a deploymentID, WorkspaceID as parameters
// and returns a deploymentURL
func GetDeploymentURL(deploymentID, workspaceID string) (string, error) {
	var (
		deploymentURL string
		ctx           config.Context
		err           error
	)

	ctx, err = config.GetCurrentContext()
	if err != nil {
		return "", err
	}
	switch ctx.Domain {
	case domainutil.LocalDomain:
		deploymentURL = ctx.Domain + ":5000/" + workspaceID + "/deployments/" + deploymentID + "/analytics"
	default:
		_, domain := domainutil.GetPRSubDomain(ctx.Domain)
		deploymentURL = "cloud." + domain + "/" + workspaceID + "/deployments/" + deploymentID + "/analytics"
	}
	return deploymentURL, nil
}

// printWarning lets the user know
// when going from CE -> KE and if multiple queues exist,
// a new default queue will get created.
// If going from KE -> CE, it lets user know that a new default worker queue will be created.
// It returns true if a warning was printed and false if not.
func printWarning(executor string, existingQLength int) bool {
	var printed bool
	if executor == KubeExecutor {
		if existingQLength > 1 {
			fmt.Println("\n Switching to KubernetesExecutor will replace all existing worker queues " +
				"with one new default worker queue for this deployment.")
			printed = true
		}
	} else {
		if executor == CeleryExecutor {
			fmt.Println("\n Switching to CeleryExecutor will replace the existing worker queue " +
				"with a new default worker queue for this deployment.")
			printed = true
		}
	}
	return printed
}

// mutateExecutor updates currentSpec.Executor if requestedExecutor is not "" and not the same value.
// It prints a helpful message describing the change and returns a bool used to confirm the change with the user.
// If a helpful message was not printed, it returns false.
func mutateExecutor(requestedExecutor string, currentSpec astro.DeploymentCreateSpec, existingQLength int) (bool, astro.DeploymentCreateSpec) {
	var printed bool
	if requestedExecutor != "" && currentSpec.Executor != requestedExecutor {
		// print helpful message describing the change
		printed = printWarning(requestedExecutor, existingQLength)
		currentSpec.Executor = requestedExecutor
	}
	return printed, currentSpec
}
