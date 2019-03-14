package logs

import (
	"fmt"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/houston"
)

func DeploymentLog(deploymentId, component, search string) error {
	req := houston.Request{
		Query: houston.DeploymentLogsGetRequest,
		Variables: map[string]interface{}{
			"component": component, "deploymentId": deploymentId, "search": search},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}
	for _, log := range r.Data.DeploymentLog {
		fmt.Print(log.Log)
	}
	return nil
}

func SubscribeDeploymentLog(deploymentId, component, search string) error {
	request, _ := houston.BuildDeploymentLogsSubscribeRequest(deploymentId, component, search)
	cl, err := cluster.GetCurrentCluster()
	if err != nil {
		return err
	}

	err = houston.Subscribe(cl.Token, cl.GetWebsocketURL(), request)
	if err != nil {
		return err
	}
	return nil
}
