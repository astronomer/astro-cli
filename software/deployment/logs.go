package deployment

import (
	"fmt"
	"io"
	"time"

	"github.com/astronomer/astro-cli/config"

	"github.com/astronomer/astro-cli/houston"
)

var subscribe = houston.Subscribe

func Log(deploymentID, component, search string, since time.Duration, client houston.ClientInterface, out io.Writer) error {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	timestamp := time.Now().UTC().Add(-since)
	request := houston.ListDeploymentLogsRequest{
		DeploymentID: deploymentID,
		Component:    component,
		Search:       search,
		Timestamp:    timestamp,
	}

	logs, err := houston.Call(client.ListDeploymentLogs)(request)
	if err != nil {
		return err
	}

	for _, log := range logs {
		fmt.Fprintln(out, log.Log)
	}
	return nil
}

func SubscribeDeploymentLog(deploymentID, component, search string, since time.Duration) error {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	timestamp := time.Now().UTC().Add(-since)
	request, _ := houston.BuildDeploymentLogsSubscribeRequest(deploymentID, component, search, timestamp)
	cl, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	err = subscribe(cl.Token, cl.GetSoftwareWebsocketURL(), request)
	if err != nil {
		return err
	}
	return nil
}
