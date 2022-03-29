package deployment

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errHoustonInvalidDeploymentTeams = errors.New(messages.HoustonInvalidDeploymentTeams)

// TeamsList returns a list of teams with deployment access
func ListTeamRoles(deploymentID string, client houston.ClientInterface, out io.Writer) error {
	deploymentTeams, err := client.ListDeploymentTeamsAndRoles(deploymentID)
	if err != nil {
		return err
	}

	if len(deploymentTeams) < 1 {
		return errHoustonInvalidDeploymentTeams
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID", "TEAM NAME", "ROLE"},
	}

	// Build rows
	for i := range deploymentTeams {
		for j := range deploymentTeams[i].RoleBindings {
			tab.AddRow([]string{deploymentID, deploymentTeams[i].ID, deploymentTeams[i].Name, deploymentTeams[i].RoleBindings[j].Role}, false)
		}
	}

	tab.Print(out)

	return nil
}

// nolint:dupl
// AddTeam adds a team to a deployment with specified role
func AddTeam(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	_, err := client.AddDeploymentTeam(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"},
	}
	tab.AddRow([]string{deploymentID, teamID, role}, false)
	tab.SuccessMsg = fmt.Sprintf("\nSuccessfully added team %s to deployment %s as a %s", teamID, deploymentID, role)
	tab.Print(out)

	return nil
}

// nolint:dupl
// UpdateTeam updates a team's deployment role
func UpdateTeamRole(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	_, err := client.UpdateDeploymentTeamRole(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"},
	}

	tab.AddRow([]string{deploymentID, teamID, role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated team %s to a %s", teamID, role)
	tab.Print(out)

	return nil
}

// RemoveTeam removes team access for a deployment
func RemoveTeam(deploymentID, teamID string, client houston.ClientInterface, out io.Writer) error {
	_, err := client.RemoveDeploymentTeam(deploymentID, teamID)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID"},
	}

	tab.AddRow([]string{deploymentID, teamID}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully removed team %s from deployment %s", teamID, deploymentID)
	tab.Print(out)

	return nil
}
