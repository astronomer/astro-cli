package organization

// OrganizationInfo represents simplified organization information for output formatting
type OrganizationInfo struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	IsCurrent bool   `json:"isCurrent"`
}

// OrganizationList represents a list of organizations for output formatting
type OrganizationList struct {
	Organizations []OrganizationInfo `json:"organizations"`
}

// ClusterInfo represents simplified cluster information for output formatting
type ClusterInfo struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	CloudProvider string `json:"cloudProvider"`
	Region        string `json:"region"`
	Type          string `json:"type"`
	Status        string `json:"status"`
}

// ClusterList represents a list of clusters for output formatting
type ClusterList struct {
	Clusters []ClusterInfo `json:"clusters"`
}
