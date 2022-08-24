package workspace

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errUserNotInWorkspace = errors.New("the user you are trying to change is not part of this workspace")

const (
	defaultPaginationOptions      = "f. first p. previous n. next q. quit\n> "
	paginationWithoutNextOptions  = "f. first p. previous q. quit\n> "
	paginationWithNextQuitOptions = "n. next q. quit\n> "
)

type PaginationOptions struct {
	cursorID   string
	pageSize   float64
	quit       bool
	pageNumber int
}

// Add a user to a workspace with specified role
// nolint: dupl
func Add(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	w, err := client.AddWorkspaceUser(houston.AddWorkspaceUserRequest{WorkspaceID: workspaceID, Email: email, Role: role})
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
	w, err := client.DeleteWorkspaceUser(houston.DeleteWorkspaceUserRequest{WorkspaceID: workspaceID, UserID: userID})
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
		tab.AddRow([]string{users[i].Username, users[i].ID, users[i].RoleBindings[0].Role}, color)
	}
	tab.Print(out)
	return nil
}

// PaginatedListRoles print users and roles from a workspace
func PaginatedListRoles(workspaceID, cursorID string, take float64, pageNumber int, client houston.ClientInterface, out io.Writer) error {
	users, err := client.ListWorkspacePaginatedUserAndRoles(houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: workspaceID, CursorID: cursorID, Take: take})
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
		tab.AddRow([]string{users[i].Username, users[i].ID, users[i].RoleBindings[0].Role}, color)
	}
	tab.Print(out)

	totalUsers := len(users)
	if pageNumber == 0 && totalUsers < int(take) {
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

	selectedOption := PromptPaginatedOption(previousCursorID, nextCursorID, take, totalUsers, pageNumber)
	if selectedOption.quit {
		return nil
	}

	return PaginatedListRoles(workspaceID, selectedOption.cursorID, selectedOption.pageSize, selectedOption.pageNumber, client, out)
}

// PromptPaginatedOption Show pagination option based on page size and total record
var PromptPaginatedOption = func(previousCursorID string, nextCursorID string, take float64, totalRecord int, pageNumber int) PaginationOptions {
	for {
		pageSize := math.Abs(take)
		gotoOptionMessage := defaultPaginationOptions
		gotoOptions := make(map[string]PaginationOptions)
		gotoOptions["f"] = PaginationOptions{cursorID: "", pageSize: pageSize, quit: false, pageNumber: 0}
		gotoOptions["p"] = PaginationOptions{cursorID: previousCursorID, pageSize: -pageSize, quit: false, pageNumber: pageNumber - 1}
		gotoOptions["n"] = PaginationOptions{cursorID: nextCursorID, pageSize: pageSize, quit: false, pageNumber: pageNumber + 1}
		gotoOptions["q"] = PaginationOptions{cursorID: "", pageSize: 0, quit: true, pageNumber: 0}

		if totalRecord < int(pageSize) {
			delete(gotoOptions, "n")
			gotoOptionMessage = paginationWithoutNextOptions
		}

		if pageNumber == 0 {
			delete(gotoOptions, "p")
			delete(gotoOptions, "f")
			gotoOptionMessage = paginationWithNextQuitOptions
		}

		in := input.Text("\n\nPlease select one of the following options\n" + gotoOptionMessage)
		value, found := gotoOptions[in]
		if found {
			return value
		}
		fmt.Print("\nInvalid option")
	}
}

// Update workspace user role
func UpdateRole(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	// get user you are updating to show role from before change
	roles, err := client.GetWorkspaceUserRole(houston.GetWorkspaceUserRoleRequest{WorkspaceID: workspaceID, Email: email})
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

	newRole, err := client.UpdateWorkspaceUserRole(houston.UpdateWorkspaceUserRoleRequest{WorkspaceID: workspaceID, Email: email, Role: role})
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", rb.Role, newRole, email)
	return nil
}
