package teams

import (
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/internal/platform/apc/utils"
	"github.com/astronomer/astro-cli/pkg/logger"
)

const ListTeamLimit = 20

var (
	errMissingTeamID = errors.New("missing team ID")
	errTeamNotFound  = errors.New("no team was found with this ID")

	// monkey patched to write tests
	promptPaginatedOption = utils.PromptPaginatedOption
)

// Detail is a team as `astro team get` shows it: the team, with its role
// bindings, and its users when they were asked for (nil otherwise).
type Detail struct {
	Team  *houston.Team
	Users []houston.User
}

// Get returns a team, and its users when withUsers is set.
func Get(teamID string, withUsers bool, client houston.ClientInterface) (Detail, error) {
	if teamID == "" {
		return Detail{}, errMissingTeamID
	}
	team, err := houston.Call(client.GetTeam)(teamID)
	if err != nil {
		return Detail{}, err
	}
	if team == nil {
		return Detail{}, fmt.Errorf("%w: %s", errTeamNotFound, teamID)
	}
	d := Detail{Team: team}
	if withUsers {
		logger.Debug("retrieving users part of team")
		users, err := houston.Call(client.GetTeamUsers)(teamID)
		if err != nil {
			return Detail{}, err
		}
		d.Users = users
		if d.Users == nil {
			d.Users = []houston.User{}
		}
	}
	return d, nil
}

// List returns every team on the platform, reading them a page at a time.
func List(client houston.ClientInterface) ([]houston.Team, error) {
	teams := []houston.Team{}
	var cursor string
	count := -1

	for len(teams) < count || count == -1 {
		resp, err := houston.Call(client.ListTeams)(houston.ListTeamsRequest{Take: ListTeamLimit, Cursor: cursor})
		if err != nil {
			return nil, err
		}
		count = resp.Count
		// A page with nothing on it ends the list, whatever the count says:
		// asking again from the same cursor would get the same empty page.
		if len(resp.Teams) == 0 {
			break
		}
		teams = append(teams, resp.Teams...)
		cursor = teams[len(teams)-1].ID
	}
	return teams, nil
}

// PaginatedList shows the platform's teams a page at a time, asking which
// page to show next, until the person quits. render draws each page.
func PaginatedList(client houston.ClientInterface, pageSize, pageNumber int, cursorID string, render func([]houston.Team) error) error {
	resp, err := houston.Call(client.ListTeams)(houston.ListTeamsRequest{Cursor: cursorID, Take: pageSize})
	if err != nil {
		return err
	}
	if err := render(resp.Teams); err != nil {
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

	selectedOption, err := promptPaginatedOption(previousCursor, nextCursor, pageSize, totalTeams, pageNumber, lastPage)
	if err != nil {
		return err
	}
	if selectedOption.Quit {
		return nil
	}
	return PaginatedList(client, selectedOption.PageSize, selectedOption.PageNumber, selectedOption.CursorID, render)
}

// RoleChange is what a team update did to the team's system role: the role
// it held before (NONE when it held none, and "" when the update did not
// look), and the role it holds now. Changed is false when the update asked
// for NONE and the team already held no system role, so nothing was sent.
type RoleChange struct {
	TeamID   string
	Previous string
	Role     string
	Changed  bool
}

// Update sets the system role of a team. NONE removes the role it holds.
func Update(teamID, role string, client houston.ClientInterface) (RoleChange, error) {
	if !isValidSystemLevelRole(role) {
		return RoleChange{}, fmt.Errorf("invalid role: %s, should be one of: %s, %s, %s or %s", role, houston.SystemAdminRole, houston.SystemEditorRole, houston.SystemViewerRole, houston.NoneRole)
	}

	if role == houston.NoneRole {
		// Get current role for the team
		team, err := houston.Call(client.GetTeam)(teamID)
		if err != nil {
			return RoleChange{}, err
		}
		if team == nil {
			return RoleChange{}, fmt.Errorf("%w: %s", errTeamNotFound, teamID)
		}

		current := houston.NoneRole
		for idx := range team.RoleBindings {
			if isValidSystemLevelRole(team.RoleBindings[idx].Role) {
				current = team.RoleBindings[idx].Role
				break
			}
		}

		if current == houston.NoneRole { // No system level role set for the team
			return RoleChange{TeamID: teamID, Previous: houston.NoneRole, Role: houston.NoneRole}, nil
		}

		_, err = houston.Call(client.DeleteTeamSystemRoleBinding)(houston.SystemRoleBindingRequest{TeamID: teamID, Role: current})
		if err != nil {
			return RoleChange{}, err
		}
		return RoleChange{TeamID: teamID, Previous: current, Role: houston.NoneRole, Changed: true}, nil
	}

	newRole, err := houston.Call(client.CreateTeamSystemRoleBinding)(houston.SystemRoleBindingRequest{TeamID: teamID, Role: role})
	if err != nil {
		return RoleChange{}, err
	}
	return RoleChange{TeamID: teamID, Role: newRole, Changed: true}, nil
}

// SystemRole is the system role a team's role bindings give it, or NONE.
func SystemRole(roles []houston.RoleBinding) string {
	return getSystemLevelRole(roles)
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
