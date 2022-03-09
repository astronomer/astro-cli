package team

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
	"github.com/sirupsen/logrus"
)

// retrieves a team and all of its users if passed optional param
func Get(args []string, usersEnabled bool, client houston.ClientInterface, out io.Writer) error {
	teamID := args[0]
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
		teamUsers := printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"USERNAME", "ID"},
			ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
		}
		for i := range users {
			user := users[i]
			teamUsers.AddRow([]string{user.Username, user.ID}, false)
		}
		return teamUsers.Print(out)
	}

	return nil
}
