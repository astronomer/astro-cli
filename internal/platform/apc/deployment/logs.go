package deployment

import (
	"io"
	"time"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

var subscribe = houston.Subscribe

// Log returns a Deployment component's log records from the last since,
// matching search when it is set.
func Log(deploymentID, component, search string, since time.Duration, client houston.ClientInterface) ([]houston.DeploymentLog, error) {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	now := time.Now().UTC()
	timestamp := now.Add(-since)
	request := houston.ListDeploymentLogsRequest{
		DeploymentID: deploymentID,
		Component:    component,
		Search:       search,
		Timestamp:    &timestamp,
	}
	// --since asks for the window it names. Sent as timestamp alone, Houston
	// searches the whole UTC day that timestamp falls in (houston-api
	// ), so --since 5m returned the
	// day; with startTime and endTime it searches exactly the window. With
	// no --since, today's logs, as before.
	if since > 0 {
		request.LogWindow = &houston.LogWindow{StartTime: timestamp, EndTime: now}
	}

	return houston.Call(client.ListDeploymentLogs)(request)
}

// SubscribeDeploymentLog follows a Deployment component's log records from
// the last since, handing each to onLog until the person interrupts it or the
// stream fails (see houston.Subscribe). Its notes go to notes.
func SubscribeDeploymentLog(deploymentID, component, search string, since time.Duration, notes io.Writer, onLog func(houston.DeploymentLog) error) error {
	// Calculate timestamp as now - since e.g:
	// (2019-04-02 17:51:03.780819 +0000 UTC - 2 mins) = 2019-04-02 17:49:03.780819 +0000 UTC
	timestamp := time.Now().UTC().Add(-since)
	request, _ := houston.BuildDeploymentLogsSubscribeRequest(deploymentID, component, search, timestamp) //nolint:errcheck // error deliberately ignored in this shell code
	cl, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	return subscribe(cl.Token, cl.GetSoftwareWebsocketURL(), request, notes, onLog)
}
