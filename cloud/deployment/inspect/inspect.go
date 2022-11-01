package inspect

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
)

var (
	jsonMarshal = json.MarshalIndent
	yamlMarshal = yaml.Marshal
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
		fmt.Fprintln(out, getSpecificField(printableDeployment, requestedField))
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
		"name":               sourceDeployment.Label,
		"description":        sourceDeployment.Description,
		"cluster_id":         sourceDeployment.Cluster.ID,
		"runtime_version":    sourceDeployment.RuntimeRelease.Version,
		"scheduler_au":       sourceDeployment.DeploymentSpec.Scheduler.AU,
		"scheduler_replicas": sourceDeployment.DeploymentSpec.Scheduler.Replicas,
	}
}

func getAdditional(sourceDeployment *astro.Deployment) map[string]interface{} {
	return map[string]interface{}{
		"alert_emails":         sourceDeployment.AlertEmails,
		"worker_queues":        getQMap(sourceDeployment.WorkerQueues),
		"astronomer_variables": getVariablesMap(sourceDeployment.DeploymentSpec.EnvironmentVariablesObjects), // API only returns values when !EnvironmentVariablesObject.isSecret
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
		infoToPrint []byte
		err         error
	)

	switch outputFormat {
	case jsonFormat:
		if infoToPrint, err = jsonMarshal(printableDeployment, "", "    "); err != nil {
			return []byte{}, err
		}
	default:
		// always yaml by default
		if infoToPrint, err = yamlMarshal(printableDeployment); err != nil {
			return []byte{}, err
		}
	}
	return infoToPrint, nil
}

// getSpecificField is used to find the requestedField in a deployment
// it splits requestedField at every "." and looks for the first 2 parts in the deployment
// if it finds both parts of the requestedField, it returns the value
// it returns nil if either part of the requestedField are not found
func getSpecificField(deploymentMap map[string]interface{}, requestedField string) any {
	keyParts := strings.Split(requestedField, ".")
	// iterate over the top level maps in a deployment like deployment.information
	for _, elem := range deploymentMap {
		// check if the first key in the requested field exists and create a subMap
		if subMap, exists := elem.(map[string]interface{})[keyParts[0]]; exists {
			if len(keyParts) > 1 {
				// check if the second key in the requested field exists
				value, ok := subMap.(map[string]interface{})[keyParts[1]]
				if ok {
					// we found the requested field so return its value
					return value
				}
			} else {
				// top level field was requested so we return it
				return subMap
			}
		}
	}
	return nil
}

func getPrintableDeployment(infoMap, configMap, additionalMap map[string]interface{}) map[string]interface{} {
	printableDeployment := map[string]interface{}{
		"deployment": map[string]interface{}{
			"information":          infoMap,
			"configuration":        configMap,
			"alert_emails":         additionalMap["alert_emails"],
			"worker_queues":        additionalMap["worker_queues"],
			"astronomer_variables": additionalMap["astronomer_variables"],
		},
	}
	return printableDeployment
}
