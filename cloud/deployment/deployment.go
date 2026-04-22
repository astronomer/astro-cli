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

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/organization"
	"github.com/astronomer/astro-cli/cloud/workspace"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/httputil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/util"
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
	SleepTime        = 180
	TickNum          = 10
	listLimit        = 1000
	dagDeployEnabled bool
)

// defaultWorkerMachineName is the machine type UpdateDeployment falls back to when
// an executor change forces it to synthesize a replacement worker queue.
const defaultWorkerMachineName = "A5"

func newTableOut() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 50, 10, 50, 10, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "NAMESPACE", "CLUSTER", "CLOUD PROVIDER", "REGION", "DEPLOYMENT ID", "RUNTIME VERSION", "DAG DEPLOY ENABLED", "CI-CD ENFORCEMENT", "DEPLOYMENT TYPE", "REMOTE EXECUTION"},
	}
}

// orNA returns notApplicable if s is empty, for table display.
func orNA(s string) string {
	if s == "" {
		return notApplicable
	}
	return s
}

func deploymentTableConfig(fromAllWorkspaces bool, ws string) *output.TableConfig {
	columns := []output.Column[DeploymentInfo]{
		{Header: "NAME", Value: func(d DeploymentInfo) string { return d.Name }},
	}
	if fromAllWorkspaces {
		columns = append(columns, output.Column[DeploymentInfo]{
			Header: "WORKSPACE", Value: func(d DeploymentInfo) string { return d.WorkspaceName },
		})
	}
	columns = append(columns,
		output.Column[DeploymentInfo]{Header: "NAMESPACE", Value: func(d DeploymentInfo) string { return d.Namespace }},
		output.Column[DeploymentInfo]{Header: "CLUSTER", Value: func(d DeploymentInfo) string { return orNA(d.ClusterName) }},
		output.Column[DeploymentInfo]{Header: "CLOUD PROVIDER", Value: func(d DeploymentInfo) string { return orNA(d.CloudProvider) }},
		output.Column[DeploymentInfo]{Header: "REGION", Value: func(d DeploymentInfo) string { return orNA(d.Region) }},
		output.Column[DeploymentInfo]{Header: "DEPLOYMENT ID", Value: func(d DeploymentInfo) string { return d.DeploymentID }},
		output.Column[DeploymentInfo]{Header: "RUNTIME VERSION", Value: func(d DeploymentInfo) string {
			return d.RuntimeVersion + " (based on Airflow " + d.AirflowVersion + ")"
		}},
		output.Column[DeploymentInfo]{Header: "DAG DEPLOY ENABLED", Value: func(d DeploymentInfo) string { return strconv.FormatBool(d.IsDagDeployEnabled) }},
		output.Column[DeploymentInfo]{Header: "CI-CD ENFORCEMENT", Value: func(d DeploymentInfo) string { return strconv.FormatBool(d.IsCicdEnforced) }},
		output.Column[DeploymentInfo]{Header: "DEPLOYMENT TYPE", Value: func(d DeploymentInfo) string { return d.Type }},
		output.Column[DeploymentInfo]{Header: "REMOTE EXECUTION", Value: func(d DeploymentInfo) string { return strconv.FormatBool(d.IsRemoteExecutionEnabled) }},
	)

	noResultsMsg := ""
	if !fromAllWorkspaces && ws != "" {
		noResultsMsg = NoDeploymentInWSMsg + " " + ansi.Bold(ws)
	}

	return output.BuildTableConfig(
		columns,
		func(d any) []DeploymentInfo { return d.(*DeploymentList).Deployments },
		output.WithPadding([]int{30, 50, 10, 50, 10, 10, 10}),
		output.WithNoResultsMsg(noResultsMsg),
	)
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

// deploymentToInfo converts a deployment to DeploymentInfo for structured output
func deploymentToInfo(d *astrov1.Deployment, includeWorkspaceName bool) DeploymentInfo {
	var cloudProvider, clusterName, region, workspaceName, deploymentType string

	if d.CloudProvider != nil {
		cloudProvider = string(*d.CloudProvider)
	}
	if d.Region != nil {
		region = *d.Region
	}
	if d.Type != nil {
		deploymentType = string(*d.Type)
		if !IsDeploymentStandard(*d.Type) && d.ClusterName != nil {
			clusterName = *d.ClusterName
		}
	}
	if includeWorkspaceName && d.WorkspaceName != nil {
		workspaceName = *d.WorkspaceName
	}

	isRemoteExecutionEnabled := IsRemoteExecutionEnabled(d)

	return DeploymentInfo{
		Name:                     d.Name,
		WorkspaceName:            workspaceName,
		Namespace:                d.Namespace,
		ClusterName:              clusterName,
		CloudProvider:            cloudProvider,
		Region:                   region,
		DeploymentID:             d.Id,
		RuntimeVersion:           d.RuntimeVersion,
		AirflowVersion:           d.AirflowVersion,
		IsDagDeployEnabled:       d.IsDagDeployEnabled,
		IsCicdEnforced:           d.IsCicdEnforced,
		Type:                     deploymentType,
		IsRemoteExecutionEnabled: isRemoteExecutionEnabled,
	}
}

// ListData returns deployment list data for structured output
func ListData(ws string, fromAllWorkspaces bool, astroV1Client astrov1.APIClient) (*DeploymentList, error) {
	c, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	if fromAllWorkspaces {
		ws = ""
	}
	deployments, err := ListDeployments(ws, c.Organization, astroV1Client)
	if err != nil {
		return nil, err
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Name > deployments[j].Name })

	result := &DeploymentList{
		Deployments: make([]DeploymentInfo, 0, len(deployments)),
	}

	for i := range deployments {
		result.Deployments = append(result.Deployments, deploymentToInfo(&deployments[i], fromAllWorkspaces))
	}

	return result, nil
}

// List all airflow deployments
func List(ws string, fromAllWorkspaces bool, astroV1Client astrov1.APIClient, out io.Writer) error {
	return ListWithFormat(ws, fromAllWorkspaces, astroV1Client, output.FormatTable, "", out)
}

// ListWithFormat lists deployments with the specified output format
func ListWithFormat(ws string, fromAllWorkspaces bool, astroV1Client astrov1.APIClient, format output.Format, tmpl string, out io.Writer) error {
	return output.PrintData(
		func() (*DeploymentList, error) { return ListData(ws, fromAllWorkspaces, astroV1Client) },
		deploymentTableConfig(fromAllWorkspaces, ws), format, tmpl, out,
	)
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Logs(deploymentID, ws, deploymentName, keyword string, logServer, logScheduler, logTriggerer, logWorkers, warnLogs, errorLogs, infoLogs bool, logCount int, astroV1Client astrov1.APIClient) error {
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
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
		return err
	}

	deploymentID = deployment.Id
	timeRange := 86400
	offset := 0

	var serverComponent astrov1.GetDeploymentLogsParamsSources
	if airflowversions.IsAirflow3(deployment.RuntimeVersion) {
		serverComponent = astrov1.GetDeploymentLogsParamsSourcesApiserver
	} else {
		serverComponent = astrov1.GetDeploymentLogsParamsSourcesWebserver
	}

	// get log source
	var componentSources []astrov1.GetDeploymentLogsParamsSources
	if logServer {
		componentSources = append(componentSources, serverComponent)
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
		componentSources = append(componentSources, serverComponent, "scheduler", "triggerer", "worker")
	}

	maxPerPage := 5000
	perPage := min(logCount, maxPerPage)
	getDeploymentLogsParams := astrov1.GetDeploymentLogsParams{
		Sources:       componentSources,
		Limit:         &perPage,
		MaxNumResults: &logCount,
		Range:         &timeRange,
		Offset:        &offset,
	}
	if logLevel != "" {
		getDeploymentLogsParams.SearchText = &logLevel
	}

	var allResults []astrov1.DeploymentLogEntry
	for {
		deploymentLogs, err := GetDeploymentLogs("", deploymentID, getDeploymentLogsParams, astroV1Client)
		if err != nil {
			return err
		}

		allResults = append(allResults, deploymentLogs.Results...)

		// Stop paginating if we got fewer results than the per-page limit
		// (meaning there are no more pages) or we've reached the requested count.
		if deploymentLogs.ResultCount < deploymentLogs.Limit || len(allResults) >= logCount {
			break
		}

		// Set up the next page request using the search cursor and updated offset.
		// Shrink the next page to only what's still needed so the API doesn't ship
		// results we'll throw away.
		searchID := deploymentLogs.SearchId
		getDeploymentLogsParams.SearchId = &searchID
		nextOffset := deploymentLogs.Offset + deploymentLogs.ResultCount
		getDeploymentLogsParams.Offset = &nextOffset
		nextPerPage := min(logCount-len(allResults), maxPerPage)
		getDeploymentLogsParams.Limit = &nextPerPage
	}

	// Belt-and-suspenders for the case where the API still over-returns.
	if len(allResults) > logCount {
		allResults = allResults[:logCount]
	}

	if len(allResults) == 0 {
		hours := timeRange / 3600
		fmt.Printf("No matching logs have been recorded in the past %d hours for Deployment %s\n", hours, deployment.Name)
		return nil
	}
	for i := range allResults {
		fmt.Printf("%f %s %s\n", allResults[i].Timestamp, allResults[i].Raw, allResults[i].Source)
	}
	return nil
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Create(name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, deploymentType astrov1.DeploymentType, schedulerAU, schedulerReplicas int, remoteExecutionEnabled bool, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogURLPattern *string, astroV1Client astrov1.APIClient, waitForStatus bool, waitTimeForDeployment time.Duration) error { //nolint
	var organizationID string
	var currentWorkspace astrov1.Workspace

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	coreDeploymentType := astrov1.GetDeploymentOptionsParamsDeploymentType(deploymentType)
	coreCloudProvider := GetCoreCloudProvider(cloudProvider)
	deploymentOptionsParams := astrov1.GetDeploymentOptionsParams{
		DeploymentType: &coreDeploymentType,
		CloudProvider:  &coreCloudProvider,
	}

	configOption, err := GetDeploymentOptions("", deploymentOptionsParams, astroV1Client)
	if err != nil {
		return err
	}

	// validate workspace
	ws, err := workspace.GetWorkspaces(astroV1Client)
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
		region, err = selectRegion(cloudProvider, region, astroV1Client)
		if err != nil {
			return err
		}
	}

	// select and validate cluster
	if !IsDeploymentStandard(deploymentType) {
		clusterID, err = selectCluster(clusterID, c.Organization, astroV1Client)
		if err != nil {
			return err
		}
	}

	if dagDeploy == enable { //nolint: goconst
		dagDeployEnabled = true
	} else {
		dagDeployEnabled = false
	}
	createDeploymentRequest := astrov1.CreateDeploymentRequest{}
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
		defaultTaskPodCPUPtr := CreateDefaultTaskPodCPU(defaultTaskPodCpu, remoteExecutionEnabled, &configOption)
		defaultTaskPodMemoryPtr := CreateDefaultTaskPodMemory(defaultTaskPodMemory, remoteExecutionEnabled, &configOption)
		resourceQuotaCPUPtr := CreateResourceQuotaCPU(resourceQuotaCpu, remoteExecutionEnabled, &configOption)
		resourceQuotaMemoryPtr := CreateResourceQuotaMemory(resourceQuotaMemory, remoteExecutionEnabled, &configOption)
		var remoteExecution *astrov1.DeploymentRemoteExecutionRequest
		if remoteExecutionEnabled {
			remoteExecution = &astrov1.DeploymentRemoteExecutionRequest{
				Enabled:                true,
				AllowedIpAddressRanges: allowedIpAddressRanges,
				TaskLogBucket:          taskLogBucket,
				TaskLogUrlPattern:      taskLogURLPattern,
			}
		}
		var deplWorkloadIdentity *string
		if workloadIdentity != "" {
			deplWorkloadIdentity = &workloadIdentity
		}
		// build standard input
		if IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astrov1.CreateStandardDeploymentRequestCloudProvider
			switch cloudProvider {
			case gcpCloud:
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderGCP
			case awsCloud:
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderAWS
			case azureCloud:
				requestedCloudProvider = astrov1.CreateStandardDeploymentRequestCloudProviderAZURE
			}
			var requestedExecutor astrov1.CreateStandardDeploymentRequestExecutor
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorCELERY
			}
			if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorKUBERNETES
			}
			if strings.EqualFold(executor, AstroExecutor) || strings.EqualFold(executor, ASTRO) {
				requestedExecutor = astrov1.CreateStandardDeploymentRequestExecutorASTRO
			}

			standardDeploymentRequest := astrov1.CreateStandardDeploymentRequest{
				AstroRuntimeVersion:  &runtimeVersion,
				CloudProvider:        &requestedCloudProvider,
				Description:          &description,
				Name:                 name,
				Executor:             &requestedExecutor,
				IsCicdEnforced:       &isCicdEnforced,
				Region:               &region,
				IsDagDeployEnabled:   &dagDeployEnabled,
				IsHighAvailability:   &highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          workspaceID,
				Type:                 new(astrov1.CreateStandardDeploymentRequestTypeSTANDARD),
				DefaultTaskPodCpu:    defaultTaskPodCPUPtr,
				DefaultTaskPodMemory: defaultTaskPodMemoryPtr,
				ResourceQuotaCpu:     resourceQuotaCPUPtr,
				ResourceQuotaMemory:  resourceQuotaMemoryPtr,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			switch schedulerSize { //nolint:dupl
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)
			}
			if highAvailability == enable { //nolint: goconst
				standardDeploymentRequest.IsHighAvailability = new(true)
			}
			err := createDeploymentRequest.FromCreateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		// build dedicated input
		if IsDeploymentDedicated(deploymentType) {
			var requestedExecutor astrov1.CreateDedicatedDeploymentRequestExecutor
			if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorCELERY
			}
			if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			if strings.EqualFold(executor, AstroExecutor) || strings.EqualFold(executor, ASTRO) {
				requestedExecutor = astrov1.CreateDedicatedDeploymentRequestExecutorASTRO
			}
			dedicatedDeploymentRequest := astrov1.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  &runtimeVersion,
				Description:          &description,
				Name:                 name,
				Executor:             &requestedExecutor,
				IsCicdEnforced:       &isCicdEnforced,
				IsDagDeployEnabled:   &dagDeployEnabled,
				IsHighAvailability:   &highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				ClusterId:            &clusterID,
				WorkspaceId:          workspaceID,
				Type:                 new(astrov1.CreateDedicatedDeploymentRequestTypeDEDICATED),
				DefaultTaskPodCpu:    defaultTaskPodCPUPtr,
				DefaultTaskPodMemory: defaultTaskPodMemoryPtr,
				ResourceQuotaCpu:     resourceQuotaCPUPtr,
				ResourceQuotaMemory:  resourceQuotaMemoryPtr,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			switch schedulerSize { //nolint:dupl
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				dedicatedDeploymentRequest.SchedulerSize = new(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeSMALL)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				dedicatedDeploymentRequest.SchedulerSize = new(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				dedicatedDeploymentRequest.SchedulerSize = new(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeLARGE)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				dedicatedDeploymentRequest.SchedulerSize = new(astrov1.CreateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE)
			}
			err := createDeploymentRequest.FromCreateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
	}
	// build hybrid input
	if IsDeploymentHybrid(deploymentType) {
		// Validate only the scheduler values the user actually supplied; the server fills in the rest.
		if schedulerAU != 0 && schedulerReplicas != 0 {
			if !validateHybridResources(schedulerAU, schedulerReplicas, configOption) {
				return ErrInvalidResourceRequest
			}
		}
		var requestedExecutor astrov1.CreateHybridDeploymentRequestExecutor
		if strings.EqualFold(executor, CeleryExecutor) || strings.EqualFold(executor, CELERY) {
			requestedExecutor = astrov1.CreateHybridDeploymentRequestExecutorCELERY
		}
		if strings.EqualFold(executor, KubeExecutor) || strings.EqualFold(executor, KUBERNETES) {
			requestedExecutor = astrov1.CreateHybridDeploymentRequestExecutorKUBERNETES
		}
		hybridDeploymentRequest := astrov1.CreateHybridDeploymentRequest{
			AstroRuntimeVersion: &runtimeVersion,
			Description:         &description,
			Name:                name,
			Executor:            &requestedExecutor,
			IsCicdEnforced:      &isCicdEnforced,
			IsDagDeployEnabled:  &dagDeployEnabled,
			ClusterId:           &clusterID,
			WorkloadIdentity:    &workloadIdentity,
			WorkspaceId:         workspaceID,
			Type:                new(astrov1.CreateHybridDeploymentRequestTypeHYBRID),
		}
		if schedulerAU != 0 || schedulerReplicas != 0 {
			hybridDeploymentRequest.Scheduler = &astrov1.CreateDeploymentInstanceSpecRequest{}
			if schedulerAU != 0 {
				hybridDeploymentRequest.Scheduler.Au = &schedulerAU
			}
			if schedulerReplicas != 0 {
				hybridDeploymentRequest.Scheduler.Replicas = &schedulerReplicas
			}
		}
		err = createDeploymentRequest.FromCreateHybridDeploymentRequest(hybridDeploymentRequest)
		if err != nil {
			return err
		}
	}

	d, err := CoreCreateDeployment(organizationID, createDeploymentRequest, astroV1Client)
	if err != nil {
		return err
	}
	if waitForStatus {
		err = HealthPoll(d.Id, workspaceID, SleepTime, TickNum, int(waitTimeForDeployment.Seconds()), astroV1Client)
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

func getDefaultNodePoolID(nodePools *[]astrov1.NodePool) string {
	if nodePools != nil && len(*nodePools) > 0 {
		return (*nodePools)[0].Id
	}
	return ""
}

func createOutput(workspaceID string, d *astrov1.Deployment) error {
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

func deploymentToTableRow(table *printutil.Table, d *astrov1.Deployment, includeWorkspaceName bool) {
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
	isRemoteExecutionEnabled := IsRemoteExecutionEnabled(d)
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
		strconv.FormatBool(isRemoteExecutionEnabled),
	}
	if includeWorkspaceName {
		cols = slices.Insert(cols, 1, *d.WorkspaceName)
	}
	table.AddRow(cols, false)
}

// Hybrid scheduler-AU/replica bounds. Hardcoded because the v1 API doesn't expose them,
// and Hybrid is a sunsetting infrastructure tier.
const (
	hybridSchedulerAuMin       = 5
	hybridSchedulerAuMax       = 30
	hybridSchedulerReplicasMin = 1
	hybridSchedulerReplicasMax = 4
)

func validateHybridResources(schedulerAU, schedulerReplicas int, _ astrov1.DeploymentOptions) bool { //nolint:gocritic
	if schedulerAU > hybridSchedulerAuMax || schedulerAU < hybridSchedulerAuMin {
		fmt.Printf("\nScheduler AUs must be between a min of %d and a max of %d AUs\n", hybridSchedulerAuMin, hybridSchedulerAuMax)
		return false
	}
	if schedulerReplicas > hybridSchedulerReplicasMax || schedulerReplicas < hybridSchedulerReplicasMin {
		fmt.Printf("\nScheduler Replicas must be between a min of %d and a max of %d Replicas\n", hybridSchedulerReplicasMin, hybridSchedulerReplicasMax)
		return false
	}
	return true
}

func ListClusterOptions(cloudProvider string, astroV1Client astrov1.APIClient) ([]astrov1.ClusterOptions, error) {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	var provider astrov1.GetClusterOptionsParamsProvider
	switch cloudProvider {
	case gcpCloud:
		provider = astrov1.GetClusterOptionsParamsProviderGCP
	case awsCloud:
		provider = astrov1.GetClusterOptionsParamsProviderAWS
	case azureCloud:
		provider = astrov1.GetClusterOptionsParamsProviderAZURE
	}
	optionsParams := &astrov1.GetClusterOptionsParams{
		Provider: &provider,
		// Only DEDICATED returns cluster options; Standard (shared) has none to enumerate.
		Type: astrov1.GetClusterOptionsParamsTypeDEDICATED,
	}
	clusterOptions, err := astroV1Client.GetClusterOptionsWithResponse(context.Background(), ctx.Organization, optionsParams)
	if err != nil {
		return nil, err
	}
	err = astrov1.NormalizeAPIError(clusterOptions.HTTPResponse, clusterOptions.Body)
	if err != nil {
		return nil, err
	}
	options := *clusterOptions.JSON200

	return options, nil
}

func selectRegion(cloudProvider, region string, astroV1Client astrov1.APIClient) (newRegion string, err error) {
	regionsTab := printutil.Table{
		Padding:        []int{5, 30},
		DynamicPadding: true,
		Header:         []string{"#", "REGION"},
	}

	// get all regions for the cloud provider
	options, err := ListClusterOptions(cloudProvider, astroV1Client)
	if err != nil {
		return "", err
	}
	regions := options[0].Regions

	if region == "" {
		fmt.Println("\nPlease select a Region for your Deployment:")

		regionMap := map[string]astrov1.ProviderRegion{}

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

func selectCluster(clusterID, organizationID string, astroV1Client astrov1.APIClient) (newClusterID string, err error) {
	clusterTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
	}

	cs, err := organization.ListClusters(organizationID, astroV1Client)
	if err != nil {
		return "", err
	}
	// select cluster
	if clusterID == "" {
		fmt.Println("\nPlease select a Cluster for your Deployment:")

		clusterMap := map[string]astrov1.Cluster{}
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

func HealthPoll(deploymentID, ws string, sleepTime, tickNum, timeoutNum int, astroV1Client astrov1.APIClient) error {
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
			currentDeployment, err := GetDeploymentByID("", deploymentID, astroV1Client)
			if err != nil {
				return err
			}

			// considering hibernating as healthy state, since hibernation can only happen when the deployment is healthy.
			// This covers for the case when the deployment is cretated and straight away goes into hibernation.
			if currentDeployment.Status == astrov1.DeploymentStatusHEALTHY || currentDeployment.Status == astrov1.DeploymentStatusHIBERNATING {
				fmt.Printf("Deployment %s is now healthy\n", currentDeployment.Name)
				return nil
			}
			continue
		}
	}
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Update(deploymentID, name, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, schedulerAU, schedulerReplicas int, wQueueList []astrov1.WorkerQueueRequest, hybridQueueList []astrov1.HybridWorkerQueueRequest, newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogUrlPattern *string, force bool, astroV1Client astrov1.APIClient) error { //nolint
	var queueCreateUpdate, confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, nil, astroV1Client)
	if err != nil {
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
	if !force && isCicdEnforced && dagDeploy != "" {
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
	configOption, err := GetDeploymentOptions("", astrov1.GetDeploymentOptionsParams{}, astroV1Client)
	if err != nil {
		return err
	}

	// build query input
	updateDeploymentRequest := astrov1.UpdateDeploymentRequest{}

	if name == "" {
		name = currentDeployment.Name
	}
	if description == "" {
		if currentDeployment.Description != nil {
			description = *currentDeployment.Description
		}
	}
	var deploymentEnvironmentVariablesRequest []astrov1.DeploymentEnvironmentVariableRequest
	if len(newEnvironmentVariables) == 0 {
		if currentDeployment.EnvironmentVariables != nil {
			environmentVariables := *currentDeployment.EnvironmentVariables
			for i := range *currentDeployment.EnvironmentVariables {
				var deploymentEnvironmentVariableRequest astrov1.DeploymentEnvironmentVariableRequest
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
		deploymentEnvironmentVariablesRequest = []astrov1.DeploymentEnvironmentVariableRequest{}
	}
	if IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type) {
		var workerQueuesRequest []astrov1.UpdateWorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = ConvertWorkerQueues(workerQueues, func(q astrov1.WorkerQueue) astrov1.UpdateWorkerQueueRequest {
				machine := astrov1.UpdateWorkerQueueRequestAstroMachine(*q.AstroMachine)
				return astrov1.UpdateWorkerQueueRequest{
					AstroMachine:      &machine,
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
			if strings.EqualFold(defaultWorkerMachineName, string(configOption.WorkerMachines[i].Name)) {
				workerConcurrency = int(configOption.WorkerMachines[i].Concurrency.Default)
			}
		}
		defaultMachine := astrov1.UpdateWorkerQueueRequestAstroMachine(defaultWorkerMachineName)
		defaultWorkerQueue := []astrov1.UpdateWorkerQueueRequest{{
			AstroMachine:      &defaultMachine,
			IsDefault:         true,
			MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
			MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
			Name:              "default",
			WorkerConcurrency: workerConcurrency,
		}}

		if len(wQueueList) > 0 {
			queueCreateUpdate = true
			workerQueuesRequest = ConvertCreateQueuesToUpdate(wQueueList)
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
		defaultTaskPodCPUPtr := UpdateDefaultTaskPodCPU(defaultTaskPodCpu, &currentDeployment, &configOption)
		defaultTaskPodMemoryPtr := UpdateDefaultTaskPodMemory(defaultTaskPodMemory, &currentDeployment, &configOption)
		resourceQuotaCPUPtr := UpdateResourceQuotaCPU(resourceQuotaCpu, &currentDeployment)
		resourceQuotaMemoryPtr := UpdateResourceQuotaMemory(resourceQuotaMemory, &currentDeployment)
		var remoteExecution *astrov1.DeploymentRemoteExecutionRequest
		if IsRemoteExecutionEnabled(&currentDeployment) {
			remoteExecution = &astrov1.DeploymentRemoteExecutionRequest{
				Enabled: true,
			}
			if allowedIpAddressRanges != nil {
				remoteExecution.AllowedIpAddressRanges = allowedIpAddressRanges
			} else {
				remoteExecution.AllowedIpAddressRanges = &currentDeployment.RemoteExecution.AllowedIpAddressRanges
			}
			if taskLogBucket != nil {
				remoteExecution.TaskLogBucket = taskLogBucket
			} else {
				remoteExecution.TaskLogBucket = currentDeployment.RemoteExecution.TaskLogBucket
			}
			if taskLogUrlPattern != nil {
				remoteExecution.TaskLogUrlPattern = taskLogUrlPattern
			} else {
				remoteExecution.TaskLogUrlPattern = currentDeployment.RemoteExecution.TaskLogUrlPattern
			}
		}
		var deplWorkloadIdentity *string
		if workloadIdentity != "" {
			deplWorkloadIdentity = &workloadIdentity
		}
		if IsDeploymentStandard(*currentDeployment.Type) {
			var requestedExecutor astrov1.UpdateStandardDeploymentRequestExecutor
			switch strings.ToUpper(executor) {
			case "":
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutor(*currentDeployment.Executor)
			case strings.ToUpper(CeleryExecutor):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(KubeExecutor):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(AstroExecutor):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorASTRO
			case strings.ToUpper(CELERY):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorCELERY
			case strings.ToUpper(KUBERNETES):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(ASTRO):
				requestedExecutor = astrov1.UpdateStandardDeploymentRequestExecutorASTRO
			}

			standardDeploymentRequest := astrov1.UpdateStandardDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astrov1.UpdateStandardDeploymentRequestTypeSTANDARD,
				ResourceQuotaCpu:     resourceQuotaCPUPtr,
				ResourceQuotaMemory:  resourceQuotaMemoryPtr,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				DefaultTaskPodCpu:    defaultTaskPodCPUPtr,
				DefaultTaskPodMemory: defaultTaskPodMemoryPtr,
				WorkloadIdentity:     deplWorkloadIdentity,
				WorkerQueues:         &workerQueuesRequest,
				RemoteExecution:      remoteExecution,
			}
			switch schedulerSize {
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				standardDeploymentRequest.SchedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				standardDeploymentRequest.SchedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				standardDeploymentRequest.SchedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				standardDeploymentRequest.SchedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				standardDeploymentRequest.SchedulerSize = astrov1.UpdateStandardDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}

			// confirm with user if the executor is changing
			if *currentDeployment.Executor != astrov1.DeploymentExecutor(standardDeploymentRequest.Executor) {
				confirmWithUser = true
			}

			if !IsRemoteExecutionEnabled(&currentDeployment) {
				standardDeploymentRequest.WorkerQueues = updateWorkerQueuesForExecutor(
					astrov1.DeploymentExecutor(standardDeploymentRequest.Executor),
					standardDeploymentRequest.WorkerQueues,
					&defaultWorkerQueue,
				)
			}

			err := updateDeploymentRequest.FromUpdateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if IsDeploymentDedicated(*currentDeployment.Type) {
			var requestedExecutor astrov1.UpdateDedicatedDeploymentRequestExecutor
			switch strings.ToUpper(executor) {
			case "":
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutor(*currentDeployment.Executor)
			case strings.ToUpper(CeleryExecutor):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(KubeExecutor):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(AstroExecutor):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorASTRO
			case strings.ToUpper(CELERY):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorCELERY
			case strings.ToUpper(KUBERNETES):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			case strings.ToUpper(ASTRO):
				requestedExecutor = astrov1.UpdateDedicatedDeploymentRequestExecutorASTRO
			}
			dedicatedDeploymentRequest := astrov1.UpdateDedicatedDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				IsDevelopmentMode:    &developmentModeValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astrov1.UpdateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCPUPtr,
				DefaultTaskPodMemory: defaultTaskPodMemoryPtr,
				ResourceQuotaCpu:     resourceQuotaCPUPtr,
				ResourceQuotaMemory:  resourceQuotaMemoryPtr,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				WorkerQueues:         &workerQueuesRequest,
				WorkloadIdentity:     deplWorkloadIdentity,
				RemoteExecution:      remoteExecution,
			}
			switch schedulerSize {
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				dedicatedDeploymentRequest.SchedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				dedicatedDeploymentRequest.SchedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				dedicatedDeploymentRequest.SchedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSizeEXTRALARGE
			case "":
				dedicatedDeploymentRequest.SchedulerSize = astrov1.UpdateDedicatedDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}
			// confirm with user if the executor is changing
			if *currentDeployment.Executor != astrov1.DeploymentExecutor(dedicatedDeploymentRequest.Executor) {
				confirmWithUser = true
			}
			if !IsRemoteExecutionEnabled(&currentDeployment) {
				dedicatedDeploymentRequest.WorkerQueues = updateWorkerQueuesForExecutor(
					astrov1.DeploymentExecutor(dedicatedDeploymentRequest.Executor),
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
		var workerQueuesRequest []astrov1.UpdateWorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = ConvertWorkerQueues(workerQueues, func(q astrov1.WorkerQueue) astrov1.UpdateWorkerQueueRequest {
				nodePoolID := ""
				if q.NodePoolId != nil {
					nodePoolID = *q.NodePoolId
				}
				return astrov1.UpdateWorkerQueueRequest{
					Id:                &q.Id,
					IsDefault:         q.IsDefault,
					MaxWorkerCount:    q.MaxWorkerCount,
					MinWorkerCount:    q.MinWorkerCount,
					Name:              q.Name,
					WorkerConcurrency: q.WorkerConcurrency,
					NodePoolId:        &nodePoolID,
				}
			})
		}

		if len(hybridQueueList) > 0 {
			queueCreateUpdate = true
			workerQueuesRequest = ConvertHybridQueuesToUpdate(hybridQueueList)
		}
		if schedulerAU == 0 {
			schedulerAU = *currentDeployment.SchedulerAu
		}
		if schedulerReplicas == 0 {
			schedulerReplicas = currentDeployment.SchedulerReplicas
		}
		if workloadIdentity == "" {
			if currentDeployment.EffectiveWorkloadIdentity != nil {
				workloadIdentity = *currentDeployment.EffectiveWorkloadIdentity
			}
		}
		// validate au resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return ErrInvalidResourceRequest
		}
		var requestedExecutor astrov1.UpdateHybridDeploymentRequestExecutor
		switch strings.ToUpper(executor) {
		case "":
			requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutor(*currentDeployment.Executor)
		case strings.ToUpper(CeleryExecutor):
			requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorCELERY
		case strings.ToUpper(KubeExecutor):
			requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorKUBERNETES
		case strings.ToUpper(CELERY):
			requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorCELERY
		case strings.ToUpper(KUBERNETES):
			requestedExecutor = astrov1.UpdateHybridDeploymentRequestExecutorKUBERNETES
		}
		hybridDeploymentRequest := astrov1.UpdateHybridDeploymentRequest{
			Description:        &description,
			Name:               name,
			Executor:           requestedExecutor,
			IsCicdEnforced:     isCicdEnforced,
			IsDagDeployEnabled: dagDeployEnabled,
			WorkspaceId:        currentDeployment.WorkspaceId,
			Scheduler: astrov1.UpdateDeploymentInstanceSpecRequest{
				Au:       schedulerAU,
				Replicas: schedulerReplicas,
			},
			Type:                 astrov1.UpdateHybridDeploymentRequestTypeHYBRID,
			EnvironmentVariables: deploymentEnvironmentVariablesRequest,
			WorkloadIdentity:     &workloadIdentity,
		}
		cluster, err := GetClusterByID("", *currentDeployment.ClusterId, astroV1Client)
		if err != nil {
			return err
		}
		nodePoolID := getDefaultNodePoolID(cluster.NodePools)
		// confirm with user if the executor is changing
		if *currentDeployment.Executor != astrov1.DeploymentExecutor(hybridDeploymentRequest.Executor) {
			confirmWithUser = true
		}
		if hybridDeploymentRequest.Executor == astrov1.UpdateHybridDeploymentRequestExecutorKUBERNETES {
			// For KUBERNETES executor, WorkerQueues should be nil
			hybridDeploymentRequest.WorkerQueues = nil
			if *currentDeployment.Executor == astrov1.DeploymentExecutorCELERY {
				confirmWithUser = true
				hybridDeploymentRequest.TaskPodNodePoolId = &nodePoolID
			} else {
				hybridDeploymentRequest.TaskPodNodePoolId = currentDeployment.TaskPodNodePoolId
			}
		} else {
			if *currentDeployment.Executor == astrov1.DeploymentExecutorKUBERNETES && workerQueuesRequest == nil {
				workerQueuesRequest = []astrov1.UpdateWorkerQueueRequest{{
					IsDefault:         true,
					MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
					MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
					Name:              "default",
					WorkerConcurrency: int(configOption.WorkerQueues.WorkerConcurrency.Default),
					NodePoolId:        &nodePoolID,
				}}
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
			} else {
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
			}
			if *currentDeployment.Executor == astrov1.DeploymentExecutorKUBERNETES {
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
	d, err := CoreUpdateDeployment(c.Organization, currentDeployment.Id, updateDeploymentRequest, astroV1Client)
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
		isRemoteExecutionEnabled := IsRemoteExecutionEnabled(&d)
		tabDeployment.AddRow([]string{d.Name, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.IsDagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type), strconv.FormatBool(isRemoteExecutionEnabled)}, false)
		tabDeployment.SuccessMsg = "\n Successfully updated Deployment"
		tabDeployment.Print(os.Stdout)
	}
	return nil
}

func ConvertHybridQueuesToUpdate(in []astrov1.HybridWorkerQueueRequest) []astrov1.UpdateWorkerQueueRequest {
	out := make([]astrov1.UpdateWorkerQueueRequest, len(in))
	for i := range in {
		nodePoolID := in[i].NodePoolId
		out[i] = astrov1.UpdateWorkerQueueRequest{
			Id:                in[i].Id,
			IsDefault:         in[i].IsDefault,
			MaxWorkerCount:    in[i].MaxWorkerCount,
			MinWorkerCount:    in[i].MinWorkerCount,
			Name:              in[i].Name,
			NodePoolId:        &nodePoolID,
			WorkerConcurrency: in[i].WorkerConcurrency,
		}
	}
	return out
}

func ConvertCreateQueuesToUpdate(in []astrov1.WorkerQueueRequest) []astrov1.UpdateWorkerQueueRequest {
	out := make([]astrov1.UpdateWorkerQueueRequest, len(in))
	for i := range in {
		machine := astrov1.UpdateWorkerQueueRequestAstroMachine(in[i].AstroMachine)
		out[i] = astrov1.UpdateWorkerQueueRequest{
			AstroMachine:      &machine,
			Id:                in[i].Id,
			IsDefault:         in[i].IsDefault,
			MaxWorkerCount:    in[i].MaxWorkerCount,
			MinWorkerCount:    in[i].MinWorkerCount,
			Name:              in[i].Name,
			WorkerConcurrency: in[i].WorkerConcurrency,
		}
	}
	return out
}

func updateWorkerQueuesForExecutor(newExecutor astrov1.DeploymentExecutor, workerQueuesRequest, defaultWorkerQueue *[]astrov1.UpdateWorkerQueueRequest) *[]astrov1.UpdateWorkerQueueRequest {
	var workerQueuesRequestOrDefault *[]astrov1.UpdateWorkerQueueRequest
	if workerQueuesRequest != nil {
		workerQueuesRequestOrDefault = workerQueuesRequest
	} else {
		workerQueuesRequestOrDefault = defaultWorkerQueue
	}
	// this function is called when executor is changing.
	switch newExecutor {
	case astrov1.DeploymentExecutorKUBERNETES:
		return nil
	case astrov1.DeploymentExecutorCELERY:
		// placeholder for celery specific logic
		// https://github.com/astronomer/astro-cli/pull/1854#discussion_r2153379104
		return workerQueuesRequestOrDefault
	case astrov1.DeploymentExecutorASTRO:
		// placeholder for astro specific logic
		// https://github.com/astronomer/astro-cli/pull/1854#discussion_r2153379104
		return workerQueuesRequestOrDefault
	default:
		return nil
	}
}

func CreateDefaultTaskPodCPU(defaultTaskPodCPU string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodCPU != "" {
		return &defaultTaskPodCPU
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.DefaultPodSize.Cpu.Default
}

func CreateDefaultTaskPodMemory(defaultTaskPodMemory string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodMemory != "" {
		return &defaultTaskPodMemory
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.DefaultPodSize.Memory.Default
}

func CreateResourceQuotaCPU(resourceQuotaCPU string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if resourceQuotaCPU != "" {
		return &resourceQuotaCPU
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.ResourceQuota.Cpu.Default
}

func CreateResourceQuotaMemory(resourceQuotaMemory string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if resourceQuotaMemory != "" {
		return &resourceQuotaMemory
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.ResourceQuota.Memory.Default
}

func UpdateDefaultTaskPodCPU(defaultTaskPodCPU string, deployment *astrov1.Deployment, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodCPU != "" {
		return &defaultTaskPodCPU
	}
	if deployment.DefaultTaskPodCpu != nil {
		return deployment.DefaultTaskPodCpu
	}
	if IsRemoteExecutionEnabled(deployment) {
		return nil
	}
	if configOption != nil {
		return &configOption.ResourceQuotas.DefaultPodSize.Cpu.Default
	}
	return nil
}

func UpdateDefaultTaskPodMemory(defaultTaskPodMemory string, deployment *astrov1.Deployment, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodMemory != "" {
		return &defaultTaskPodMemory
	}
	if deployment.DefaultTaskPodMemory != nil {
		return deployment.DefaultTaskPodMemory
	}
	if IsRemoteExecutionEnabled(deployment) {
		return nil
	}
	if configOption != nil {
		return &configOption.ResourceQuotas.DefaultPodSize.Memory.Default
	}
	return nil
}

func UpdateResourceQuotaCPU(resourceQuotaCPU string, deployment *astrov1.Deployment) *string {
	if resourceQuotaCPU != "" {
		return &resourceQuotaCPU
	}
	return deployment.ResourceQuotaCpu
}

func UpdateResourceQuotaMemory(resourceQuotaMemory string, deployment *astrov1.Deployment) *string {
	if resourceQuotaMemory != "" {
		return &resourceQuotaMemory
	}
	return deployment.ResourceQuotaMemory
}

func Delete(deploymentID, ws, deploymentName string, forceDelete bool, astroV1Client astrov1.APIClient) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, nil, astroV1Client)
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

	err = CoreDeleteDeployment(currentDeployment.OrganizationId, currentDeployment.Id, astroV1Client)
	if err != nil {
		return err
	}

	fmt.Println("\nSuccessfully deleted deployment " + ansi.Bold(currentDeployment.Name))

	return nil
}

func UpdateDeploymentHibernationOverride(deploymentID, ws, deploymentName string, isHibernating bool, overrideUntil *time.Time, force bool, astroV1Client astrov1.APIClient) error {
	// Set wording based on the hibernation action
	var action string
	if isHibernating {
		action = "hibernate"
	} else {
		action = "wake up"
	}

	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, isDevelopmentDeployment, astroV1Client)
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

	overrideDeploymentHibernationBody := astrov1.OverrideDeploymentHibernationBody{
		IsHibernating: &isHibernating,
		OverrideUntil: overrideUntil,
	}

	deploymentHibernationOverride, err := CoreUpdateDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, overrideDeploymentHibernationBody, astroV1Client)
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

func DeleteDeploymentHibernationOverride(deploymentID, ws, deploymentName string, force bool, astroV1Client astrov1.APIClient) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, isDevelopmentDeployment, astroV1Client)
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

	err = CoreDeleteDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, astroV1Client)
	if err != nil {
		return err
	}

	fmt.Println("\nSuccessfully removed hibernation override")
	fmt.Println("If set, hibernation schedule will resume immediately.")

	return nil
}

func IsDeploymentStandard(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeSTANDARD
}

func IsDeploymentDedicated(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeDEDICATED
}

func IsDeploymentHybrid(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeHYBRID
}

func IsRemoteExecutionEnabled(deployment *astrov1.Deployment) bool {
	return deployment.RemoteExecution != nil && deployment.RemoteExecution.Enabled
}

// CoreUpdateDeploymentHibernationOverride updates a deployment hibernation override with the core API
//
//nolint:dupl
var CoreUpdateDeploymentHibernationOverride = func(orgID, deploymentID string, overrideDeploymentHibernationRequest astrov1.OverrideDeploymentHibernationBody, astroV1Client astrov1.APIClient) (astrov1.DeploymentHibernationOverride, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.DeploymentHibernationOverride{}, err
		}
		orgID = c.Organization
	}
	resp, err := astroV1Client.UpdateDeploymentHibernationOverrideWithResponse(context.Background(), orgID, deploymentID, overrideDeploymentHibernationRequest)
	if err != nil {
		return astrov1.DeploymentHibernationOverride{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.DeploymentHibernationOverride{}, err
	}
	deploymentHibernationOverride := *resp.JSON200
	return deploymentHibernationOverride, nil
}

// CoreDeleteDeploymentHibernationOverride deletes a deployment hibernation override with the core API
//
//nolint:dupl
var CoreDeleteDeploymentHibernationOverride = func(orgID, deploymentID string, astroV1Client astrov1.APIClient) error {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
	}
	resp, err := astroV1Client.DeleteDeploymentHibernationOverrideWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

var ListDeployments = func(ws, orgID string, astroV1Client astrov1.APIClient) ([]astrov1.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return []astrov1.Deployment{}, err
		}
		orgID = c.Organization
	}
	deploymentListParams := &astrov1.ListDeploymentsParams{
		Limit: &listLimit,
	}
	if ws != "" {
		deploymentListParams.WorkspaceIds = &[]string{ws}
	}

	resp, err := astroV1Client.ListDeploymentsWithResponse(context.Background(), orgID, deploymentListParams)
	if err != nil {
		return []astrov1.Deployment{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return []astrov1.Deployment{}, err
	}

	deploymentResponse := *resp.JSON200
	deployments := deploymentResponse.Deployments

	return deployments, nil
}

//nolint:dupl
var GetClusterByID = func(orgID, clusterID string, astroV1Client astrov1.APIClient) (astrov1.Cluster, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.Cluster{}, err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.GetClusterWithResponse(context.Background(), orgID, clusterID)
	if err != nil {
		return astrov1.Cluster{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.Cluster{}, err
	}

	cluster := *resp.JSON200

	return cluster, nil
}

//nolint:dupl
var GetDeploymentByID = func(orgID, deploymentID string, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.GetDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.Deployment{}, err
	}

	deployment := *resp.JSON200

	return deployment, nil
}

// CoreCreateDeployment creates a deployment with the core API
var CoreCreateDeployment = func(orgID string, createDeploymentRequest astrov1.CreateDeploymentJSONRequestBody, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.CreateDeploymentWithResponse(context.Background(), orgID, createDeploymentRequest)
	if err != nil {
		return astrov1.Deployment{}, err
	}

	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	deployment := *resp.JSON200

	return deployment, nil
}

// CoreUpdateDeployment updates a deployment with the core API
//
//nolint:dupl
var CoreUpdateDeployment = func(orgID, deploymentID string, updateDeploymentRequest astrov1.UpdateDeploymentJSONRequestBody, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.Deployment{}, err
		}
		orgID = c.Organization
	}
	resp, err := astroV1Client.UpdateDeploymentWithResponse(context.Background(), orgID, deploymentID, updateDeploymentRequest)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	deployment := *resp.JSON200

	return deployment, nil
}

// CoreDeleteDeployment deletes a deployment with the core API
var CoreDeleteDeployment = func(orgID, deploymentID string, astroV1Client astrov1.APIClient) error {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.DeleteDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}

	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

//nolint:dupl
var GetDeploymentOptions = func(orgID string, deploymentOptionsParams astrov1.GetDeploymentOptionsParams, astroV1Client astrov1.APIClient) (astrov1.DeploymentOptions, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.DeploymentOptions{}, err
		}
		orgID = c.Organization
	}
	resp, err := astroV1Client.GetDeploymentOptionsWithResponse(context.Background(), orgID, &deploymentOptionsParams)
	if err != nil {
		return astrov1.DeploymentOptions{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.DeploymentOptions{}, err
	}
	DeploymentOptions := *resp.JSON200

	return DeploymentOptions, nil
}

//nolint:dupl
func GetPlatformDeploymentOptions(orgID string, deploymentOptionsParams astrov1.GetDeploymentOptionsParams, astroV1Client astrov1.APIClient) (astrov1.DeploymentOptions, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.DeploymentOptions{}, err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.GetDeploymentOptionsWithResponse(context.Background(), orgID, &deploymentOptionsParams)
	if err != nil {
		return astrov1.DeploymentOptions{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.DeploymentOptions{}, err
	}
	DeploymentOptions := *resp.JSON200

	return DeploymentOptions, nil
}

var GetDeploymentLogs = func(orgID string, deploymentID string, getDeploymentLogsParams astrov1.GetDeploymentLogsParams, astroV1Client astrov1.APIClient) (astrov1.DeploymentLog, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrov1.DeploymentLog{}, err
		}
		orgID = c.Organization
	}

	resp, err := astroV1Client.GetDeploymentLogsWithResponse(context.Background(), orgID, deploymentID, &getDeploymentLogsParams)
	if err != nil {
		return astrov1.DeploymentLog{}, err
	}
	err = astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return astrov1.DeploymentLog{}, err
	}
	DeploymentLog := *resp.JSON200

	return DeploymentLog, nil
}

var SelectDeployment = func(deployments []astrov1.Deployment, message string) (astrov1.Deployment, error) {
	// select deployment
	if len(deployments) == 0 {
		return astrov1.Deployment{}, nil
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

	deployMap := map[string]astrov1.Deployment{}
	for i := range deployments {
		index := i + 1
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Name, deployments[i].Namespace, deployments[i].Id, strconv.FormatBool(deployments[i].IsDagDeployEnabled)}, false)

		deployMap[strconv.Itoa(index)] = deployments[i]
	}

	tab.Print(os.Stdout)
	choice := input.Text("\n> ")
	selected, ok := deployMap[choice]
	if !ok {
		return astrov1.Deployment{}, ErrInvalidDeploymentKey
	}
	return selected, nil
}

func GetDeployment(ws, deploymentID, deploymentName string, disableCreateFlow bool, selectionFilter func(deployment astrov1.Deployment) bool, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) { //nolint:gocognit
	deployments, err := ListDeployments(ws, "", astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, errors.Wrap(err, errInvalidDeployment.Error())
	}

	if len(deployments) == 0 && disableCreateFlow {
		return astrov1.Deployment{}, nil
	}

	if deploymentID != "" && deploymentName != "" && !CleanOutput {
		fmt.Printf("Both a Deployment ID and Deployment name have been supplied. The Deployment ID %s will be used\n", deploymentID)
	}
	// find deployment by name
	if deploymentID == "" && deploymentName != "" {
		var stageDeployments []astrov1.Deployment
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
			return astrov1.Deployment{}, errInvalidDeployment
		}
		currentDeployment, err := GetDeploymentByID("", stageDeployments[0].Id, astroV1Client)
		if err != nil {
			return astrov1.Deployment{}, err
		}
		return currentDeployment, nil
	}

	var currentDeployment astrov1.Deployment

	// select deployment if deploymentID is empty
	if deploymentID == "" {
		currentDeployment, err = deploymentSelectionProcess(ws, deployments, selectionFilter, astroV1Client, disableCreateFlow)
		if err != nil {
			return astrov1.Deployment{}, err
		}
	}
	// find deployment by ID
	for i := range deployments {
		if deployments[i].Id == deploymentID {
			currentDeployment = deployments[i]
		}
	}
	if currentDeployment.Id == "" {
		return astrov1.Deployment{}, errInvalidDeployment
	}

	currentDeployment, err = GetDeploymentByID("", currentDeployment.Id, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	return currentDeployment, nil
}

func deploymentSelectionProcess(ws string, deployments []astrov1.Deployment, deploymentFilter func(deployment astrov1.Deployment) bool, astroV1Client astrov1.APIClient, disableCreateFlow bool) (astrov1.Deployment, error) {
	// filter deployments
	if deploymentFilter != nil {
		deployments = util.Filter(deployments, deploymentFilter)
	}
	if len(deployments) == 0 && disableCreateFlow {
		return astrov1.Deployment{}, fmt.Errorf("%s %s", NoDeploymentInWSMsg, ws)
	}
	currentDeployment, err := SelectDeployment(deployments, "Select a Deployment")
	if err != nil {
		return astrov1.Deployment{}, err
	}
	if currentDeployment.Id == "" {
		// get latest runtime version
		airflowVersionClient := airflowversions.NewClient(httputil.NewHTTPClient(), false, false)
		runtimeVersion, err := airflowversions.GetDefaultImageTag(airflowVersionClient, "", "", false)
		if err != nil {
			return astrov1.Deployment{}, err
		}
		cicdEnforcement := disable
		// walk user through creating a deployment
		var coreDeploymentType astrov1.DeploymentType
		var dagDeploy string
		if !organization.IsOrgHosted() {
			coreDeploymentType = astrov1.DeploymentTypeHYBRID
			dagDeploy = disable
		} else {
			coreDeploymentType = astrov1.DeploymentTypeSTANDARD
			dagDeploy = enable
		}

		err = createDeployment("", ws, "", "", runtimeVersion, dagDeploy, CeleryExecutor, "azure", "", "", "", "disable", cicdEnforcement, "", "", "", "", "", coreDeploymentType, 0, 0, false, nil, nil, nil, astroV1Client, false, 0*time.Second)
		if err != nil {
			return astrov1.Deployment{}, err
		}
		// get a new deployment list
		deployments, err = ListDeployments(ws, "", astroV1Client)
		if err != nil {
			return astrov1.Deployment{}, err
		}
		currentDeployment, err = SelectDeployment(deployments, "Select which Deployment you want to update")
		if err != nil {
			return astrov1.Deployment{}, err
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
		deploymentURL = ctx.Domain + ":5000/" + workspaceID + "/deployments/" + deploymentID
	default:
		_, domain := domainutil.GetPRSubDomain(ctx.Domain)
		deploymentURL = "cloud." + domain + "/" + workspaceID + "/deployments/" + deploymentID
	}
	return deploymentURL, nil
}

func GetCoreCloudProvider(cloudProvider string) astrov1.GetDeploymentOptionsParamsCloudProvider {
	var coreCloudProvider astrov1.GetDeploymentOptionsParamsCloudProvider
	switch strings.ToUpper(cloudProvider) {
	case strings.ToUpper(awsCloud):
		coreCloudProvider = astrov1.GetDeploymentOptionsParamsCloudProvider("AWS")
	case strings.ToUpper(gcpCloud):
		coreCloudProvider = astrov1.GetDeploymentOptionsParamsCloudProvider("GCP")
	case strings.ToUpper(azureCloud):
		coreCloudProvider = astrov1.GetDeploymentOptionsParamsCloudProvider("AZURE")
	}
	return coreCloudProvider
}

func isDevelopmentDeployment(deployment astrov1.Deployment) bool { //nolint:gocritic
	return deployment.IsDevelopmentMode != nil && *deployment.IsDevelopmentMode
}

// ConvertWorkerQueues is a generic function to convert a slice of WorkerQueue to a slice of T (WorkerQueueRequest or HybridWorkerQueueRequest)
func ConvertWorkerQueues[T any](workerQueues []astrov1.WorkerQueue, convertFn func(astrov1.WorkerQueue) T) []T {
	result := make([]T, 0, len(workerQueues))
	for i := range workerQueues {
		result = append(result, convertFn(workerQueues[i]))
	}
	return result
}

func IsValidExecutor(executor, runtimeVersion, deploymentType string) bool {
	validExecutors := []string{
		KubeExecutor,
		CeleryExecutor,
		CELERY,
		KUBERNETES,
	}
	// runtime version is empty on update cmds
	if runtimeVersion == "" || (airflowversions.IsAirflow3(runtimeVersion) && !strings.EqualFold(deploymentType, "hybrid")) {
		validExecutors = append(validExecutors, AstroExecutor, ASTRO)
	}
	for _, e := range validExecutors {
		if strings.EqualFold(executor, e) {
			return true
		}
	}
	return false
}
