package houston

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
func (h ClientImplementation) AddDeploymentTeam(deploymentID, teamID, role string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentTeamAddRequest,
		Variables: map[string]interface{}{
			"teamUuid":       teamID,
			"deploymentUuid": deploymentID,
			"role":           role,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddDeploymentTeam, nil
}

// UpdateTeamInDeployment - update a team's role inside a deployment
func (h ClientImplementation) UpdateDeploymentTeamRole(deploymentID, teamID, role string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentTeamUpdateRequest,
		Variables: map[string]interface{}{
			"teamUuid":       teamID,
			"deploymentUuid": deploymentID,
			"role":           role,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateDeploymentTeam, nil
}

// DeleteTeamFromDeployment - remove a team from a deployment
func (h ClientImplementation) RemoveDeploymentTeam(deploymentID, teamID string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentTeamRemoveRequest,
		Variables: map[string]interface{}{
			"teamUuid":       teamID,
			"deploymentUuid": deploymentID,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.RemoveDeploymentTeam, nil
}
