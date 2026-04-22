package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/astro-client-v1"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/pkg/ansi"
)

type deploymentMetadata struct {
	DeploymentID        *string              `mapstructure:"deployment_id" yaml:"deployment_id" json:"deployment_id"`
	WorkspaceID         *string              `mapstructure:"workspace_id" yaml:"workspace_id" json:"workspace_id"`
	ClusterID           *string              `mapstructure:"cluster_id" yaml:"cluster_id" json:"cluster_id"`
	ReleaseName         *string              `mapstructure:"release_name" yaml:"release_name" json:"release_name"`
	AirflowVersion      *string              `mapstructure:"airflow_version" yaml:"airflow_version" json:"airflow_version"`
	CurrentTag          *string              `mapstructure:"current_tag" yaml:"current_tag" json:"current_tag"`
	Status              *string              `mapstructure:"status" yaml:"status" json:"status"`
	CreatedAt           *time.Time           `mapstructure:"created_at" yaml:"created_at" json:"created_at"`
	UpdatedAt           *time.Time           `mapstructure:"updated_at" yaml:"updated_at" json:"updated_at"`
	DeploymentURL       *string              `mapstructure:"deployment_url" yaml:"deployment_url" json:"deployment_url"`
	WebserverURL        *string              `mapstructure:"webserver_url" yaml:"webserver_url" json:"webserver_url"`
	AirflowAPIURL       *string              `mapstructure:"airflow_api_url" yaml:"airflow_api_url" json:"airflow_api_url"`
	HibernationOverride *HibernationOverride `mapstructure:"hibernation_override,omitempty" yaml:"hibernation_override,omitempty" json:"hibernation_override,omitempty"`
}

type HibernationOverride struct {
	IsHibernating *bool      `mapstructure:"is_hibernating,omitempty" yaml:"is_hibernating,omitempty" json:"is_hibernating,omitempty"`
	OverrideUntil *time.Time `mapstructure:"override_until,omitempty" yaml:"override_until,omitempty" json:"override_until,omitempty"`
}

type RemoteExecution struct {
	Enabled                bool      `mapstructure:"enabled,omitempty" yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AllowedIPAddressRanges *[]string `mapstructure:"allowed_ip_address_ranges,omitempty" yaml:"allowed_ip_address_ranges,omitempty" json:"allowed_ip_address_ranges,omitempty"`
	TaskLogBucket          *string   `mapstructure:"task_log_bucket,omitempty" yaml:"task_log_bucket,omitempty" json:"task_log_bucket,omitempty"`
	TaskLogURLPattern      *string   `mapstructure:"task_log_url_pattern,omitempty" yaml:"task_log_url_pattern,omitempty" json:"task_log_url_pattern,omitempty"`
}

type deploymentConfig struct {
	Name                  string           `mapstructure:"name" yaml:"name" json:"name"`
	Description           string           `mapstructure:"description" yaml:"description" json:"description"`
	RunTimeVersion        string           `mapstructure:"runtime_version" yaml:"runtime_version" json:"runtime_version"`
	DagDeployEnabled      *bool            `mapstructure:"dag_deploy_enabled,omitempty" yaml:"dag_deploy_enabled,omitempty" json:"dag_deploy_enabled,omitempty"`
	APIKeyOnlyDeployments bool             `mapstructure:"ci_cd_enforcement" yaml:"ci_cd_enforcement" json:"ci_cd_enforcement"`
	SchedulerSize         string           `mapstructure:"scheduler_size,omitempty" yaml:"scheduler_size,omitempty" json:"scheduler_size,omitempty"`
	IsHighAvailability    bool             `mapstructure:"is_high_availability" yaml:"is_high_availability" json:"is_high_availability"`
	IsDevelopmentMode     bool             `mapstructure:"is_development_mode" yaml:"is_development_mode" json:"is_development_mode"`
	Executor              string           `mapstructure:"executor" yaml:"executor" json:"executor"`
	SchedulerAU           int              `mapstructure:"scheduler_au,omitempty" yaml:"scheduler_au,omitempty" json:"scheduler_au,omitempty"`
	SchedulerCount        int              `mapstructure:"scheduler_count" yaml:"scheduler_count" json:"scheduler_count"`
	ClusterName           string           `mapstructure:"cluster_name,omitempty" yaml:"cluster_name,omitempty" json:"cluster_name,omitempty"`
	WorkspaceName         string           `mapstructure:"workspace_name" yaml:"workspace_name" json:"workspace_name"`
	DeploymentType        string           `mapstructure:"deployment_type" yaml:"deployment_type" json:"deployment_type"`
	CloudProvider         string           `mapstructure:"cloud_provider" yaml:"cloud_provider" json:"cloud_provider"`
	Region                string           `mapstructure:"region" yaml:"region" json:"region"`
	DefaultTaskPodCPU     string           `mapstructure:"default_task_pod_cpu,omitempty" yaml:"default_task_pod_cpu,omitempty" json:"default_task_pod_cpu,omitempty"`
	DefaultTaskPodMemory  string           `mapstructure:"default_task_pod_memory,omitempty" yaml:"default_task_pod_memory,omitempty" json:"default_task_pod_memory,omitempty"`
	ResourceQuotaCPU      string           `mapstructure:"resource_quota_cpu,omitempty" yaml:"resource_quota_cpu,omitempty" json:"resource_quota_cpu,omitempty"`
	ResourceQuotaMemory   string           `mapstructure:"resource_quota_memory,omitempty" yaml:"resource_quota_memory,omitempty" json:"resource_quota_memory,omitempty"`
	DefaultWorkerType     string           `mapstructure:"default_worker_type,omitempty" yaml:"default_worker_type,omitempty" json:"default_worker_type,omitempty"`
	WorkloadIdentity      string           `mapstructure:"workload_identity" yaml:"workload_identity" json:"workload_identity"` // intentionally removing omitempty so we have an empty placeholder for this value if someone wants to set it
	RemoteExecution       *RemoteExecution `mapstructure:"remote_execution,omitempty" yaml:"remote_execution,omitempty" json:"remote_execution,omitempty"`
}

type Workerq struct {
	Name              string `mapstructure:"name" yaml:"name" json:"name"`
	MaxWorkerCount    int    `mapstructure:"max_worker_count,omitempty" yaml:"max_worker_count,omitempty" json:"max_worker_count,omitempty"`
	MinWorkerCount    int    `mapstructure:"min_worker_count" yaml:"min_worker_count" json:"min_worker_count"`
	WorkerConcurrency int    `mapstructure:"worker_concurrency,omitempty" yaml:"worker_concurrency,omitempty" json:"worker_concurrency,omitempty"`
	WorkerType        string `mapstructure:"worker_type" yaml:"worker_type" json:"worker_type"`
	PodCPU            string `mapstructure:"pod_cpu,omitempty" yaml:"pod_cpu,omitempty" json:"pod_cpu,omitempty"`
	PodRAM            string `mapstructure:"pod_ram,omitempty" yaml:"pod_ram,omitempty" json:"pod_ram,omitempty"`
}

type EnvironmentVariable struct {
	IsSecret  bool    `mapstructure:"is_secret" yaml:"is_secret" json:"is_secret"`
	Key       string  `mapstructure:"key" yaml:"key" json:"key"`
	UpdatedAt string  `mapstructure:"updated_at,omitempty" yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
	Value     *string `mapstructure:"value" yaml:"value" json:"value"`
}

type HibernationSchedule struct {
	HibernateAt string `mapstructure:"hibernate_at,omitempty" yaml:"hibernate_at,omitempty" json:"hibernate_at,omitempty"`
	WakeAt      string `mapstructure:"wake_at,omitempty" yaml:"wake_at,omitempty" json:"wake_at,omitempty"`
	Description string `mapstructure:"description,omitempty" yaml:"description,omitempty" json:"description,omitempty"`
	Enabled     bool   `mapstructure:"enabled,omitempty" yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

type orderedPieces struct {
	EnvVars              []EnvironmentVariable `mapstructure:"environment_variables,omitempty" yaml:"environment_variables,omitempty" json:"environment_variables,omitempty"`
	Configuration        deploymentConfig      `mapstructure:"configuration" yaml:"configuration" json:"configuration"`
	WorkerQs             []Workerq             `mapstructure:"worker_queues" yaml:"worker_queues" json:"worker_queues"`
	Metadata             *deploymentMetadata   `mapstructure:"metadata,omitempty" yaml:"metadata,omitempty" json:"metadata,omitempty"`
	AlertEmails          []string              `mapstructure:"alert_emails,omitempty" yaml:"alert_emails,omitempty" json:"alert_emails,omitempty"`
	HibernationSchedules []HibernationSchedule `mapstructure:"hibernation_schedules,omitempty" yaml:"hibernation_schedules,omitempty" json:"hibernation_schedules,omitempty"`
}

type FormattedDeployment struct {
	Deployment orderedPieces `mapstructure:"deployment" yaml:"deployment" json:"deployment"`
}

var (
	jsonMarshal    = json.MarshalIndent
	yamlMarshal    = yaml.Marshal
	decodeToStruct = mapstructure.Decode
	errKeyNotFound = errors.New("not found in deployment")
)

const (
	jsonFormat    = "json"
	notApplicable = "N/A"
)

func Inspect(wsID, deploymentName, deploymentID, outputFormat string, astroV1Client astrov1.APIClient, out io.Writer, requestedField string, template, showWorkloadIdentity bool) error {
	var (
		requestedDeployment                                                        astrov1.Deployment
		err                                                                        error
		infoToPrint                                                                []byte
		deploymentInfoMap, deploymentConfigMap, additionalMap, printableDeployment map[string]interface{}
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(wsID, deploymentID, deploymentName, true, nil, astroV1Client)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("%s %s\n", deployment.NoDeploymentInWSMsg, ansi.Bold(wsID))
		return nil
	}
	// create a map for deployment.information
	deploymentInfoMap, err = getDeploymentInfo(requestedDeployment)
	if err != nil {
		return err
	}
	// create a map for deployment.configuration
	deploymentConfigMap, err = getDeploymentConfig(&requestedDeployment, astroV1Client, showWorkloadIdentity)
	if err != nil {
		return err
	}
	// create a map for deployment.alert_emails, deployment.worker_queues and deployment.astronomer_variables
	nodePools := []astrov1.NodePool{}
	if requestedDeployment.ClusterId != nil {
		cluster, err := deployment.GetClusterByID("", *requestedDeployment.ClusterId, astroV1Client)
		if err != nil {
			return err
		}
		if cluster.NodePools != nil {
			nodePools = *cluster.NodePools
		}
	}
	additionalMap = getAdditionalNullableFields(&requestedDeployment, nodePools)
	// create a map for the entire deployment
	printableDeployment = getPrintableDeployment(deploymentInfoMap, deploymentConfigMap, additionalMap)
	// get specific field if requested
	if requestedField != "" {
		value, err := getSpecificField(printableDeployment, requestedField)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, value)
	} else {
		// print the entire deployment in outputFormat
		infoToPrint, err = formatPrintableDeployment(outputFormat, template, printableDeployment)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(infoToPrint))
	}
	return nil
}

func getDeploymentInfo(deploymentObj astrov1.Deployment) (map[string]interface{}, error) { //nolint
	deploymentURL, err := deployment.GetDeploymentURL(deploymentObj.Id, deploymentObj.WorkspaceId)
	if err != nil {
		return nil, err
	}
	var clusterID string
	releaseName := deploymentObj.Namespace
	if deployment.IsDeploymentStandard(*deploymentObj.Type) || deployment.IsDeploymentDedicated(*deploymentObj.Type) {
		releaseName = notApplicable
	}
	if !deployment.IsDeploymentStandard(*deploymentObj.Type) {
		clusterID = *deploymentObj.ClusterId
	}
	if deployment.IsDeploymentStandard(*deploymentObj.Type) {
		clusterID = notApplicable
	}
	metadata := map[string]interface{}{
		"deployment_id":   deploymentObj.Id,
		"workspace_id":    deploymentObj.WorkspaceId,
		"cluster_id":      clusterID,
		"airflow_version": deploymentObj.AirflowVersion,
		"current_tag":     deploymentObj.ImageTag,
		"release_name":    releaseName,
		"deployment_url":  deploymentURL,
		"webserver_url":   deploymentObj.WebServerUrl,
		"airflow_api_url": deploymentObj.WebServerAirflowApiUrl,
		"created_at":      deploymentObj.CreatedAt,
		"updated_at":      deploymentObj.UpdatedAt,
		"status":          deploymentObj.Status,
	}
	if deploymentObj.ScalingSpec != nil && deploymentObj.ScalingSpec.HibernationSpec != nil {
		if override := deploymentObj.ScalingSpec.HibernationSpec.Override; override != nil && override.IsActive != nil && *override.IsActive {
			metadata["hibernation_override"] = HibernationOverride{
				IsHibernating: override.IsHibernating,
				OverrideUntil: override.OverrideUntil,
			}
		}
	}
	return metadata, nil
}

func getDeploymentConfig(deploymentPointer *astrov1.Deployment, astroV1Client astrov1.APIClient, showWorkloadIdentity bool) (map[string]interface{}, error) {
	var clusterName string
	var defaultWorkerType string
	var err error
	deploymentObj := *deploymentPointer
	if !deployment.IsDeploymentStandard(*deploymentObj.Type) {
		clusterName = *deploymentObj.ClusterName
		if deploymentObj.TaskPodNodePoolId != nil {
			defaultWorkerType, err = GetDefaultWorkerType(*deploymentObj.TaskPodNodePoolId, *deploymentObj.ClusterId, astroV1Client)
			if err != nil {
				return nil, err
			}
		}
	}

	deploymentMap := map[string]interface{}{
		"name":               deploymentObj.Name,
		"workspace_name":     *deploymentObj.WorkspaceName,
		"deployment_type":    string(*deploymentObj.Type),
		"cluster_name":       clusterName,
		"runtime_version":    deploymentObj.RuntimeVersion,
		"dag_deploy_enabled": deploymentObj.IsDagDeployEnabled,
		"ci_cd_enforcement":  deploymentObj.IsCicdEnforced,
		"scheduler_count":    deploymentObj.SchedulerReplicas,
		"executor":           *deploymentObj.Executor,
	}
	if deployment.IsDeploymentStandard(*deploymentObj.Type) || deployment.IsDeploymentDedicated(*deploymentObj.Type) {
		deploymentMap["scheduler_size"] = *deploymentObj.SchedulerSize
		if deployment.IsDeploymentStandard(*deploymentObj.Type) || !deployment.IsRemoteExecutionEnabled(&deploymentObj) {
			deploymentMap["default_task_pod_cpu"] = *deploymentObj.DefaultTaskPodCpu
			deploymentMap["default_task_pod_memory"] = *deploymentObj.DefaultTaskPodMemory
			deploymentMap["resource_quota_cpu"] = *deploymentObj.ResourceQuotaCpu
			deploymentMap["resource_quota_memory"] = *deploymentObj.ResourceQuotaMemory
		}
	}
	if !deployment.IsDeploymentStandard(*deploymentObj.Type) {
		deploymentMap["default_worker_type"] = defaultWorkerType
	}

	if deploymentObj.Description != nil {
		deploymentMap["description"] = *deploymentObj.Description
	}
	if deploymentObj.IsHighAvailability != nil {
		deploymentMap["is_high_availability"] = *deploymentObj.IsHighAvailability
	}
	if deploymentObj.IsDevelopmentMode != nil {
		deploymentMap["is_development_mode"] = *deploymentObj.IsDevelopmentMode
	}
	if deploymentObj.SchedulerAu != nil {
		deploymentMap["scheduler_au"] = *deploymentObj.SchedulerAu
	}
	if deploymentObj.CloudProvider != nil {
		deploymentMap["cloud_provider"] = *deploymentObj.CloudProvider
	}
	if deploymentObj.Region != nil {
		deploymentMap["region"] = *deploymentObj.Region
	}
	if showWorkloadIdentity && deploymentObj.EffectiveWorkloadIdentity != nil {
		deploymentMap["workload_identity"] = *deploymentObj.EffectiveWorkloadIdentity
	}
	if deploymentObj.RemoteExecution != nil {
		remoteExecution := deploymentObj.RemoteExecution
		deploymentMap["remote_execution"] = map[string]interface{}{
			"enabled":                   remoteExecution.Enabled,
			"allowed_ip_address_ranges": remoteExecution.AllowedIpAddressRanges,
			"task_log_bucket":           remoteExecution.TaskLogBucket,
			"task_log_url_pattern":      remoteExecution.TaskLogUrlPattern,
		}
	}
	return deploymentMap, nil
}

func getAdditionalNullableFields(deploymentObj *astrov1.Deployment, nodePools []astrov1.NodePool) map[string]interface{} {
	qList := getQMap(deploymentObj, nodePools)
	var envVarList []astrov1.DeploymentEnvironmentVariable
	if deploymentObj.EnvironmentVariables != nil {
		envVarList = *deploymentObj.EnvironmentVariables
	}
	var hibernationSchedulesList []astrov1.DeploymentHibernationSchedule
	if deploymentObj.ScalingSpec != nil && deploymentObj.ScalingSpec.HibernationSpec != nil && deploymentObj.ScalingSpec.HibernationSpec.Schedules != nil {
		hibernationSchedulesList = *deploymentObj.ScalingSpec.HibernationSpec.Schedules
	}
	return map[string]interface{}{
		"alert_emails":          deploymentObj.ContactEmails,
		"worker_queues":         qList,
		"environment_variables": getVariablesMap(envVarList), // API only returns values when !EnvironmentVariablesObject.isSecret
		"hibernation_schedules": getHibernationSchedulesMap(hibernationSchedulesList),
	}
}

func ReturnSpecifiedValue(depl *astrov1.Deployment, requestedField string, astroV1Client astrov1.APIClient) (value any, err error) {
	showWorkloadIdentity := strings.Contains(requestedField, "workload_identity") // if the caller has requested for workload_identity, we set the flag to true to fetch the deployment workload_identity

	// create a map for deployment.information
	deploymentInfoMap, err := getDeploymentInfo(*depl)
	if err != nil {
		return nil, err
	}
	// create a map for deployment.configuration
	deploymentConfigMap, err := getDeploymentConfig(depl, astroV1Client, showWorkloadIdentity)
	if err != nil {
		return nil, err
	}
	nodePools := []astrov1.NodePool{}
	// create a map for deployment.alert_emails, deployment.worker_queues and deployment.environment_variables
	if depl.ClusterId != nil {
		cluster, err := deployment.GetClusterByID("", *depl.ClusterId, astroV1Client)
		if err != nil {
			return nil, err
		}
		if cluster.NodePools != nil {
			nodePools = *cluster.NodePools
		}
	}
	additionalMap := getAdditionalNullableFields(depl, nodePools)
	// create a map for the entire deployment
	printableDeployment := getPrintableDeployment(deploymentInfoMap, deploymentConfigMap, additionalMap)
	value, err = getSpecificField(printableDeployment, requestedField)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func getQMap(deploymentPointer *astrov1.Deployment, sourceNodePools []astrov1.NodePool) []map[string]interface{} {
	deploymentObj := *deploymentPointer
	var sourceDeploymentQs []astrov1.WorkerQueue
	if deploymentObj.WorkerQueues != nil {
		sourceDeploymentQs = *deploymentObj.WorkerQueues
	}
	var resources map[string]interface{}
	queueMap := make([]map[string]interface{}, 0, len(sourceDeploymentQs))
	for _, queue := range sourceDeploymentQs { //nolint
		if *deploymentObj.Executor == astrov1.DeploymentExecutorCELERY || *deploymentObj.Executor == astrov1.DeploymentExecutorASTRO {
			resources = map[string]interface{}{
				"max_worker_count":   queue.MaxWorkerCount,
				"min_worker_count":   queue.MinWorkerCount,
				"worker_concurrency": queue.WorkerConcurrency,
			}
		} else {
			resources = map[string]interface{}{
				"pod_cpu": queue.PodCpu,
				"pod_ram": queue.PodMemory,
			}
		}
		var workerType string
		if deployment.IsDeploymentDedicated(*deploymentObj.Type) || deployment.IsDeploymentStandard(*deploymentObj.Type) {
			workerType = *queue.AstroMachine
		} else {
			workerType = getWorkerTypeFromNodePoolID(*queue.NodePoolId, sourceNodePools)
		}
		newQ := map[string]interface{}{
			"name": queue.Name,
			// map worker type to node pool id
			"worker_type": workerType,
		}

		// add resources to queue
		for k, v := range resources {
			newQ[k] = v
		}
		queueMap = append(queueMap, newQ)
	}
	return queueMap
}

func getVariablesMap(sourceDeploymentVars []astrov1.DeploymentEnvironmentVariable) []map[string]interface{} {
	variablesMap := make([]map[string]interface{}, 0, len(sourceDeploymentVars))
	for _, variable := range sourceDeploymentVars {
		updatedAt := ""
		if !variable.UpdatedAt.IsZero() {
			updatedAt = variable.UpdatedAt.Format(time.RFC3339)
		}
		newVar := map[string]interface{}{
			"key":        variable.Key,
			"value":      variable.Value,
			"is_secret":  variable.IsSecret,
			"updated_at": updatedAt,
		}
		variablesMap = append(variablesMap, newVar)
	}
	return variablesMap
}

func getHibernationSchedulesMap(sourceHibernationSchedules []astrov1.DeploymentHibernationSchedule) []map[string]interface{} {
	hibernationSchedulesMap := make([]map[string]interface{}, 0, len(sourceHibernationSchedules))
	for _, schedule := range sourceHibernationSchedules {
		newSchedule := map[string]interface{}{
			"hibernate_at": schedule.HibernateAtCron,
			"wake_at":      schedule.WakeAtCron,
			"description":  schedule.Description,
			"enabled":      schedule.IsEnabled,
		}
		hibernationSchedulesMap = append(hibernationSchedulesMap, newSchedule)
	}
	return hibernationSchedulesMap
}

func formatPrintableDeployment(outputFormat string, template bool, printableDeployment map[string]interface{}) ([]byte, error) {
	var (
		infoToPrint     []byte
		err             error
		formatWithOrder FormattedDeployment
	)

	// use mapstructure to decode to a struct
	err = decodeToStruct(printableDeployment, &formatWithOrder)
	if err != nil {
		return []byte{}, err
	}
	if template {
		formatWithOrder = getTemplate(&formatWithOrder)
	}
	switch outputFormat {
	case jsonFormat:
		if infoToPrint, err = jsonMarshal(formatWithOrder, "", "    "); err != nil {
			return []byte{}, err
		}
	default:
		// always yaml by default
		if infoToPrint, err = yamlMarshal(formatWithOrder); err != nil {
			return []byte{}, err
		}
	}
	return infoToPrint, nil
}

// getSpecificField is used to find the requestedField in a deployment.
// it splits requestedField at every "." and looks for the first 2 parts in the deployment.
// if it finds any part of the requestedField, it returns the value.
// it returns errKeyNotFound if either part of the requestedField are not found.
func getSpecificField(deploymentMap map[string]interface{}, requestedField string) (any, error) {
	keyParts := strings.Split(strings.ToLower(requestedField), ".")
	// iterate over the top level maps in a deployment like deployment.information
	for _, elem := range deploymentMap {
		// check if the first key in the requested field exists and create a subMap
		if subMap, exists := elem.(map[string]interface{})[keyParts[0]]; exists {
			if len(keyParts) > 1 {
				// check if the second key in the requested field exists
				value, ok := subMap.(map[string]interface{})[keyParts[1]]
				if ok {
					// we found the requested field so return its value
					return value, nil
				}
			} else {
				// top level field was requested so we return it
				return subMap, nil
			}
		}
	}
	return nil, fmt.Errorf("requested key %s %w", requestedField, errKeyNotFound)
}

func getPrintableDeployment(infoMap, configMap, additionalMap map[string]interface{}) map[string]interface{} {
	printableDeployment := map[string]interface{}{
		"deployment": map[string]interface{}{
			"metadata":              infoMap,
			"configuration":         configMap,
			"alert_emails":          additionalMap["alert_emails"],
			"worker_queues":         additionalMap["worker_queues"],
			"environment_variables": additionalMap["environment_variables"],
			"hibernation_schedules": additionalMap["hibernation_schedules"],
		},
	}
	return printableDeployment
}

// getWorkerTypeFromNodePoolID takes maps the workerType to a node pool id in nodePools.
// It returns an error if the worker type does not exist in any node pool in nodePools.
func getWorkerTypeFromNodePoolID(poolID string, nodePools []astrov1.NodePool) string {
	for i := range nodePools {
		if nodePools[i].Id == poolID {
			return nodePools[i].NodeInstanceType
		}
	}
	return ""
}

// getTemplate returns a Formatted Deployment that can be used as a template.
// It has no metadata, no name and no updatedAt timestamp for environment_variables.
// The output templates can be modified and used to create deployments.
func getTemplate(formattedDeployment *FormattedDeployment) FormattedDeployment {
	template := *formattedDeployment
	template.Deployment.Configuration.Name = ""
	template.Deployment.Metadata = nil
	newEnvVars := []EnvironmentVariable{}

	for i := range template.Deployment.EnvVars {
		if !template.Deployment.EnvVars[i].IsSecret {
			newEnvVars = append(newEnvVars, template.Deployment.EnvVars[i])
		}
	}
	template.Deployment.EnvVars = newEnvVars
	if template.Deployment.Configuration.Executor == deployment.KubeExecutor {
		var newWorkerQs []Workerq
		for i := range template.Deployment.WorkerQs {
			if template.Deployment.WorkerQs[i].Name == "default" {
				template.Deployment.WorkerQs[i].PodCPU = ""
				template.Deployment.WorkerQs[i].PodRAM = ""
				newWorkerQs = append(newWorkerQs, template.Deployment.WorkerQs[i])
			}
		}
		template.Deployment.WorkerQs = newWorkerQs
	}

	return template
}

func GetDefaultWorkerType(taskPodNodePoolID, clusterID string, astroV1Client astrov1.APIClient) (string, error) {
	var defaultWorkerType string
	cluster, err := deployment.GetClusterByID("", clusterID, astroV1Client)
	if err != nil {
		return "", err
	}
	nodePools := *cluster.NodePools
	for i := range nodePools {
		if nodePools[i].Id == taskPodNodePoolID {
			defaultWorkerType = nodePools[i].NodeInstanceType
		}
	}

	return defaultWorkerType, nil
}
