package deployment

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/printutil"
	log "github.com/sirupsen/logrus"
)

var errHoustonInvalidDeploymentTeams = errors.New(messages.HoustonInvalidDeploymentTeams)

// TeamsList returns a list of teams with deployment access
func ListTeamRoles(deploymentID string, client houston.ClientInterface, out io.Writer) error {
	deploymentTeams, err := client.ListDeploymentTeamsAndRoles(deploymentID)
	if err != nil {
		log.Error(err)
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
// AddTeam add's a team to a deployment with specified role
func AddTeam(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.AddDeploymentTeam(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT ID", "TEAM ID", "ROLE"},
	}
	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.Team.ID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", teamID, role)
	tab.Print(out)

	return nil
}

// nolint:dupl
// UpdateTeam updates a team's deployment role
func UpdateTeamRole(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.UpdateDeploymentTeamRole(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"},
	}

	tab.AddRow([]string{d.Deployment.ID, d.Team.ID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", teamID, role)
	tab.Print(out)

	return nil
}

// DeleteTeam removes team access for a deployment
func RemoveTeam(deploymentID, teamID string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.RemoveDeploymentTeam(deploymentID, teamID)
	if err != nil {
		log.Error(err)
		return err
	}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"},
	}

	tab.AddRow([]string{deploymentID, teamID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully removed the %s role for %s from deployment %s", d.Role, d.Team.Name, deploymentID)
	tab.Print(out)

	return nil
}
