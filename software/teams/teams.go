package teams

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/logger"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/astronomer/astro-cli/software/deployment"
	"github.com/astronomer/astro-cli/software/utils"
	"github.com/astronomer/astro-cli/software/workspace"
)

const ListTeamLimit = 20

var (
	errMissingTeamID = errors.New("missing team ID")

	// monkey patched to write tests
	promptPaginatedOption = utils.PromptPaginatedOption
)

// retrieves a team and all of its users if passed optional param
func Get(teamID string, getUserInfo, getRoleInfo, allFilters bool, client houston.ClientInterface, out io.Writer) error {
	if teamID == "" {
		return errMissingTeamID
	}
	team, err := houston.Call(client.GetTeam)(teamID)
	if err != nil {
		return err
	}
	role := getSystemLevelRole(team.RoleBindings)

	fmt.Fprintf(out, "\nTeam Name: %s\nTeam ID: %s \nSystem Role: %s\n", team.Name, team.ID, role)

	if getRoleInfo || allFilters {
		workspaceRolesTable := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"WORKSPACE ID", "WORKSPACE NAME", "ROLE"},
			ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
		}

		deploymentRolesTable := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"DEPLOYMENT ID", "DEPLOYMENT NAME", "ROLE"},
			ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
		}

		for i := range team.RoleBindings {
			if workspace.IsValidWorkspaceLevelRole(team.RoleBindings[i].Role) && team.RoleBindings[i].Role != houston.NoneRole {
				workspaceRolesTable.AddRow([]string{team.RoleBindings[i].Workspace.ID, team.RoleBindings[i].Workspace.Label, team.RoleBindings[i].Role}, false)
			}
			if deployment.IsValidDeploymentLevelRole(team.RoleBindings[i].Role) && team.RoleBindings[i].Role != houston.NoneRole {
				deploymentRolesTable.AddRow([]string{team.RoleBindings[i].Deployment.ID, team.RoleBindings[i].Deployment.Label, team.RoleBindings[i].Role}, false)
			}
		}
		if len(workspaceRolesTable.Rows) > 0 {
			fmt.Fprintln(out, "\nWorkspace Level Roles:")
			workspaceRolesTable.Print(out)
		}

		if len(deploymentRolesTable.Rows) > 0 {
			fmt.Fprintln(out, "\nDeployment Level Roles:")
			deploymentRolesTable.Print(out)
		}
	}

	if getUserInfo || allFilters {
		logger.Logger.Debug("retrieving users part of team")
		fmt.Fprintln(out, "\nUsers part of Team:")
		users, err := houston.Call(client.GetTeamUsers)(teamID)
		if err != nil {
			return err
		}
		teamUsersTable := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"USERNAME", "ID"},
			ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
		}
		for i := range users {
			user := users[i]
			teamUsersTable.AddRow([]string{user.Username, user.ID}, false)
		}
		return teamUsersTable.Print(out)
	}

	return nil
}

// retrieves all teams present with the platform
func List(client houston.ClientInterface, out io.Writer) error {
	var teams []houston.Team
	var cursor string
	count := -1

	for len(teams) < count || count == -1 {
		resp, err := houston.Call(client.ListTeams)(houston.ListTeamsRequest{Take: ListTeamLimit, Cursor: cursor})
		if err != nil {
			return err
		}
		count = resp.Count
		teams = append(teams, resp.Teams...)
		if count > 0 {
			cursor = teams[len(teams)-1].ID
		}
	}

	teamsTable := printutil.Table{
		Padding:        []int{50, 50},
		DynamicPadding: true,
		Header:         []string{"TEAM ID", "TEAM NAME", "ROLE"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	for i := range teams {
		role := getSystemLevelRole(teams[i].RoleBindings)
		teamsTable.AddRow([]string{teams[i].ID, teams[i].Name, role}, false)
	}
	return teamsTable.Print(out)
}

func PaginatedList(client houston.ClientInterface, out io.Writer, pageSize, pageNumber int, cursorID string) error {
	resp, err := houston.Call(client.ListTeams)(houston.ListTeamsRequest{Cursor: cursorID, Take: pageSize})
	if err != nil {
		return err
	}
	teamsTable := printutil.Table{
		Padding:        []int{50, 50},
		DynamicPadding: true,
		Header:         []string{"TEAM ID", "TEAM NAME", "ROLE"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	for i := range resp.Teams {
		role := getSystemLevelRole(resp.Teams[i].RoleBindings)
		teamsTable.AddRow([]string{resp.Teams[i].ID, resp.Teams[i].Name, role}, false)
	}
	if err := teamsTable.Print(out); err != nil {
		return err
	}

	totalTeams := len(resp.Teams)
	var (
		previousCursor string
		nextCursor     string
	)
	if totalTeams > 0 {
		previousCursor = resp.Teams[0].ID
		nextCursor = resp.Teams[len(resp.Teams)-1].ID
	}
	if totalTeams == 0 && pageSize < 0 {
		nextCursor = ""
	} else if totalTeams == 0 && pageSize > 0 {
		previousCursor = ""
	}
	lastPage := false
	if resp.Count <= pageNumber*pageSize+totalTeams {
		lastPage = true
	}

	selectedOption := promptPaginatedOption(previousCursor, nextCursor, pageSize, totalTeams, pageNumber, lastPage)
	if selectedOption.Quit {
		return nil
	}
	return PaginatedList(client, out, selectedOption.PageSize, selectedOption.PageNumber, selectedOption.CursorID)
}

// Update will update the system role associated with the team
func Update(teamID, role string, client houston.ClientInterface, out io.Writer) error {
	if !isValidSystemLevelRole(role) {
		return fmt.Errorf("invalid role: %s, should be one of: %s, %s, %s or %s", role, houston.SystemAdminRole, houston.SystemEditorRole, houston.SystemViewerRole, houston.NoneRole)
	}

	if role == houston.NoneRole {
		// Get current role for the team
		team, err := houston.Call(client.GetTeam)(teamID)
		if err != nil {
			return err
		}

		for idx := range team.RoleBindings {
			if isValidSystemLevelRole(team.RoleBindings[idx].Role) {
				role = team.RoleBindings[idx].Role
				break
			}
		}

		if role == houston.NoneRole { // No system level role set for the team
			fmt.Fprintf(out, "Role for the team %s already set to None, nothing to update\n", teamID)
			return nil
		}

		_, err = houston.Call(client.DeleteTeamSystemRoleBinding)(houston.SystemRoleBindingRequest{TeamID: teamID, Role: role})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Role has been changed from %s to %s for team %s\n\n", role, houston.NoneRole, teamID)
		return nil
	}

	newRole, err := houston.Call(client.CreateTeamSystemRoleBinding)(houston.SystemRoleBindingRequest{TeamID: teamID, Role: role})
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed to %s for team %s\n\n", newRole, teamID)
	return nil
}

// isValidSystemLevelRole checks if the role is amongst valid system adming role
func isValidSystemLevelRole(role string) bool {
	switch role {
	case houston.SystemAdminRole, houston.SystemEditorRole, houston.SystemViewerRole, houston.NoneRole:
		return true
	}
	return false
}

// getSystemLevelRole returns the first system level role from a slice of roles
func getSystemLevelRole(roles []houston.RoleBinding) string {
	for i := range roles {
		if isValidSystemLevelRole(roles[i].Role) {
			return roles[i].Role
		}
	}
	return houston.NoneRole
}
