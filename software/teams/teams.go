package teams

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/sirupsen/logrus"
)

const (
	ListTeamLimit = 25

	noRoleSpecifiedMsg = "No role specified, nothing to update"
)

var errMissingTeamID = errors.New("missing team ID")

// retrieves a team and all of its users if passed optional param
func Get(teamID string, usersEnabled bool, client houston.ClientInterface, out io.Writer) error {
	if teamID == "" {
		return errMissingTeamID
	}
	team, err := client.GetTeam(teamID)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\nTeam Name: %s\nTeam ID: %s \n\n", team.Name, team.ID)

	if usersEnabled {
		logrus.Debug("retrieving users part of team")
		fmt.Fprintln(out, "Users part of Team:")
		users, err := client.GetTeamUsers(teamID)
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

// retrieves all teams
func List(client houston.ClientInterface, out io.Writer) error {
	var teams []houston.Team
	var cursor string
	count := -1

	for len(teams) < count || count == -1 {
		resp, err := client.ListTeams(cursor, ListTeamLimit)
		if err != nil {
			return err
		}
		count = resp.Count
		teams = append(teams, resp.Teams...)
		cursor = teams[len(teams)-1].ID
	}

	teamsTable := printutil.Table{
		Padding:        []int{50, 50},
		DynamicPadding: true,
		Header:         []string{"TEAM ID", "TEAM NAME"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}
	for i := range teams {
		teamsTable.AddRow([]string{teams[i].ID, teams[i].Name}, false)
	}
	return teamsTable.Print(out)
}

func Update(teamID, role string, client houston.ClientInterface, out io.Writer) error {
	if role == "" {
		fmt.Fprintln(out, noRoleSpecifiedMsg)
		return nil
	} else if role != houston.SystemAdminRole && role != houston.SystemEditorRole && role != houston.SystemViewerRole && role != houston.NoneTeamRole {
		return fmt.Errorf("invalid role: %s, should be one of: %s, %s, %s or %s", role, houston.SystemAdminRole, houston.SystemEditorRole, houston.SystemViewerRole, houston.NoneTeamRole) //nolint:goerr113
	}

	if role == houston.NoneTeamRole {
		// Get current role for the team
		team, err := client.GetTeam(teamID)
		if err != nil {
			return err
		}

		for idx := range team.RoleBindings {
			if team.RoleBindings[idx].Role == houston.SystemAdminRole || team.RoleBindings[idx].Role == houston.SystemEditorRole || team.RoleBindings[idx].Role == houston.SystemViewerRole {
				role = team.RoleBindings[idx].Role
				break
			}
		}

		if role == houston.NoneTeamRole { // No system level role set for the team
			fmt.Fprintf(out, "Role for the team %s already set to None, nothing to update\n", teamID)
			return nil
		}

		_, err = client.DeleteTeamSystemRoleBinding(teamID, role)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Role has been changed from %s to %s for team %s\n\n", role, houston.NoneTeamRole, teamID)
		return nil
	}

	newRole, err := client.CreateTeamSystemRoleBinding(teamID, role)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Role has been changed to %s for team %s\n\n", newRole, teamID)
	return nil
}
