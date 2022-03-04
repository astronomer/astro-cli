package team

import (
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// retrieves a team and all of its users if passed optional param
func Get(teamID string, usersEnabled bool, client houston.ClientInterface, out io.Writer) error {

	team, err := client.GetTeam(teamID)
	if err != nil {
		return err
	}
	tableTeam := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "ID"},
		ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
	}

	tableTeam.AddRow([]string{team.Name, team.ID}, false)

	tableTeam.Print(out)

	if usersEnabled {
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

		teamUsers.AddRow([]string{team.Name, team.ID}, false)
		teamUsers.Print(out)
	}

	return nil
}
