package workspace

import (
	"errors"
	"strings"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/utils"
)

var (
	errUserNotInWorkspace = errors.New("the user you are trying to change is not part of this workspace")

	// monkey patched to write unit tests
	promptPaginatedOption = utils.PromptPaginatedOption
)

// UserRole is a user and the role they hold on a Workspace. ID is empty
// when Houston's answer did not name them, FullName when it did not give it.
type UserRole struct {
	ID       string
	Username string
	FullName string
	Role     string
}

// UserRoleChange is what a Workspace user update did: the user, the role
// they held before, and the role they hold now.
type UserRoleChange struct {
	User     UserRole
	Previous string
}

// Add gives the user with email role on a Workspace, and returns the
// Workspace as Houston returned it and the user it added: named as Houston
// names them, and unnamed when its answer does not list them.
func Add(workspaceID, email, role string, client houston.ClientInterface) (*houston.Workspace, UserRole, error) {
	w, err := houston.Call(client.AddWorkspaceUser)(houston.AddWorkspaceUserRequest{WorkspaceID: workspaceID, Email: email, Role: role})
	if err != nil {
		return nil, UserRole{}, err
	}
	w = orWorkspace(w, workspaceID)
	added := UserRole{Role: role}
	// Houston returns the Workspace with its users, the one added among them:
	// its ID is there, under its username.
	for i := range w.Users {
		if strings.EqualFold(w.Users[i].Username, email) {
			added.ID, added.Username = w.Users[i].ID, w.Users[i].Username
			break
		}
	}
	return w, added, nil
}

// Remove takes the user with userID off a Workspace, and returns the
// Workspace as Houston returned it.
func Remove(workspaceID, userID string, client houston.ClientInterface) (*houston.Workspace, error) {
	w, err := houston.Call(client.DeleteWorkspaceUser)(houston.DeleteWorkspaceUserRequest{WorkspaceID: workspaceID, UserID: userID})
	if err != nil {
		return nil, err
	}
	return orWorkspace(w, workspaceID), nil
}

// ListRoles returns the users with a role on a Workspace, and the role each
// holds there.
func ListRoles(workspaceID string, client houston.ClientInterface) ([]UserRole, error) {
	users, err := houston.Call(client.ListWorkspaceUserAndRoles)(workspaceID)
	if err != nil {
		return nil, err
	}
	return withWorkspaceRole(users, workspaceID), nil
}

func withWorkspaceRole(users []houston.WorkspaceUserRoleBindings, workspaceID string) []UserRole {
	out := make([]UserRole, 0, len(users))
	for i := range users {
		role := getWorkspaceLevelRole(users[i].RoleBindings, workspaceID)
		if role != houston.NoneRole {
			out = append(out, UserRole{ID: users[i].ID, Username: users[i].Username, FullName: users[i].FullName, Role: role})
		}
	}
	return out
}

// PaginatedListRoles shows a Workspace's users a page at a time, asking which
// page to show next, until the person quits. render draws each page.
func PaginatedListRoles(workspaceID, cursorID string, take, pageNumber int, client houston.ClientInterface, render func([]UserRole) error) error {
	users, err := houston.Call(client.ListWorkspacePaginatedUserAndRoles)(houston.PaginatedWorkspaceUserRolesRequest{WorkspaceID: workspaceID, CursorID: cursorID, Take: float64(take)})
	if err != nil {
		return err
	}
	if err := render(withWorkspaceRole(users, workspaceID)); err != nil {
		return err
	}

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
	selectedOption, err := promptPaginatedOption(previousCursorID, nextCursorID, take, totalUsers, pageNumber, false)
	if err != nil {
		return err
	}
	if selectedOption.Quit {
		return nil
	}

	return PaginatedListRoles(workspaceID, selectedOption.CursorID, selectedOption.PageSize, selectedOption.PageNumber, client, render)
}

// UserRoleIn returns the user with email and the role they hold on a
// Workspace. It refuses one who has no role there.
func UserRoleIn(workspaceID, email string, client houston.ClientInterface) (UserRole, error) {
	u, err := houston.Call(client.GetWorkspaceUserRole)(houston.GetWorkspaceUserRoleRequest{WorkspaceID: workspaceID, Email: email})
	if err != nil {
		return UserRole{}, err
	}
	for i := range u.RoleBindings {
		if u.RoleBindings[i].Workspace.ID == workspaceID && strings.Contains(u.RoleBindings[i].Role, "WORKSPACE") {
			return UserRole{ID: u.ID, Username: u.Username, FullName: u.FullName, Role: u.RoleBindings[i].Role}, nil
		}
	}
	return UserRole{}, errUserNotInWorkspace
}

// UpdateRole changes the role of the user with email on a Workspace. It
// looks the user up first, and refuses one who has no role there.
func UpdateRole(workspaceID, email, role string, client houston.ClientInterface) (UserRoleChange, error) {
	user, err := UserRoleIn(workspaceID, email, client)
	if err != nil {
		return UserRoleChange{}, err
	}
	newRole, err := houston.Call(client.UpdateWorkspaceUserRole)(houston.UpdateWorkspaceUserRoleRequest{WorkspaceID: workspaceID, Email: email, Role: role})
	if err != nil {
		return UserRoleChange{}, err
	}
	previous := user.Role
	// Houston answers null when the user holds more than one binding there,
	// having set the role all the same.
	if newRole == "" {
		newRole = role
	}
	user.Role = newRole
	return UserRoleChange{User: user, Previous: previous}, nil
}
