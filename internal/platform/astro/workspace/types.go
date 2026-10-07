package workspace

// WorkspaceInfo represents simplified workspace information for output formatting.
//
// It is also the one Workspace `workspace create`, `update` and `switch`
// publish, and `organization switch` beside its Organization: the Workspace as
// `workspace list` shows it.
type WorkspaceInfo struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	IsCurrent bool   `json:"is_current"`
}

// WorkspaceList represents a list of workspaces for output formatting
type WorkspaceList struct {
	Workspaces []WorkspaceInfo `json:"workspaces"`
}

// Updated is what an update did: the Workspace as the update left it, and the
// name it had before, which is the one the text names.
type Updated struct {
	Workspace    WorkspaceInfo
	PreviousName string
}

// Removal is what `workspace delete` deleted, under the keys the Workspace's
// other shapes use, and what was done to it.
type Removal struct {
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Action      string `json:"action"`
}

// ActionDeleted is a Removal's action.
const ActionDeleted = "deleted"
