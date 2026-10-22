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

// Logs returns the log records of each of components from the last since,
// matching search when it is set, each with Component set to the one asked
// for. Houston answers for one component at a time, so this asks it once for
// each. One component's records keep Houston's order. Several are merged
// oldest first when Houston gave every record a time it can read, and
// otherwise left one component after another, as Houston gave them, rather
// than ordered by a guess.
func Logs(deploymentID string, components []string, search string, since time.Duration, client houston.ClientInterface) ([]houston.DeploymentLog, error) {
	var all []houston.DeploymentLog
	for _, c := range components {
		logs, err := Log(deploymentID, c, search, since, client)
		if err != nil {
			return nil, err
		}
		for _, l := range logs {
			l.Component = c
			all = append(all, l)
		}
	}
	if len(components) < 2 {
		return all, nil
	}
	type timed struct {
		at  time.Time
		log houston.DeploymentLog
	}
	merged := make([]timed, len(all))
	for i, l := range all {
		at, err := time.Parse(time.RFC3339Nano, l.CreatedAt)
		if err != nil {
			return all, nil
		}
		merged[i] = timed{at: at, log: l}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].at.Before(merged[j].at) })
	for i := range merged {
		all[i] = merged[i].log
	}
	return all, nil
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
