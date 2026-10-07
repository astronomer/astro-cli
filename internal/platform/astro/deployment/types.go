package deployment

import (
	"time"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// DeploymentInfo represents simplified deployment information for output
// formatting. Its keys are snake_case like every other -o json shape, and
// where `deployment inspect` publishes the same fact they use its name
// (deployment_id, dag_deploy_enabled, ci_cd_enforcement), so one jq path
// reads both.
type DeploymentInfo struct {
	Name                     string `json:"name"`
	WorkspaceName            string `json:"workspace_name,omitempty"`
	Namespace                string `json:"namespace"`
	ClusterName              string `json:"cluster_name,omitempty"`
	CloudProvider            string `json:"cloud_provider,omitempty"`
	Region                   string `json:"region,omitempty"`
	DeploymentID             string `json:"deployment_id"`
	RuntimeVersion           string `json:"runtime_version"`
	AirflowVersion           string `json:"airflow_version"`
	IsDagDeployEnabled       bool   `json:"dag_deploy_enabled"`
	IsCicdEnforced           bool   `json:"ci_cd_enforcement"`
	Type                     string `json:"type"`
	IsRemoteExecutionEnabled bool   `json:"is_remote_execution_enabled"`
}

// DeploymentList represents a list of deployments for output formatting
type DeploymentList struct {
	Deployments []DeploymentInfo `json:"deployments"`
}

// UpdateResult is what Update did. Deployment is the Deployment as the update
// left it when Updated, and as it was otherwise: when it already had the DAG
// deploy setting asked for, or the question before the update was declined.
type UpdateResult struct {
	Deployment astrov1.Deployment
	Updated    bool
}

// ActionDeleted is a Removal's action: the Deployment is gone.
const ActionDeleted = "deleted"

// Removal is what `astro deployment delete` did, as it publishes it under
// --output json. Its keys are the ones the Deployment's other shapes use for
// the same facts (deployment_id, workspace_id), and action is the one
// `astro deployment token delete` publishes for a token it deleted.
type Removal struct {
	DeploymentID string `json:"deployment_id"`
	Name         string `json:"name"`
	WorkspaceID  string `json:"workspace_id"`
	Action       string `json:"action"`
}

// HibernationResult is the hibernation override a development Deployment has
// after `astro deployment hibernate` or `wake-up`, as they publish it under
// --output json. Override is null after --remove-override, when the
// Deployment's hibernation schedule, if it has one, applies again.
type HibernationResult struct {
	DeploymentID string               `json:"deployment_id"`
	Name         string               `json:"name"`
	Override     *HibernationOverride `json:"hibernation_override"`
}

// HibernationOverride is an override of a Deployment's hibernation schedule,
// under the key and with the fields `deployment inspect` gives it in the
// Deployment's metadata: whether it holds the Deployment hibernating or
// awake, and until when. override_until is null for an override that lasts
// until it is removed.
type HibernationOverride struct {
	IsHibernating bool       `json:"is_hibernating"`
	OverrideUntil *time.Time `json:"override_until"`
}

// LogsResult is what Logs found: the entries, oldest first as the API
// returns them, and, for the line that says there were none, the Deployment
// and how many hours back it looked.
type LogsResult struct {
	DeploymentName string
	Hours          int
	Entries        []LogEntry
}

// LogEntry is one line of a Deployment's logs, as `astro deployment logs -o
// json` streams it, one object per line: the component it came from
// (scheduler, worker, ...) and the line as the component wrote it, which
// begins with its own timestamp. The API's separate timestamp is not
// published: its client decodes it to a float32, which cannot hold a time in
// seconds to better than about two minutes.
type LogEntry struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}
