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

// Action is what a command did to a team, or to a team's member.
type Action string

const (
	// Deleted: the team no longer exists.
	Deleted Action = "deleted"
	// Removed: the team, or the user, exists without the role or the
	// membership the command is about.
	Removed Action = "removed"
	// Added: the user is now a member of the team.
	Added Action = "added"
)

// WorkspaceRemoval names the team a workspace team remove acted on, and what
// it did, which is always Removed: the team stays in the Organization.
type WorkspaceRemoval struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id"`
	Action      Action `json:"action"`
}

// OrganizationRemoval names the team an organization team delete acted on,
// and what it did, which is always Deleted.
type OrganizationRemoval struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	OrganizationID string `json:"organization_id"`
	Action         Action `json:"action"`
}

// Member is a user as a team's member list reports it.
type Member struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

// MemberList is what `astro organization team user list` publishes. Members
// is empty, never nil, when the team has none.
type MemberList struct {
	Members []Member `json:"members"`
}

// Membership names the team and the user a team user add or remove acted on,
// and what it did: Added or Removed.
type Membership struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Action   Action `json:"action"`
}

// Update is what an organization team update did: the team as it left it,
// the name it had before (the name the text reports), and whether it changed
// the team's Organization role. An update whose role change failed after the
// rename went through returns the rename with the error.
type Update struct {
	Team         TeamInfo
	PreviousName string
	RoleChanged  bool
}
