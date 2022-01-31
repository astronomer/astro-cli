package workspace

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errUserNotInWorkspace = errors.New("the user you are trying to change is not part of this workspace")

// Add a user to a workspace with specified role
func Add(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.AddUserToWorkspace(workspaceID, email, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "WORKSPACE ID", "EMAIL", "ROLE"},
	}

	tab.AddRow([]string{w.Label, w.ID, email, role}, false)
	tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", email, w.Label)
	tab.Print(out)

	return nil
}

// Remove a user from a workspace
func Remove(workspaceID, userID string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.DeleteUserFromWorkspace(workspaceID, userID)
	if err != nil {
		return err
	}

	utab := printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "WORKSPACE ID", "USER_ID"},
	}

	utab.AddRow([]string{w.Label, w.ID, userID}, false)
	utab.SuccessMsg = "Successfully removed user from workspace"
	utab.Print(out)
	return nil
}

// ListRoles print users and roles from a workspace
func ListRoles(workspaceID string, client houston.ClientInterface, out io.Writer) error {
	workspace, err := client.ListUserAndRolesFromWorkspace(workspaceID)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID", "ROLE"},
	}
	for i := range workspace.RoleBindings {
		role := workspace.RoleBindings[i]
		var color bool
		if role.User.Username != "" {
			tab.AddRow([]string{role.User.Username, role.User.ID, role.Role}, color)
		} else {
			tab.AddRow([]string{role.ServiceAccount.Label, role.ServiceAccount.ID, role.Role}, color)
		}
	}
	tab.Print(out)
	return nil
}

// Update workspace user role
func UpdateRole(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	// get user you are updating to show role from before change
	roles, err := client.GetUserRoleInWorkspace(workspaceID, email)
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
		return errUserNotInWorkspace
	}

	newRole, err := client.UpdateUserRoleInWorkspace(workspaceID, email, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", rb.Role, newRole, email)
	return nil
}
