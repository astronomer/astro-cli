package workspace

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/pkg/errors"
)

// Add a user to a workspace with specified role
func Add(workspaceID, email, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserAddRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID, "email": email, "role": role},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	w := r.Data.AddWorkspaceUser

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
func Remove(workspaceID, userID string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserRemoveRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID, "userId": userID},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	w := r.Data.RemoveWorkspaceUser

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
func ListRoles(workspaceID string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID},
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	workspace := r.Data.GetWorkspaces[0]

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID", "ROLE"},
	}
	for i := range workspace.RoleBindings {
		role := workspace.RoleBindings[i]
		var color bool
		if len(role.User.Username) != 0 {
			tab.AddRow([]string{role.User.Username, role.User.ID, role.Role}, color)
		} else {
			tab.AddRow([]string{role.ServiceAccount.Label, role.ServiceAccount.ID, role.Role}, color)
		}
	}
	tab.Print(out)
	return nil
}

// Update workspace user role
func UpdateRole(workspaceID, email, role string, client *houston.Client, out io.Writer) error {
	// get user you are updating to show role from before change
	roles, err := getUserRole(workspaceID, email, client)
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
		return errors.New("the user you are trying to change is not part of this workspace")
	}

	req := houston.Request{
		Query:     houston.WorkspaceUserUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "email": email, "role": role},
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	newRole := r.Data.WorkspaceUpdateUserRole

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", rb.Role, newRole, email)
	return nil
}

func getUserRole(workspaceID, email string, client *houston.Client) (workspaceUserRolebindings houston.WorkspaceUserRoleBindings, err error) {
	req := houston.Request{
		Query:     houston.WorkspaceGetUserRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "email": email},
	}
	r, err := req.DoWithClient(client)
	if err != nil {
		return workspaceUserRolebindings, err
	}
	workspaceUserRolebindings = r.Data.WorkspaceGetUser

	return workspaceUserRolebindings, nil
}
