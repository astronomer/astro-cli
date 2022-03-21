package deployment

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// TeamsList returns a list of teams with deployment access
func TeamsList(deploymentID string, teamID string, client houston.ClientInterface, out io.Writer) error {
	deploymentTeams, err := client.ListDeploymentTeamsAndRoles(deploymentID)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if len(deploymentTeams) < 1 {
		_, err = out.Write([]byte(messages.HoustonInvalidDeploymentTeams))
		return err
	}

	header = []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"}
	tab = printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}

	// Build rows
	for _, d := range deploymentTeams {
		role := filterByRoleType(d.RoleBindings, houston.DeploymentRole)
		tab.AddRow([]string{deploymentID, d.ID, role}, false)
	}

	tab.Print(out)

	return nil
}

// nolint:dupl
// Add a user to a deployment with specified role
func AddTeam(deploymentID, teamID string, role string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.AddDeploymentTeam(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.Team.ID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", teamID, role)
	tab.Print(out)

	return nil
}

// nolint:dupl
// UpdateUser updates a user's deployment role
func UpdateTeam(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.UpdateDeploymentTeamRole(deploymentID, teamID, role)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.Team.ID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", teamID, role)
	tab.Print(out)

	return nil
}

// DeleteUser removes user access for a deployment
func RemoveTeam(deploymentID, teamID string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.DeleteDeploymentTeam(deploymentID, teamID)
	if err != nil {
		fmt.Println(err)
		return err
	}
	header := []string{"DEPLOYMENT ID", "TEAM ID", "ROLE"}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}

	tab.AddRow([]string{deploymentID, teamID, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully removed the %s role for %s from deployment %s", d.Role, d.Team.Name, deploymentID)
	tab.Print(out)

	return nil
}
