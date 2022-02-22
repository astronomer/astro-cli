package houston

// Houston Constants
const (
	// RBAC
	SystemAdminRole      = "SYSTEM_ADMIN"
	WorkspaceAdminRole   = "WORKSPACE_ADMIN"
	WorkspaceViewerRole  = "WORKSPACE_VIEWER"
	WorkspaceEditorRole  = "WORKSPACE_EDITOR"
	DeploymentRole       = "DEPLOYMENT"
	DeploymentAdminRole  = "DEPLOYMENT_ADMIN"
	DeploymentEditorRole = "DEPLOYMENT_EDITOR"
	DeploymentViewerRole = "DEPLOYMENT_VIEWER"

	// Deployment
	AirflowURLType = "airflow"

	CeleryExecutorType     = "CeleryExecutor"
	LocalExecutorType      = "LocalExecutor"
	KubernetesExecutorType = "KubernetesExecutor"

	GitSyncDeploymentType = "git_sync"
	VolumeDeploymentType  = "volume"
	ImageDeploymentType   = "image"
)
