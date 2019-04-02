package logs

import (
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/cluster"
	"github.com/astronomer/astro-cli/houston"
)

func DeploymentLog(deploymentId, component, search string, since time.Duration) error {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	timestamp := time.Now().UTC().Add(-since)
	req := houston.Request{
		Query: houston.DeploymentLogsGetRequest,
		Variables: map[string]interface{}{
			"component": component, "deploymentId": deploymentId, "search": search, "timestamp": timestamp},
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

func SubscribeDeploymentLog(deploymentId, component, search string, since time.Duration) error {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	timestamp := time.Now().UTC().Add(-since)
	request, _ := houston.BuildDeploymentLogsSubscribeRequest(deploymentId, component, search, timestamp)
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
