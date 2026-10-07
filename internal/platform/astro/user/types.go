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

// Action is what a remove did to a user's role.
type Action string

// Removed: the user exists, without a role on the object the command is about.
const Removed Action = "removed"

// WorkspaceRemoval names the user a workspace user remove acted on, and what
// it did, which is always Removed: the user stays in the Organization.
type WorkspaceRemoval struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	WorkspaceID string `json:"workspace_id"`
	Action      Action `json:"action"`
}

// DeploymentRemoval names the user a deployment user remove acted on, and
// what it did, which is always Removed: the user stays in the Organization.
type DeploymentRemoval struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DeploymentID string `json:"deployment_id"`
	Action       Action `json:"action"`
}

// Invite is the invitation `astro organization user invite` sent. UserID is
// set when the API already knows the person invited; ExpiresAt when the API
// says when the invitation lapses.
type Invite struct {
	InviteID       string     `json:"invite_id"`
	Email          string     `json:"email"`
	Role           string     `json:"role"`
	OrganizationID string     `json:"organization_id"`
	UserID         string     `json:"user_id,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}
