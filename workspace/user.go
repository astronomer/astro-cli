package workspace

import (
	"fmt"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	utab = printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "WORKSPACE ID", "EMAIL"},
	}
)

// Add a user to a workspace with specified role
func Add(workspaceId, email, role string) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserAddRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId, "email": email, "role": role},
	}

	r, err := req.Do()
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
	tab.Print()

	return nil
}

// Remove a user from a workspace
func Remove(workspaceId, email string) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserRemoveRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId, "email": email},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}
	w := r.Data.RemoveWorkspaceUser

	utab.AddRow([]string{w.Label, w.Id, email}, false)
	utab.SuccessMsg = "Successfully removed user from workspace"
	utab.Print()
	return nil
}

// ListRoles print users and roles from a workspace
func ListRoles(workspaceId string) error {
	req := houston.Request{
		Query:     houston.WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId},
	}
	r, err := req.Do()

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

	tab.Print()
	return nil
}

func UpdateRole(workspaceId, email, role string) error {
	req := houston.Request{
		Query:     houston.WorkspaceUserUpdateRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceId, "email": email, "role": role},
	}
	r, err := req.Do()

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

	tab.Print()
	return nil
}
