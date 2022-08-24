package deployment

import (
	"errors"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var errHoustonInvalidDeploymentTeams = errors.New("no teams were found for this deployment. Check the deploymentId and try again")

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
		role := getDeploymentLevelRole(deploymentTeams[i].RoleBindings, deploymentID)
		if role != houston.NoneTeamRole {
			tab.AddRow([]string{deploymentID, deploymentTeams[i].ID, deploymentTeams[i].Name, role}, false)
		}
	}

	tab.Print(out)

	return nil
}

// nolint:dupl
// AddTeam adds a team to a deployment with specified role
func AddTeam(deploymentID, teamID, role string, client houston.ClientInterface, out io.Writer) error {
	_, err := client.AddDeploymentTeam(houston.AddDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID, Role: role})
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
	_, err := client.UpdateDeploymentTeamRole(houston.UpdateDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID, Role: role})
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
	_, err := client.RemoveDeploymentTeam(houston.RemoveDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID})
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

// isValidDeploymentLevelRole checks if the role is amongst valid workspace roles
func isValidDeploymentLevelRole(role string) bool {
	switch role {
	case houston.DeploymentAdminRole, houston.DeploymentEditorRole, houston.DeploymentViewerRole, houston.NoneTeamRole:
		return true
	}
	return false
}

// getDeploymentLevelRole returns the first system level role from a slice of roles
func getDeploymentLevelRole(roles []houston.RoleBinding, deploymentID string) string {
	for i := range roles {
		if isValidDeploymentLevelRole(roles[i].Role) && roles[i].Deployment.ID == deploymentID {
			return roles[i].Role
		}
	}
	return houston.NoneTeamRole
}
