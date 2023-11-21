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

// type CreateDeploymentInput{

// }

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
func List(ws string, all bool, platformCoreClient astroplatformcore.CoreClient, out io.Writer) error {
	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	tab := newTableOut()

	if all {
		ws = ""
		tab = newTableOutAll()
	}
	deployments, err := CoreGetDeployments(ws, c.Organization, platformCoreClient)
	if err != nil {
		return err
	}

	if len(deployments) == 0 {
		fmt.Printf("No Deployments found in workspace %s\n", ansi.Bold(ws))
		return nil
	}

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Name > deployments[j].Name })

	// Build rows
	for i := range deployments {
		d := deployments[i]
		// change to cluster name
		var clusterName string
		if !IsDeploymentStandard(*d.Type) {
			clusterName = *d.ClusterName // *d.ClusterName
		} else {
			clusterName = notApplicable
		}
		runtimeVersionText := d.RuntimeVersion // + " (based on Airflow " + d.RuntimeRelease.AirflowVersion + ")"
		releaseName := d.Namespace
		// change to workspace name
		workspaceID := d.WorkspaceId
		region := notApplicable
		cloudProvider := notApplicable
		if IsDeploymentStandard(*d.Type) || IsDeploymentDedicated(*d.Type) {
			// region := d.Region
			// cloudProvider := d.CloudProvider
			region = *d.Region
			cloudProvider = *d.CloudProvider
		}
		if all {
			tab.AddRow([]string{d.Name, workspaceID, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type)}, false)
		} else {
			tab.AddRow([]string{d.Name, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type)}, false)
		}
	}

	return tab.Print(out)
}

func Logs(deploymentID, ws, deploymentName string, warnLogs, errorLogs, infoLogs bool, logCount int, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) error {
	var logLevel string
	var i int
	// log level
	if warnLogs {
		logLevel = "WARN"
		i += 1
	}
	if errorLogs {
		logLevel = "ERROR"
		i += 1
	}
	if infoLogs {
		logLevel = "INFO"
		i += 1
	}
	if i > 1 {
		return errors.New("cannot query for more than one log level at a time")
	}

	// get deployment
	deployment, err := GetDeployment(ws, deploymentID, deploymentName, false, platformCoreClient, nil)
	if err != nil {
		return err
	}

	deploymentID = deployment.Id
	timeRange := 86400
	offset := 0
	getDeploymentLogsParams := astrocore.GetDeploymentLogsParams{
		Sources: []astrocore.GetDeploymentLogsParamsSources{
			"scheduler",
			"webserver",
			"triggerer",
			"worker",
		},
		MaxNumResults: &logCount,
		Range:         &timeRange,
		Offset:        &offset,
	}
	if logLevel != "" {
		getDeploymentLogsParams.SearchText = &logLevel
	}

	deploymentLogs, err := GetDeploymentLogs("", deploymentID, getDeploymentLogsParams, coreClient)

	if len(deploymentLogs.Results) == 0 {
		fmt.Println("No matching logs have been recorded in the past 24 hours for Deployment " + deployment.Name)
		return nil
	}
	for i := range deploymentLogs.Results {
		fmt.Printf("%f %s %s\n", deploymentLogs.Results[i].Timestamp, deploymentLogs.Results[i].Raw, deploymentLogs.Results[i].Source)
	}
	return nil
}

func Create(name, workspaceID, description, clusterID, runtimeVersion, dagDeploy, executor, cloudProvider, region, schedulerSize, highAvailability, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string, deploymentType astroplatformcore.DeploymentType, schedulerAU, schedulerReplicas int, corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, waitForStatus bool) error { //nolint
	var organizationID string
	var currentWorkspace astrocore.Workspace
	var dagDeployEnabled bool

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	coreDeploymentType := astrocore.GetDeploymentOptionsParamsDeploymentType(deploymentType)
	var coreCloudProvider astrocore.GetDeploymentOptionsParamsCloudProvider
	if cloudProvider == "aws" {
		coreCloudProvider = astrocore.GetDeploymentOptionsParamsCloudProvider("AWS")
	}
	if cloudProvider == "gcp" {
		coreCloudProvider = astrocore.GetDeploymentOptionsParamsCloudProvider("GCP")
	}
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

	var isCicdEnforced bool
	if cicdEnforcement == "" {
		isCicdEnforced = false
	}
	if cicdEnforcement == "disable" {
		isCicdEnforced = false
	}
	if cicdEnforcement == "enable" {
		isCicdEnforced = true
	}

	if organizationID == "" {
		return fmt.Errorf(noWorkspaceMsg, workspaceID) //nolint:goerr113
	}
	fmt.Printf("Current Workspace: %s\n\n", currentWorkspace.Name)

	// name input
	if name == "" {
		fmt.Println("Please specify a name for your Deployment")
		name = input.Text(ansi.Bold("\nDeployment name: "))
		if name == "" {
			return errors.New("you must give your Deployment a name")
		}
		// deployments, err := CoreGetDeployments(workspaceID, organizationID, platformCoreClient)
		// if err != nil {
		// 	return errors.Wrap(err, errInvalidDeployment.Error())
		// }

		// for i := range deployments {
		// 	if deployments[i].Name == name {
		// 		return errors.New("A Deployment with that name already exists")
		// 	}
		// }
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
		clusterID, err = selectCluster(clusterID, c.Organization, corePlatformClient)
		if err != nil {
			return err
		}
	}

	if dagDeploy == "enable" { //nolint: goconst
		dagDeployEnabled = true
	} else {
		dagDeployEnabled = false
	}
	createDeploymentRequest := astroplatformcore.CreateDeploymentRequest{}
	// build standard input
	if IsDeploymentStandard(deploymentType) || IsDeploymentDedicated(deploymentType) {
		var highAvailabilityValue bool
		if highAvailability == "enable" { //nolint: goconst
			highAvailabilityValue = true
		} else {
			highAvailabilityValue = false
		}
		var workerConcurrency int
		for i := range configOption.WorkerMachines {
			if configOption.DefaultValues.WorkerMachineName == strings.ToUpper(configOption.WorkerMachines[i].Name) {
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
		// validate hosted resources requests
		resourcesValid := validateHostedResources(defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, configOption)
		if !resourcesValid {
			return nil
		}
		if IsDeploymentStandard(deploymentType) {
			var requestedCloudProvider astroplatformcore.CreateStandardDeploymentRequestCloudProvider
			if cloudProvider == "gcp" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderGCP
			}
			if cloudProvider == "aws" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAWS
			}
			if cloudProvider == "azure" {
				requestedCloudProvider = astroplatformcore.CreateStandardDeploymentRequestCloudProviderAZURE
			}
			var requestedExecutor astroplatformcore.CreateStandardDeploymentRequestExecutor
			if executor == "CeleryExecutor" {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorCELERY
			}
			if executor == "KubernetesExecutor" {
				requestedExecutor = astroplatformcore.CreateStandardDeploymentRequestExecutorKUBERNETES
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
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateStandardDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
			}
			if executor == "CeleryExecutor" {
				standardDeploymentRequest.WorkerQueues = &defautWorkerQueue
			}
			if schedulerSize != "" {
				if schedulerSize == "small" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeSMALL
				}
				if schedulerSize == "medium" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeMEDIUM
				}
				if schedulerSize == "large" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSizeLARGE
				}
			} else {
				standardDeploymentRequest.SchedulerSize = astroplatformcore.CreateStandardDeploymentRequestSchedulerSize(configOption.DefaultValues.SchedulerSize)
			}
			if highAvailability == "enable" { //nolint: goconst
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
			if executor == "CeleryExecutor" {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorCELERY
			}
			if executor == "KubernetesExecutor" {
				requestedExecutor = astroplatformcore.CreateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			dedicatedDeploymentRequest := astroplatformcore.CreateDedicatedDeploymentRequest{
				AstroRuntimeVersion:  runtimeVersion,
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				ClusterId:            clusterID,
				WorkspaceId:          workspaceID,
				Type:                 astroplatformcore.CreateDedicatedDeploymentRequestTypeDEDICATED,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
			}
			if executor == "CeleryExecutor" {
				dedicatedDeploymentRequest.WorkerQueues = &defautWorkerQueue
			}
			if schedulerSize != "" {
				if schedulerSize == "small" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeSMALL
				}
				if schedulerSize == "medium" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeMEDIUM
				}
				if schedulerSize == "large" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSizeLARGE
				}
			} else {
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.CreateDedicatedDeploymentRequestSchedulerSize(configOption.DefaultValues.SchedulerSize)
			}
			err := createDeploymentRequest.FromCreateDedicatedDeploymentRequest(dedicatedDeploymentRequest)
			if err != nil {
				return err
			}
		}
	}
	// build hybrid input
	if !IsDeploymentStandard(deploymentType) && !IsDeploymentDedicated(deploymentType) {
		cluster, err := CoreGetCluster("", clusterID, corePlatformClient)
		if err != nil {
			return err
		}
		nodePools := *cluster.NodePools
		defautWorkerQueue := []astroplatformcore.HybridWorkerQueueRequest{{
			IsDefault:         true,
			MaxWorkerCount:    int(configOption.WorkerQueues.MaxWorkers.Default),
			MinWorkerCount:    int(configOption.WorkerQueues.MinWorkers.Default),
			Name:              "default",
			WorkerConcurrency: int(configOption.WorkerQueues.WorkerConcurrency.Default),
			NodePoolId:        nodePools[0].Id,
		}}
		if schedulerAU == 0 {
			schedulerAU = int(configOption.LegacyAstro.SchedulerAstroUnitRange.Default)
		}

		if schedulerReplicas == 0 {
			schedulerReplicas = int(configOption.LegacyAstro.SchedulerReplicaRange.Default)
		}

		if schedulerSize == "" {
			schedulerSize = configOption.DefaultValues.SchedulerSize
		}

		// validate hybrid resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return nil
		}
		var requestedExecutor astroplatformcore.CreateHybridDeploymentRequestExecutor
		if executor == "CeleryExecutor" {
			requestedExecutor = astroplatformcore.CreateHybridDeploymentRequestExecutorCELERY
		}
		if executor == "KubernetesExecutor" {
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
			WorkspaceId:         workspaceID,

			Scheduler: astroplatformcore.DeploymentInstanceSpecRequest{
				Au:       schedulerAU,
				Replicas: schedulerReplicas,
			},
			Type: astroplatformcore.CreateHybridDeploymentRequestTypeHYBRID,
		}
		if executor == "CeleryExecutor" {
			hybridDeploymentRequest.WorkerQueues = &defautWorkerQueue
		} else {
			hybridDeploymentRequest.TaskPodNodePoolId = &nodePools[0].Id
		}
		err = createDeploymentRequest.FromCreateHybridDeploymentRequest(hybridDeploymentRequest)
		if err != nil {
			return err
		}
	}

	d, err := CoreCreateDeployment(organizationID, createDeploymentRequest, corePlatformClient)
	if err != nil {
		return err
	}

	if waitForStatus {
		err = HealthPoll(d.Id, workspaceID, sleepTime, tickNum, timeoutNum, corePlatformClient)
		if err != nil {
			errOutput := createOutput(workspaceID, deploymentType, &d)
			if errOutput != nil {
				return errOutput
			}
			return err
		}
	}

	err = createOutput(workspaceID, deploymentType, &d)
	if err != nil {
		return err
	}

	return nil
}

func createOutput(workspaceID string, deploymentType astroplatformcore.DeploymentType, d *astroplatformcore.Deployment) error {
	tab := newTableOut()

	runtimeVersionText := d.RuntimeVersion + " (based on Airflow " + d.AirflowVersion + ")"
	clusterName := notApplicable
	releaseName := d.Namespace
	// if !IsDeploymentStandard(deploymentType) {
	// 	clusterName = *d.ClusterName
	// }
	cloudProvider := *d.CloudProvider
	region := notApplicable // *d.Region
	tab.AddRow([]string{d.Name, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type)}, false)
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

func validateHybridResources(schedulerAU, schedulerReplicas int, configOption astrocore.DeploymentOptions) bool { //nolint:gocritic
	schedulerAuMin := int(configOption.LegacyAstro.SchedulerAstroUnitRange.Floor)
	schedulerAuMax := int(configOption.LegacyAstro.SchedulerAstroUnitRange.Ceiling)
	schedulerReplicasMin := int(configOption.LegacyAstro.SchedulerReplicaRange.Floor)
	schedulerReplicasMax := int(configOption.LegacyAstro.SchedulerReplicaRange.Ceiling)
	if schedulerAU > schedulerAuMax || schedulerAU < schedulerAuMin {
		fmt.Printf("\nScheduler AUs must be between a min of %d and a max of %d AUs", schedulerAuMin, schedulerAuMax)
		return false
	}
	if schedulerReplicas > schedulerReplicasMax || schedulerReplicas < schedulerReplicasMin {
		fmt.Printf("\nScheduler Replicas must between a min of %d and a max of %d Replicas", schedulerReplicasMin, schedulerReplicasMax)
		return false
	}
	return true
}

func validateHostedResources(defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string, configOption astrocore.DeploymentOptions) bool { //nolint:gocritic
	defaultTaskPodCpuMin := configOption.ResourceQuotas.DefaultPodSize.Cpu.Floor
	defaultTaskPodCpuMax := configOption.ResourceQuotas.DefaultPodSize.Cpu.Ceiling
	defaultTaskPodMemoryMin := configOption.ResourceQuotas.DefaultPodSize.Memory.Floor
	defaultTaskPodMemoryMax := configOption.ResourceQuotas.DefaultPodSize.Memory.Ceiling
	resourceQuotaCpuMin := configOption.ResourceQuotas.ResourceQuota.Cpu.Floor
	resourceQuotaCpuMax := configOption.ResourceQuotas.ResourceQuota.Cpu.Ceiling
	// resourceQuotaMemoryMin := configOption.ResourceQuotas.ResourceQuota.Memory.Floor
	// resourceQuotaMemoryMax := configOption.ResourceQuotas.ResourceQuota.Memory.Ceiling
	if defaultTaskPodCpu > defaultTaskPodCpuMax || defaultTaskPodCpu < defaultTaskPodCpuMin {
		fmt.Printf("\nDefault Task Pod CPU must be between a min of %s and a max of %s CPU cores", defaultTaskPodCpuMin, defaultTaskPodCpuMax)
		return false
	}

	if defaultTaskPodMemory > defaultTaskPodMemoryMax || defaultTaskPodMemory < defaultTaskPodMemoryMin {
		fmt.Printf("\nDefault Task Pod Memory must be between a min of %s and a max of %s Gis", defaultTaskPodMemoryMin, defaultTaskPodMemoryMax)
		return false
	}

	if resourceQuotaCpu > resourceQuotaCpuMax || resourceQuotaCpu < resourceQuotaCpuMin {
		fmt.Printf("\nDefault Resource Quota CPU must be between a min of %s and a max of %s CPU cores", resourceQuotaCpuMin, resourceQuotaCpuMax)
		return false
	}

	// if resourceQuotaMemory > resourceQuotaMemoryMax || resourceQuotaMemory < resourceQuotaMemoryMin {
	// 	fmt.Printf("\nDefault Resource Quota Memory must be between a min of %s and a max of %s Gis", resourceQuotaMemoryMin, resourceQuotaMemoryMax)
	// 	return false
	// }

	return true
}

// func validateRuntimeVersion(runtimeVersion string, client astro.Client) (bool, error) {
// 	runtimeReleases, err := GetRuntimeReleases(client)
// 	if err != nil {
// 		return false, err
// 	}
// 	if !util.Contains(runtimeReleases, runtimeVersion) {
// 		fmt.Printf("\nRuntime version not valid. Must be one of the following: %v\n", runtimeReleases)
// 		return false, nil
// 	}
// 	return true, nil
// }

// func GetRuntimeReleases(client astro.Client) ([]string, error) {
// 	// get deployment config options
// 	runtimeReleases := []string{}

// 	ConfigOptions, err := client.GetDeploymentConfig()
// 	if err != nil {
// 		return runtimeReleases, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
// 	}

// 	for i := range ConfigOptions.RuntimeReleases {
// 		runtimeReleases = append(runtimeReleases, ConfigOptions.RuntimeReleases[i].Version)
// 	}

// 	return runtimeReleases, nil
// }

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

func selectCluster(clusterID, organizationID string, corePlatformClient astroplatformcore.CoreClient) (newClusterID string, err error) {
	clusterTab := printutil.Table{
		Padding:        []int{5, 30, 30, 50},
		DynamicPadding: true,
		Header:         []string{"#", "CLUSTER NAME", "CLOUD PROVIDER", "CLUSTER ID"},
	}

	cs, err := organization.ListClusters(organizationID, corePlatformClient)
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
// func useSharedCluster(cloudProvider astrocore.SharedClusterCloudProvider, region string, coreClient astrocore.CoreClient) (clusterID string, err error) {
// 	// get shared cluster request
// 	getSharedClusterParams := astrocore.GetSharedClusterParams{
// 		Region:        region,
// 		CloudProvider: astrocore.GetSharedClusterParamsCloudProvider(cloudProvider),
// 	}
// 	response, err := coreClient.GetSharedClusterWithResponse(context.Background(), &getSharedClusterParams)
// 	if err != nil {
// 		return "", err
// 	}
// 	err = astrocore.NormalizeAPIError(response.HTTPResponse, response.Body)
// 	if err != nil {
// 		return "", err
// 	}
// 	return response.JSON200.Id, nil
// }

// useSharedClusterOrSelectDedicatedCluster decides how to derive the clusterID to use for a deployment.
// if cloudProvider and region are provided, it uses a useSharedCluster to get the ClusterID.
// if not, it uses selectCluster to get the ClusterID.
// func useSharedClusterOrSelectDedicatedCluster(cloudProvider, region, organizationShortName, clusterID, clusterType string, coreClient astrocore.CoreClient) (derivedClusterID string, err error) {
// 	// if cloud provider and region are requested
// 	// if clusterType == standard && cloudProvider != "" && region != "" {
// 	// 	// use a shared cluster for the deployment
// 	// 	derivedClusterID, err = useSharedCluster(astrocore.SharedClusterCloudProvider(cloudProvider), region, coreClient)
// 	// 	if err != nil {
// 	// 		return "", err
// 	// 	}
// 	// } else {
// 	// select and validate cluster
// 	derivedClusterID, err = selectCluster(clusterID, organizationShortName, coreClient)
// 	if err != nil {
// 		return "", err
// 	}
// 	return derivedClusterID, nil
// }

func HealthPoll(deploymentID, ws string, sleepTime, tickNum, timeoutNum int, corePlatformClient astroplatformcore.CoreClient) error {
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
			currentDeployment, err := CoreGetDeployment("", deploymentID, corePlatformClient)
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

func Update(deploymentID, name, ws, description, deploymentName, dagDeploy, executor, schedulerSize, highAvailability, cicdEnforcement, defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory string, schedulerAU, schedulerReplicas int, wQueueList []astroplatformcore.WorkerQueueRequest, hybridQueueList []astroplatformcore.HybridWorkerQueueRequest, newEnvironmentVariables []astroplatformcore.DeploymentEnvironmentVariableRequest, force bool, coreClient astrocore.CoreClient, platformCoreClient astroplatformcore.CoreClient) error { //nolint
	var queueCreateUpdate, confirmWithUser bool
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, false, platformCoreClient, nil)
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
	if cicdEnforcement == "disable" {
		isCicdEnforced = false
	}
	if cicdEnforcement == "enable" {
		isCicdEnforced = true
	}
	if isCicdEnforced && dagDeploy != "" {
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
	// determine isDagDeployEnabled
	var dagDeployEnabled bool
	if dagDeploy == "enable" {
		if currentDeployment.DagDeployEnabled {
			fmt.Println("\nDAG deploys are already enabled for this Deployment. Your DAGs will continue to run as scheduled.")
			return nil
		}

		fmt.Printf("\nYou enabled DAG-only deploys for this Deployment. Running tasks are not interrupted but new tasks will not be scheduled." +
			"\nRun `astro deploy --dags` to complete enabling this feature and resume your DAGs. It may take a few minutes for the Airflow UI to update..\n\n")
		dagDeployEnabled = true
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
		dagDeployEnabled = false
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
			EnvironmentVariables := *currentDeployment.EnvironmentVariables
			for i := range *currentDeployment.EnvironmentVariables {
				var deploymentEnvironmentVariableRequest astroplatformcore.DeploymentEnvironmentVariableRequest
				deploymentEnvironmentVariableRequest.IsSecret = EnvironmentVariables[i].IsSecret
				deploymentEnvironmentVariableRequest.Key = EnvironmentVariables[i].Key
				deploymentEnvironmentVariableRequest.Value = EnvironmentVariables[i].Value
				deploymentEnvironmentVariablesRequest = append(deploymentEnvironmentVariablesRequest, deploymentEnvironmentVariableRequest)
			}
		} else {
			deploymentEnvironmentVariablesRequest = newEnvironmentVariables
		}
	} else {
		queueCreateUpdate = true
		deploymentEnvironmentVariablesRequest = newEnvironmentVariables
	}
	if IsDeploymentStandard(*currentDeployment.Type) || IsDeploymentDedicated(*currentDeployment.Type) {
		var workerQueuesRequest []astroplatformcore.WorkerQueueRequest
		if currentDeployment.WorkerQueues != nil {
			workerQueues := *currentDeployment.WorkerQueues
			for i := range *currentDeployment.WorkerQueues {
				var workerQueueRequest astroplatformcore.WorkerQueueRequest
				workerQueueRequest.AstroMachine = astroplatformcore.WorkerQueueRequestAstroMachine(*workerQueues[i].AstroMachine)
				workerQueueRequest.Id = &workerQueues[i].Id
				workerQueueRequest.IsDefault = workerQueues[i].IsDefault
				workerQueueRequest.MaxWorkerCount = workerQueues[i].MaxWorkerCount
				workerQueueRequest.MinWorkerCount = workerQueues[i].MinWorkerCount
				workerQueueRequest.Name = workerQueues[i].Name
				workerQueueRequest.WorkerConcurrency = workerQueues[i].WorkerConcurrency
				workerQueuesRequest = append(workerQueuesRequest, workerQueueRequest)
			}
		}

		var workerConcurrency int
		for i := range configOption.WorkerMachines {
			if configOption.DefaultValues.WorkerMachineName == strings.ToUpper(configOption.WorkerMachines[i].Name) {
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

		if len(wQueueList) > 0 {
			queueCreateUpdate = true
			workerQueuesRequest = wQueueList
		}
		var highAvailabilityValue bool
		if highAvailability == "" { //nolint: goconst
			highAvailabilityValue = *currentDeployment.IsHighAvailability
		}
		if highAvailability == "enable" {
			highAvailabilityValue = true
		}
		if highAvailability == "disable" {
			highAvailabilityValue = false
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
		// validate hosted resources requests
		resourcesValid := validateHostedResources(defaultTaskPodCpu, defaultTaskPodMemory, resourceQuotaCpu, resourceQuotaMemory, configOption)
		if !resourcesValid {
			return nil
		}
		if IsDeploymentStandard(*currentDeployment.Type) {
			var requestedExecutor astroplatformcore.UpdateStandardDeploymentRequestExecutor
			if executor == "" {
				requestedExecutor = astroplatformcore.UpdateStandardDeploymentRequestExecutor(*currentDeployment.Executor)
			}
			if executor == "CeleryExecutor" {
				requestedExecutor = astroplatformcore.CELERY
			}
			if executor == "KubernetesExecutor" {
				requestedExecutor = astroplatformcore.KUBERNETES
			}
			standardDeploymentRequest := astroplatformcore.UpdateStandardDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astroplatformcore.UpdateStandardDeploymentRequestTypeSTANDARD,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
			}
			if schedulerSize != "" {
				if schedulerSize == "small" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.SMALL
				}
				if schedulerSize == "medium" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.MEDIUM
				}
				if schedulerSize == "large" {
					standardDeploymentRequest.SchedulerSize = astroplatformcore.LARGE
				}
			} else {
				standardDeploymentRequest.SchedulerSize = astroplatformcore.UpdateStandardDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}
			if standardDeploymentRequest.Executor == astroplatformcore.CELERY {
				if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorKUBERNETES && workerQueuesRequest == nil {
					standardDeploymentRequest.WorkerQueues = &defautWorkerQueue
				} else {
					fmt.Println(workerQueuesRequest)
					standardDeploymentRequest.WorkerQueues = &workerQueuesRequest
				}
			}
			err := updateDeploymentRequest.FromUpdateStandardDeploymentRequest(standardDeploymentRequest)
			if err != nil {
				return err
			}
		}
		if IsDeploymentDedicated(*currentDeployment.Type) {
			var requestedExecutor astroplatformcore.UpdateDedicatedDeploymentRequestExecutor
			if executor == "" {
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutor(*currentDeployment.Executor)
			}
			if executor == "CeleryExecutor" {
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY
			}
			if executor == "KubernetesExecutor" {
				requestedExecutor = astroplatformcore.UpdateDedicatedDeploymentRequestExecutorKUBERNETES
			}
			dedicatedDeploymentRequest := astroplatformcore.UpdateDedicatedDeploymentRequest{
				Description:          &description,
				Name:                 name,
				Executor:             requestedExecutor,
				IsCicdEnforced:       isCicdEnforced,
				IsDagDeployEnabled:   dagDeployEnabled,
				IsHighAvailability:   highAvailabilityValue,
				WorkspaceId:          currentDeployment.WorkspaceId,
				Type:                 astroplatformcore.UpdateDedicatedDeploymentRequestTypeSTANDARD,
				DefaultTaskPodCpu:    defaultTaskPodCpu,
				DefaultTaskPodMemory: defaultTaskPodMemory,
				ResourceQuotaCpu:     resourceQuotaCpu,
				ResourceQuotaMemory:  resourceQuotaMemory,
				EnvironmentVariables: deploymentEnvironmentVariablesRequest,
				WorkerQueues:         &workerQueuesRequest,
			}
			if schedulerSize != "" {
				if schedulerSize == "small" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeSMALL
				}
				if schedulerSize == "medium" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeMEDIUM
				}
				if schedulerSize == "large" {
					dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSizeLARGE
				}
			} else {
				dedicatedDeploymentRequest.SchedulerSize = astroplatformcore.UpdateDedicatedDeploymentRequestSchedulerSize(*currentDeployment.SchedulerSize)
			}
			if dedicatedDeploymentRequest.Executor == astroplatformcore.UpdateDedicatedDeploymentRequestExecutorCELERY {
				if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorKUBERNETES && workerQueuesRequest == nil {
					dedicatedDeploymentRequest.WorkerQueues = &defautWorkerQueue
				} else {
					dedicatedDeploymentRequest.WorkerQueues = &workerQueuesRequest
				}
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
			for i := range workerQueues {
				var workerQueueRequest astroplatformcore.HybridWorkerQueueRequest
				workerQueueRequest.Id = &workerQueues[i].Id
				workerQueueRequest.IsDefault = workerQueues[i].IsDefault
				workerQueueRequest.MaxWorkerCount = workerQueues[i].MaxWorkerCount
				workerQueueRequest.MinWorkerCount = workerQueues[i].MinWorkerCount
				workerQueueRequest.Name = workerQueues[i].Name
				workerQueueRequest.WorkerConcurrency = workerQueues[i].WorkerConcurrency
				workerQueueRequest.NodePoolId = *workerQueues[i].NodePoolId
				workerQueuesRequest = append(workerQueuesRequest, workerQueueRequest)
			}
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
		// validate au resources requests
		resourcesValid := validateHybridResources(schedulerAU, schedulerReplicas, configOption)
		if !resourcesValid {
			return nil
		}
		var requestedExecutor astroplatformcore.UpdateHybridDeploymentRequestExecutor
		if executor == "" {
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutor(*currentDeployment.Executor)
		}
		if executor == "CeleryExecutor" {
			requestedExecutor = astroplatformcore.UpdateHybridDeploymentRequestExecutorCELERY
		}
		if executor == "KubernetesExecutor" {
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
		}
		cluster, err := CoreGetCluster("", *currentDeployment.ClusterId, platformCoreClient)
		if err != nil {
			return err
		}
		nodePools := *cluster.NodePools
		if hybridDeploymentRequest.Executor == astroplatformcore.UpdateHybridDeploymentRequestExecutorKUBERNETES {
			if *currentDeployment.Executor == astroplatformcore.DeploymentExecutorCELERY {
				hybridDeploymentRequest.TaskPodNodePoolId = &nodePools[0].Id
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
					NodePoolId:        nodePools[0].Id,
				}}
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
			} else {
				hybridDeploymentRequest.WorkerQueues = &workerQueuesRequest
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
		// if !IsDeploymentStandard(deploymentType) {
		// 	clusterName = *d.ClusterName
		// }
		cloudProvider := *d.CloudProvider
		region := notApplicable // *d.Region
		tabDeployment.AddRow([]string{d.Name, releaseName, clusterName, cloudProvider, region, d.Id, runtimeVersionText, strconv.FormatBool(d.DagDeployEnabled), strconv.FormatBool(d.IsCicdEnforced), string(*d.Type)}, false)
		tabDeployment.SuccessMsg = "\n Successfully updated Deployment"
		tabDeployment.Print(os.Stdout)
	}
	return nil
}

func Delete(deploymentID, ws, deploymentName string, forceDelete bool, platformCoreClient astroplatformcore.CoreClient) error {
	// get deployment
	currentDeployment, err := GetDeployment(ws, deploymentID, deploymentName, true, platformCoreClient, nil)
	if err != nil {
		return err
	}

	if currentDeployment.Id == "" {
		fmt.Printf("No Deployments found in workspace %s to delete\n", ansi.Bold(ws))
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

func IsDeploymentStandard(deploymentType astroplatformcore.DeploymentType) bool {
	return deploymentType == astroplatformcore.DeploymentTypeSTANDARD
}

func IsDeploymentDedicated(deploymentType astroplatformcore.DeploymentType) bool {
	return deploymentType == astroplatformcore.DeploymentTypeDEDICATED
}

// var GetDeployments = func(ws, org string, client astro.Client) ([]astro.Deployment, error) {
// 	if org == "" {
// 		c, err := config.GetCurrentContext()
// 		if err != nil {
// 			return []astro.Deployment{}, err
// 		}
// 		org = c.Organization
// 	}

// 	deployments, err := client.ListDeployments(org, ws)
// 	if err != nil {
// 		return deployments, errors.Wrap(err, astro.AstronomerConnectionErrMsg)
// 	}

// 	return deployments, nil
// }

var CoreGetDeployments = func(ws, orgID string, corePlatformClient astroplatformcore.CoreClient) ([]astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return []astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}
	deploymentListParams := &astroplatformcore.ListDeploymentsParams{}
	if ws != "" {
		deploymentListParams.WorkspaceIds = &[]string{ws}
	}

	resp, err := corePlatformClient.ListDeploymentsWithResponse(context.Background(), orgID, deploymentListParams)
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

var CoreGetCluster = func(orgID, clusterID string, corePlatformClient astroplatformcore.CoreClient) (astroplatformcore.Cluster, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Cluster{}, err
		}
		orgID = c.Organization
	}

	resp, err := corePlatformClient.GetClusterWithResponse(context.Background(), orgID, clusterID)
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

var CoreGetDeployment = func(orgID, deploymentId string, corePlatformClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := corePlatformClient.GetDeploymentWithResponse(context.Background(), orgID, deploymentId)
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

// create deployment with core API
var CoreCreateDeployment = func(orgID string, createDeploymentRequest astroplatformcore.CreateDeploymentJSONRequestBody, corePlatformClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}

	resp, err := corePlatformClient.CreateDeploymentWithResponse(context.Background(), orgID, createDeploymentRequest)
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

// update deployment with core API
var CoreUpdateDeployment = func(orgID, deploymentID string, updateDeploymentRequest astroplatformcore.UpdateDeploymentJSONRequestBody, corePlatformClient astroplatformcore.CoreClient) (astroplatformcore.Deployment, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}
		orgID = c.Organization
	}
	resp, err := corePlatformClient.UpdateDeploymentWithResponse(context.Background(), orgID, deploymentID, updateDeploymentRequest)
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

// delete deployment with core API
var CoreDeleteDeployment = func(orgID, deploymentID string, corePlatformClient astroplatformcore.CoreClient) error {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return err
		}
		orgID = c.Organization
	}

	resp, err := corePlatformClient.DeleteDeploymentWithResponse(context.Background(), orgID, deploymentID)
	if err != nil {
		return err
	}

	err = astrocore.NormalizeAPIError(resp.HTTPResponse, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

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

var GetPlatformDeploymentOptions = func(orgID string, deploymentOptionsParams astroplatformcore.GetDeploymentOptionsParams, coreClient astroplatformcore.CoreClient) (astroplatformcore.DeploymentOptions, error) {
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

var GetDeploymentLogs = func(orgID string, deploymentId string, getDeploymentLogsParams astrocore.GetDeploymentLogsParams, coreClient astrocore.CoreClient) (astrocore.DeploymentLog, error) {
	if orgID == "" {
		c, err := config.GetCurrentContext()
		if err != nil {
			return astrocore.DeploymentLog{}, err
		}
		orgID = c.Organization
	}

	resp, err := coreClient.GetDeploymentLogsWithResponse(context.Background(), orgID, deploymentId, &getDeploymentLogsParams)
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

	if len(deployments) == 1 {
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
		tab.AddRow([]string{strconv.Itoa(index), deployments[i].Name, deployments[i].Namespace, deployments[i].Id, strconv.FormatBool(deployments[i].DagDeployEnabled)}, false)

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

func GetDeployment(ws, deploymentID, deploymentName string, disableCreateFlow bool, corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) (astroplatformcore.Deployment, error) {
	deployments, err := CoreGetDeployments(ws, "", corePlatformClient)
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
		if len(stageDeployments) == 1 {
			return stageDeployments[0], nil
		}
		if len(stageDeployments) < 1 {
			fmt.Printf("No Deployment with the name %s was found\n", deploymentName)
			return astroplatformcore.Deployment{}, errInvalidDeployment
		}
	}

	var currentDeployment astroplatformcore.Deployment

	// select deployment if deploymentID is empty
	if deploymentID == "" {
		currentDeployment, err = deploymentSelectionProcess(ws, deployments, corePlatformClient, coreClient)
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

	currentDeployment, err = CoreGetDeployment("", currentDeployment.Id, corePlatformClient)
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	return currentDeployment, nil
}

func deploymentSelectionProcess(ws string, deployments []astroplatformcore.Deployment, corePlatformClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient) (astroplatformcore.Deployment, error) {
	currentDeployment, err := SelectDeployment(deployments, "Select a Deployment")
	if err != nil {
		return astroplatformcore.Deployment{}, err
	}
	if currentDeployment.Id == "" {
		// get latest runtime version
		airflowVersionClient := airflowversions.NewClient(httputil.NewHTTPClient(), false)
		runtimeVersion, err := airflowversions.GetDefaultImageTag(airflowVersionClient, "")
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}

		// configOption, err := client.GetDeploymentConfig()
		// if err != nil {
		// 	return astroplatformcore.Deployment{}, err
		// }

		// schedulerAU := configOption.Components.Scheduler.AU.Default
		// schedulerReplicas := configOption.Components.Scheduler.Replicas.Default
		cicdEnforcement := "disable"
		// walk user through creating a deployment
		err = createDeployment("", ws, "", "", runtimeVersion, "disable", CeleryExecutor, "", "", "medium", "", "", cicdEnforcement, "", "", "", "", 0, 0, corePlatformClient, coreClient, false)
		if err != nil {
			return astroplatformcore.Deployment{}, err
		}

		// get a new deployment list
		deployments, err = CoreGetDeployments(ws, "", corePlatformClient)
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
// func mutateExecutor(requestedExecutor string, currentSpec astro.DeploymentCreateSpec, existingQLength int) (bool, astro.DeploymentCreateSpec) {
// 	var printed bool
// 	if requestedExecutor != "" && currentSpec.Executor != requestedExecutor {
// 		// print helpful message describing the change
// 		printed = printWarning(requestedExecutor, existingQLength)
// 		currentSpec.Executor = requestedExecutor
// 	}
// 	return printed, currentSpec
// }
