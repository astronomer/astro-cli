package houston

// ListDeploymentUsersRequest - properties to filter users in a deployment
type ListDeploymentTeamsRequest struct {
	DeploymentID string `json:"deploymentId"`
}

// UpdateDeploymentUserRequest - properties to create a user in a deployment
type UpdateDeploymentTeamRequest struct {
	TeamID       string `json:"teamUuid"`
	DeploymentID string `json:"deploymentId"`
}

// ListUsersInDeployment - list users with deployment access
func (h ClientImplementation) ListDeploymentTeams(variables ListDeploymentTeamsRequest) ([]DeploymentTeam, error) {
	req := Request{
		Query:     DeploymentGetTeamsRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []DeploymentTeam{}, handleAPIErr(err)
	}

	return r.Data.DeploymentTeamsList, nil
}

// AddUserToDeployment - Add a user to a deployment with specified role
func (h ClientImplementation) AddDeploymentTeam(deploymentID, teamID string, role string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentTeamAddRequest,
		Variables: map[string]interface{}{
			"teamId":       teamID,
			"deploymentId": deploymentID,
			"role":         role,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddDeploymentTeam, nil
}

// UpdateUserInDeployment - update a user's role inside a deployment
func (h ClientImplementation) UpdateDeploymentTeam(variables UpdateDeploymentTeamRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentTeamUpdateRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateDeploymentTeam, nil
}

// DeleteUserFromDeployment - remove a user from a deployment
func (h ClientImplementation) DeleteDeploymentTeam(deploymentID, teamID string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentTeamRemoveRequest,
		Variables: map[string]interface{}{
			"teamId":       teamID,
			"deploymentId": deploymentID,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.DeleteDeploymentTeam, nil
}
