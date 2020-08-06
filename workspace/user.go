package workspace

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// Add a user to a workspace with specified role
func Add(workspaceId, email, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserAddRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId, "email": email, "role": role},
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

	tab.AddRow([]string{w.Label, w.Id, email, role}, false)
	tab.SuccessMsg = fmt.Sprintf("Successfully added %s to %s", email, w.Label)
	tab.Print(out)

	return nil
}

// Remove a user from a workspace
func Remove(workspaceId, userId string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserRemoveRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId, "userId": userId},
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

	utab.AddRow([]string{w.Label, w.Id, userId}, false)
	utab.SuccessMsg = "Successfully removed user from workspace"
	utab.Print(out)
	return nil
}

// ListRoles print users and roles from a workspace
func ListRoles(workspaceId string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId},
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
	for _, role := range workspace.RoleBindings {
		var color bool
		tab.AddRow([]string{role.User.Username, role.User.Id, role.Role}, color)
	}

	tab.Print(out)
	return nil
}

// Update workspace user role
func UpdateRole(workspaceId, email, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceId, "email": email, "role": role},
	}
	r, err := req.DoWithClient(client)

	if err != nil {
		return err
	}
	newRole := r.Data.WorkspaceUpdateUserRole

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", role, newRole, email)
	return nil
}
