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
	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/util"
	"golang.org/x/exp/slices"

	"github.com/pkg/errors"
)

var (
	errInvalidDeployment         = errors.New("the Deployment specified was not found in this workspace. Your account or API Key may not have access to the deployment specified")
	ErrInvalidDeploymentKey      = errors.New("invalid Deployment selected")
	ErrInvalidClusterKey         = errors.New("invalid Cluster selected")
	ErrInvalidRegionKey          = errors.New("invalid Region selected")
	ErrTimedOut                  = errors.New("timed out waiting for the Deployment to enter a Healthy state")
	ErrWrongEnforceInput         = errors.New("the input to the `--enforce-cicd` flag is invalid. Make sure to use either 'enable' or 'disable'")
	ErrInvalidResourceRequest    = errors.New("invalid resource request")
	ErrNotADevelopmentDeployment = errors.New("the Deployment specified is not a development Deployment")
	ErrInvalidTokenName          = errors.New("no name provided for the deployment token. Retry with a valid name")

	// Monkey patched to write unit tests
	createDeployment = Create
	canCiCdDeploy    = CanCiCdDeploy
	parseToken       = util.ParseAPIToken
	CleanOutput      = false
)

const (
	NoDeploymentInWSMsg = "no Deployments found in workspace"
	noWorkspaceMsg      = "no Workspace with id (%s) found"
	KubeExecutor        = "KubernetesExecutor"
	CeleryExecutor      = "CeleryExecutor"
	AstroExecutor       = "AstroExecutor"
	KUBERNETES          = "KUBERNETES"
	CELERY              = "CELERY"
	ASTRO               = "ASTRO"
	notApplicable       = "N/A"
	gcpCloud            = "gcp"
	awsCloud            = "aws"
	azureCloud          = "azure"
	standard            = "standard"
	disable             = "disable"
	enable              = "enable"
)

var (
	sleepTime        = 180
	tickNum          = 10
	timeoutNum       = 180
	listLimit        = 1000
	dagDeployEnabled bool
)

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "NAMESPACE", "CLUSTER", "CLOUD PROVIDER", "REGION", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED", "CI-CD ENFORCEMENT", "DEPLOYMENT TYPE"},
	}
}

func newTableOutAll() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE", "NAMESPACE", "CLUSTER", "CLOUD PROVIDER", "REGION", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED", "CI-CD ENFORCEMENT", "DEPLOYMENT TYPE"},
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
func List(ws string, fromAllWorkspaces bool, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	tab := newTableOut()

	if fromAllWorkspaces {
		ws = ""
		tab = newTableOutAll()
	}
	deployments, err := CoreGetDeployments(ws, c.Organization, platformCoreClient)
	if err != nil {
		return err
	}

	if len(deployments) == 0 {
		fmt.Printf("%s %s\n", NoDeploymentInWSMsg, ansi.Bold(ws))
		return nil
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Name > deployments[j].Name })

	for i := range deployments {
		deploymentToTableRow(tab, &deployments[i], fromAllWorkspaces)
	}
	tab.Print(out)
	return nil
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Logs(deploymentID, ws, deploymentName, keyword string, logWebserver, logScheduler, logTriggerer, logWorkers, warnLogs, errorLogs, infoLogs bool, logCount int, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
	var logLevel string
	var i int
	// log level
	if warnLogs {
		logLevel = "WARN"
		i++
	}
	if errorLogs {
		logLevel = "ERROR"
		i++
	}
	if infoLogs {
		logLevel = "INFO"
		i++
	}
	if keyword != "" {
		logLevel = keyword
		i++
	}
	if i > 1 {
		return errors.New("cannot query for more than one log level and/or keyword at a time")
	}

	// get deployment
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, nil)
	if err != nil {
		return err
	}

	// Check if deployment is using Airflow 3
	if err := airflowversions.ValidateNoAirflow3Support(deployment.RuntimeVersion); err != nil {
		return err
	}

	deploymentID = deployment.Id
	timeRange := 86400
	offset := 0

	// get log source
	var componentSources []astrocore.GetDeploymentLogsParamsSources
	if logWebserver {
		componentSources = append(componentSources, "webserver")
	}
	if logScheduler {
		componentSources = append(componentSources, "scheduler")
	}
	if logTriggerer {
		componentSources = append(componentSources, "triggerer")
	}
	if logWorkers {
		componentSources = append(componentSources, "worker")
	}
	if len(componentSources) == 0 {
		componentSources = append(componentSources, "webserver", "scheduler", "triggerer", "worker")
	}

	getDeploymentLogsParams := astrocore.GetDeploymentLogsParams{
		Sources:       componentSources,
		MaxNumResults: &logCount,
		Range:         &timeRange,
		Offset:        &offset,
	}
	if logLevel != "" {
		getDeploymentLogsParams.SearchText = &logLevel
	}

	deploymentLogs, err := GetDeploymentLogs("", deploymentID, getDeploymentLogsParams, coreClient)
	if err != nil {
		return err
	}

	if len(deploymentLogs.Results) == 0 {
		fmt.Println("No matching logs have been recorded in the past 24 hours for Deployment " + deployment.Name)
		return nil
	}
	for i := range deploymentLogs.Results {
		fmt.Printf("%f %s %s\n", deploymentLogs.Results[i].Timestamp, deploymentLogs.Results[i].Raw, deploymentLogs.Results[i].Source)
	}
	return nil
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Create(name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool) error { //nolint
	var organizationID string
	var currentWorkspace astrocore.Workspace

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	coreDeploymentType := astrocore.GetDeploymentOptionsParamsDeploymentType(deploymentType)
	coreCloudProvider := GetCoreCloudProvider(cloudProvider)
	deploymentOptionsParams := astrocore.GetDeploymentOptionsParams{
		DeploymentType: &coreDeploymentType,
		CloudProvider:  &coreCloudProvider,
	}

	configOption, err := GetDeploymentOptions("", deploymentOptionsParams, coreClient)
	if err != nil {
		return err
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

	isCicdEnforced := false
	if cicdEnforcement == enable {
		isCicdEnforced = true
	}

	if organizationID == "" {
		return fmt.Errorf(noWorkspaceMsg, workspaceID)
	}
	fmt.Printf("Current Workspace: %s\n\n", currentWorkspace.Name)

	// name input
	if name == "" {
		fmt.Println("Please specify a name for your Deployment")
		name = input.Text(ansi.Bold("\nDeployment name: "))
		if name == "" {
			return errors.New("you must give your Deployment a name")
		}
	}

	if region == "" && IsDeploymentStandard(deploymentType) {
		// select and validate region
		region, err = selectRegion(cloudProvider, region, coreClient)
		if err != nil {
			return err
		}
	}

	// select and validate cluster
	if !IsDeploymentStandard(deploymentType) {
		clusterID, err = selectCluster(clusterID, c.Organization, platformCoreClient)
		if err != nil {
			return err
		}
	}

	if dagDeploy == enable { //nolint: goconst
		dagDeployEnabled = true
	} else {
		dagDeployEnabled = false
	}
	createDeploymentRequest := astroplatformcore.CreateDeploymentRequest{}
	if IsDeploymentStandard(deploymentType) || IsDeploymentDedicated(deploymentType) {
		var highAvailabilityValue bool
		if highAvailability == enable { //nolint: goconst
			highAvailabilityValue = true
		} else {
			highAvailabilityValue = false
		}
		var developmentModeValue bool
		if developmentMode == enable { //nolint: goconst
			developmentModeValue = true
		} else {
			developmentModeValue = false
		}
		var workerConcurrency int
		for i := range configOption.WorkerMachines {
			if strings.EqualFold(configOption.DefaultValues.WorkerMachineName, string(configOption.WorkerMachines[i].Name)) {
				workerConcurrency = int(configOption.WorkerMachines[i].Concurrency.Default)
			}
		}
		defautWorkerQueue := []astroplatformcore.WorkerQueueRequest{{
			AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(configOption.DefaultValues.WorkerMachineName),
			IsDefault:         true,
			MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
			MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
			Name:              "default",
			WorkerConcurrency: workerConcurrency,
		}}
		if defaultTaskPodCpu == "" {
			defaultTaskPodCpu = configOption.ResourceQuotas.DefaultPodSize.Cpu.Default
		}
		if defaultTaskPodMemory == "" {
			defaultTaskPodMemory = configOption.ResourceQuotas.DefaultPodSize.Memory.Default
		}
		if resourceQuotaCpu == "" {
			resourceQuotaCpu = configOption.ResourceQuotas.ResourceQuota.Cpu.Default
		}
		if resourceQuotaMemory == "" {
			resourceQuotaMemory = configOption.ResourceQuotas.ResourceQuota.Memory.Default
		}
		var deplWorkloadIdentity *string
		if workloadIdentity != "" {
			deplWorkloadIdentity = &workloadIdentity
		}
		// build standard input
		if IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astroplatformcore.CreateStandardDeploymentRequestCloudProvider
			switch cloudProvider {
			case gcpCloud:
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderGCP
			case awsCloud:
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAWS
			case azureCloud:
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAZURE
			}
			var requestedExecutor astroplatformcore.CreateStandardDeploymentRequestExecutor
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorCELERY
			}
			if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorKUBERNETES
			}
			if strings.EqualFold(executor, AstroExecutor) || strings.EqualFold(executor, ASTRO) {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorASTRO
			}

			standardDeploymentRequest := astroplatformcore.CreateStandardDeploymentRequest{
				AstroRuntimeVersion:  runtimeVersion,
				CloudProvider:        &requestedCloudProvider,
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				Region:               &region,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) || strings.EqualFold(executor, AstroExecutor) || strings.EqualFold(executor, ASTRO) {
				standardDeploymentRequest.WorkerQueues = &defautWorkerQueue
			}
			switch schedulerSize {
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSize(configOption.DefaultValues.SchedulerSize)
			}
			if highAvailability == enable { //nolint: goconst
				standardDeploymentRequest.IsHighAvailability = true
			}
			err := createDeploymentRequest.FromCreateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		// build dedicated input
		if IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astroplatformcore.CreateDedicatedDeploymentRequestExecutor
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorCELERY
			}
			if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			if strings.EqualFold(executor, AstroExecutor) || strings.EqualFold(executor, ASTRO) {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorASTRO
			}
			dedicatedDeploymentRequest := astroplatformcore.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  runtimeVersion,
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				ClusterId:            clusterID,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
				dedicatedDeploymentRequest.WorkerQueues = &defautWorkerQueue
			}
			switch schedulerSize {
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSize(configOption.DefaultValues.SchedulerSize)
			}
			err := createDeploymentRequest.FromCreateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
	}
	// build hybrid input
	if IsDeploymentHybrid(deploymentType) {
		cluster, err := CoreGetCluster("", clusterID, platformCoreClient)
		if err != nil {
			return err
		}
		nodePoolID := getDefaultNodePoolID(cluster.NodePools)
		defautWorkerQueue := []astroplatformcore.HybridWorkerQueueRequest{{
			IsDefault:         true,
			MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
			MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
			Name:              "default",
			WorkerConcurrency: int(configOption.WorkerQueues.WorkerConcurrency.Default),
			NodePoolId:        nodePoolID,
		}}
		if schedulerAU == 0 {
			schedulerAU = int(configOption.LegacyAstro.SchedulerAstroUnitRange.Default)
		}
		if schedulerReplicas == 0 {
			schedulerReplicas = int(configOption.LegacyAstro.SchedulerReplicaRange.Default)
		}

		// validate hybrid resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return ErrInvalidResourceRequest
		}
		var requestedExecutor astroplatformcore.CreateHybridDeploymentRequestExecutor
		if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
			requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorCELERY
		}
		if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
			requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorKUBERNETES
		}
		hybridDeploymentRequest := astroplatformcore.CreateHybridDeploymentRequest{
			AstroRuntimeVersion: runtimeVersion,
			Description:         &description,
			Name:                name,
			Executor:            requestedExecutor,
			IsCicdEnforced:      isCicdEnforced,
			IsDagDeployEnabled:  dagDeployEnabled,
			ClusterId:           clusterID,
			WorkloadIdentity:    &workloadIdentity,
			WorkspaceId:         workspaceID,

			Scheduler: astroplatformcore.DeploymentInstanceSpecRequest{
				Au:       schedulerAU,
				Replicas: schedulerReplicas,
			},
			Type: astroplatformcore.CreateHybridDeploymentRequestTypeHYBRID,
		}
		// TODO (https://github.com/astronomer/astro-cli/issues/1853) when we add remote execution we'll need to omit worker queues if remote execution is enabled
		if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
			hybridDeploymentRequest.WorkerQueues = &defautWorkerQueue
		} else {
			hybridDeploymentRequest.TaskPodNodePoolId = &nodePoolID
		}
		err = createDeploymentRequest.FromCreateHybridDeploymentRequest(hybridDeploymentRequest)
		if err != nil {
			return err
		}
	}

	d, err := CoreCreateDeployment(organizationID, createDeploymentRequest, platformCoreClient)
	if err != nil {
		return err
	}

	if waitForStatus {
		err = HealthPoll(d.Id, workspaceID, sleepTime, tickNum, timeoutNum, platformCoreClient)
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

func getDefaultNodePoolID(nodePools *[]astroplatformcore.NodePool) string {
	if nodePools != nil && len(*nodePools) > 0 {
		return (*nodePools)[0].Id
	}
	return ""
}

func createOutput(workspaceID string, d *astroplatformcore.Deployment) error {
	tab := newTableOut()
	deploymentToTableRow(tab, d, false)

	deploymentURL, err := GetDeploymentURL(d.Id, workspaceID)
	if err != nil {
		return err
	}
	tab.SuccessMsg = fmt.Sprintf("\n Successfully created Deployment: %s", ansi.Bold(d.Name)) +
		"\n Deployment can be accessed at the following URLs \n" +
		fmt.Sprintf("\n Deployment Dashboard: %s", ansi.Bold(deploymentURL)) +
		fmt.Sprintf("\n Airflow Dashboard: %s", ansi.Bold(d.WebServerUrl))
	tab.Print(os.Stdout)
	return nil
}

func deploymentToTableRow(table *printutil.Table, d *astroplatformcore.Deployment, includeWorkspaceName bool) {
	runtimeVersionText := d.RuntimeVersion + " (based on Airflow " + d.AirflowVersion + ")"
	cloudProvider := notApplicable
	clusterName := notApplicable
	region := notApplicable
	if d.CloudProvider != nil {
		cloudProvider = string(*d.CloudProvider)
	}
	if d.Region != nil {
		region = *d.Region
	}
	releaseName := d.Namespace
	if !IsDeploymentStandard(*d.Type) {
		clusterName = *d.ClusterName
	}
	cols := []string{
		d.Name,
		releaseName,
		clusterName,
		cloudProvider,
		region,
		d.Id,
		runtimeVersionText,
		strconv.FormatBool(d.IsDagDeployEnabled),
		strconv.FormatBool(d.IsCicdEnforced),
		string(*d.Type),
	}
	if includeWorkspaceName {
		cols = slices.Insert(cols, 1, *d.WorkspaceName)
	}
	table.AddRow(cols, false)
}

func validateHybridResources(schedulerAU, schedulerReplicas int, configOption astrocore.DeploymentOptions) bool { //nolint:gocritic
	schedulerAuMin := int(configOption.LegacyAstro.SchedulerAstroUnitRange.Floor)
	schedulerAuMax := int(configOption.LegacyAstro.SchedulerAstroUnitRange.Ceiling)
	schedulerReplicasMin := int(configOption.LegacyAstro.SchedulerReplicaRange.Floor)
	schedulerReplicasMax := int(configOption.LegacyAstro.SchedulerReplicaRange.Ceiling)
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

func ListClusterOptions(cloudProvider string, coreClient astrocore.CoreClient) ([]astrocore.ClusterOptions, error) {
	var provider astrocore.GetClusterOptionsParamsProvider
	if cloudProvider == gcpCloud {
		provider = astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderGcp) //nolint
	}
	if cloudProvider == awsCloud {
		provider = astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderAws) //nolint
	}
	if cloudProvider == azureCloud {
		provider = astrocore.GetClusterOptionsParamsProvider(astrocore.GetClusterOptionsParamsProviderAzure) //nolint
	}
	optionsParams := &astrocore.GetClusterOptionsParams{
		Provider: &provider,
		Type:     astrocore.GetClusterOptionsParamsType(astrocore.GetClusterOptionsParamsTypeSHARED), //nolint
	}
	clusterOptions, err := coreClient.GetClusterOptionsWithResponse(context.Background(), optionsParams)
	if err != nil {
		return nil, err
	}
	err = astroplatformcore.NormalizeAPIError(clusterOptions.HTTPResponse, clusterOptions.Body)
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

func selectCluster(clusterID, organizationID string, platformCoreClient astroplatformcore.CoreClient) (newClusterID string, err error) {
	clusterTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
	}

	cs, err := organization.ListClusters(organizationID, platformCoreClient)
	if err != nil {
		return "", err
	}
	// select cluster
	if clusterID == "" {
		fmt.Println("\nPlease select a Cluster for your Deployment:")

		clusterMap := map[string]astroplatformcore.Cluster{}
		for i := range cs {
			index := i + 1
			clusterTab.AddRow([]string{strconv.Itoa(index), cs[i].Name, string(cs[i].CloudProvider), cs[i].Id}, false)

			clusterMap[strconv.Itoa(index)] = cs[i]
		}

		clusterTab.Print(os.Stdout)
		choice := input.Text("\n> ")
		selected, ok := clusterMap[choice]
		if !ok {
			return "", ErrInvalidClusterKey
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

func HealthPoll(deploymentID, ws string, sleepTime, tickNum, timeoutNum int, platformCoreClient astroplatformcore.CoreClient) error {
	fmt.Printf("\nWaiting for the deployment to become healthy…\n\nThis may take a few minutes\n")
	time.Sleep(time.Duration(sleepTime) * time.Second)
	buf := new(bytes.Buffer)
	timeout := time.After(time.Duration(timeoutNum) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Second)
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return ErrTimedOut
		// Got a tick, we should check if deployment is healthy
		case <-ticker.C:
			buf.Reset()
			// get core deployment
			currentDeployment, err := CoreGetDeployment("", deploymentID, platformCoreClient)
			if err != nil {
				return err
			}

			if currentDeployment.Status == astroplatformcore.DeploymentStatusHEALTHY {
				fmt.Printf("Deployment %s is now healthy\n", currentDeployment.Name)
				return nil
			}
			continue
		}
	}
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Update(deploymentID, name, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, schedulerAU, schedulerReplicas int, wQueueList []astroplatformcore.WorkerQueueRequest, hybridQueueList []astroplatformcore.HybridWorkerQueueRequest, newEnvironmentVariables []astroplatformcore.DeploymentEnvironmentVariableRequest, force bool, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient) error {
	var queueCreateUpdate, confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, platformCoreClient, coreClient)
	if err != nil {
		return err
	}

	// Check if deployment is using Airflow 3
	if err := airflowversions.ValidateNoAirflow3Support(currentDeployment.RuntimeVersion); err != nil {
		return err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	var isCicdEnforced bool
	if cicdEnforcement == "" {
		isCicdEnforced = currentDeployment.IsCicdEnforced
	}
	if cicdEnforcement == disable {
		isCicdEnforced = false
	}
	if cicdEnforcement == enable {
		isCicdEnforced = true
	}
	if isCicdEnforced && dagDeploy != "" {
		if !canCiCdDeploy(c.Token) {
			fmt.Printf("\nWarning: You are trying to update the dag deploy setting with ci-cd enforcement enabled. Once the setting is updated, you will not be able to deploy your dags using the CLI. Until you deploy your dags, dags will not be visible in the UI nor will new tasks start." +
				"\nAfter the setting is updated, either disable cicd enforcement and then deploy your dags OR deploy your dags via CICD or using API Tokens.")
			y, _ := input.Confirm("\n\nAre you sure you want to continue?")

			if !y {
				fmt.Println("Canceling Deployment update")
				return nil
			}
		}
	}
	// determine isDagDeployEnabled
	switch dagDeploy {
	case enable:
		if currentDeployment.IsDagDeployEnabled {
			fmt.Println("\nDAG deploys are already enabled for this Deployment. Your DAGs will continue to run as scheduled.")
			return nil
		}

		fmt.Printf("\nYou enabled DAG-only deploys for this Deployment. Running tasks are not interrupted but new tasks will not be scheduled." +
			"\nRun `astro deploy --dags` to complete enabling this feature and resume your DAGs. It may take a few minutes for the Airflow UI to update..\n\n")
		dagDeployEnabled = true
	case disable:
		if !currentDeployment.IsDagDeployEnabled {
			fmt.Println("\nDAG-only deploys is already disabled for this deployment.")
			return nil
		}
		if config.CFG.ShowWarnings.GetBool() {
			fmt.Printf("\nWarning: This command will disable DAG-only deploys for this Deployment. Running tasks will not be interrupted, but new tasks will not be scheduled" +
				"\nRun `astro deploy` after this command to restart your DAGs. It may take a few minutes for the Airflow UI to update.")
			confirmWithUser = true
		}
		dagDeployEnabled = false
	case "":
		dagDeployEnabled = currentDeployment.IsDagDeployEnabled
	}
	configOption, err := GetDeploymentOptions("", astrocore.GetDeploymentOptionsParams{}, coreClient)
	if err != nil {
		return err
	}

	// build query input
	updateDeploymentRequest := astroplatformcore.UpdateDeploymentRequest{}

	if name == "" {
		name = currentDeployment.Name
	}
	if description == "" {
		if currentDeployment.Description != nil {
			description = *currentDeployment.Description
		}
	}
	var deploymentEnvironmentVariablesRequest []astroplatformcore.DeploymentEnvironmentVariableRequest
	if len(newEnvironmentVariables) == 0 {
		if currentDeployment.EnvironmentVariables != nil {
			environmentVariables := *currentDeployment.EnvironmentVariables
			for i := range *currentDeployment.EnvironmentVariables {
				var deploymentEnvironmentVariableRequest astroplatformcore.DeploymentEnvironmentVariableRequest
				deploymentEnvironmentVariableRequest.IsSecret = environmentVariables[i].IsSecret
				deploymentEnvironmentVariableRequest.Key = environmentVariables[i].Key
				deploymentEnvironmentVariableRequest.Value = environmentVariables[i].Value
				deploymentEnvironmentVariablesRequest = append(deploymentEnvironmentVariablesRequest, deploymentEnvironmentVariableRequest)
			}
		} else {
			deploymentEnvironmentVariablesRequest = newEnvironmentVariables
		}
	} else {
		queueCreateUpdate = true
		deploymentEnvironmentVariablesRequest = newEnvironmentVariables
	}
	if deploymentEnvironmentVariablesRequest == nil {
		deploymentEnvironmentVariablesRequest = []astroplatformcore.DeploymentEnvironmentVariableRequest{}
	}
	if IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type) {
		var workerQueuesRequest []astroplatformcore.WorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = ConvertWorkerQueues(workerQueues, func(q astroplatformcore.WorkerQueue) astroplatformcore.WorkerQueueRequest {
				return astroplatformcore.WorkerQueueRequest{
					AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(*q.AstroMachine),
					Id:                &q.Id,
					IsDefault:         q.IsDefault,
					MaxWorkerCount:    q.MaxWorkerCount,
					MinWorkerCount:    q.MinWorkerCount,
					Name:              q.Name,
					WorkerConcurrency: q.WorkerConcurrency,
				}
			})
		}

		var workerConcurrency int
		for i := range configOption.WorkerMachines {
			if strings.EqualFold(configOption.DefaultValues.WorkerMachineName, string(configOption.WorkerMachines[i].Name)) {
				workerConcurrency = int(configOption.WorkerMachines[i].Concurrency.Default)
			}
		}
		defaultWorkerQueue := []astroplatformcore.WorkerQueueRequest{{
			AstroMachine:      astroplatformcore.WorkerQueueRequestAstroMachine(configOption.DefaultValues.WorkerMachineName),
			IsDefault:         true,
			MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
			MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
			Name:              "default",
			WorkerConcurrency: workerConcurrency,
		}}

		if len(wQueueList) > 0 {
			queueCreateUpdate = true
			workerQueuesRequest = wQueueList
		}
		var highAvailabilityValue bool
		switch highAvailability {
		case "":
			highAvailabilityValue = *currentDeployment.IsHighAvailability
		case enable:
			highAvailabilityValue = true
		case disable:
			highAvailabilityValue = false
		default:
			return errors.New("Invalid --high-availability value")
		}
		var developmentModeValue bool
		switch developmentMode {
		case "":
			developmentModeValue = *currentDeployment.IsDevelopmentMode
		case enable:
			developmentModeValue = true
		case disable:
			developmentModeValue = false
		default:
			return errors.New("Invalid --development-mode value")
		}
		if defaultTaskPodCpu == "" {
			if currentDeployment.DefaultTaskPodCpu != nil {
				defaultTaskPodCpu = *currentDeployment.DefaultTaskPodCpu
			} else {
				defaultTaskPodCpu = configOption.ResourceQuotas.DefaultPodSize.Cpu.Default
			}
		}
		if defaultTaskPodMemory == "" {
			if currentDeployment.DefaultTaskPodMemory != nil {
				defaultTaskPodMemory = *currentDeployment.DefaultTaskPodMemory
			} else {
				defaultTaskPodMemory = configOption.ResourceQuotas.DefaultPodSize.Memory.Default
			}
		}
		if resourceQuotaCpu == "" {
			resourceQuotaCpu = *currentDeployment.ResourceQuotaCpu
		}
		if resourceQuotaMemory == "" {
			resourceQuotaMemory = *currentDeployment.ResourceQuotaMemory
		}
		var deplWorkloadIdentity *string
		if workloadIdentity != "" {
			deplWorkloadIdentity = &workloadIdentity
		}
		if IsDeploymentStandard(*currentDeployment.Type) {
			var requestedExecutor astroplatformcore.UpdateStandardDeploymentRequestExecutor
			switch strings.ToUpper(executor) {
			case "":
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutor(*currentDeployment.Executor)
			case strings.ToUpper(CeleryExecutor):
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(KubeExecutor):
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(CELERY):
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(KUBERNETES):
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutorKUBERNETES
			}

			standardDeploymentRequest := astroplatformcore.UpdateStandardDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astroplatformcore.UpdateStandardDeploymentRequestTypeSTANDARD,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			switch schedulerSize {
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}

			// confirm with user if the executor is changing
			if *currentDeployment.Executor != astroplatformcore.DeploymentExecutor(standardDeploymentRequest.Executor) {
				confirmWithUser = true
			}

			switch standardDeploymentRequest.Executor {
			case astroplatformcore.UpdateStandardDeploymentRequestExecutorCELERY:
				standardDeploymentRequest.WorkerQueues = fillWorkerQueues(
					astroplatformcore.DeploymentExecutor(standardDeploymentRequest.Executor),
					standardDeploymentRequest.WorkerQueues,
					&defaultWorkerQueue,
				)
			case astroplatformcore.UpdateStandardDeploymentRequestExecutorASTRO:
				standardDeploymentRequest.WorkerQueues = fillWorkerQueues(
					astroplatformcore.DeploymentExecutor(standardDeploymentRequest.Executor),
					standardDeploymentRequest.WorkerQueues,
					&defaultWorkerQueue,
				)
			case astroplatformcore.UpdateStandardDeploymentRequestExecutorKUBERNETES:
				standardDeploymentRequest.WorkerQueues = nil
			}

			err := updateDeploymentRequest.FromUpdateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if IsDeploymentDedicated(*currentDeployment.Type) {
			var requestedExecutor astroplatformcore.UpdateDedicatedDeploymentRequestExecutor
			switch strings.ToUpper(executor) {
			case "":
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutor(*currentDeployment.Executor)
			case strings.ToUpper(CeleryExecutor):
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(KubeExecutor):
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(CELERY):
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(KUBERNETES):
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			dedicatedDeploymentRequest := astroplatformcore.UpdateDedicatedDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astroplatformcore.UpdateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				WorkerQueues:         &workerQueuesRequest,
				WorkloadIdentity:     deplWorkloadIdentity,
			}
			switch schedulerSize {
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrocore.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}
			// confirm with user if the executor is changing
			if *currentDeployment.Executor != astroplatformcore.DeploymentExecutor(dedicatedDeploymentRequest.Executor) {
				confirmWithUser = true
			}
			if dedicatedDeploymentRequest.Executor == astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES {
				// For KUBERNETES executor, WorkerQueues should be nil
				dedicatedDeploymentRequest.WorkerQueues = nil
			} else {
				dedicatedDeploymentRequest.WorkerQueues = fillWorkerQueues(
					astroplatformcore.DeploymentExecutor(dedicatedDeploymentRequest.Executor),
					&workerQueuesRequest,
					&defaultWorkerQueue,
				)
			}
			err := updateDeploymentRequest.FromUpdateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
	}
	if !(IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type)) {
		var workerQueuesRequest []astroplatformcore.HybridWorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = ConvertWorkerQueues(workerQueues, func(q astroplatformcore.WorkerQueue) astroplatformcore.HybridWorkerQueueRequest {
				return astroplatformcore.HybridWorkerQueueRequest{
					Id:                &q.Id,
					IsDefault:         q.IsDefault,
					MaxWorkerCount:    q.MaxWorkerCount,
					MinWorkerCount:    q.MinWorkerCount,
					Name:              q.Name,
					WorkerConcurrency: q.WorkerConcurrency,
					NodePoolId:        *q.NodePoolId,
				}
			})
		}

		if len(hybridQueueList) > 0 {
			queueCreateUpdate = true
			workerQueuesRequest = hybridQueueList
		}
		if schedulerAU == 0 {
			schedulerAU = *currentDeployment.SchedulerAu
		}
		if schedulerReplicas == 0 {
			schedulerReplicas = currentDeployment.SchedulerReplicas
		}
		if workloadIdentity == "" {
			if currentDeployment.WorkloadIdentity != nil {
				workloadIdentity = *currentDeployment.WorkloadIdentity
			}
		}
		// validate au resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return ErrInvalidResourceRequest
		}
		var requestedExecutor astroplatformcore.UpdateHybridDeploymentRequestExecutor
		switch strings.ToUpper(executor) {
		case "":
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutor(*currentDeployment.Executor)
		case strings.ToUpper(CeleryExecutor):
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorCELERY
		case strings.ToUpper(KubeExecutor):
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES
		case strings.ToUpper(CELERY):
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorCELERY
		case strings.ToUpper(KUBERNETES):
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES
		}
		hybridDeploymentRequest := astroplatformcore.UpdateHybridDeploymentRequest{
			Description:        &description,
			Name:               name,
			Executor:           requestedExecutor,
			IsCicdEnforced:     isCicdEnforced,
			IsDagDeployEnabled: dagDeployEnabled,
			WorkspaceId:        currentDeployment.WorkspaceId,
			Scheduler: astroplatformcore.DeploymentInstanceSpecRequest{
				Au:       schedulerAU,
				Replicas: schedulerReplicas,
			},
			Type:                 astroplatformcore.UpdateHybridDeploymentRequestTypeHYBRID,
			EnvironmentVariables: deploymentEnvironmentVariablesRequest,
			WorkloadIdentity:     &workloadIdentity,
		}
		cluster, err := CoreGetCluster("", *currentDeployment.ClusterId, platformCoreClient)
		if err != nil {
			return err
		}
		nodePoolID := getDefaultNodePoolID(cluster.NodePools)
		// confirm with user if the executor is changing
		if *currentDeployment.Executor != astroplatformcore.DeploymentExecutor(hybridDeploymentRequest.Executor) {
			confirmWithUser = true
		}
		if hybridDeploymentRequest.Executor == astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES {
			// For KUBERNETES executor, WorkerQueues should be nil
			hybridDeploymentRequest.WorkerQueues = nil
			if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorCELERY {
				confirmWithUser = true
				hybridDeploymentRequest.TaskPodNodePoolId = &nodePoolID
			} else {
				hybridDeploymentRequest.TaskPodNodePoolId = currentDeployment.TaskPodNodePoolId
			}
		} else {
			if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorKUBERNETES && workerQueuesRequest == nil {
				workerQueuesRequest = []astroplatformcore.HybridWorkerQueueRequest{{
					IsDefault:         true,
					MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
					MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
					Name:              "default",
					WorkerConcurrency: int(configOption.WorkerQueues.WorkerConcurrency.Default),
					NodePoolId:        nodePoolID,
				}}
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
			} else {
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
			}
			if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorKUBERNETES {
				confirmWithUser = true
			}
		}
		err = updateDeploymentRequest.FromUpdateHybridDeploymentRequest(hybridDeploymentRequest)
		if err != nil {
			return err
		}
	}

	// confirm changes with user only if force=false
	if !force {
		if confirmWithUser {
			y, _ := input.Confirm(
				fmt.Sprintf("\nAre you sure you want to update the %s Deployment?", ansi.Bold(currentDeployment.Name)))

			if !y {
				fmt.Println("Canceling Deployment update")
				return nil
			}
		}
	}
	// update deployment
	d, err := CoreUpdateDeployment(c.Organization, currentDeployment.Id, updateDeploymentRequest, platformCoreClient)
	if err != nil {
		return err
	}
	if d.Id == "" {
		fmt.Printf("Something went wrong. Deployment %s was not updated", currentDeployment.Name)
	}

	// do not print table if worker queue create or update was used
	if !queueCreateUpdate {
		tabDeployment := newTableOut()

		runtimeVersionText := d.RuntimeVersion + " (based on Airflow " + d.AirflowVersion + ")"
		releaseName := d.Namespace
		clusterName := notApplicable
		cloudProvider := notApplicable
		region := notApplicable
		if d.CloudProvider != nil {
			cloudProvider = string(*d.CloudProvider)
		}
		if d.Region != nil {
			region = *d.Region
		}
		if !IsDeploymentStandard(*d.Type) {
			clusterName = *d.ClusterName
		}
		tabDeployment.AddRow([]string{d.Name, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.IsDagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type)}, false)
		tabDeployment.SuccessMsg = "\n Successfully updated Deployment"
		tabDeployment.Print(os.Stdout)
	}
	return nil
}

func fillWorkerQueues(executor astroplatformcore.DeploymentExecutor, workerQueuesRequest, defautWorkerQueue *[]astroplatformcore.WorkerQueueRequest) *[]astroplatformcore.WorkerQueueRequest {
	if executor == astroplatformcore.DeploymentExecutorKUBERNETES && workerQueuesRequest == nil {
		return defautWorkerQueue
	} else {
		return workerQueuesRequest
	}
}

func Delete(deploymentID, ws, deploymentName string, forceDelete bool, platformCoreClient astroplatformcore.CoreClient) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, nil, platformCoreClient, nil)
	if err != nil {
		return err
	}

	if currentDeployment.Id == "" {
		fmt.Printf("%s %s to delete\n", NoDeploymentInWSMsg, ansi.Bold(ws))
		return nil
	}

	// prompt user
	if !forceDelete {
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to delete the %s Deployment?", ansi.Bold(currentDeployment.Name)))

		if !i {
			fmt.Println("Canceling deployment deletion")
			return nil
		}
	}

	err = CoreDeleteDeployment(currentDeployment.OrganizationId, currentDeployment.Id, platformCoreClient)
	if err != nil {
		return err
	}

	fmt.Println("\nSuccessfully deleted deployment " + ansi.Bold(currentDeployment.Name))

	return nil
}

func UpdateDeploymentHibernationOverride(deploymentID, ws, deploymentName string, isHibernating bool, overrideUntil *time.Time, force bool, platformCoreClient astroplatformcore.CoreClient) error {
	// Set wording based on the hibernation action
	var action string
	if isHibernating {
		action = "hibernate"
	} else {
		action = "wake up"
	}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, isDevelopmentDeployment, platformCoreClient, nil)
	if err != nil {
		return err
	}
	if currentDeployment.Id == "" {
		fmt.Printf("%s %s to %s\n", NoDeploymentInWSMsg, ansi.Bold(ws), action)
		return nil
	}

	if currentDeployment.IsDevelopmentMode == nil || !*currentDeployment.IsDevelopmentMode {
		return ErrNotADevelopmentDeployment
	}

	// prompt user
	if !force {
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to override to %s for %s Deployment?", ansi.Bold(action), ansi.Bold(currentDeployment.Name)))

		if !i {
			fmt.Printf("\nCanceling %s override", action)
			return nil
		}
	}

	overrideDeploymentHibernationBody := astroplatformcore.OverrideDeploymentHibernationBody{
		IsHibernating: isHibernating,
		OverrideUntil: overrideUntil,
	}

	deploymentHibernationOverride, err := CoreUpdateDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, overrideDeploymentHibernationBody, platformCoreClient)
	if err != nil {
		return err
	}

	if deploymentHibernationOverride.OverrideUntil != nil {
		fmt.Printf("\nSuccessfully overrode to %s until %s\n", ansi.Bold(action), ansi.Bold(deploymentHibernationOverride.OverrideUntil.Format(time.RFC3339)))
		fmt.Printf("If set, hibernation schedule will resume in %s.\n", ansi.Bold(time.Until(*deploymentHibernationOverride.OverrideUntil).Round(time.Second).String()))
	} else {
		fmt.Printf("\nSuccessfully overrode to %s until further notice\n", ansi.Bold(action))
		fmt.Println("Any configured hibernation schedules will not resume until override is removed.")
	}

	return nil
}

func DeleteDeploymentHibernationOverride(deploymentID, ws, deploymentName string, force bool, platformCoreClient astroplatformcore.CoreClient) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, isDevelopmentDeployment, platformCoreClient, nil)
	if err != nil {
		return err
	}
	if currentDeployment.Id == "" {
		fmt.Printf("No Deployments with a hibernation override that can be removed found in Workspace %s\n", ansi.Bold(ws))
		return nil
	}

	if currentDeployment.IsDevelopmentMode == nil || !*currentDeployment.IsDevelopmentMode {
		return ErrNotADevelopmentDeployment
	}

	// prompt user
	if !force {
		i, _ := input.Confirm(
			fmt.Sprintf("\nAre you sure you want to remove the hibernation override and resume schedule for %s Deployment?", ansi.Bold(currentDeployment.Name)))

		if !i {
			fmt.Println("Canceling hibernation override removal")
			return nil
		}
	}

	err = CoreDeleteDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, platformCoreClient)
	if err != nil {
		return err
	}

	fmt.Println("\nSuccessfully removed hibernation override")
	fmt.Println("If set, hibernation schedule will resume immediately.")

	return nil
}

func IsDeploymentStandard(deploymentType astroplatformcore.DeploymentType) bool {
	return deploymentType == astroplatformcore.DeploymentTypeSTANDARD
}

func IsDeploymentDedicated(deploymentType astroplatformcore.DeploymentType) bool {
	return deploymentType == astroplatformcore.DeploymentTypeDEDICATED
}

func IsDeploymentHybrid(deploymentType astroplatformcore.DeploymentType) bool {
	return deploymentType == astroplatformcore.DeploymentTypeHYBRID
}

// CoreUpdateDeploymentHibernationOverride updates a deployment hibernation override with the core API
//
//nolint:dupl
var CoreUpdateDeploymentHibernationOverride = func(orgID, deploymentID string, overrideDeploymentHibernationRequest astroplatformcore.OverrideDeploymentHibernationBody, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.DeploymentHibernationOverride, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.DeploymentHibernationOverride{}, err
		}
		orgID = c.Organization
	}
	resp, err := platformCoreClient.UpdateDeploymentHibernationOverrideWithResponse(context.Background(), orgID, deploymentID, overrideDeploymentHibernationRequest)
	if err != nil {
		return astroplatformcore.DeploymentHibernationOverride{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.DeploymentHibernationOverride{}, err
	}
	deploymentHibernationOverride := *resp.JSON200
	return deploymentHibernationOverride, nil
}

// CoreDeleteDeploymentHibernationOverride deletes a deployment hibernation override with the core API
//
//nolint:dupl
var CoreDeleteDeploymentHibernationOverride = func(orgID, deploymentID string, platformCoreClient astroplatformcore.CoreClient) error {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
	}
	resp, err := platformCoreClient.DeleteDeploymentHibernationOverrideWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

var CoreGetDeployments = func(ws, orgID string, platformCoreClient astroplatformcore.CoreClient) ([]astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return []astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}
	deploymentListParams := &astroplatformcore.ListDeploymentsParams{
		Limit: &listLimit,
	}
	if ws != "" {
		deploymentListParams.WorkspaceIds = &[]string{ws}
	}

	resp, err := platformCoreClient.ListDeploymentsWithResponse(context.Background(), orgID, deploymentListParams)
	if err != nil {
		return []astroplatformcore.Deployment{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astroplatformcore.Deployment{}, err
	}

	deploymentResponse := *resp.JSON200
	deployments := deploymentResponse.Deployments

	return deployments, nil
}

//nolint:dupl
var CoreGetCluster = func(orgID, clusterID string, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.Cluster, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Cluster{}, err
		}
		orgID = c.Organization
	}

	resp, err := platformCoreClient.GetClusterWithResponse(context.Background(), orgID, clusterID)
	if err != nil {
		return astroplatformcore.Cluster{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.Cluster{}, err
	}

	cluster := *resp.JSON200

	return cluster, nil
}

//nolint:dupl
var CoreGetDeployment = func(orgID, deploymentID string, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := platformCoreClient.GetDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}

	deployment := *resp.JSON200

	return deployment, nil
}

// CoreCreateDeployment creates a deployment with the core API
var CoreCreateDeployment = func(orgID string, createDeploymentRequest astroplatformcore.CreateDeploymentJSONRequestBody, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := platformCoreClient.CreateDeploymentWithResponse(context.Background(), orgID, createDeploymentRequest)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}

	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	deployment := *resp.JSON200

	return deployment, nil
}

// CoreUpdateDeployment updates a deployment with the core API
//
//nolint:dupl
var CoreUpdateDeployment = func(orgID, deploymentID string, updateDeploymentRequest astroplatformcore.UpdateDeploymentJSONRequestBody, platformCoreClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}
	resp, err := platformCoreClient.UpdateDeploymentWithResponse(context.Background(), orgID, deploymentID, updateDeploymentRequest)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	deployment := *resp.JSON200

	return deployment, nil
}

// CoreDeleteDeployment deletes a deployment with the core API
var CoreDeleteDeployment = func(orgID, deploymentID string, platformCoreClient astroplatformcore.CoreClient) error {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
	}

	resp, err := platformCoreClient.DeleteDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}

	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

//nolint:dupl
var GetDeploymentOptions = func(orgID string, deploymentOptionsParams astrocore.GetDeploymentOptionsParams, coreClient astrocore.CoreClient) (astrocore.DeploymentOptions, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrocore.DeploymentOptions{}, err
		}
		orgID = c.Organization
	}
	resp, err := coreClient.GetDeploymentOptionsWithResponse(context.Background(), orgID, &deploymentOptionsParams)
	if err != nil {
		return astrocore.DeploymentOptions{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrocore.DeploymentOptions{}, err
	}
	DeploymentOptions := *resp.JSON200

	return DeploymentOptions, nil
}

//nolint:dupl
func GetPlatformDeploymentOptions(orgID string, deploymentOptionsParams astroplatformcore.GetDeploymentOptionsParams, coreClient astroplatformcore.CoreClient) (astroplatformcore.DeploymentOptions, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.DeploymentOptions{}, err
		}
		orgID = c.Organization
	}

	resp, err := coreClient.GetDeploymentOptionsWithResponse(context.Background(), orgID, &deploymentOptionsParams)
	if err != nil {
		return astroplatformcore.DeploymentOptions{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astroplatformcore.DeploymentOptions{}, err
	}
	DeploymentOptions := *resp.JSON200

	return DeploymentOptions, nil
}

var GetDeploymentLogs = func(orgID string, deploymentID string, getDeploymentLogsParams astrocore.GetDeploymentLogsParams, coreClient astrocore.CoreClient) (astrocore.DeploymentLog, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrocore.DeploymentLog{}, err
		}
		orgID = c.Organization
	}

	resp, err := coreClient.GetDeploymentLogsWithResponse(context.Background(), orgID, deploymentID, &getDeploymentLogsParams)
	if err != nil {
		return astrocore.DeploymentLog{}, err
	}
	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrocore.DeploymentLog{}, err
	}
	DeploymentLog := *resp.JSON200

	return DeploymentLog, nil
}

var SelectDeployment = func(deployments []astroplatformcore.Deployment, message string) (astroplatformcore.Deployment, error) {
	// select deployment
	if len(deployments) == 0 {
		return astroplatformcore.Deployment{}, nil
	}

	if len(deployments) == 1 && (os.Getenv("ASTRO_API_TOKEN") != "" || os.Getenv("ASTRONOMER_KEY_ID") != "" || os.Getenv("ASTRONOMER_KEY_SECRET") != "" || config.CFG.AutoSelect.GetBool()) {
		if !CleanOutput {
			fmt.Println("Only one Deployment was found. Using the following Deployment by default: \n" +
				fmt.Sprintf("\n Deployment Name: %s", ansi.Bold(deployments[0].Name)) +
				fmt.Sprintf("\n Deployment ID: %s\n", ansi.Bold(deployments[0].Id)))
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

	deployMap := map[string]astroplatformcore.Deployment{}
	for i := range deployments {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Name, deployments[i].Namespace, deployments[i].Id, strconv.FormatBool(deployments[i].IsDagDeployEnabled)}, false)

		deployMap[strconv.Itoa(index)] = deployments[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return astroplatformcore.Deployment{}, ErrInvalidDeploymentKey
	}
	return selected, nil
}

func GetDeployment(ws, deploymentID, deploymentName string, disableCreateFlow bool, selectionFilter func(deployment astroplatformcore.Deployment) bool, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) (astroplatformcore.Deployment, error) { //nolint:gocognit
	deployments, err := CoreGetDeployments(ws, "", platformCoreClient)
	if err != nil {
		return astroplatformcore.Deployment{}, errors.Wrap(err, errInvalidDeployment.Error())
	}

	if len(deployments) == 0 && disableCreateFlow {
		return astroplatformcore.Deployment{}, nil
	}

	if deploymentID != "" && deploymentName != "" && !CleanOutput {
		fmt.Printf("Both a Deployment ID and Deployment name have been supplied. The Deployment ID %s will be used\n", deploymentID)
	}
	// find deployment by name
	if deploymentID == "" && deploymentName != "" {
		var stageDeployments []astroplatformcore.Deployment
		for i := range deployments {
			if deployments[i].Name == deploymentName {
				stageDeployments = append(stageDeployments, deployments[i])
			}
		}
		if len(stageDeployments) > 1 {
			fmt.Printf("More than one Deployment with the name %s was found\n", deploymentName)
		}
		if len(stageDeployments) < 1 {
			fmt.Printf("No Deployment with the name %s was found\n", deploymentName)
			return astroplatformcore.Deployment{}, errInvalidDeployment
		}
		currentDeployment, err := CoreGetDeployment("", stageDeployments[0].Id, platformCoreClient)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		return currentDeployment, nil
	}

	var currentDeployment astroplatformcore.Deployment

	// select deployment if deploymentID is empty
	if deploymentID == "" {
		currentDeployment, err = deploymentSelectionProcess(ws, deployments, selectionFilter, platformCoreClient, coreClient, disableCreateFlow)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
	}
	// find deployment by ID
	for i := range deployments {
		if deployments[i].Id == deploymentID {
			currentDeployment = deployments[i]
		}
	}
	if currentDeployment.Id == "" {
		return astroplatformcore.Deployment{}, errInvalidDeployment
	}

	currentDeployment, err = CoreGetDeployment("", currentDeployment.Id, platformCoreClient)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	return currentDeployment, nil
}

func deploymentSelectionProcess(ws string, deployments []astroplatformcore.Deployment, deploymentFilter func(deployment astroplatformcore.Deployment) bool, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, disableCreateFlow bool) (astroplatformcore.Deployment, error) {
	// filter deployments
	if deploymentFilter != nil {
		deployments = util.Filter(deployments, deploymentFilter)
	}
	if len(deployments) == 0 && disableCreateFlow {
		return astroplatformcore.Deployment{}, fmt.Errorf("%s %s", NoDeploymentInWSMsg, ws)
	}
	currentDeployment, err := SelectDeployment(deployments, "Select a Deployment")
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	if currentDeployment.Id == "" {
		// get latest runtime version
		airflowVersionClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
		runtimeVersion, err := airflowversions.GetDefaultImageTag(airflowVersionClient, "", false)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		cicdEnforcement := disable
		// walk user through creating a deployment
		var coreDeploymentType astroplatformcore.DeploymentType
		var dagDeploy string
		if !organization.IsOrgHosted() {
			coreDeploymentType = astroplatformcore.DeploymentTypeHYBRID
			dagDeploy = disable
		} else {
			coreDeploymentType = astroplatformcore.DeploymentTypeSTANDARD
			dagDeploy = enable
		}

		err = createDeployment("", ws, "", "", runtimeVersion, dagDeploy, CeleryExecutor, "azure", "", "", "", "disable", cicdEnforcement, "", "", "", "", "", coreDeploymentType, 0, 0, platformCoreClient, coreClient, false)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		// get a new deployment list
		deployments, err = CoreGetDeployments(ws, "", platformCoreClient)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		currentDeployment, err = SelectDeployment(deployments, "Select which Deployment you want to update")
		if err != nil {
			return astroplatformcore.Deployment{}, err
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
	if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
		if existingQLength > 1 {
			fmt.Println("\n Switching to KubernetesExecutor will replace all existing worker queues " +
				"with one new default worker queue for this deployment.")
			printed = true
		}
	} else {
		if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
			fmt.Println("\n Switching to CeleryExecutor will replace the existing worker queue " +
				"with a new default worker queue for this deployment.")
			printed = true
		}
	}
	return printed
}

func GetCoreCloudProvider(cloudProvider string) astrocore.GetDeploymentOptionsParamsCloudProvider {
	var coreCloudProvider astrocore.GetDeploymentOptionsParamsCloudProvider
	switch strings.ToUpper(cloudProvider) {
	case strings.ToUpper(awsCloud):
		coreCloudProvider = astrocore.GetDeploymentOptionsParamsCloudProvider("AWS")
	case strings.ToUpper(gcpCloud):
		coreCloudProvider = astrocore.GetDeploymentOptionsParamsCloudProvider("GCP")
	case strings.ToUpper(azureCloud):
		coreCloudProvider = astrocore.GetDeploymentOptionsParamsCloudProvider("AZURE")
	}
	return coreCloudProvider
}

func isDevelopmentDeployment(deployment astroplatformcore.Deployment) bool { //nolint:gocritic
	return deployment.IsDevelopmentMode != nil && *deployment.IsDevelopmentMode
}

// ConvertWorkerQueues is a generic function to convert a slice of WorkerQueue to a slice of T (WorkerQueueRequest or HybridWorkerQueueRequest)
func ConvertWorkerQueues[T any](workerQueues []astroplatformcore.WorkerQueue, convertFn func(astroplatformcore.WorkerQueue) T) []T {
	result := make([]T, 0, len(workerQueues))
	for i := range workerQueues {
		result = append(result, convertFn(workerQueues[i]))
	}
	return result
}

func IsValidExecutor(executor, runtimeVersion string) bool {
	validExecutors := []string{
		KubeExecutor,
		CeleryExecutor,
		CELERY,
		KUBERNETES,
	}
	if airflowversions.IsAirflow3(runtimeVersion) {
		validExecutors = append(validExecutors, AstroExecutor, ASTRO)
	}
	for _, e := range validExecutors {
		if strings.EqualFold(executor, e) {
			return true
		}
	}
	return false
}
