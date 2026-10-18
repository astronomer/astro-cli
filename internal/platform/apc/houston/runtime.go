package houston

import "golang.org/x/mod/semver"

// RuntimeReleasesRequest is the runtimeReleases query. From Houston 2.0.0 it
// also takes workspaceUuid and deploymentUuid, so the settings a workspace or
// Deployment overrides (Airflow 3 on, its minimum runtime, listing every
// runtime) decide what it lists, as they decide appConfig's answer.
var RuntimeReleasesRequest = queryList{
	{
		version: "0.29.0",
		query: `
	query runtimeReleases($airflowVersion: String, $clusterId: Uuid) {
		runtimeReleases(airflowVersion: $airflowVersion, clusterId: $clusterId) {
			version
			airflowVersion
			airflowDatabaseMigrations
		}
	}`,
	},
	{
		version: "2.0.0",
		query: `
	query runtimeReleases($airflowVersion: String, $clusterId: Uuid, $workspaceUuid: Uuid, $deploymentUuid: Uuid) {
		runtimeReleases(airflowVersion: $airflowVersion, clusterId: $clusterId, workspaceUuid: $workspaceUuid, deploymentUuid: $deploymentUuid) {
			version
			airflowVersion
			airflowDatabaseMigrations
		}
	}`,
	},
}

// GetRuntimeReleases lists the runtime releases the platform offers. vars may
// carry airflowVersion and clusterId, and workspaceUuid and deploymentUuid,
// which a Houston before 2.0.0 does not take and is not sent.
func (h ClientImplementation) GetRuntimeReleases(vars map[string]interface{}) (RuntimeReleases, error) {
	if version != "" && semver.Compare(sanitiseVersionString(version), "v2.0.0") < 0 {
		trimmed := make(map[string]interface{}, len(vars))
		for k, v := range vars {
			if k != "workspaceUuid" && k != "deploymentUuid" {
				trimmed[k] = v
			}
		}
		vars = trimmed
	}
	dReq := Request{
		Query:     RuntimeReleasesRequest.GreatestLowerBound(version),
		Variables: vars,
	}

	resp, err := dReq.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.RuntimeReleases, nil
}
