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
func AddTeam(workspaceID, teamUuid, role string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.AddWorkspaceTeam(workspaceID, teamUuid, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE ID", "TEAM ID", "ROLE"},
	}

	tab.AddRow([]string{w.Label, w.ID, teamUuid, role}, false)
	tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", teamUuid, w.Label)
	tab.Print(out)

	return nil
}

// Remove a team from a workspace
func RemoveTeam(workspaceID, teamUuid string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.DeleteWorkspaceTeam(workspaceID, teamUuid)
	if err != nil {
		return err
	}

	utab := printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "WORKSPACE ID", "TEAM ID"},
	}

	utab.AddRow([]string{w.Label, w.ID, teamUuid}, false)
	utab.SuccessMsg = "Successfully removed team from workspace"
	utab.Print(out)
	return nil
}

// ListRoles print teams and roles from a workspace
func ListTeamRoles(workspaceID string, client houston.ClientInterface, out io.Writer) error {
	workspace, err := client.ListWorkspaceTeamsAndRoles(workspaceID)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"TEAM NAME", "ID", "ROLE"},
	}
	for i := range workspace.RoleBindings {
		role := workspace.RoleBindings[i]
		var color bool
		if role.Team.Name != "" {
			tab.AddRow([]string{role.Team.Name, role.Team.ID, role.Role}, color)
		} else {
			tab.AddRow([]string{role.ServiceAccount.Label, role.ServiceAccount.ID, role.Role}, color)
		}
	}
	tab.Print(out)
	return nil
}

// Update workspace team role
func UpdateTeamRole(workspaceID, teamUuid, role string, client houston.ClientInterface, out io.Writer) error {
	// get team you are updating to show role from before change
	roles, err := client.GetWorkspaceTeamRole(workspaceID, teamUuid)
	if err != nil {
		return err
	}

	var rb houston.RoleBindingWorkspace
	for _, val := range roles.RoleBindings {
		if val.Workspace.ID == workspaceID && strings.Contains(val.Role, "WORKSPACE") {
			rb = val
			break
		}
	}
	// check if rolebinding is an empty structure
	if (rb == houston.RoleBindingWorkspace{}) {
		return errTeamNotInWorkspace
	}

	newRole, err := client.UpdateWorkspaceTeamRole(workspaceID, teamUuid, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for team %s", rb.Role, newRole, teamUuid)
	return nil
}
