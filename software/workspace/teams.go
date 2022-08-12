package workspace

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errTeamNotInWorkspace = errors.New("the team you are trying to change is not part of this workspace")

// Add a team to a workspace with specified role
// nolint: dupl
func AddTeam(workspaceID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	w, err := houston.Call(client.AddWorkspaceTeam, houston.AddWorkspaceTeamRequest{WorkspaceID: workspaceID, TeamID: teamID, Role: role})
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE ID", "TEAM ID", "ROLE"},
	}

	tab.AddRow([]string{w.Label, w.ID, teamID, role}, false)
	tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", teamID, w.Label)
	tab.Print(out)

	return nil
}

// Remove a team from a workspace
func RemoveTeam(workspaceID, teamID string, client houston.ClientInterface, out io.Writer) error {
	w, err := houston.Call(client.DeleteWorkspaceTeam, houston.DeleteWorkspaceTeamRequest{WorkspaceID: workspaceID, TeamID: teamID})
	if err != nil {
		return err
	}

	utab := printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "WORKSPACE ID", "TEAM ID"},
	}

	utab.AddRow([]string{w.Label, w.ID, teamID}, false)
	utab.SuccessMsg = "Successfully removed team from workspace"
	utab.Print(out)
	return nil
}

// ListRoles print teams and roles from a workspace
func ListTeamRoles(workspaceID string, client houston.ClientInterface, out io.Writer) error {
	workspaceTeams, err := houston.Call(client.ListWorkspaceTeamsAndRoles, workspaceID)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"WORKSPACE ID", "TEAM ID", "TEAM NAME", "ROLE"},
	}
	for i := range workspaceTeams {
		for j := range workspaceTeams[i].RoleBindings {
			tab.AddRow([]string{workspaceID, workspaceTeams[i].ID, workspaceTeams[i].Name, workspaceTeams[i].RoleBindings[j].Role}, false)
		}
	}
	tab.Print(out)
	return nil
}

// Update workspace team role
// nolint: dupl
func UpdateTeamRole(workspaceID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	// get team you are updating to show role from before change
	teams, err := houston.Call(client.GetWorkspaceTeamRole, houston.GetWorkspaceTeamRoleRequest{WorkspaceID: workspaceID, TeamID: teamID})

	if teams == nil || err != nil {
		return errTeamNotInWorkspace
	}

	var rb *houston.RoleBinding
	roles := teams.RoleBindings
	for i := range roles {
		if roles[i].Workspace.ID == workspaceID && strings.Contains(roles[i].Role, "WORKSPACE") {
			rb = &roles[i]
			break
		}
	}
	// check if rolebinding is an empty structure
	if rb == nil {
		return errTeamNotInWorkspace
	}

	newRole, err := houston.Call(client.UpdateWorkspaceTeamRole, houston.UpdateWorkspaceTeamRoleRequest{WorkspaceID: workspaceID, TeamID: teamID, Role: role})
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for team %s\n", rb.Role, newRole, teamID)
	return nil
}
