package deployment

// DeploymentInfo represents simplified deployment information for output formatting
type DeploymentInfo struct {
	Name                     string `json:"name"`
	WorkspaceName            string `json:"workspaceName,omitempty"`
	Namespace                string `json:"namespace"`
	ClusterName              string `json:"clusterName,omitempty"`
	CloudProvider            string `json:"cloudProvider,omitempty"`
	Region                   string `json:"region,omitempty"`
	DeploymentID             string `json:"deploymentId"`
	RuntimeVersion           string `json:"runtimeVersion"`
	AirflowVersion           string `json:"airflowVersion"`
	IsDagDeployEnabled       bool   `json:"isDagDeployEnabled"`
	IsCicdEnforced           bool   `json:"isCicdEnforced"`
	Type                     string `json:"type"`
	IsRemoteExecutionEnabled bool   `json:"isRemoteExecutionEnabled"`
}

// DeploymentList represents a list of deployments for output formatting
type DeploymentList struct {
	Deployments []DeploymentInfo `json:"deployments"`
}
