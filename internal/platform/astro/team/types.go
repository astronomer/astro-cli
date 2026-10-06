package team

import "time"

// TeamInfo represents a team in structured output format
type TeamInfo struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	WorkspaceRole  string    `json:"workspace_role,omitempty"`
	DeploymentRole string    `json:"deployment_role,omitempty"`
	OrgRole        string    `json:"org_role,omitempty"`
	IsIdpManaged   bool      `json:"is_idp_managed,omitempty"`
}

// TeamList represents a list of teams for structured output
type TeamList struct {
	Teams []TeamInfo `json:"teams"`
}
