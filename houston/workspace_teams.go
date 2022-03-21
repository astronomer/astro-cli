package houston

// AddTeamToWorkspace - add a team to a workspace
func (h ClientImplementation) AddWorkspaceTeam(workspaceID, teamID, role string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceTeamAddRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "teamUuid": teamID, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddWorkspaceTeam, nil
}

// RemoveTeamFromWorkspace - remove a team from a workspace
func (h ClientImplementation) DeleteWorkspaceTeam(workspaceID, teamID string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceTeamRemoveRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "teamUuid": teamID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.RemoveWorkspaceTeam, nil
}

// ListTeamAndRolesFromWorkspace - list teams and roles from a workspace
func (h ClientImplementation) ListWorkspaceTeamsAndRoles(workspaceID string) ([]Team, error) {
	req := Request{
		Query:     WorkspaceGetTeamsRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.WorkspaceGetTeams, nil
}

// UpdateTeamRoleInWorkspace - update a team role in a workspace
func (h ClientImplementation) UpdateWorkspaceTeamRole(workspaceID, teamID, role string) (string, error) {
	req := Request{
		Query:     WorkspaceTeamUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "teamUuid": teamID, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.WorkspaceUpdateTeamRole, nil
}

// GetTeamRoleInWorkspace - get a team role in a workspace
func (h ClientImplementation) GetWorkspaceTeamRole(workspaceID, teamID string) (WorkspaceTeamRoleBindings, error) {
	req := Request{
		Query:     WorkspaceGetTeamRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "teamUuid": teamID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return WorkspaceTeamRoleBindings{}, handleAPIErr(err)
	}

	return r.Data.WorkspaceGetTeam, nil
}
