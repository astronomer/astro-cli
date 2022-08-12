package houston

// ListTeamsRequest - list team paginated request properties
type ListTeamsRequest struct {
	Cursor string `json:"cursor"`
	Take   int    `json:"take"`
}

// SystemRoleBindingRequest - properties to create or drop a team role binding
type SystemRoleBindingRequest struct {
	TeamID string `json:"teamUuid"`
	Role   string `json:"role"`
}

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
func (h ClientImplementation) ListTeams(request ListTeamsRequest) (ListTeamsResp, error) {
	req := Request{
		Query:     PaginatedTeamsRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return ListTeamsResp{}, handleAPIErr(err)
	}
	return r.Data.ListTeams, nil
}

// CreateTeamSystemRoleBinding - create system role binding for a team
func (h ClientImplementation) CreateTeamSystemRoleBinding(request SystemRoleBindingRequest) (string, error) {
	req := Request{
		Query:     CreateTeamSystemRoleBindingMutation,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.CreateTeamSystemRoleBinding.Role, nil
}

// DeleteTeamSystemRoleBinding - delete system role binding for a team
func (h ClientImplementation) DeleteTeamSystemRoleBinding(request SystemRoleBindingRequest) (string, error) {
	req := Request{
		Query:     DeleteTeamSystemRoleBindingMutation,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.DeleteTeamSystemRoleBinding.Role, nil
}
