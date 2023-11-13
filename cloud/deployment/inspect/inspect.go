package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"

	astrocore "github.com/astronomer/astro-cli/astro-client-core"
	astroplatformcore "github.com/astronomer/astro-cli/astro-client-platform-core"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/cloud/organization"
)

type deploymentMetadata struct {
	DeploymentID     *string    `mapstructure:"deployment_id" yaml:"deployment_id" json:"deployment_id"`
	WorkspaceID      *string    `mapstructure:"workspace_id" yaml:"workspace_id" json:"workspace_id"`
	ClusterID        *string    `mapstructure:"cluster_id" yaml:"cluster_id" json:"cluster_id"`
	ReleaseName      *string    `mapstructure:"release_name" yaml:"release_name" json:"release_name"`
	AirflowVersion   *string    `mapstructure:"airflow_version" yaml:"airflow_version" json:"airflow_version"`
	CurrentTag       *string    `mapstructure:"current_tag" yaml:"current_tag" json:"current_tag"`
	Status           *string    `mapstructure:"status" yaml:"status" json:"status"`
	CreatedAt        *time.Time `mapstructure:"created_at" yaml:"created_at" json:"created_at"`
	UpdatedAt        *time.Time `mapstructure:"updated_at" yaml:"updated_at" json:"updated_at"`
	DeploymentURL    *string    `mapstructure:"deployment_url" yaml:"deployment_url" json:"deployment_url"`
	WebserverURL     *string    `mapstructure:"webserver_url" yaml:"webserver_url" json:"webserver_url"`
	WorkloadIdentity *string    `mapstructure:"workload_identity" yaml:"workload_identity" json:"workload_identity"`
}

type deploymentConfig struct {
	Name                  string `mapstructure:"name" yaml:"name" json:"name"`
	Description           string `mapstructure:"description" yaml:"description" json:"description"`
	RunTimeVersion        string `mapstructure:"runtime_version" yaml:"runtime_version" json:"runtime_version"`
	DagDeployEnabled      *bool  `mapstructure:"dag_deploy_enabled" yaml:"dag_deploy_enabled" json:"dag_deploy_enabled"`
	APIKeyOnlyDeployments bool   `mapstructure:"ci_cd_enforcement" yaml:"ci_cd_enforcement" json:"ci_cd_enforcement"`
	SchedulerSize         string `mapstructure:"scheduler_size" yaml:"scheduler_size" json:"scheduler_size"`
	IsHighAvailability    bool   `mapstructure:"is_high_availability" yaml:"is_high_availability" json:"is_high_availability"`
	Executor              string `mapstructure:"executor" yaml:"executor" json:"executor"`
	SchedulerAU           int    `mapstructure:"scheduler_au" yaml:"scheduler_au" json:"scheduler_au"`
	SchedulerCount        int    `mapstructure:"scheduler_count" yaml:"scheduler_count" json:"scheduler_count"`
	ClusterName           string `mapstructure:"cluster_name" yaml:"cluster_name" json:"cluster_name"`
	WorkspaceName         string `mapstructure:"workspace_name" yaml:"workspace_name" json:"workspace_name"`
	DeploymentType        string `mapstructure:"deployment_type" yaml:"deployment_type" json:"deployment_type"`
	CloudProvider         string `mapstructure:"cloud_provider" yaml:"cloud_provider" json:"cloud_provider"`
	Region                string `mapstructure:"region" yaml:"region" json:"region"`
	DefaultTaskPodCpu     string `mapstructure:"default_task_pod_cpu" yaml:"default_task_pod_cpu" json:"default_task_pod_cpu"`
	DefaultTaskPodMemory  string `mapstructure:"default_task_pod_memory" yaml:"default_task_pod_memory" json:"default_task_pod_memory"`
	ResourceQuotaCpu      string `mapstructure:"resource_quota_cpu" yaml:"resource_quota_cpu" json:"resource_quota_cpu"`
	ResourceQuotaMemory   string `mapstructure:"resource_quota_memory" yaml:"resource_quota_memory" json:"resource_quota_memory"`
}

type Workerq struct {
	Name              string `mapstructure:"name" yaml:"name" json:"name"`
	MaxWorkerCount    int    `mapstructure:"max_worker_count,omitempty" yaml:"max_worker_count,omitempty" json:"max_worker_count,omitempty"`
	MinWorkerCount    *int   `mapstructure:"min_worker_count,omitempty" yaml:"min_worker_count,omitempty" json:"min_worker_count,omitempty"`
	WorkerConcurrency int    `mapstructure:"worker_concurrency,omitempty" yaml:"worker_concurrency,omitempty" json:"worker_concurrency,omitempty"`
	WorkerType        string `mapstructure:"worker_type" yaml:"worker_type" json:"worker_type"`
	PodCPU            string `mapstructure:"pod_cpu,omitempty" yaml:"pod_cpu,omitempty" json:"pod_cpu,omitempty"`
	PodRAM            string `mapstructure:"pod_ram,omitempty" yaml:"pod_ram,omitempty" json:"pod_ram,omitempty"`
}

type EnvironmentVariable struct {
	IsSecret  bool   `mapstructure:"is_secret" yaml:"is_secret" json:"is_secret"`
	Key       string `mapstructure:"key" yaml:"key" json:"key"`
	UpdatedAt string `mapstructure:"updated_at,omitempty" yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
	Value     string `mapstructure:"value" yaml:"value" json:"value"`
}

type orderedPieces struct {
	EnvVars       []EnvironmentVariable `mapstructure:"environment_variables,omitempty" yaml:"environment_variables,omitempty" json:"environment_variables,omitempty"`
	Configuration deploymentConfig      `mapstructure:"configuration" yaml:"configuration" json:"configuration"`
	WorkerQs      []Workerq             `mapstructure:"worker_queues" yaml:"worker_queues" json:"worker_queues"`
	Metadata      *deploymentMetadata   `mapstructure:"metadata,omitempty" yaml:"metadata,omitempty" json:"metadata,omitempty"`
	AlertEmails   []string              `mapstructure:"alert_emails,omitempty" yaml:"alert_emails,omitempty" json:"alert_emails,omitempty"`
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

func Inspect(wsID, deploymentName, deploymentID, outputFormat string, platformCoreClient astroplatformcore.CoreClient, coreClient astrocore.CoreClient, out io.Writer, requestedField string, template bool) error {
	var (
		requestedDeployment                                                        astroplatformcore.Deployment
		err                                                                        error
		infoToPrint                                                                []byte
		deploymentInfoMap, deploymentConfigMap, additionalMap, printableDeployment map[string]interface{}
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(wsID, deploymentID, deploymentName, true, platformCoreClient, coreClient)
	if err != nil {
		return err
	}

	if requestedDeployment.Id == "" {
		fmt.Printf("No Deployments found in workspace %s\n", ansi.Bold(wsID))
		return nil
	}

	// // get core deployment
	// coreDeployment, err := deployment.CoreGetDeployment(wsID, "", requestedDeployment.Id, platformCoreClient)
	// if err != nil {
	// 	return err
	// }

	// create a map for deployment.information
	deploymentInfoMap, err = getDeploymentInfo(requestedDeployment)
	if err != nil {
		return err
	}
	// create a map for deployment.configuration
	deploymentConfigMap = getDeploymentConfig(requestedDeployment)
	// create a map for deployment.alert_emails, deployment.worker_queues and deployment.astronomer_variables
	cluster, err := deployment.CoreGetCluster("", *requestedDeployment.ClusterId, platformCoreClient)
	if err != nil {
		return err
	}
	var nodePools = []astroplatformcore.NodePool{}
	if cluster.NodePools != nil {
		nodePools = *cluster.NodePools
	}
	additionalMap = getAdditional(requestedDeployment, nodePools)
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

func getDeploymentInfo(coreDeployment astroplatformcore.Deployment) (map[string]interface{}, error) { //nolint
	var (
		deploymentURL    string
		workloadIdentity string
		err              error
	)

	deploymentURL, err = deployment.GetDeploymentURL(coreDeployment.Id, coreDeployment.WorkspaceId)
	if err != nil {
		return nil, err
	}
	clusterID := coreDeployment.ClusterId
	releaseName := coreDeployment.Namespace
	if organization.IsOrgHosted() {
		if deployment.IsDeploymentStandard(*coreDeployment.Type) {
			clusterID = nil
		}
		releaseName = notApplicable
	}
	if coreDeployment.WorkloadIdentity != nil {
		workloadIdentity = *coreDeployment.WorkloadIdentity
	}
	return map[string]interface{}{
		"deployment_id":     coreDeployment.Id,
		"workspace_id":      coreDeployment.WorkspaceId,
		"cluster_id":        clusterID,
		"airflow_version":   nil, // coreDeployment.AirflowVersion, // need to change to Airflow version when available
		"current_tag":       coreDeployment.ImageTag,
		"release_name":      releaseName,
		"deployment_url":    deploymentURL,
		"webserver_url":     coreDeployment.WebServerUrl,
		"created_at":        coreDeployment.CreatedAt,
		"updated_at":        coreDeployment.UpdatedAt,
		"workload_identity": workloadIdentity,
		"status":            coreDeployment.Status,
	}, nil
}

func getDeploymentConfig(coreDeployment astroplatformcore.Deployment) map[string]interface{} {
	clusterName := "" // coreDeployment.ClusterName // undo once added to coreDeployment
	if organization.IsOrgHosted() {
		if deployment.IsDeploymentStandard(*coreDeployment.Type) {
			clusterName = "" // coreDeployment.ClusterRegion // undo once added to coreDeployment
		}
	}
	return map[string]interface{}{
		"name":                 coreDeployment.Name,
		"description":          coreDeployment.Description,
		"workspace_name":       nil, // coreDeployment.WorkspaceName, // undo once added to coreDeployment
		"deployment_type":      coreDeployment.Type,
		"cloud_provider":       nil, // coreDeployment.Cluster.CloudProvider, // undo once added to coreDeployment
		"region":               nil, // sourceDeployment.Cluster.Region, // undo once added to coreDeployment
		"cluster_name":         clusterName,
		"runtime_version":      coreDeployment.RuntimeVersion,
		"dag_deploy_enabled":   coreDeployment.DagDeployEnabled,
		"ci_cd_enforcement":    coreDeployment.IsCicdEnforced,
		"is_high_availability": coreDeployment.IsHighAvailability,
		"scheduler_au":         coreDeployment.SchedulerAu,
		"scheduler_count":      coreDeployment.SchedulerReplicas,
		"executor":             coreDeployment.Executor,
	}
}

func getAdditional(coreDeployment astroplatformcore.Deployment, NodePools []astroplatformcore.NodePool) map[string]interface{} {
	qList := getQMap(*coreDeployment.WorkerQueues, NodePools, coreDeployment.Executor, coreDeployment.Type)
	return map[string]interface{}{
		"alert_emails":          coreDeployment.ContactEmails,
		"worker_queues":         qList,
		"environment_variables": getVariablesMap(*coreDeployment.EnvironmentVariables), // API only returns values when !EnvironmentVariablesObject.isSecret
	}
}

func ReturnSpecifiedValue(wsID, deploymentName, deploymentID string, astroPlatformCore astroplatformcore.CoreClient, coreClient astrocore.CoreClient, requestedField string) (value any, err error) {
	var (
		requestedDeployment                                                        astroplatformcore.Deployment
		deploymentInfoMap, deploymentConfigMap, additionalMap, printableDeployment map[string]interface{}
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(wsID, deploymentID, deploymentName, false, astroPlatformCore, coreClient)
	if err != nil {
		return nil, err
	}

	// get core deployment
	// coreDeployment, err := deployment.CoreGetDeployment(wsID, "", requestedDeployment.Id, coreClient)
	// if err != nil {
	// 	return nil, err
	// }

	// create a map for deployment.information
	deploymentInfoMap, err = getDeploymentInfo(requestedDeployment)
	if err != nil {
		return nil, err
	}
	// create a map for deployment.configuration
	deploymentConfigMap = getDeploymentConfig(requestedDeployment)
	// create a map for deployment.alert_emails, deployment.worker_queues and deployment.astronomer_variables
	cluster, err := deployment.CoreGetCluster("", *requestedDeployment.ClusterId, astroPlatformCore)
	if err != nil {
		return nil, err
	}
	var nodePools = []astroplatformcore.NodePool{}
	if cluster.NodePools != nil {
		nodePools = *cluster.NodePools
	}
	additionalMap = getAdditional(requestedDeployment, nodePools)
	// create a map for the entire deployment
	printableDeployment = getPrintableDeployment(deploymentInfoMap, deploymentConfigMap, additionalMap)

	value, err = getSpecificField(printableDeployment, requestedField)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func getQMap(sourceDeploymentQs []astroplatformcore.WorkerQueue, sourceNodePools []astroplatformcore.NodePool, sourceExecutor *astroplatformcore.DeploymentExecutor, deploymentType *astroplatformcore.DeploymentType) []map[string]interface{} {
	var resources map[string]interface{}
	queueMap := make([]map[string]interface{}, 0, len(sourceDeploymentQs))
	for _, queue := range sourceDeploymentQs { //nolint
		if *sourceExecutor == astroplatformcore.DeploymentExecutorCELERY {
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
		if deployment.IsDeploymentDedicated(*deploymentType) || deployment.IsDeploymentStandard(*deploymentType) {
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

func getVariablesMap(sourceDeploymentVars []astroplatformcore.DeploymentEnvironmentVariable) []map[string]interface{} {
	variablesMap := make([]map[string]interface{}, 0, len(sourceDeploymentVars))
	for _, variable := range sourceDeploymentVars {
		newVar := map[string]interface{}{
			"key":        variable.Key,
			"value":      variable.Value,
			"is_secret":  variable.IsSecret,
			"updated_at": variable.UpdatedAt,
		}
		variablesMap = append(variablesMap, newVar)
	}
	return variablesMap
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
		},
	}
	return printableDeployment
}

// getWorkerTypeFromNodePoolID takes maps the workerType to a node pool id in nodePools.
// It returns an error if the worker type does not exist in any node pool in nodePools.
func getWorkerTypeFromNodePoolID(poolID string, nodePools []astroplatformcore.NodePool) string {
	var pool astroplatformcore.NodePool
	for _, pool = range nodePools {
		if pool.Id == poolID {
			return pool.NodeInstanceType
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
		template.Deployment.WorkerQs = nil
	}

	return template
}
