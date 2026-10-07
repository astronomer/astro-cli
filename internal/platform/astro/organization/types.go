package organization

import "github.com/astronomer/astro-cli/internal/platform/astro/workspace"

// Switched is what Switch did: the Organization now current, and whether it
// is another one than before.
type Switched struct {
	Organization OrganizationInfo
	Changed      bool
}

// SwitchResult is what `astro organization switch` publishes: what the run
// left current. The Organization as `organization list` shows one, and the
// Workspace as `workspace list` (and `workspace switch`) shows one, or null
// when no Workspace of the Organization is current.
type SwitchResult struct {
	Organization OrganizationInfo         `json:"organization"`
	Workspace    *workspace.WorkspaceInfo `json:"workspace"`
}

// OrganizationInfo represents simplified organization information for output formatting
type OrganizationInfo struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	IsCurrent bool   `json:"is_current"`
}

// OrganizationList represents a list of organizations for output formatting
type OrganizationList struct {
	Organizations []OrganizationInfo `json:"organizations"`
}

// ClusterInfo represents simplified cluster information for output formatting
type ClusterInfo struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	CloudProvider string `json:"cloud_provider"`
	Region        string `json:"region"`
	Type          string `json:"type"`
	Status        string `json:"status"`
}

// ClusterList represents a list of clusters for output formatting
type ClusterList struct {
	Clusters []ClusterInfo `json:"clusters"`
}
