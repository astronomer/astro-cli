package inspect

import (
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/astro-client"
	"github.com/astronomer/astro-cli/cloud/deployment"
	"github.com/astronomer/astro-cli/config"
)

var (
	jsonMarshal = json.MarshalIndent
	yamlMarshal = yaml.Marshal
)

const (
	jsonFormat = "json"
)

func Inspect(wsID, deploymentName, deploymentID, outputFormat string, client astro.Client, out io.Writer) error {
	var (
		requestedDeployment                                   astro.Deployment
		err                                                   error
		infoToPrint                                           []byte
		deploymentInfoMap, deploymentConfigMap, additionalMap map[string]interface{}
	)
	// get or select the deployment
	requestedDeployment, err = deployment.GetDeployment(wsID, deploymentID, deploymentName, client)
	if err != nil {
		return err
	}

	deploymentInfoMap, err = getDeploymentInspectInfo(&requestedDeployment)
	if err != nil {
		return err
	}

	deploymentConfigMap = getDeploymentConfig(&requestedDeployment)

	additionalMap = getAdditional(&requestedDeployment)

	infoToPrint, err = formatPrintableDeployment(deploymentInfoMap, deploymentConfigMap, additionalMap, outputFormat)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(infoToPrint))
	return nil
}

func getDeploymentInspectInfo(sourceDeployment *astro.Deployment) (map[string]interface{}, error) {
	var (
		deploymentURL string
		c             config.Context
		err           error
	)
	c, err = config.GetCurrentContext()
	if err != nil {
		return nil, err
	}

	deploymentURL = "cloud." + c.Domain + "/" + sourceDeployment.Workspace.ID + "/deployments/" + sourceDeployment.ID + "/analytics"

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

func formatPrintableDeployment(information, configuration, additional map[string]interface{}, outputFormat string) ([]byte, error) {
	var (
		printableDeployment map[string]interface{}
		infoToPrint         []byte
		err                 error
	)
	printableDeployment = map[string]interface{}{
		"deployment": map[string]interface{}{
			"information":          information,
			"configuration":        configuration,
			"alert_emails":         additional["alert_emails"],
			"worker_queues":        additional["worker_queues"],
			"astronomer_variables": additional["astronomer_variables"],
		},
	}
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
