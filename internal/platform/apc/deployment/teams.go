package deployment

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// ErrNoDeploymentTeams is ListTeamRoles' answer when Houston lists no team
// for the Deployment at all.
var ErrNoDeploymentTeams = errors.New("no teams were found for this deployment. Check the deploymentId and try again")

// TeamRole is a team and the role it holds on a Deployment.
type TeamRole struct {
	ID   string
	Name string
	Role string
}

// ListTeamRoles returns the teams with a role on the Deployment, and the role
// each holds there. It is ErrNoDeploymentTeams when Houston lists none.
func ListTeamRoles(deploymentID string, client houston.ClientInterface) ([]TeamRole, error) {
	deploymentTeams, err := houston.Call(client.ListDeploymentTeamsAndRoles)(deploymentID)
	if err != nil {
		return nil, err
	}
	if len(deploymentTeams) < 1 {
		return nil, ErrNoDeploymentTeams
	}
	teams := make([]TeamRole, 0, len(deploymentTeams))
	for i := range deploymentTeams {
		role := getDeploymentLevelRole(deploymentTeams[i].RoleBindings, deploymentID)
		if role != houston.NoneRole {
			teams = append(teams, TeamRole{ID: deploymentTeams[i].ID, Name: deploymentTeams[i].Name, Role: role})
		}
	}
	return teams, nil
}

// AddTeam gives a team role on a Deployment, and returns the role Houston
// recorded.
func AddTeam(deploymentID, teamID, role string, client houston.ClientInterface) (string, error) {
	rb, err := houston.Call(client.AddDeploymentTeam)(houston.AddDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID, Role: role})
	if err != nil {
		return "", err
	}
	return boundRole(rb, role), nil
}

// UpdateTeamRole changes a team's role on a Deployment, and returns the role
// Houston recorded.
func UpdateTeamRole(deploymentID, teamID, role string, client houston.ClientInterface) (string, error) {
	rb, err := houston.Call(client.UpdateDeploymentTeamRole)(houston.UpdateDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID, Role: role})
	if err != nil {
		return "", err
	}
	return boundRole(rb, role), nil
}

// RemoveTeam removes a team's role on a Deployment.
func RemoveTeam(deploymentID, teamID string, client houston.ClientInterface) error {
	_, err := houston.Call(client.RemoveDeploymentTeam)(houston.RemoveDeploymentTeamRequest{DeploymentID: deploymentID, TeamID: teamID})
	return err
}

// boundRole is the role a role binding Houston returned holds, or asked when
// it returned none.
func boundRole(rb *houston.RoleBinding, asked string) string {
	if rb == nil || rb.Role == "" {
		return asked
	}
	return rb.Role
}

// IsValidDeploymentLevelRole checks if the role is amongst valid deployment roles
func IsValidDeploymentLevelRole(role string) bool {
	switch role {
	case houston.DeploymentAdminRole, houston.DeploymentEditorRole, houston.DeploymentViewerRole, houston.NoneRole:
		return true
	}
	return false
}

// getDeploymentLevelRole returns the first deployment level role from a slice of roles
func getDeploymentLevelRole(roles []houston.RoleBinding, deploymentID string) string {
	for i := range roles {
		if IsValidDeploymentLevelRole(roles[i].Role) && roles[i].Deployment.ID == deploymentID {
			return roles[i].Role
		}
	}
	return houston.NoneRole
}
