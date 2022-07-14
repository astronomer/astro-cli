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
	"github.com/astronomer/astro-cli/pkg/util"
)

var errUserNotInWorkspace = errors.New("the user you are trying to change is not part of this workspace")

var (
	defaultPaginationOptions         = "f. first p. previous n. next q. quit\n"
	paginationWithoutNextOptions     = "f. first p. previous q. quit\n"
	paginationWithoutPreviousOptions = "f. first n. next q. quit\n"
)

type PaginationOptions struct {
	cursorID string
	pageSize float64
	quit     bool
	init     bool
}

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
		tab.AddRow([]string{users[i].Username, users[i].ID, users[i].RoleBindings[0].Role}, color)
	}
	tab.Print(out)
	return nil
}

// PaginatedListRoles print users and roles from a workspace
func PaginatedListRoles(workspaceID string, cursorID string, take float64, init bool, client houston.ClientInterface, out io.Writer) error {
	err := util.ClearTerminal()
	if err != nil {
		return err
	}

	users, err := client.ListWorkspacePaginatedUserAndRoles(workspaceID, cursorID, take)
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
	if init && totalUsers < int(take) {
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

	selectedOption := PromptPaginatedOption(previousCursorID, nextCursorID, take, totalUsers)
	if selectedOption.quit {
		return nil
	}

	return PaginatedListRoles(workspaceID, selectedOption.cursorID, selectedOption.pageSize, selectedOption.init, client, out)
}

// PromptPaginatedOption Show pagination option based on page size and total record
func PromptPaginatedOption(previousCursorID string, nextCursorID string, take float64, totalRecord int) PaginationOptions {
	pageSize := math.Abs(take)
	gotoOptionMessage := defaultPaginationOptions
	gotoOptions := make(map[string]PaginationOptions)
	gotoOptions["f"] = PaginationOptions{cursorID: "", pageSize: pageSize, quit: false, init: false}
	gotoOptions["p"] = PaginationOptions{cursorID: previousCursorID, pageSize: -pageSize, quit: false, init: false}
	gotoOptions["n"] = PaginationOptions{cursorID: nextCursorID, pageSize: pageSize, quit: false, init: false}
	gotoOptions["q"] = PaginationOptions{cursorID: "", pageSize: 0, quit: true, init: false}

	if totalRecord < int(take) {
		delete(gotoOptions, "n")
		gotoOptionMessage = paginationWithoutNextOptions
	} else if int(take) < 0 && int(pageSize) > totalRecord {
		delete(gotoOptions, "p")
		gotoOptionMessage = paginationWithoutPreviousOptions
	}

	in := input.Text("\n\nSelect below options to goto\n" + gotoOptionMessage)
	value, found := gotoOptions[in]
	if found {
		return value
	}
	fmt.Print("\nInvalid option")
	return PromptPaginatedOption(previousCursorID, nextCursorID, take, totalRecord)
}

// Update workspace user role
func UpdateRole(workspaceID, email, role string, client houston.ClientInterface, out io.Writer) error {
	// get user you are updating to show role from before change
	roles, err := client.GetWorkspaceUserRole(workspaceID, email)
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

	newRole, err := client.UpdateWorkspaceUserRole(workspaceID, email, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed from %s to %s for user %s", rb.Role, newRole, email)
	return nil
}
