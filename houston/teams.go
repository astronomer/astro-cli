package houston

// GetTeam - return a specific team
func (h ClientImplementation) GetTeam(teamID string) (*Team, error) {
	req := Request{
		Query:     TeamGetRequest,
		Variables: map[string]interface{}{"teamUuid": teamID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	return r.Data.GetTeam, nil
}

// GetTeamUsers - return a specific teams Users
func (h ClientImplementation) GetTeamUsers(teamID string) ([]User, error) {
	req := Request{
		Query:     TeamGetUsersRequest,
		Variables: map[string]interface{}{"teamUuid": teamID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}
	return r.Data.GetTeamUsers, nil
}

// ListTeams - return list of available teams
func (h ClientImplementation) ListTeams(cursor string, take int) (ListTeamsResp, error) {
	req := Request{
		Query:     ListTeamsRequest,
		Variables: map[string]interface{}{"take": take, "cursor": cursor},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return ListTeamsResp{}, handleAPIErr(err)
	}
	return r.Data.ListTeams, nil
}

// CreateTeamSystemRoleBinding - create system role binding for a team
func (h ClientImplementation) CreateTeamSystemRoleBinding(teamID, role string) (string, error) {
	req := Request{
		Query:     CreateTeamSystemRoleBindingRequest,
		Variables: map[string]interface{}{"teamUuid": teamID, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.CreateTeamSystemRoleBinding.Role, nil
}

// DeleteTeamSystemRoleBinding - delete system role binding for a team
func (h ClientImplementation) DeleteTeamSystemRoleBinding(teamID, role string) (string, error) {
	req := Request{
		Query:     DeleteTeamSystemRoleBindingRequest,
		Variables: map[string]interface{}{"teamUuid": teamID, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.DeleteTeamSystemRoleBinding.Role, nil
}
