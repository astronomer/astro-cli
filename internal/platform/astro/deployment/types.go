package deployment

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
