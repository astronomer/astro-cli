package user

import "time"

// UserInfo represents a user in structured output format
type UserInfo struct {
	FullName       string    `json:"full_name"`
	Email          string    `json:"email"`
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	WorkspaceRole  string    `json:"workspace_role,omitempty"`
	DeploymentRole string    `json:"deployment_role,omitempty"`
	OrgRole        string    `json:"org_role,omitempty"`
	IsIdpManaged   *bool     `json:"is_idp_managed,omitempty"`
}

// UserList represents a list of users for structured output
type UserList struct {
	Users []UserInfo `json:"users"`
}
