package workspace

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/software/utils"
)

var (
	errUserNotInWorkspace = errors.New("the user you are trying to change is not part of this workspace")

	// monkey patched to write unit tests
	promptPaginatedOption = utils.PromptPaginatedOption
)

// Add a user to a workspace with specified role
// nolint: dupl
func Add(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.AddWorkspaceUser(workspaceID, email, role)
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
// nolint: dupl
func Remove(workspaceID, userID string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.DeleteWorkspaceUser(workspaceID, userID)
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
	users, err := client.ListWorkspaceUserAndRoles(workspaceID)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID", "ROLE"},
	}
	for i := range users {
		var color bool
		role := getWorkspaceLevelRole(users[i].RoleBindings, workspaceID)
		if role != houston.NoneTeamRole {
			tab.AddRow([]string{users[i].Username, users[i].ID, role}, color)
		}
	}
	tab.Print(out)
	return nil
}

// PaginatedListRoles print users and roles from a workspace
func PaginatedListRoles(workspaceID, cursorID string, take, pageNumber int, client houston.ClientInterface, out io.Writer) error {
	users, err := client.ListWorkspacePaginatedUserAndRoles(workspaceID, cursorID, float64(take))
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"USERNAME", "ID", "ROLE"},
	}
	for i := range users {
		var color bool
		role := getWorkspaceLevelRole(users[i].RoleBindings, workspaceID)
		if role != houston.NoneTeamRole {
			tab.AddRow([]string{users[i].Username, users[i].ID, role}, color)
		}
	}
	tab.Print(out)

	totalUsers := len(users)
	if pageNumber == 0 && totalUsers < take {
		return nil
	}

	var (
		previousCursorID string
		nextCursorID     string
	)
	if totalUsers > 0 {
		previousCursorID = users[0].ID
		nextCursorID = users[totalUsers-1].ID
	}

	if totalUsers == 0 && take < 0 {
		nextCursorID = ""
	} else if totalUsers == 0 && take > 0 {
		previousCursorID = ""
	}

	// Houston query does not send back total records in response to calculate if its last page or not
	selectedOption := promptPaginatedOption(previousCursorID, nextCursorID, take, totalUsers, pageNumber, false)
	if selectedOption.Quit {
		return nil
	}

	return PaginatedListRoles(workspaceID, selectedOption.CursorID, selectedOption.PageSize, selectedOption.PageNumber, client, out)
}

// Update workspace user role
func UpdateRole(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	// get user you are updating to show role from before change
	roles, err := client.GetWorkspaceUserRole(workspaceID, email)
	if err != nil {
		return err
	}

	var rb houston.RoleBinding
	var found bool
	for idx := range roles.RoleBindings {
		if roles.RoleBindings[idx].Workspace.ID == workspaceID && strings.Contains(roles.RoleBindings[idx].Role, "WORKSPACE") {
			rb = roles.RoleBindings[idx]
			found = true
			break
		}
	}
	// check if rolebinding is an empty structure
	if !found {
		return errUserNotInWorkspace
	}

	newRole, err := client.UpdateWorkspaceUserRole(workspaceID, email, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", rb.Role, newRole, email)
	return nil
}
