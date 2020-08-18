package workspace

import (
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/user"
)

// Add a user to a workspace with specified role
func Add(workspaceID string, email, role string, client *houston.Client, out io.Writer) error {
	if !user.IsValidEmail(email) {
		return errors.New(email + " is an invalid email address")
	}

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

	tab.AddRow([]string{w.Label, w.Id, email, role}, false)
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

	utab.AddRow([]string{w.Label, w.Id, userID}, false)
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
	for _, role := range workspace.RoleBindings {
		var color bool
		tab.AddRow([]string{role.User.Username, role.User.Id, role.Role}, color)
	}

	tab.Print(out)
	return nil
}

// UpdateRole workspace user role
func UpdateRole(workspaceID string, email string, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "email": email, "role": role},
	}
	r, err := req.DoWithClient(client)

	if err != nil {
		return err
	}
	newRole := r.Data.WorkspaceUpdateUserRole

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", role, newRole, email)
	return nil
}
