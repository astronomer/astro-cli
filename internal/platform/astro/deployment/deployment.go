package deployment

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	airflowversions "github.com/astronomer/astro-cli/airflow_versions"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/astrosession"
	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
	"github.com/astronomer/astro-cli/internal/platform/astro/organization"
	"github.com/astronomer/astro-cli/internal/platform/astro/pagination"
	"github.com/astronomer/astro-cli/internal/platform/astro/workspace"
	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/domainutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/output"
	"github.com/astronomer/astro-cli/pkg/picker"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/pkg/util"
)

var (
	errInvalidDeployment         = errors.New("the Deployment specified was not found in this workspace. Your account or API Key may not have access to the deployment specified")
	ErrInvalidDeploymentKey      = errors.New("invalid Deployment selected")
	errInvalidClusterKey         = errors.New("invalid Cluster selected")
	errInvalidRegionKey          = errors.New("invalid Region selected")
	ErrTimedOut                  = errors.New("timed out waiting for the Deployment to enter a Healthy state")
	ErrTimedOutHibernating       = errors.New("timed out waiting for the Deployment to hibernate")
	errAirflowNotAnswering       = errors.New("timed out waiting for the Deployment's Airflow API to answer, though Astro reports the Deployment healthy")
	errInvalidResourceRequest    = errors.New("invalid resource request")
	errNotADevelopmentDeployment = errors.New("the Deployment specified is not a development Deployment")
	errInvalidTokenName          = errors.New("no name provided for the deployment token. Retry with a valid name")

	// Monkey patched to write unit tests
	canCiCdDeploy = CanCiCdDeploy
	parseToken    = util.ParseAPIToken
	CleanOutput   = false

	// mayAsk reports whether this run may ask which Deployment to pick (input.CanAsk).
	// A var so a test can answer either way.
	mayAsk = input.CanAsk
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

// CanCiCdDeploy reports whether the stored token is one that may act past
// CI/CD enforcement: an API token, which carries permissions. It takes the
// token with or without its "Bearer " scheme. A token it cannot read — empty,
// logged out, not a JWT — is not one that can, so a caller refuses or warns
// exactly as it does for a user's token.
func CanCiCdDeploy(bearerToken string) bool {
	token := astrosession.Credential(bearerToken)
	if token == "" {
		return false
	}
	// Parse the token to peek at the custom claims
	claims, err := parseToken(token)
	if err != nil {
		// A warning, so stderr: stdout is the command's result.
		fmt.Fprintln(os.Stderr, "Unable to Parse Token")
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

// listData returns deployment list data for structured output
func listData(ws string, fromAllWorkspaces bool, astroV1Client astrov1.APIClient) (*DeploymentList, error) {
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

// ListWithFormat lists deployments with the specified output format
func ListWithFormat(ws string, fromAllWorkspaces bool, astroV1Client astrov1.APIClient, r output.Emitter) error {
	return output.PrintData(
		func() (*DeploymentList, error) { return listData(ws, fromAllWorkspaces, astroV1Client) },
		deploymentTableConfig(fromAllWorkspaces, ws), r,
	)
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Logs(deploymentID, ws, deploymentName, keyword string, logServer, logScheduler, logTriggerer, logWorkers, logDagProcessor bool, extraComponents []string, warnLogs, errorLogs, infoLogs bool, logCount int, astroV1Client astrov1.APIClient) (*LogsResult, error) {
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
		return nil, errors.New("cannot query for more than one log level and/or keyword at a time")
	}

	// get deployment
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, ToRead, nil, astroV1Client)
	if err != nil {
		return nil, err
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
	if logDagProcessor {
		componentSources = append(componentSources, astrov1.GetDeploymentLogsParamsSourcesDagProcessor)
	}
	for _, c := range extraComponents {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		componentSources = append(componentSources, astrov1.GetDeploymentLogsParamsSources(c))
	}
	if len(componentSources) == 0 {
		componentSources = append(componentSources, serverComponent, "scheduler", "triggerer", "worker", astrov1.GetDeploymentLogsParamsSourcesDagProcessor)
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
		deploymentLogs, err := getDeploymentLogs("", deploymentID, getDeploymentLogsParams, astroV1Client)
		if err != nil {
			return nil, err
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

	res := &LogsResult{DeploymentName: deployment.Name, Hours: timeRange / 3600, Entries: make([]LogEntry, 0, len(allResults))}
	for i := range allResults {
		res.Entries = append(res.Entries, LogEntry{Source: string(allResults[i].Source), Message: allResults[i].Raw})
	}
	return res, nil
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Create(name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, deploymentType astrov1.DeploymentType, schedulerAU, schedulerReplicas int, remoteExecutionEnabled bool, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogURLPattern *string, astroV1Client astrov1.APIClient, waitForStatus bool, waitTimeForDeployment time.Duration, progress io.Writer) (astrov1.Deployment, error) { //nolint:gocognit,gocyclo,gocritic,staticcheck // v1 complexity, refactor tracked separately
	var organizationID string
	var currentWorkspace astrov1.Workspace

	c, err := config.GetCurrentContext()
	if err != nil {
		return astrov1.Deployment{}, err
	}
	coreDeploymentType := astrov1.GetDeploymentOptionsParamsDeploymentType(deploymentType)
	coreCloudProvider := getCoreCloudProvider(cloudProvider)
	deploymentOptionsParams := astrov1.GetDeploymentOptionsParams{
		DeploymentType: &coreDeploymentType,
		CloudProvider:  &coreCloudProvider,
	}

	configOption, err := getDeploymentOptions("", deploymentOptionsParams, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
	}

	// validate workspace
	ws, err := workspace.GetWorkspaces(astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
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
		return astrov1.Deployment{}, fmt.Errorf(noWorkspaceMsg, workspaceID)
	}
	// Refused before the lines that lead up to the question, so a run that
	// cannot ask prints nothing about it.
	if name == "" {
		if err := input.MayAsk("Deployment name", input.AnsweredBy("--name")); err != nil {
			return astrov1.Deployment{}, err
		}
	}
	// A note, so progress: stdout is the Deployment the create made.
	fmt.Fprintf(progressTo(progress), "Current Workspace: %s\n\n", currentWorkspace.Name)

	// name input
	if name == "" {
		fmt.Fprintln(os.Stderr, "Please specify a name for your Deployment")
		name, err = input.Text(ansi.ForStderr().Bold("\nDeployment name: "), input.AnsweredBy("--name"))
		if err != nil {
			return astrov1.Deployment{}, err
		}
		if name == "" {
			return astrov1.Deployment{}, errors.New("you must give your Deployment a name")
		}
	}

	if region == "" && IsDeploymentStandard(deploymentType) {
		// select and validate region
		region, err = selectRegion(cloudProvider, region, astroV1Client)
		if err != nil {
			return astrov1.Deployment{}, err
		}
	}

	// select and validate cluster
	if !IsDeploymentStandard(deploymentType) {
		clusterID, err = selectCluster(clusterID, c.Organization, astroV1Client)
		if err != nil {
			return astrov1.Deployment{}, err
		}
	}

	if dagDeploy == enable {
		dagDeployEnabled = true
	} else {
		dagDeployEnabled = false
	}
	createDeploymentRequest := astrov1.CreateDeploymentRequest{}
	if IsDeploymentStandard(deploymentType) || IsDeploymentDedicated(deploymentType) {
		var highAvailabilityValue bool
		if highAvailability == enable {
			highAvailabilityValue = true
		} else {
			highAvailabilityValue = false
		}
		var developmentModeValue bool
		if developmentMode == enable {
			developmentModeValue = true
		} else {
			developmentModeValue = false
		}
		defaultTaskPodCPUPtr := createDefaultTaskPodCPU(defaultTaskPodCpu, remoteExecutionEnabled, &configOption)
		defaultTaskPodMemoryPtr := createDefaultTaskPodMemory(defaultTaskPodMemory, remoteExecutionEnabled, &configOption)
		resourceQuotaCPUPtr := createResourceQuotaCPU(resourceQuotaCpu, remoteExecutionEnabled, &configOption)
		resourceQuotaMemoryPtr := createResourceQuotaMemory(resourceQuotaMemory, remoteExecutionEnabled, &configOption)
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
			switch strings.ToLower(cloudProvider) {
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
			switch schedulerSize {
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeSMALL)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeMEDIUM)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeLARGE)
			case strings.ToLower(string(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)):
				standardDeploymentRequest.SchedulerSize = new(astrov1.CreateStandardDeploymentRequestSchedulerSizeEXTRALARGE)
			}
			if highAvailability == enable {
				standardDeploymentRequest.IsHighAvailability = new(true)
			}
			err := createDeploymentRequest.FromCreateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return astrov1.Deployment{}, err
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
			switch schedulerSize {
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
				return astrov1.Deployment{}, err
			}
		}
	}
	// build hybrid input
	if isDeploymentHybrid(deploymentType) {
		// Validate only the scheduler values the user actually supplied; the server fills in the rest.
		if schedulerAU != 0 && schedulerReplicas != 0 {
			if !validateHybridResources(schedulerAU, schedulerReplicas, configOption) {
				return astrov1.Deployment{}, errInvalidResourceRequest
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
			return astrov1.Deployment{}, err
		}
	}

	d, err := coreCreateDeployment(organizationID, createDeploymentRequest, astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	if waitForStatus {
		// The Deployment exists whether or not it becomes healthy in time, so
		// it is returned with the error: the caller still reports what was
		// made.
		if err := HealthPoll(progress, d.Id, SleepTime, TickNum, int(waitTimeForDeployment.Seconds()), astroV1Client); err != nil {
			return d, err
		}
	}
	return d, nil
}

func getDefaultNodePoolID(nodePools *[]astrov1.NodePool) string {
	if nodePools != nil && len(*nodePools) > 0 {
		return (*nodePools)[0].Id
	}
	return ""
}

// WriteCreated writes what `astro deployment create` prints in text for d, a
// Deployment it made in workspaceID: its row in the Deployment table, and
// where to reach it.
func WriteCreated(w io.Writer, workspaceID string, d *astrov1.Deployment) error {
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
	return tab.Print(w)
}

// WriteUpdated writes what `astro deployment update` prints in text for d, the
// Deployment as an update left it.
func WriteUpdated(w io.Writer, d *astrov1.Deployment) error {
	tab := newTableOut()
	deploymentToTableRow(tab, d, false)
	tab.SuccessMsg = "\n Successfully updated Deployment"
	return tab.Print(w)
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

func validateHybridResources(schedulerAU, schedulerReplicas int, _ astrov1.DeploymentOptions) bool { //nolint:gocritic // intentional in this shell code
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

func listClusterOptions(cloudProvider string, astroV1Client astrov1.APIClient) ([]astrov1.ClusterOptions, error) {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return nil, err
	}
	var provider astrov1.GetClusterOptionsParamsProvider
	switch strings.ToLower(cloudProvider) {
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
	// get all regions for the cloud provider
	options, err := listClusterOptions(cloudProvider, astroV1Client)
	if err != nil {
		return "", err
	}
	regions := options[0].Regions

	if region == "" {
		list := picker.List{
			Title:   "\nPlease select a Region for your Deployment:",
			Header:  []string{"REGION"},
			Ask:     []input.Option{input.About("a Region"), input.AnsweredBy("--region")},
			Invalid: errInvalidRegionKey,
		}
		for i := range regions {
			list.AddRow(false, regions[i].Name)
		}
		i, err := list.Pick(os.Stderr, os.Stdin)
		if err != nil {
			return "", err
		}
		region = regions[i].Name
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
	cs, err := organization.ListClusters(organizationID, astroV1Client)
	if err != nil {
		return "", err
	}
	// select cluster
	if clusterID == "" {
		list := picker.List{
			Title:   "\nPlease select a Cluster for your Deployment:",
			Header:  []string{"CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
			Ask:     []input.Option{input.About("a Cluster"), input.AnsweredBy("--cluster-id")},
			Invalid: errInvalidClusterKey,
		}
		for i := range cs {
			list.AddRow(false, cs[i].Name, string(cs[i].CloudProvider), cs[i].Id)
		}
		i, err := list.Pick(os.Stderr, os.Stdin)
		if err != nil {
			return "", err
		}
		clusterID = cs[i].Id
	}

	// validate cluster
	csID := ""
	for i := range cs {
		if clusterID == cs[i].Id {
			csID = cs[i].Id
		}
	}
	if csID == "" {
		return "", fmt.Errorf("unable to find specified Cluster %q in this Organization. Run \"astro organization cluster list\" to see its clusters", clusterID)
	}
	return clusterID, nil
}

// progressTo is where a wait, a warning or a note goes: w, or stderr when w
// is nil. It is never stdout, which carries a command's result and nothing
// else, in text as under --output json.
func progressTo(w io.Writer) io.Writer {
	if w == nil {
		return os.Stderr
	}
	return w
}

// HealthPoll waits for the Deployment to become healthy, writing its progress
// to progress (stderr when nil).
func HealthPoll(progress io.Writer, deploymentID string, sleepTime, tickNum, timeoutNum int, astroV1Client astrov1.APIClient) error {
	return HealthPollIn(progress, "", "", deploymentID, sleepTime, tickNum, timeoutNum, astroV1Client)
}

// HealthPollIn is HealthPoll for a Deployment in orgID, which need not be the
// current context's, with token authorizing the check that its Airflow
// answers. An empty orgID or token is the current context's.
func HealthPollIn(progress io.Writer, orgID, token, deploymentID string, sleepTime, tickNum, timeoutNum int, astroV1Client astrov1.APIClient) error {
	progress = progressTo(progress)
	fmt.Fprintf(progress, "\nWaiting for the deployment to become healthy…\n\nThis may take a few minutes\n")
	// considering hibernating as healthy state, since hibernation can only happen when the deployment is healthy.
	// This covers for the case when the deployment is created and straight away goes into hibernation.
	d, err := pollStatus(orgID, token, deploymentID, sleepTime, tickNum, timeoutNum, astroV1Client, ErrTimedOut,
		astrov1.DeploymentStatusHEALTHY, astrov1.DeploymentStatusHIBERNATING)
	if err != nil {
		return err
	}
	fmt.Fprintf(progress, "Deployment %s is now healthy\n", d.Name)
	return nil
}

// pollStatus waits sleepTime seconds, then checks the Deployment every
// tickNum seconds until it reports one of statuses, and returns timedOut when
// timeoutNum seconds pass first. A Deployment reporting HEALTHY also has to
// have an Airflow that answers, and errAirflowNotAnswering is the timeout when
// that was all that was missing.
func pollStatus(orgID, token, deploymentID string, sleepTime, tickNum, timeoutNum int, astroV1Client astrov1.APIClient, timedOut error, statuses ...astrov1.DeploymentStatus) (astrov1.Deployment, error) {
	time.Sleep(time.Duration(sleepTime) * time.Second)
	timeout := time.After(time.Duration(timeoutNum) * time.Second)
	ticker := time.NewTicker(time.Duration(tickNum) * time.Second)
	defer ticker.Stop()
	missing := timedOut
	for {
		select {
		case <-timeout:
			return astrov1.Deployment{}, missing
		case <-ticker.C:
			currentDeployment, err := GetDeploymentByID(orgID, deploymentID, astroV1Client)
			if err != nil {
				return astrov1.Deployment{}, err
			}
			missing = timedOut
			if !slices.Contains(statuses, currentDeployment.Status) {
				continue
			}
			if currentDeployment.Status != astrov1.DeploymentStatusHEALTHY || airflowAnswering(currentDeployment, token) {
				return currentDeployment, nil
			}
			missing = errAirflowNotAnswering
		}
	}
}

// airflowProbeTimeout bounds one airflowAnswering request. A request that
// times out counts as answering, like any other connection error.
const airflowProbeTimeout = 10 * time.Second

// airflowAnswering reports whether the Deployment's Airflow API answers with
// anything but the 502, 503 or 504 its ingress gives while no Airflow is
// behind it. Astro reports a Deployment HEALTHY some seconds before that
// after a wake-up. A Deployment with no Airflow API URL has nothing to probe.
// An empty token is the current context's.
var airflowAnswering = func(d astrov1.Deployment, token string) bool {
	apiURL := d.WebServerAirflowApiUrl
	if apiURL == "" {
		return true
	}
	if !strings.HasPrefix(apiURL, "http://") && !strings.HasPrefix(apiURL, "https://") {
		apiURL = "https://" + apiURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), airflowProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(apiURL, "/")+"/version", http.NoBody)
	if err != nil {
		return true
	}
	if token == "" {
		if c, err := config.GetCurrentContext(); err == nil {
			token = c.Token
		}
	}
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// A machine that cannot reach the Airflow host at all, such as a CI
		// runner allowed only the Astro API, would otherwise wait out the whole
		// --wait-time. Only the ingress's own answers mean "not up yet".
		return true
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return false
	}
	return true
}

// TODO (https://github.com/astronomer/astro-cli/issues/1709): move these input arguments to a struct, and drop the nolint
func Update(deploymentID, name, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability, developmentMode, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, workloadIdentity string, schedulerAU, schedulerReplicas int, wQueueList []astrov1.WorkerQueueRequest, hybridQueueList []astrov1.HybridWorkerQueueRequest, newEnvironmentVariables []astrov1.DeploymentEnvironmentVariableRequest, allowedIpAddressRanges *[]string, taskLogBucket *string, taskLogUrlPattern *string, force bool, astroV1Client astrov1.APIClient, notes io.Writer) (UpdateResult, error) { //nolint:gocognit,gocyclo,gocritic,staticcheck // v1 complexity, refactor tracked separately
	var confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, ToChange, nil, astroV1Client)
	if err != nil {
		return UpdateResult{}, err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return UpdateResult{}, err
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
			// Refused before the warning that leads up to the question, so a
			// run that cannot ask prints nothing about it.
			if err := input.MayAsk("Are you sure you want to continue?", input.AnsweredBy("--yes")); err != nil {
				return UpdateResult{}, err
			}
			pw := progressTo(notes)
			fmt.Fprintf(pw, "\n%s You are trying to update the dag deploy setting with ci-cd enforcement enabled. Once the setting is updated, you will not be able to deploy your dags using the CLI. Until you deploy your dags, dags will not be visible in the UI nor will new tasks start."+
				"\nAfter the setting is updated, either disable cicd enforcement and then deploy your dags OR deploy your dags via CICD or using API Tokens.", ansi.WarningLabel(pw))
			if err := input.ConfirmOrAbort("\n\nAre you sure you want to continue?", input.AnsweredBy("--yes")); err != nil {
				return UpdateResult{}, err
			}
		}
	}
	// determine isDagDeployEnabled
	switch dagDeploy {
	case enable:
		if currentDeployment.IsDagDeployEnabled {
			fmt.Println("\nDag deploys are already enabled for this Deployment. Your Dags will continue to run as scheduled.")
			return UpdateResult{Deployment: currentDeployment}, nil
		}

		fmt.Fprintf(progressTo(notes), "\nYou enabled Dag-only deploys for this Deployment. Running tasks are not interrupted but new tasks will not be scheduled."+
			"\nRun astro deploy --dags to complete enabling this feature and resume your Dags. It may take a few minutes for the Airflow UI to update..\n\n")
		dagDeployEnabled = true
	case disable:
		if !currentDeployment.IsDagDeployEnabled {
			fmt.Println("\nDag-only deploys is already disabled for this deployment.")
			return UpdateResult{Deployment: currentDeployment}, nil
		}
		if config.CFG.ShowWarnings.GetBool() {
			pw := progressTo(notes)
			fmt.Fprintf(pw, "\n%s This command will disable Dag-only deploys for this Deployment. Running tasks will not be interrupted, but new tasks will not be scheduled"+
				"\nRun astro deploy after this command to restart your Dags. It may take a few minutes for the Airflow UI to update.", ansi.WarningLabel(pw))
			confirmWithUser = true
		}
		dagDeployEnabled = false
	case "":
		dagDeployEnabled = currentDeployment.IsDagDeployEnabled
	}
	configOption, err := getDeploymentOptions("", astrov1.GetDeploymentOptionsParams{}, astroV1Client)
	if err != nil {
		return UpdateResult{}, err
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
		deploymentEnvironmentVariablesRequest = newEnvironmentVariables
	}
	if deploymentEnvironmentVariablesRequest == nil {
		deploymentEnvironmentVariablesRequest = []astrov1.DeploymentEnvironmentVariableRequest{}
	}
	if IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type) {
		var workerQueuesRequest []astrov1.UpdateWorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = convertWorkerQueues(workerQueues, func(q astrov1.WorkerQueue) astrov1.UpdateWorkerQueueRequest {
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
			workerQueuesRequest = convertCreateQueuesToUpdate(wQueueList)
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
			return UpdateResult{}, errors.New("Invalid --high-availability value")
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
			return UpdateResult{}, errors.New("Invalid --development-mode value")
		}
		defaultTaskPodCPUPtr := updateDefaultTaskPodCPU(defaultTaskPodCpu, &currentDeployment, &configOption)
		defaultTaskPodMemoryPtr := updateDefaultTaskPodMemory(defaultTaskPodMemory, &currentDeployment, &configOption)
		resourceQuotaCPUPtr := updateResourceQuotaCPU(resourceQuotaCpu, &currentDeployment)
		resourceQuotaMemoryPtr := updateResourceQuotaMemory(resourceQuotaMemory, &currentDeployment)
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
				return UpdateResult{}, err
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
				return UpdateResult{}, err
			}
		}
	}
	if !(IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type)) {
		var workerQueuesRequest []astrov1.UpdateWorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			workerQueuesRequest = convertWorkerQueues(workerQueues, func(q astrov1.WorkerQueue) astrov1.UpdateWorkerQueueRequest {
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
			workerQueuesRequest = convertHybridQueuesToUpdate(hybridQueueList)
		}
		if schedulerAU == 0 {
			schedulerAU = *currentDeployment.SchedulerAu
		}
		if schedulerReplicas == 0 {
			schedulerReplicas = currentDeployment.SchedulerReplicas
		}
		// Workload identity uses PATCH semantics on the server: a nil pointer
		// omits the field, leaving the deployment's current value (user-set
		// or platform-managed default) untouched. Do NOT fall back to
		// currentDeployment.EffectiveWorkloadIdentity here — that would
		// silently pin the platform-managed default as if it were
		// user-configured.
		var deplWorkloadIdentity *string
		if workloadIdentity != "" {
			deplWorkloadIdentity = &workloadIdentity
		}
		// validate au resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return UpdateResult{}, errInvalidResourceRequest
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
			WorkloadIdentity:     deplWorkloadIdentity,
		}
		cluster, err := GetClusterByID("", *currentDeployment.ClusterId, astroV1Client)
		if err != nil {
			return UpdateResult{}, err
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
			return UpdateResult{}, err
		}
	}

	// confirm changes with user only if force=false
	if !force {
		if confirmWithUser {
			if err := input.ConfirmOrAbort(
				fmt.Sprintf("\nAre you sure you want to update the %s Deployment?", ansi.ForStderr().Bold(currentDeployment.Name)), input.AnsweredBy("--yes")); err != nil {
				return UpdateResult{}, err
			}
		}
	}
	// update deployment
	d, err := coreUpdateDeployment(c.Organization, currentDeployment.Id, updateDeploymentRequest, astroV1Client)
	if err != nil {
		return UpdateResult{}, err
	}
	if d.Id == "" {
		// An update the API answered without a Deployment changed nothing
		// anyone can see, so it is a failure, not a success with no table.
		return UpdateResult{}, fmt.Errorf("something went wrong: Deployment %s was not updated", currentDeployment.Name)
	}
	return UpdateResult{Deployment: d, Updated: true}, nil
}

func convertHybridQueuesToUpdate(in []astrov1.HybridWorkerQueueRequest) []astrov1.UpdateWorkerQueueRequest {
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

func convertCreateQueuesToUpdate(in []astrov1.WorkerQueueRequest) []astrov1.UpdateWorkerQueueRequest {
	out := make([]astrov1.UpdateWorkerQueueRequest, len(in))
	for i := range in {
		machine := astrov1.UpdateWorkerQueueRequestAstroMachine(in[i].AstroMachine)
		out[i] = astrov1.UpdateWorkerQueueRequest{
			AstroMachine:        &machine,
			Id:                  in[i].Id,
			IsDefault:           in[i].IsDefault,
			MaxWorkerCount:      in[i].MaxWorkerCount,
			MinWorkerCount:      in[i].MinWorkerCount,
			Name:                in[i].Name,
			PodEphemeralStorage: in[i].PodEphemeralStorage,
			WorkerConcurrency:   in[i].WorkerConcurrency,
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

func createDefaultTaskPodCPU(defaultTaskPodCPU string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodCPU != "" {
		return &defaultTaskPodCPU
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.DefaultPodSize.Cpu.Default
}

func createDefaultTaskPodMemory(defaultTaskPodMemory string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if defaultTaskPodMemory != "" {
		return &defaultTaskPodMemory
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.DefaultPodSize.Memory.Default
}

func createResourceQuotaCPU(resourceQuotaCPU string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if resourceQuotaCPU != "" {
		return &resourceQuotaCPU
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.ResourceQuota.Cpu.Default
}

func createResourceQuotaMemory(resourceQuotaMemory string, isRemoteExecutionEnabled bool, configOption *astrov1.DeploymentOptions) *string {
	if resourceQuotaMemory != "" {
		return &resourceQuotaMemory
	}
	if isRemoteExecutionEnabled {
		return nil
	}
	return &configOption.ResourceQuotas.ResourceQuota.Memory.Default
}

func updateDefaultTaskPodCPU(defaultTaskPodCPU string, deployment *astrov1.Deployment, configOption *astrov1.DeploymentOptions) *string {
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

func updateDefaultTaskPodMemory(defaultTaskPodMemory string, deployment *astrov1.Deployment, configOption *astrov1.DeploymentOptions) *string {
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

func updateResourceQuotaCPU(resourceQuotaCPU string, deployment *astrov1.Deployment) *string {
	if resourceQuotaCPU != "" {
		return &resourceQuotaCPU
	}
	return deployment.ResourceQuotaCpu
}

func updateResourceQuotaMemory(resourceQuotaMemory string, deployment *astrov1.Deployment) *string {
	if resourceQuotaMemory != "" {
		return &resourceQuotaMemory
	}
	return deployment.ResourceQuotaMemory
}

var (
	// errDeleteNeedsTarget is a delete with no Deployment named and nobody to
	// pick one. --yes answers the confirmation, never which Deployment.
	errDeleteNeedsTarget = errors.New("astro deployment delete needs the Deployment to delete, and this run cannot ask which one: pass --deployment, or the Deployment id as the argument. --yes skips the confirmation and never chooses the Deployment")
	// errDeleteNotConfirmed is a confirmation answered with anything but the
	// Deployment's name: declined, as every declined confirmation is
	// (input.ErrAborted), saying why.
	errDeleteNotConfirmed = fmt.Errorf("%w: the name typed does not match the Deployment's name", input.ErrAborted)
)

// Delete deletes a Deployment, after the person types its name unless
// forceDelete. A run that cannot ask must name the Deployment: it is never
// picked for it, whatever forceDelete says.
func Delete(deploymentID, ws, deploymentName string, forceDelete bool, astroV1Client astrov1.APIClient) (*Removal, error) {
	if deploymentID == "" && deploymentName == "" && !mayAsk() {
		return nil, input.Required(errDeleteNeedsTarget)
	}

	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, ToPick, nil, astroV1Client)
	if err != nil {
		return nil, err
	}

	// A delete cannot be undone, so the confirmation is the Deployment's
	// name typed out, not a y that a habit or a pipe can supply.
	// The name is shown as it is, unescaped, so what is shown is what to type.
	if !forceDelete {
		typed, err := input.Text(
			fmt.Sprintf("\nThis permanently deletes the Deployment %s and all of its data.\n", ansi.ForStderr().Bold(currentDeployment.Name))+
				`To confirm, type the Deployment name "`+currentDeployment.Name+`": `,
			input.About("the Deployment's name, to confirm the delete"), input.AnsweredBy("--yes"))
		if err != nil {
			return nil, err
		}
		if typed != currentDeployment.Name {
			return nil, fmt.Errorf("%w (\"%s\"): the Deployment was not deleted", errDeleteNotConfirmed, currentDeployment.Name)
		}
	}

	err = coreDeleteDeployment(currentDeployment.OrganizationId, currentDeployment.Id, astroV1Client)
	if err != nil {
		return nil, err
	}
	return &Removal{
		DeploymentID: currentDeployment.Id,
		Name:         currentDeployment.Name,
		WorkspaceID:  currentDeployment.WorkspaceId,
		Action:       actionDeleted,
	}, nil
}

// UpdateDeploymentHibernationOverride sets a development Deployment to
// hibernate, or to stay awake, until overrideUntil, or until the override is
// removed when it is nil. It asks first unless force, and returns the
// override the Deployment now has; a declined question is input.ErrAborted.
// WaitForHibernationOverride waits for the Deployment to get there.
func UpdateDeploymentHibernationOverride(deploymentID, ws, deploymentName string, isHibernating bool, overrideUntil *time.Time, force bool, astroV1Client astrov1.APIClient) (*HibernationResult, error) {
	// Set wording based on the hibernation action
	var action string
	if isHibernating {
		action = "hibernate"
	} else {
		action = "wake up"
	}

	// get deployment
	currentDeployment, err := getDevelopmentDeployment(ws, deploymentID, deploymentName, astroV1Client)
	if err != nil {
		return nil, err
	}

	if currentDeployment.IsDevelopmentMode == nil || !*currentDeployment.IsDevelopmentMode {
		return nil, errNotADevelopmentDeployment
	}

	// prompt user
	if !force {
		if err := input.ConfirmOrAbort(
			fmt.Sprintf("\nAre you sure you want to override to %s for %s Deployment?", ansi.ForStderr().Bold(action), ansi.ForStderr().Bold(currentDeployment.Name)), input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}

	overrideDeploymentHibernationBody := astrov1.OverrideDeploymentHibernationBody{
		IsHibernating: &isHibernating,
		OverrideUntil: overrideUntil,
	}

	deploymentHibernationOverride, err := coreUpdateDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, overrideDeploymentHibernationBody, astroV1Client)
	if err != nil {
		return nil, err
	}

	// The override as the API reports it, falling back to what was asked for
	// where the answer leaves a field out.
	override := HibernationOverride{IsHibernating: isHibernating, OverrideUntil: deploymentHibernationOverride.OverrideUntil}
	if deploymentHibernationOverride.IsHibernating != nil {
		override.IsHibernating = *deploymentHibernationOverride.IsHibernating
	}
	return &HibernationResult{
		DeploymentID: currentDeployment.Id,
		Name:         currentDeployment.Name,
		Override:     &override,
	}, nil
}

// WaitForHibernationOverride waits until the Deployment reports the state an
// override asked for: hibernating, or healthy again. Unlike HealthPoll it
// has no initial sleep, which would be longer than a whole wake-up.
// Its progress goes to progress (stderr when nil).
func WaitForHibernationOverride(progress io.Writer, deploymentID string, isHibernating bool, waitTime time.Duration, astroV1Client astrov1.APIClient) error {
	progress = progressTo(progress)
	status, timedOut, waiting, done := astrov1.DeploymentStatusHEALTHY, ErrTimedOut, "wake up", "awake"
	if isHibernating {
		status, timedOut, waiting, done = astrov1.DeploymentStatusHIBERNATING, ErrTimedOutHibernating, "hibernate", "hibernating"
	}
	fmt.Fprintf(progress, "\nWaiting for the Deployment to %s…\n", waiting)
	d, err := pollStatus("", "", deploymentID, 0, TickNum, int(waitTime.Seconds()), astroV1Client, timedOut, status)
	if err != nil {
		return err
	}
	fmt.Fprintf(progress, "Deployment %s is now %s\n", d.Name, done)
	return nil
}

// DeleteDeploymentHibernationOverride removes a development Deployment's
// hibernation override, so its schedule applies again. It asks first unless
// force, and returns the Deployment with no override; a declined question
// is input.ErrAborted.
func DeleteDeploymentHibernationOverride(deploymentID, ws, deploymentName string, force bool, astroV1Client astrov1.APIClient) (*HibernationResult, error) {
	// get deployment
	currentDeployment, err := getDevelopmentDeployment(ws, deploymentID, deploymentName, astroV1Client)
	if err != nil {
		return nil, err
	}

	if currentDeployment.IsDevelopmentMode == nil || !*currentDeployment.IsDevelopmentMode {
		return nil, errNotADevelopmentDeployment
	}

	// prompt user
	if !force {
		if err := input.ConfirmOrAbort(
			fmt.Sprintf("\nAre you sure you want to remove the hibernation override and resume schedule for %s Deployment?", ansi.ForStderr().Bold(currentDeployment.Name)), input.AnsweredBy("--yes")); err != nil {
			return nil, err
		}
	}

	err = coreDeleteDeploymentHibernationOverride(currentDeployment.OrganizationId, currentDeployment.Id, astroV1Client)
	if err != nil {
		return nil, err
	}
	return &HibernationResult{DeploymentID: currentDeployment.Id, Name: currentDeployment.Name}, nil
}

func IsDeploymentStandard(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeSTANDARD
}

func IsDeploymentDedicated(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeDEDICATED
}

func isDeploymentHybrid(deploymentType astrov1.DeploymentType) bool {
	return deploymentType == astrov1.DeploymentTypeHYBRID
}

func IsRemoteExecutionEnabled(deployment *astrov1.Deployment) bool {
	return deployment.RemoteExecution != nil && deployment.RemoteExecution.Enabled
}

// coreUpdateDeploymentHibernationOverride updates a deployment hibernation override with the core API
//
//nolint:dupl // the duplication is acceptable here
var coreUpdateDeploymentHibernationOverride = func(orgID, deploymentID string, overrideDeploymentHibernationRequest astrov1.OverrideDeploymentHibernationBody, astroV1Client astrov1.APIClient) (astrov1.DeploymentHibernationOverride, error) {
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

// coreDeleteDeploymentHibernationOverride deletes a deployment hibernation override with the core API
var coreDeleteDeploymentHibernationOverride = func(orgID, deploymentID string, astroV1Client astrov1.APIClient) error {
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

// ListDeployments returns every Deployment in the given Workspace (or the whole
// Organization when ws is empty), paging through the API as needed.
var ListDeployments = func(ws, orgID string, astroV1Client astrov1.APIClient) ([]astrov1.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return []astrov1.Deployment{}, err
		}
		orgID = c.Organization
	}
	return pagination.Collect("deployments", func(offset int) ([]astrov1.Deployment, int, error) {
		params := &astrov1.ListDeploymentsParams{Limit: &listLimit, Offset: &offset}
		if ws != "" {
			params.WorkspaceIds = &[]string{ws}
		}
		resp, err := astroV1Client.ListDeploymentsWithResponse(context.Background(), orgID, params)
		if err != nil {
			return nil, 0, err
		}
		if err := astrov1.NormalizeAPIError(resp.HTTPResponse, resp.Body); err != nil {
			return nil, 0, err
		}
		return resp.JSON200.Deployments, resp.JSON200.TotalCount, nil
	})
}

//nolint:dupl // the duplication is acceptable here
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

//nolint:dupl // the duplication is acceptable here
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

// coreCreateDeployment creates a deployment with the core API
var coreCreateDeployment = func(orgID string, createDeploymentRequest astrov1.CreateDeploymentJSONRequestBody, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
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

// coreUpdateDeployment updates a deployment with the core API
//
//nolint:dupl // the duplication is acceptable here
var coreUpdateDeployment = func(orgID, deploymentID string, updateDeploymentRequest astrov1.UpdateDeploymentJSONRequestBody, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
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

// coreDeleteDeployment deletes a deployment with the core API
var coreDeleteDeployment = func(orgID, deploymentID string, astroV1Client astrov1.APIClient) error {
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

var getDeploymentOptions = func(orgID string, deploymentOptionsParams astrov1.GetDeploymentOptionsParams, astroV1Client astrov1.APIClient) (astrov1.DeploymentOptions, error) {
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

var getDeploymentLogs = func(orgID string, deploymentID string, getDeploymentLogsParams astrov1.GetDeploymentLogsParams, astroV1Client astrov1.APIClient) (astrov1.DeploymentLog, error) {
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

// SelectDeployment asks which of deployments to act on, with a numbered
// table on stderr. It never answers for the person: GetDeployment decides
// when a run may take a workspace's only Deployment without asking.
// deployments is never empty.
var SelectDeployment = func(deployments []astrov1.Deployment, message string) (astrov1.Deployment, error) {
	sort.Slice(deployments, func(i, j int) bool {
		return deployments[i].CreatedAt.Before(deployments[j].CreatedAt)
	})

	list := picker.List{
		Title:   message,
		Header:  []string{"DEPLOYMENT NAME", "RELEASE NAME", "DEPLOYMENT ID", "DAG DEPLOY ENABLED"},
		Ask:     []input.Option{input.About("a Deployment"), input.AnsweredBy("--deployment")},
		Invalid: ErrInvalidDeploymentKey,
	}
	for i := range deployments {
		list.AddRow(false, deployments[i].Name, deployments[i].Namespace, deployments[i].Id, strconv.FormatBool(deployments[i].IsDagDeployEnabled))
	}
	i, err := list.Pick(os.Stderr, os.Stdin)
	if err != nil {
		return astrov1.Deployment{}, err
	}
	return deployments[i], nil
}

// Use is what a caller of GetDeployment will do with the Deployment it gets,
// which decides whether a run that named none may have one chosen for it.
type Use int

const (
	// ToRead only reads the Deployment (inspect, logs, variable list). With
	// no Deployment named, a workspace's only one is taken without asking
	// when the run is authenticated by an API token or deployment.auto_select
	// is on, as 1.x did for every command.
	ToRead Use = iota
	// ToChange changes the Deployment or ships to it. It acts only on a
	// Deployment the run named, the person picked, or the run's
	// Deployment-scoped ASTRO_API_TOKEN is for (see UseTokenDeployment);
	// never on one taken because it was the only one.
	ToChange
	// ToPick is ToChange for a Deployment that is always named or picked,
	// never taken from the token either: a delete, since what it removes is
	// on the command line, and a link, which the person writes into the
	// project.
	ToPick
)

// tokenDeployment is the Deployment this process's ASTRO_API_TOKEN is scoped
// to, or "" (see UseTokenDeployment).
var tokenDeployment atomic.Value

// UseTokenDeployment records, for this process only, the Deployment the run's
// ASTRO_API_TOKEN is scoped to: "" for a token scoped to a workspace or an
// organization, or for none. A run that names no Deployment acts on it (see
// Use), since it is the one Deployment such a token can act on.
func UseTokenDeployment(id string) { tokenDeployment.Store(id) }

// TokenDeployment is the Deployment UseTokenDeployment recorded, or "".
func TokenDeployment() string {
	id, _ := tokenDeployment.Load().(string)
	return id
}

// ErrTokenDeploymentNotFound is a run that named no Deployment, whose
// ASTRO_API_TOKEN is scoped to a Deployment the workspace does not list.
var ErrTokenDeploymentNotFound = errors.New("the Deployment ASTRO_API_TOKEN is scoped to was not found")

// fromToken is the Deployment of deployments, listed in ws, that the run's
// Deployment-scoped token is for, and whether the token names one. One the
// workspace does not list is an error, as is one filter leaves out: nothing
// else is picked in its place.
func fromToken(ws string, deployments []astrov1.Deployment, filter func(astrov1.Deployment) bool) (astrov1.Deployment, bool, error) {
	id := TokenDeployment()
	if id == "" {
		return astrov1.Deployment{}, false, nil
	}
	i := slices.IndexFunc(deployments, func(d astrov1.Deployment) bool { return d.Id == id })
	if i < 0 {
		return astrov1.Deployment{}, true, fmt.Errorf("%w: %s is not in workspace %s. Pass --deployment, or --workspace with its workspace", ErrTokenDeploymentNotFound, id, ws)
	}
	d := deployments[i]
	if filter != nil && !filter(d) {
		return astrov1.Deployment{}, true, &NoneEligibleError{Workspace: ws, Token: d.Name + " (" + d.Id + ")"}
	}
	NoteTokenDeployment(os.Stderr, d.Name+" ("+d.Id+")")
	return d, true, nil
}

// NoteTokenDeployment writes to w, the run's stderr, that the run acts on
// deployment (as it is to be shown: a name and id, or an id) because its
// ASTRO_API_TOKEN is scoped to it. Every command that takes the token's
// Deployment says so in these words.
func NoteTokenDeployment(w io.Writer, deployment string) {
	ansi.Note(w, "Using Deployment "+deployment+", the one ASTRO_API_TOKEN is scoped to")
}

// NoDeploymentsError is a command that needed a Deployment picked in a
// workspace that has none. Nothing is created in its place.
type NoDeploymentsError struct {
	// Workspace is the workspace that has none.
	Workspace string
}

func (e *NoDeploymentsError) Error() string {
	return fmt.Sprintf("%s %s: create one with astro deployment create", NoDeploymentInWSMsg, e.Workspace)
}

// NoneEligibleError is a workspace with Deployments, none of which the
// command can act on (hibernate takes development Deployments alone, and a
// link picker leaves out what the project already links).
type NoneEligibleError struct {
	// Workspace is the workspace looked in.
	Workspace string
	// Token, when set, is the Deployment the run's ASTRO_API_TOKEN is scoped
	// to (its name and id), which the command cannot act on.
	Token string
	// Need, when set, says what the command can act on ("a development
	// Deployment"); the message says so instead of "one this command can act
	// on".
	Need string
}

func (e *NoneEligibleError) Error() string {
	need := cmp.Or(e.Need, "one this command can act on")
	if e.Token != "" {
		return fmt.Sprintf("the Deployment ASTRO_API_TOKEN is scoped to, %s, is not %s", e.Token, need)
	}
	return fmt.Sprintf("none of the Deployments in workspace %s is %s", e.Workspace, need)
}

// TargetsDisagreeError is a run that named its Deployment both by id and by
// name, where the two are different Deployments. It is a usage error: which
// one the run meant is not something to guess.
type TargetsDisagreeError struct {
	// ID and Name are what the run gave; IDsName is the name of the
	// Deployment with that id.
	ID, Name, IDsName string
}

func (e *TargetsDisagreeError) Error() string {
	return fmt.Sprintf("the Deployment id %s and the name %q name different Deployments (%s is %q): pass one of them", e.ID, e.Name, e.ID, e.IDsName)
}

// IsUsageError marks TargetsDisagreeError as a usage error for
// cliout.IsUsage, which this package cannot import.
func (e *TargetsDisagreeError) IsUsageError() bool { return true }

// mayAutoSelect reports whether this run is set up to take a workspace's only
// Deployment without asking: authenticated by an API token, or with
// deployment.auto_select on.
func mayAutoSelect() bool {
	return astrosession.HasAPIToken() || os.Getenv("ASTRONOMER_KEY_ID") != "" || os.Getenv("ASTRONOMER_KEY_SECRET") != "" || config.CFG.AutoSelect.GetBool()
}

// GetDeployment is the Deployment in workspace ws that deploymentID or
// deploymentName names, or, with neither, the one the person picks. Given
// both, they must name the same Deployment. selectionFilter, when set, limits
// what a name or a pick can choose (hibernate takes development Deployments
// alone). use says what the caller will do with it (see Use). It never
// creates a Deployment: a workspace with none to pick from is a
// *NoDeploymentsError.
func GetDeployment(ws, deploymentID, deploymentName string, use Use, selectionFilter func(deployment astrov1.Deployment) bool, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	deployments, err := ListDeployments(ws, "", astroV1Client)
	if err != nil {
		return astrov1.Deployment{}, errors.Wrap(err, errInvalidDeployment.Error())
	}

	var id string
	switch {
	case deploymentID != "":
		i := slices.IndexFunc(deployments, func(d astrov1.Deployment) bool { return d.Id == deploymentID })
		if i < 0 {
			return astrov1.Deployment{}, errInvalidDeployment
		}
		if deploymentName != "" && deployments[i].Name != deploymentName {
			return astrov1.Deployment{}, &TargetsDisagreeError{ID: deploymentID, Name: deploymentName, IDsName: deployments[i].Name}
		}
		id = deploymentID
	case deploymentName != "":
		if id, err = oneNamed(deployments, deploymentName, ws, selectionFilter); err != nil {
			return astrov1.Deployment{}, err
		}
	default:
		picked, err := deploymentSelectionProcess(ws, deployments, selectionFilter, use)
		if err != nil {
			return astrov1.Deployment{}, err
		}
		id = picked.Id
	}
	return GetDeploymentByID("", id, astroV1Client)
}

// deploymentSelectionProcess is the Deployment, of those in ws that filter
// leaves, that the run's Deployment-scoped ASTRO_API_TOKEN is for (except for
// a delete), else for a ToRead caller the only one when mayAutoSelect, else
// the one the person picks.
func deploymentSelectionProcess(ws string, deployments []astrov1.Deployment, filter func(deployment astrov1.Deployment) bool, use Use) (astrov1.Deployment, error) {
	if len(deployments) == 0 {
		return astrov1.Deployment{}, &NoDeploymentsError{Workspace: ws}
	}
	if use != ToPick {
		if d, ok, err := fromToken(ws, deployments, filter); ok || err != nil {
			return d, err
		}
	}
	if filter != nil {
		deployments = util.Filter(deployments, filter)
	}
	if len(deployments) == 0 {
		return astrov1.Deployment{}, &NoneEligibleError{Workspace: ws}
	}
	if len(deployments) == 1 && mayAutoSelect() {
		if use == ToRead {
			if !CleanOutput {
				fmt.Fprintf(os.Stderr, "Only one Deployment was found. Using the following Deployment by default: \n\n Deployment Name: %s\n Deployment ID: %s\n",
					ansi.ForStderr().Bold(deployments[0].Name), ansi.ForStderr().Bold(deployments[0].Id))
			}
			return deployments[0], nil
		}
		// 1.x took it here for every command. A change waits for someone to
		// pick it, and a run with nobody to ask is told to name it.
		if !mayAsk() {
			return astrov1.Deployment{}, input.Required(fmt.Errorf("%s (%s) is the only Deployment in workspace %s this command can act on, but a command that changes a Deployment does not choose one by itself: pass --deployment", deployments[0].Name, deployments[0].Id, ws))
		}
	}
	return SelectDeployment(deployments, "Select a Deployment")
}

// GetDeploymentURL takes a deploymentID, WorkspaceID as parameters
// and returns a deploymentURL
func GetDeploymentURL(deploymentID, workspaceID string) (string, error) {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return "", err
	}
	return DeploymentURLOn(ctx.Domain, deploymentID, workspaceID), nil
}

// DeploymentURLOn is GetDeploymentURL for a Deployment on domain, which need
// not be the current context's.
func DeploymentURLOn(domain, deploymentID, workspaceID string) string {
	if domain == domainutil.LocalDomain {
		return domain + ":5000/" + workspaceID + "/deployments/" + deploymentID
	}
	_, domain = domainutil.GetPRSubDomain(domain)
	return "cloud." + domain + "/" + workspaceID + "/deployments/" + deploymentID
}

func getCoreCloudProvider(cloudProvider string) astrov1.GetDeploymentOptionsParamsCloudProvider {
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

func isDevelopmentDeployment(deployment astrov1.Deployment) bool { //nolint:gocritic // intentional in this shell code
	return deployment.IsDevelopmentMode != nil && *deployment.IsDevelopmentMode
}

// getDevelopmentDeployment is GetDeployment for hibernate and wake-up, which
// act on development Deployments alone, and say so when there is none.
func getDevelopmentDeployment(ws, deploymentID, deploymentName string, astroV1Client astrov1.APIClient) (astrov1.Deployment, error) {
	d, err := GetDeployment(ws, deploymentID, deploymentName, ToChange, isDevelopmentDeployment, astroV1Client)
	var none *NoneEligibleError
	if errors.As(err, &none) {
		none.Need = "a development Deployment, which hibernate and wake-up act on"
	}
	return d, err
}

// convertWorkerQueues is a generic function to convert a slice of WorkerQueue to a slice of T (WorkerQueueRequest or HybridWorkerQueueRequest)
func convertWorkerQueues[T any](workerQueues []astrov1.WorkerQueue, convertFn func(astrov1.WorkerQueue) T) []T {
	result := make([]T, 0, len(workerQueues))
	for i := range workerQueues {
		result = append(result, convertFn(workerQueues[i]))
	}
	return result
}

// SchedulerSizes are the scheduler sizes a hosted Deployment takes, spelled as
// --scheduler-size and a deployment file's scheduler_size accept them: the
// API's enum, lowercased.
var SchedulerSizes = []string{
	strings.ToLower(string(astrov1.DeploymentSchedulerSizeSMALL)),
	strings.ToLower(string(astrov1.DeploymentSchedulerSizeMEDIUM)),
	strings.ToLower(string(astrov1.DeploymentSchedulerSizeLARGE)),
	strings.ToLower(string(astrov1.DeploymentSchedulerSizeEXTRALARGE)),
}

// NormalizeSchedulerSize returns size as Create and Update match it, in
// lowercase, and whether it is one of SchedulerSizes. The match ignores case,
// as an executor's or a cloud provider's does. An unknown size is the
// caller's to refuse: Create and Update would send no size for it.
func NormalizeSchedulerSize(size string) (string, bool) {
	lower := strings.ToLower(size)
	return lower, slices.Contains(SchedulerSizes, lower)
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
