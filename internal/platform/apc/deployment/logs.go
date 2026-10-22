package deployment

import (
	"io"
	"sort"
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
	// searches the whole UTC day that timestamp falls in, so --since 5m
	// returned the day; with startTime and endTime it searches exactly the
	// window. With no --since, today's logs, as before.
	if since > 0 {
		request.LogWindow = &houston.LogWindow{StartTime: timestamp, EndTime: now}
	}

	return houston.Call(client.ListDeploymentLogs)(request)
}

// ComponentLog is a log record and the component Houston gave it for.
type ComponentLog struct {
	Component string
	houston.DeploymentLog
}

// Logs returns the log records of each of components from the last since,
// matching search when it is set, oldest first. Houston answers for one
// component at a time, so this asks it once for each and merges the answers
// by their time; records it gives no time it can read keep their place
// ahead of the rest.
func Logs(deploymentID string, components []string, search string, since time.Duration, client houston.ClientInterface) ([]ComponentLog, error) {
	var all []ComponentLog
	for _, c := range components {
		logs, err := Log(deploymentID, c, search, since, client)
		if err != nil {
			return nil, err
		}
		for _, l := range logs {
			all = append(all, ComponentLog{Component: c, DeploymentLog: l})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return logTime(all[i].CreatedAt).Before(logTime(all[j].CreatedAt)) })
	return all, nil
}

// logTime is the time a record's createdAt names, or the zero time when it
// names none Houston's format can read.
func logTime(createdAt string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return time.Time{}
	}
	return t
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
