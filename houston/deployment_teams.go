package houston

// AddDeploymentTeamRequest - properties to add a team to a deployment
type AddDeploymentTeamRequest struct {
	TeamID       string `json:"teamUuid"`
	DeploymentID string `json:"deploymentUuid"`
	Role         string `json:"role"`
}

// UpdateDeploymentTeamRequest - properties to update a team's role in a deployment
type UpdateDeploymentTeamRequest struct {
	TeamID       string `json:"teamUuid"`
	DeploymentID string `json:"deploymentUuid"`
	Role         string `json:"role"`
}

// RemoveDeploymentTeamRequest - properties to remove a team from a deployment
type RemoveDeploymentTeamRequest struct {
	TeamID       string `json:"teamUuid"`
	DeploymentID string `json:"deploymentUuid"`
}

var (
	DeploymentGetTeamsRequest = `
	query deploymentTeams($deploymentUuid: Uuid!) {
		deploymentTeams(deploymentUuid: $deploymentUuid) {
			id
			name
			roleBindings {
				role
				workspace {
					id
					label
				}
				deployment {
					id
					label
				}
			}
		}
	}`

	DeploymentTeamAddRequest = `
	mutation deploymentAddTeamRole(
		$teamUuid: Uuid!
		$deploymentUuid: Uuid!
		$role: Role! = WORKSPACE_VIEWER
	){
		deploymentAddTeamRole(
			teamUuid: $teamUuid
			deploymentUuid: $deploymentUuid
			role: $role
		){
			id
			role
		}
	}`

	DeploymentTeamRemoveRequest = `
	mutation deploymentRemoveTeamRole($deploymentUuid: Uuid!, $teamUuid: Uuid!) {
		deploymentRemoveTeamRole(
			deploymentUuid: $deploymentUuid
			teamUuid: $teamUuid
		){
			id
		}
	}`

	DeploymentTeamUpdateRequest = `
	mutation deploymentUpdateTeamRole(
		$deploymentUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role!
	){
		deploymentUpdateTeamRole(
			deploymentUuid: $deploymentUuid
			teamUuid: $teamUuid
			role: $role
		){
			id
			role
		}
	}`
)

// ListTeamsInDeployment - list teams with deployment access
func (h ClientImplementation) ListDeploymentTeamsAndRoles(deploymentID string) ([]Team, error) {
	req := Request{
		Query: DeploymentGetTeamsRequest,
		Variables: map[string]interface{}{
			"deploymentUuid": deploymentID,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.DeploymentGetTeams, nil
}

// AddTeamToDeployment - Add a team to a deployment with specified role
func (h ClientImplementation) AddDeploymentTeam(request AddDeploymentTeamRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentTeamAddRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddDeploymentTeam, nil
}

// UpdateTeamInDeployment - update a team's role inside a deployment
func (h ClientImplementation) UpdateDeploymentTeamRole(request UpdateDeploymentTeamRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentTeamUpdateRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateDeploymentTeam, nil
}

// RemoveDeploymentTeam - remove a team from a deployment
func (h ClientImplementation) RemoveDeploymentTeam(request RemoveDeploymentTeamRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentTeamRemoveRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.RemoveDeploymentTeam, nil
}
