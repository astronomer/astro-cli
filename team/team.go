package team

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/sirupsen/logrus"
)

var errGetMissingTeamID = errors.New("missing team Id")

// retrieves a team and all of its users if passed optional param
func Get(teamID string, usersEnabled bool, client houston.ClientInterface, out io.Writer) error {
	if teamID == "" {
		return errGetMissingTeamID
	}
	team, err := client.GetTeam(teamID)
	if err != nil {
		return err
	}

	fmt.Printf("\n Team Name: %s and team id: %s \n", team.Name, team.ID)

	if usersEnabled {
		logrus.Debug("retrieving users part of team")
		fmt.Println("Users part of Team:")
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
