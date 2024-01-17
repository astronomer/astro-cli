package deployment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	astro "github.com/astronomer/astro-cli/astro-client"
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
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
	ErrInvalidRegionKey     = errors.New("invalid Region selected")
	errTimedOut             = errors.New("timed out waiting for the deployment to become healthy")
	ErrWrongEnforceInput    = errors.New("the input to the `--enforce-cicd` flag")
	ErrNoDeploymentExists   = errors.New("no deployment was found in this workspace")
	// Monkey patched to write unit tests
	createDeployment = Create
	canCiCdDeploy    = CanCiCdDeploy
	parseToken       = util.ParseAPIToken
	CleanOutput      = false
)

const (
	noWorkspaceMsg = "no workspaces with id (%s) found"
	KubeExecutor   = "KubernetesExecutor"
	CeleryExecutor = "CeleryExecutor"
	notApplicable  = "N/A"
	gcpCloud       = "gcp"
	awsCloud       = "aws"
	standard       = "standard"
)

var (
	sleepTime  = 180
	tickNum    = 10
	timeoutNum = 180
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "NAMESPACE", "CLUSTER", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED", "CI-CD ENFORCEMENT"},
	}
}

func newTableOutAll() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE", "NAMESPACE", "CLUSTER", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED", "CI-CD ENFORCEMENT"},
	}
}

func CanCiCdDeploy(bearerToken string) bool {
	token := strings.Split(bearerToken, " ")[1] // Stripping Bearer
	// Parse the token to peek at the custom claims
	claims, err := parseToken(token)
	if err != nil {
		fmt.Println("Unable to Parse Token")
		return false
	}

	// Only API Tokens and API Keys have permissions
	if len(claims.Permissions) > 0 {
		return true
	}

	return false
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
	deployments, err := GetDeployments(ws, c.Organization, client)
	if err != nil {
		return errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	if len(deployments) == 0 {
		fmt.Printf("No Deployments found in workspace %s\n", ansi.Bold(ws))
		return nil
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })

	// Build rows
	for i := range deployments {
		d := deployments[i]
		clusterName := d.Cluster.Name
		runtimeVersionText := d.RuntimeRelease.Version + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"
		releaseName := d.ReleaseName
		if organization.IsOrgHosted() {
			if IsDeploymentHosted(d.Type) {
				clusterName = d.Cluster.Region
			}
			releaseName = notApplicable
		}

		if all {
			tab.AddRow([]string{d.Label, d.Workspace.Label, releaseName, clusterName, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.APIKeyOnlyDeployments)}, false)
		} else {
			tab.AddRow([]string{d.Label, releaseName, clusterName, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.APIKeyOnlyDeployments)}, false)
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
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, client, nil)
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

func Create(label, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, clusterType string, schedulerAU, schedulerReplicas int, client astro.Client, coreClient astrocore.CoreClient, waitForStatus bool, enforceCD *bool) error { //nolint
	var organizationID string
	var currentWorkspace astrocore.Workspace
	var dagDeployEnabled bool

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	configOption, err := client.GetDeploymentConfig()
	if err != nil {
		return err
	}

	if schedulerAU == 0 {
		schedulerAU = configOption.Components.Scheduler.AU.Default
	}

	if schedulerReplicas == 0 {
		schedulerReplicas = configOption.Components.Scheduler.Replicas.Default
	}

	if schedulerSize == "" {
		schedulerSize = configOption.DefaultSchedulerSize.Size
	}

	// validate resources requests
	resourcesValid := validateResources(schedulerAU, schedulerReplicas, configOption)
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
	ws, err := workspace.GetWorkspaces(coreClient)
	if err != nil {
		return err
	}

	for i := range ws {
		if workspaceID == ws[i].Id {
			organizationID = ws[i].OrganizationId
			currentWorkspace = ws[i]
		}
	}

	if organizationID == "" {
		return fmt.Errorf(noWorkspaceMsg, workspaceID) //nolint:goerr113
	}
	fmt.Printf("Current Workspace: %s\n\n", currentWorkspace.Name)

	// label input
	if label == "" {
		fmt.Println("Please specify a name for your Deployment")
		label = input.Text(ansi.Bold("\nDeployment name: "))
		if label == "" {
			return errors.New("you must give your Deployment a name")
		}
		deployments, err := GetDeployments(workspaceID, organizationID, client)
		if err != nil {
			return errors.Wrap(err, errInvalidDeployment.Error())
		}

		for i := range deployments {
			if deployments[i].Label == label {
				return errors.New("A Deployment with that name already exists")
			}
		}
	}

	if region == "" && organization.IsOrgHosted() && clusterType == standard {
		// select and validate region
		region, err = selectRegion(cloudProvider, region, coreClient)
		if err != nil {
			return err
		}
	}

	// select and validate cluster
	clusterID, err = useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, c.Organization, clusterID, clusterType, coreClient)
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
		APIKeyOnlyDeployments: *enforceCD,
	}

	if organization.IsOrgHosted() {
		createInput.SchedulerSize = schedulerSize
		createInput.IsHighAvailability = false
		if highAvailability == "enable" { //nolint: goconst
			createInput.IsHighAvailability = true
		}
	}

	// Create request
	d, err := client.CreateDeployment(createInput)
	if err != nil {
		return err
	}

	if waitForStatus {
		err = HealthPoll(d.ID, workspaceID, sleepTime, tickNum, timeoutNum, coreClient)
		if err != nil {
			errOutput := createOutput(workspaceID, clusterType, &d)
			if errOutput != nil {
				return errOutput
			}
			return err
		}
	}

	err = createOutput(workspaceID, clusterType, &d)
	if err != nil {
		return err
	}

	return nil
}

func createOutput(workspaceID, clusterType string, d *astro.Deployment) error {
	tab := newTableOut()

	runtimeVersionText := d.RuntimeRelease.Version + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"
	clusterName := d.Cluster.Name
	releaseName := d.ReleaseName
	if organization.IsOrgHosted() {
		if clusterType == standard {
			clusterName = d.Cluster.Region
		}
		releaseName = notApplicable
	}
	tab.AddRow([]string{d.Label, releaseName, clusterName, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.APIKeyOnlyDeployments)}, false)
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

func validateResources(schedulerAU, schedulerReplicas int, configOption astro.DeploymentConfig) bool { //nolint:gocritic
	schedulerAuMin := configOption.Components.Scheduler.AU.Default
	schedulerAuMax := configOption.Components.Scheduler.AU.Limit
	schedulerReplicasMin := configOption.Components.Scheduler.Replicas.Minimum
	schedulerReplicasMax := configOption.Components.Scheduler.Replicas.Limit
	if schedulerAU > schedulerAuMax || schedulerAU < schedulerAuMin {
		fmt.Printf("\nScheduler AUs must be between a min of %d and a max of %d AUs\n", schedulerAuMin, schedulerAuMax)
		return false
	}
	if schedulerReplicas > schedulerReplicasMax || schedulerReplicas < schedulerReplicasMin {
		fmt.Printf("\nScheduler Replicas must be between a min of %d and a max of %d Replicas\n", schedulerReplicasMin, schedulerReplicasMax)
		return false
	}
	return true
}

func validateRuntimeVersion(runtimeVersion string, client astro.Client) (bool, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return false, err
	}

	runtimeReleases, err := GetRuntimeReleases(c.Organization, client)
	if err != nil {
		return false, err
	}
	if !util.Contains(runtimeReleases, runtimeVersion) {
		fmt.Printf("\nRuntime version not valid. Must be one of the following: %v\n", runtimeReleases)
		return false, nil
	}
	return true, nil
}

func GetRuntimeReleases(organizationID string, client astro.Client) ([]string, error) {
	// get deployment config options
	runtimeReleases := []string{}

	ConfigOptions, err := client.GetDeploymentConfigWithOrganization(organizationID)
	if err != nil {
		return runtimeReleases, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	for i := range ConfigOptions.RuntimeReleases {
		runtimeReleases = append(runtimeReleases, ConfigOptions.RuntimeReleases[i].Version)
	}

	return runtimeReleases, nil
}

func ListClusterOptions(cloudProvider string, coreClient astrocore.CoreClient) ([]astrocore.ClusterOptions, error) {
	var provider astrocore.GetClusterOptionsParamsProvider
	if cloudProvider == gcpCloud {
		provider = astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
	}
	if cloudProvider == awsCloud {
		provider = astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderAws) //nolint
	}
	optionsParams := &astrocore.GetClusterOptionsParams{
		Provider: &provider,
		Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
	}
	clusterOptions, err := coreClient.GetClusterOptionsWithResponse(context.Background(), optionsParams)
	if err != nil {
		return nil, err
	}
	err = astrocore.NormalizeAPIError(clusterOptions.HTTPResponse, clusterOptions.Body)
	if err != nil {
		return nil, err
	}
	options := *clusterOptions.JSON200

	return options, nil
}

func selectRegion(cloudProvider, region string, coreClient astrocore.CoreClient) (newRegion string, err error) {
	regionsTab := printutil.Table{
		Padding:        []int{5, 30},
		DynamicPadding: true,
		Header:         []string{"#", "REGION"},
	}

	// get all regions for the cloud provider
	options, err := ListClusterOptions(cloudProvider, coreClient)
	if err != nil {
		return "", err
	}
	regions := options[0].Regions

	if region == "" {
		fmt.Println("\nPlease select a Region for your Deployment:")

		regionMap := map[string]astrocore.ProviderRegion{}

		for i := range regions {
			index := i + 1
			regionsTab.AddRow([]string{strconv.Itoa(index), regions[i].Name}, false)

			regionMap[strconv.Itoa(index)] = regions[i]
		}

		regionsTab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := regionMap[choice]
		if !ok {
			return "", ErrInvalidRegionKey
		}

		region = selected.Name
	}

	// validate region
	reg := ""
	for i := range regions {
		if region == regions[i].Name {
			reg = regions[i].Name
		}
	}
	if reg == "" {
		return "", errors.New("unable to find specified Region")
	}
	return region, nil
}

func selectCluster(clusterID, org string, coreClient astrocore.CoreClient) (newClusterID string, err error) {
	clusterTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
	}

	cs, err := organization.ListClusters(org, coreClient)
	if err != nil {
		return "", err
	}
	// select cluster
	if clusterID == "" {
		fmt.Println("\nPlease select a Cluster for your Deployment:")

		clusterMap := map[string]astrocore.Cluster{}
		for i := range cs {
			index := i + 1
			clusterTab.AddRow([]string{strconv.Itoa(index), cs[i].Name, string(cs[i].CloudProvider), cs[i].Id}, false)

			clusterMap[strconv.Itoa(index)] = cs[i]
		}

		clusterTab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := clusterMap[choice]
		if !ok {
			return "", ErrInvalidDeploymentKey
		}

		clusterID = selected.Id
	}

	// validate cluster
	csID := ""
	for i := range cs {
		if clusterID == cs[i].Id {
			csID = cs[i].Id
		}
	}
	if csID == "" {
		return "", errors.New("unable to find specified Cluster")
	}
	return clusterID, nil
}

// useSharedCluster takes astrocore.SharedClusterCloudProvider and a region string as input.
// It returns the clusterID of the cluster if one exists in the provider/region.
// It returns a 404 if a cluster does not exist.
func useSharedCluster(cloudProvider astrocore.SharedClusterCloudProvider, region string, coreClient astrocore.CoreClient) (clusterID string, err error) {
	// get shared cluster request
	getSharedClusterParams := astrocore.GetSharedClusterParams{
		Region:        region,
		CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(cloudProvider),
	}
	response, err := coreClient.GetSharedClusterWithResponse(context.Background(), &getSharedClusterParams)
	if err != nil {
		return "", err
	}
	err = astrocore.NormalizeAPIError(response.HTTPResponse, response.Body)
	if err != nil {
		return "", err
	}
	return response.JSON200.Id, nil
}

// useSharedClusterOrSelectDedicatedCluster decides how to derive the clusterID to use for a deployment.
// if cloudProvider and region are provided, it uses a useSharedCluster to get the ClusterID.
// if not, it uses selectCluster to get the ClusterID.
func useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, org, clusterID, clusterType string, coreClient astrocore.CoreClient) (derivedClusterID string, err error) {
	// if cloud provider and region are requested
	if clusterType == standard && cloudProvider != "" && region != "" {
		// use a shared cluster for the deployment
		derivedClusterID, err = useSharedCluster(astrocore.SharedClusterCloudProvider(cloudProvider), region, coreClient)
		if err != nil {
			return "", err
		}
	} else {
		// select and validate cluster
		derivedClusterID, err = selectCluster(clusterID, org, coreClient)
		if err != nil {
			return "", err
		}
	}
	return derivedClusterID, nil
}

func HealthPoll(deploymentID, ws string, sleepTime, tickNum, timeoutNum int, coreClient astrocore.CoreClient) error {
	fmt.Printf("\nWaiting for the deployment to become healthy…\n\nThis may take a few minutes\n")
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
			// get core deployment
			currentDeployment, err := CoreGetDeployment(ws, "", deploymentID, coreClient)
			if err != nil {
				return err
			}

			if currentDeployment.Status == "HEALTHY" {
				fmt.Printf("Deployment %s is now healthy\n", currentDeployment.Name)
				return nil
			}
			continue
		}
	}
}

func Update(deploymentID, label, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability string, schedulerAU, schedulerReplicas int, wQueueList []astro.WorkerQueue, forceDeploy bool, enforceCD *bool, client astro.Client) error { //nolint
	var queueCreateUpdate, confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, client, nil)
	if err != nil {
		return err
	}

	configOption, err := client.GetDeploymentConfig()
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

	if enforceCD == nil {
		deploymentUpdate.APIKeyOnlyDeployments = currentDeployment.APIKeyOnlyDeployments
	} else {
		deploymentUpdate.APIKeyOnlyDeployments = *enforceCD
	}

	if organization.IsOrgHosted() {
		if schedulerSize != "" {
			deploymentUpdate.SchedulerSize = schedulerSize
		} else {
			deploymentUpdate.SchedulerSize = currentDeployment.SchedulerSize
		}
		if highAvailability == "enable" {
			deploymentUpdate.IsHighAvailability = true
		} else if highAvailability == "disable" { //nolint
			deploymentUpdate.IsHighAvailability = false
		}
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	if deploymentUpdate.APIKeyOnlyDeployments && dagDeploy != "" {
		if !canCiCdDeploy(c.Token) {
			fmt.Printf("\nWarning: You are trying to update the dag deploy setting with ci-cd enforcement enabled. Once the setting is updated, you will not be able to deploy your dags using the CLI. Until you deploy your dags, dags will not be visible in the UI nor will new tasks start." +
				"\nAfter the setting is updated, either disable cicd enforcement and then deploy your dags OR deploy your dags via CICD or using API Keys/Token.")
			y, _ := input.Confirm("\n\nAre you sure you want to continue?")

			if !y {
				fmt.Println("Canceling Deployment update")
				return nil
			}
		}
	}

	// determine dagDeploy enabled/disabled
	if dagDeploy == "" {
		deploymentUpdate.DagDeployEnabled = currentDeployment.DagDeployEnabled
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

	if !(IsDeploymentHosted(currentDeployment.Type) || IsDeploymentDedicated(currentDeployment.Type)) {
		// validate au resources requests
		resourcesValid := validateResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return nil
		}
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
		clusterName := d.Cluster.Name
		releaseName := d.ReleaseName
		if organization.IsOrgHosted() {
			if IsDeploymentHosted(d.Type) {
				clusterName = d.Cluster.Region
			}
			releaseName = notApplicable
		}
		tabDeployment.AddRow([]string{d.Label, releaseName, clusterName, d.ID, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.APIKeyOnlyDeployments)}, false)
		tabDeployment.SuccessMsg = "\n Successfully updated Deployment"
		tabDeployment.Print(os.Stdout)
	}
	return nil
}

func Delete(deploymentID, ws, deploymentName string, forceDelete bool, client astro.Client) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, client, nil)
	if err != nil {
		return err
	}

	if currentDeployment.ID == "" {
		fmt.Printf("No Deployments found in workspace %s to delete\n", ansi.Bold(ws))
		return nil
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

func IsDeploymentHosted(deploymentType string) bool {
	return deploymentType == "HOSTED_SHARED"
}

func IsDeploymentDedicated(deploymentType string) bool {
	return deploymentType == "HOSTED_DEDICATED"
}

var GetDeployments = func(ws, org string, client astro.Client) ([]astro.Deployment, error) {
	if org == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return []astro.Deployment{}, err
		}
		org = c.Organization
	}

	deployments, err := client.ListDeployments(org, ws)
	if err != nil {
		return deployments, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
	}

	return deployments, nil
}

var CoreGetDeployment = func(ws, org, deploymentId string, coreClient astrocore.CoreClient) (astrocore.Deployment, error) {
	if org == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrocore.Deployment{}, err
		}
		org = c.Organization
	}

	depIds := []string{deploymentId}
	deploymentListParams := &astrocore.ListDeploymentsParams{
		DeploymentIds: &depIds,
	}
	resp, err := coreClient.ListDeploymentsWithResponse(context.Background(), org, deploymentListParams)
	if err != nil {
		return astrocore.Deployment{}, err
	}

	deploymentResponse := *resp.JSON200
	deployments := deploymentResponse.Deployments

	if len(deployments) == 0 {
		return astrocore.Deployment{}, ErrNoDeploymentExists
	}
	return deployments[0], nil
}

var SelectDeployment = func(deployments []astro.Deployment, message string) (astro.Deployment, error) {
	// select deployment
	if len(deployments) == 0 {
		return astro.Deployment{}, nil
	}

	if len(deployments) == 1 {
		if !CleanOutput {
			fmt.Println("Only one Deployment was found. Using the following Deployment by default: \n" +
				fmt.Sprintf("\n Deployment Name: %s", ansi.Bold(deployments[0].Label)) +
				fmt.Sprintf("\n Deployment ID: %s\n", ansi.Bold(deployments[0].ID)))
		}

		return deployments[0], nil
	}

	tab := printutil.Table{
		Padding:        []int{5, 30, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID", "DAG DEPLOY ENABLED"},
	}

	fmt.Println(message)

	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})

	deployMap := map[string]astro.Deployment{}
	for i := range deployments {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Label, deployments[i].ReleaseName, deployments[i].ID, strconv.FormatBool(deployments[i].DagDeployEnabled)}, false)

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

func GetDeployment(ws, deploymentID, deploymentName string, disableCreateFlow bool, client astro.Client, coreClient astrocore.CoreClient) (astro.Deployment, error) {
	deployments, err := GetDeployments(ws, "", client)
	if err != nil {
		return astro.Deployment{}, errors.Wrap(err, errInvalidDeployment.Error())
	}

	if len(deployments) == 0 && disableCreateFlow {
		return astro.Deployment{}, nil
	}

	if deploymentID != "" && deploymentName != "" && !CleanOutput {
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
		return deploymentSelectionProcess(ws, deployments, client, coreClient)
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

func deploymentSelectionProcess(ws string, deployments []astro.Deployment, client astro.Client, coreClient astrocore.CoreClient) (astro.Deployment, error) {
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

		configOption, err := client.GetDeploymentConfig()
		if err != nil {
			return astro.Deployment{}, err
		}

		schedulerAU := configOption.Components.Scheduler.AU.Default
		schedulerReplicas := configOption.Components.Scheduler.Replicas.Default
		cicdEnforcement := false
		// walk user through creating a deployment
		err = createDeployment("", ws, "", "", runtimeVersion, "disable", CeleryExecutor, "", "", "medium", "", "", schedulerAU, schedulerReplicas, client, coreClient, false, &cicdEnforcement)
		if err != nil {
			return astro.Deployment{}, err
		}

		// get a new deployment list
		deployments, err = GetDeployments(ws, "", client)
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
		deploymentURL = ctx.Domain + ":5000/" + workspaceID + "/deployments/" + deploymentID + "/overview"
	default:
		_, domain := domainutil.GetPRSubDomain(ctx.Domain)
		deploymentURL = "cloud." + domain + "/" + workspaceID + "/deployments/" + deploymentID + "/overview"
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
