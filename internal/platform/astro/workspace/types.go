package workspace

// WorkspaceInfo represents simplified workspace information for output formatting
type WorkspaceInfo struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	IsCurrent bool   `json:"is_current"`
}

// WorkspaceList represents a list of workspaces for output formatting
type WorkspaceList struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`
}
