package workspace

import (
	"errors"
	"fmt"
	"strings"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

var errTeamNotInWorkspace = errors.New("the team you are trying to change is not part of this workspace")

// TeamRole is a team and the role it holds on a Workspace. Name is empty
// when Houston's answer did not give it.
type TeamRole struct {
	ID   string
	Name string
	Role string
}

// TeamRoleChange is what a Workspace team update did: the team, the role it
// held before, and the role it holds now.
type TeamRoleChange struct {
	Team     TeamRole
	Previous string
}

// AddTeam gives a team role on a Workspace, and returns the Workspace as
// Houston returned it.
func AddTeam(workspaceID, teamID, role string, client houston.ClientInterface) (*houston.Workspace, error) {
	w, err := houston.Call(client.AddWorkspaceTeam)(houston.AddWorkspaceTeamRequest{WorkspaceID: workspaceID, TeamID: teamID, Role: role})
	if err != nil {
		return nil, err
	}
	return orWorkspace(w, workspaceID), nil
}

// permissionDenied is the message of Houston's shield refusal, which the
// client hands on as the error's text.
const permissionDenied = "Insufficient permissions."

// TeamRemoval is what a Workspace team remove did. Verified is false when
// the team's role on the Workspace could not be read, so the removal was
// sent without knowing the team was there.
type TeamRemoval struct {
	Workspace *houston.Workspace
	Verified  bool
}

// RemoveTeam removes a team from a Workspace, and returns the Workspace as
// Houston returned it.
//
// Houston removes nothing for a team with no binding in the Workspace and
// still answers with the Workspace, no error, so it looks the team up first
// and refuses one that holds no Workspace role there; a team with only a
// custom role assignment is one, since the removal deletes role bindings
// only.
//
// The lookup needs workspace.teams.get, which the removal does not, and
// Houston refuses it as "Insufficient permissions." both for a login
// without that and for a team with no binding in the Workspace. The two
// cannot be told apart, so on that refusal the removal is sent anyway,
// unverified, and Houston decides.
func RemoveTeam(workspaceID, teamID string, client houston.ClientInterface) (TeamRemoval, error) {
	verified := true
	team, err := houston.Call(client.GetWorkspaceTeamRole)(houston.GetWorkspaceTeamRoleRequest{WorkspaceID: workspaceID, TeamID: teamID})
	switch {
	case err != nil && err.Error() == permissionDenied:
		verified = false
	case err != nil:
		return TeamRemoval{}, err
	case team == nil || getWorkspaceLevelRole(team.RoleBindings, workspaceID) == houston.NoneRole:
		return TeamRemoval{}, fmt.Errorf("%w: %s", errTeamNotInWorkspace, teamID)
	}
	w, err := houston.Call(client.DeleteWorkspaceTeam)(houston.DeleteWorkspaceTeamRequest{WorkspaceID: workspaceID, TeamID: teamID})
	if err != nil {
		return TeamRemoval{}, err
	}
	return TeamRemoval{Workspace: orWorkspace(w, workspaceID), Verified: verified}, nil
}

// ListTeamRoles returns the teams with a role on a Workspace, and the role
// each holds there.
func ListTeamRoles(workspaceID string, client houston.ClientInterface) ([]TeamRole, error) {
	workspaceTeams, err := houston.Call(client.ListWorkspaceTeamsAndRoles)(workspaceID)
	if err != nil {
		return nil, err
	}
	teams := make([]TeamRole, 0, len(workspaceTeams))
	for i := range workspaceTeams {
		role := getWorkspaceLevelRole(workspaceTeams[i].RoleBindings, workspaceID)
		if role != houston.NoneRole {
			teams = append(teams, TeamRole{ID: workspaceTeams[i].ID, Name: workspaceTeams[i].Name, Role: role})
		}
	}
	return teams, nil
}

// UpdateTeamRole changes a team's role on a Workspace. It looks the team up
// first, and refuses one that has no role on the Workspace.
func UpdateTeamRole(workspaceID, teamID, role string, client houston.ClientInterface) (TeamRoleChange, error) {
	team, err := houston.Call(client.GetWorkspaceTeamRole)(houston.GetWorkspaceTeamRoleRequest{WorkspaceID: workspaceID, TeamID: teamID})
	if team == nil || err != nil {
		return TeamRoleChange{}, errTeamNotInWorkspace
	}

	var previous string
	for i := range team.RoleBindings {
		if team.RoleBindings[i].Workspace.ID == workspaceID && strings.Contains(team.RoleBindings[i].Role, "WORKSPACE") {
			previous = team.RoleBindings[i].Role
			break
		}
	}
	if previous == "" {
		return TeamRoleChange{}, errTeamNotInWorkspace
	}

	newRole, err := houston.Call(client.UpdateWorkspaceTeamRole)(houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: workspaceID, TeamID: teamID, Role: role})
	if err != nil {
		return TeamRoleChange{}, err
	}
	return TeamRoleChange{Team: TeamRole{ID: teamID, Name: team.Name, Role: newRole}, Previous: previous}, nil
}

// orWorkspace is w, or a Workspace that has only the ID asked about when
// Houston returned none.
func orWorkspace(w *houston.Workspace, workspaceID string) *houston.Workspace {
	if w == nil {
		return &houston.Workspace{ID: workspaceID}
	}
	return w
}

// IsValidWorkspaceLevelRole checks if the role is amongst valid workspace roles
func IsValidWorkspaceLevelRole(role string) bool {
	switch role {
	case houston.WorkspaceAdminRole, houston.WorkspaceEditorRole, houston.WorkspaceViewerRole, houston.NoneRole:
		return true
	}
	return false
}

// getWorkspaceLevelRole returns the first workspace level role from a slice of roles
func getWorkspaceLevelRole(roles []houston.RoleBinding, workspaceID string) string {
	for i := range roles {
		if IsValidWorkspaceLevelRole(roles[i].Role) && roles[i].Workspace.ID == workspaceID {
			return roles[i].Role
		}
	}
	return houston.NoneRole
}
