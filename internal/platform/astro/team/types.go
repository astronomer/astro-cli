package team

import "time"

// TeamInfo represents a team in structured output format
type TeamInfo struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	WorkspaceRole  string    `json:"workspaceRole,omitempty"`
	DeploymentRole string    `json:"deploymentRole,omitempty"`
	OrgRole        string    `json:"orgRole,omitempty"`
	IsIdpManaged   bool      `json:"isIdpManaged,omitempty"`
}

// TeamList represents a list of teams for structured output
type TeamList struct {
	Teams []TeamInfo `json:"teams"`
}
