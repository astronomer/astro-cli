package houston

// AddWorkspaceTeamRequest - properties to add a team to workspace
type AddWorkspaceTeamRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	TeamID      string `json:"teamUuid"`
	Role        string `json:"role"`
}

// DeleteWorkspaceTeamRequest - properties required to remove a team from a workspace
type DeleteWorkspaceTeamRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	TeamID      string `json:"teamUuid"`
}

// UpdateWorkspaceTeamRoleRequest - properties to update a team role in a workspace
type UpdateWorkspaceTeamRoleRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	TeamID      string `json:"teamUuid"`
	Role        string `json:"role"`
}

// GetWorkspaceTeamRoleRequest - properties to fetch a team role in a workspace
type GetWorkspaceTeamRoleRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	TeamID      string `json:"teamUuid"`
}

// AddTeamToWorkspace - add a team to a workspace
func (h ClientImplementation) AddWorkspaceTeam(request AddWorkspaceTeamRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceTeamAddRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddWorkspaceTeam, nil
}

// RemoveTeamFromWorkspace - remove a team from a workspace
func (h ClientImplementation) DeleteWorkspaceTeam(request DeleteWorkspaceTeamRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceTeamRemoveRequest,
		Variables: request,
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
func (h ClientImplementation) UpdateWorkspaceTeamRole(request UpdateWorkspaceTeamRoleRequest) (string, error) {
	req := Request{
		Query:     WorkspaceTeamUpdateRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.WorkspaceUpdateTeamRole, nil
}

// GetTeamRoleInWorkspace - get a team role in a workspace
func (h ClientImplementation) GetWorkspaceTeamRole(request GetWorkspaceTeamRoleRequest) (*Team, error) {
	req := Request{
		Query:     TeamGetRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.GetTeam, nil
}
