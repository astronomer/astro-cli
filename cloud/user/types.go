package user

import "time"

// UserInfo represents a user in structured output format
type UserInfo struct {
	FullName       string    `json:"fullName"`
	Email          string    `json:"email"`
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"createdAt"`
	WorkspaceRole  string    `json:"workspaceRole,omitempty"`
	DeploymentRole string    `json:"deploymentRole,omitempty"`
	OrgRole        string    `json:"orgRole,omitempty"`
	IsIdpManaged   *bool     `json:"isIdpManaged,omitempty"`
}

// UserList represents a list of users for structured output
type UserList struct {
	Users []UserInfo `json:"users"`
}
