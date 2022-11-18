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

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
)

type deploymentMetadata struct {
	DeploymentID   string    `mapstructure:"deployment_id" yaml:"deployment_id" json:"deployment_id"`
	WorkspaceID    string    `mapstructure:"workspace_id" yaml:"workspace_id" json:"workspace_id"`
	ClusterID      string    `mapstructure:"cluster_id" yaml:"cluster_id" json:"cluster_id"`
	ReleaseName    string    `mapstructure:"release_name" yaml:"release_name" json:"release_name"`
	AirflowVersion string    `mapstructure:"airflow_version" yaml:"airflow_version" json:"airflow_version"`
	Status         string    `mapstructure:"status" yaml:"status" json:"status"`
	CreatedAt      time.Time `mapstructure:"created_at" yaml:"created_at" json:"created_at"`
	UpdatedAt      time.Time `mapstructure:"updated_at" yaml:"updated_at" json:"updated_at"`
	DeploymentURL  string    `mapstructure:"deployment_url" yaml:"deployment_url" json:"deployment_url"`
	WebserverURL   string    `mapstructure:"webserver_url" yaml:"webserver_url" json:"webserver_url"`
}

type deploymentConfig struct {
	Name           string `mapstructure:"name" yaml:"name" json:"name"`
	Description    string `mapstructure:"description" yaml:"description" json:"description"`
	RunTimeVersion string `mapstructure:"runtime_version" yaml:"runtime_version" json:"runtime_version"`
	SchedulerAU    int    `mapstructure:"scheduler_au" yaml:"scheduler_au" json:"scheduler_au"`
	SchedulerCount int    `mapstructure:"scheduler_count" yaml:"scheduler_count" json:"scheduler_count"`
	ClusterID      string `mapstructure:"cluster_id" yaml:"cluster_id" json:"cluster_id"` // this is also in deploymentMetadata
}

type workerq struct {
	Name              string `mapstructure:"name" yaml:"name" json:"name"`
	ID                string `mapstructure:"id" yaml:"id" json:"id"`
	IsDefault         bool   `mapstructure:"is_default" yaml:"is_default" json:"is_default"`
	MaxWorkerCount    int    `mapstructure:"max_worker_count" yaml:"max_worker_count" json:"max_worker_count"`
	MinWorkerCount    int    `mapstructure:"min_worker_count" yaml:"min_worker_count" json:"min_worker_count"`
	WorkerConcurrency int    `mapstructure:"worker_concurrency" yaml:"worker_concurrency" json:"worker_concurrency"`
	NodePoolID        string `mapstructure:"node_pool_id" yaml:"node_pool_id" json:"node_pool_id"`
}

type environmentVariable struct {
	IsSecret  bool   `mapstructure:"is_secret" yaml:"is_secret" json:"is_secret"`
	Key       string `mapstructure:"key" yaml:"key" json:"key"`
	UpdatedAt string `mapstructure:"updated_at" yaml:"updated_at" json:"updated_at"`
	Value     string `mapstructure:"value" yaml:"value" json:"value"`
}

type orderedPieces struct {
	EnvVars       []environmentVariable `mapstructure:"environment_variables" yaml:"environment_variables" json:"environment_variables"`
	Configuration deploymentConfig      `mapstructure:"configuration" yaml:"configuration" json:"configuration"`
	WorkerQs      []workerq             `mapstructure:"worker_queues" yaml:"worker_queues" json:"worker_queues"`
	Metadata      deploymentMetadata    `mapstructure:"metadata" yaml:"metadata" json:"metadata"`
	AlertEmails   []string              `mapstructure:"alert_emails" yaml:"alert_emails" json:"alert_emails"`
}

type formattedDeployment struct {
	Deployment orderedPieces `mapstructure:"deployment" yaml:"deployment" json:"deployment"`
}

var (
	jsonMarshal    = json.MarshalIndent
	yamlMarshal    = yaml.Marshal
	decodeToStruct = mapstructure.Decode
	errKeyNotFound = errors.New("not found in deployment")
)

const (
	jsonFormat = "json"
)

func Inspect(wsID, deploymentName, deploymentID, outputFormat string, client astro.Client, out io.Writer, requestedField string) error {
	var (
		requestedDeployment                                                        astro.Deployment
		err                                                                        error
		infoToPrint                                                                []byte
		deploymentInfoMap, deploymentConfigMap, additionalMap, printableDeployment map[string]interface{}
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(wsID, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	// create a map for deployment.information
	deploymentInfoMap, err = getDeploymentInspectInfo(&requestedDeployment)
	if err != nil {
		return err
	}
	// create a map for deployment.configuration
	deploymentConfigMap = getDeploymentConfig(&requestedDeployment)
	// create a map for deployment.alert_emails, deployment.worker_queues and deployment.astronomer_variables
	additionalMap = getAdditional(&requestedDeployment)
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
		infoToPrint, err = formatPrintableDeployment(outputFormat, printableDeployment)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(infoToPrint))
	}
	return nil
}

func getDeploymentInspectInfo(sourceDeployment *astro.Deployment) (map[string]interface{}, error) {
	var (
		deploymentURL string
		err           error
	)

	deploymentURL, err = deployment.GetDeploymentURL(sourceDeployment.ID, sourceDeployment.Workspace.ID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"deployment_id":   sourceDeployment.ID,
		"workspace_id":    sourceDeployment.Workspace.ID,
		"cluster_id":      sourceDeployment.Cluster.ID,
		"airflow_version": sourceDeployment.RuntimeRelease.AirflowVersion,
		"release_name":    sourceDeployment.ReleaseName,
		"deployment_url":  deploymentURL,
		"webserver_url":   sourceDeployment.DeploymentSpec.Webserver.URL,
		"created_at":      sourceDeployment.CreatedAt,
		"updated_at":      sourceDeployment.UpdatedAt,
		"status":          sourceDeployment.Status,
	}, nil
}

func getDeploymentConfig(sourceDeployment *astro.Deployment) map[string]interface{} {
	return map[string]interface{}{
		"name":            sourceDeployment.Label,
		"description":     sourceDeployment.Description,
		"cluster_id":      sourceDeployment.Cluster.ID,
		"runtime_version": sourceDeployment.RuntimeRelease.Version,
		"scheduler_au":    sourceDeployment.DeploymentSpec.Scheduler.AU,
		"scheduler_count": sourceDeployment.DeploymentSpec.Scheduler.Replicas,
	}
}

func getAdditional(sourceDeployment *astro.Deployment) map[string]interface{} {
	return map[string]interface{}{
		"alert_emails":          sourceDeployment.AlertEmails,
		"worker_queues":         getQMap(sourceDeployment.WorkerQueues),
		"environment_variables": getVariablesMap(sourceDeployment.DeploymentSpec.EnvironmentVariablesObjects), // API only returns values when !EnvironmentVariablesObject.isSecret
	}
}

func getQMap(sourceDeploymentQs []astro.WorkerQueue) []map[string]interface{} {
	queueMap := make([]map[string]interface{}, 0, len(sourceDeploymentQs))
	for _, queue := range sourceDeploymentQs {
		newQ := map[string]interface{}{
			"id":                 queue.ID,
			"name":               queue.Name,
			"is_default":         queue.IsDefault,
			"max_worker_count":   queue.MaxWorkerCount,
			"min_worker_count":   queue.MinWorkerCount,
			"worker_concurrency": queue.WorkerConcurrency,
			"node_pool_id":       queue.NodePoolID,
		}
		queueMap = append(queueMap, newQ)
	}
	return queueMap
}

func getVariablesMap(sourceDeploymentVars []astro.EnvironmentVariablesObject) []map[string]interface{} {
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

func formatPrintableDeployment(outputFormat string, printableDeployment map[string]interface{}) ([]byte, error) {
	var (
		infoToPrint     []byte
		err             error
		formatWithOrder formattedDeployment
	)

	// use mapstructure to decode to a struct
	err = decodeToStruct(printableDeployment, &formatWithOrder)
	if err != nil {
		return []byte{}, err
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
