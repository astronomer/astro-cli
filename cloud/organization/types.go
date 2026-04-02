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
