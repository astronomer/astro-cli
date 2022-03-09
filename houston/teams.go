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
