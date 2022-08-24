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

var (
	WorkspaceTeamAddRequest = `
	mutation AddWorkspaceTeam(
		$workspaceUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role! = WORKSPACE_VIEWER
		$deploymentRoles: [DeploymentRoles!] = []
	){
		workspaceAddTeam(
			workspaceUuid: $workspaceUuid
			teamUuid: $teamUuid
			role: $role
			deploymentRoles: $deploymentRoles) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceTeamUpdateRequest = `
	mutation workspaceUpdateTeamRole(
		$workspaceUuid: Uuid!
		$teamUuid: Uuid!
		$role: Role!
	){
		workspaceUpdateTeamRole(
      		workspaceUuid: $workspaceUuid
      		teamUuid: $teamUuid
      		role: $role
    	)
	}`

	WorkspaceTeamRemoveRequest = `
	mutation workspaceRemoveTeam(
		$workspaceUuid: Uuid!,
		$teamUuid: Uuid!
	){
		workspaceRemoveTeam(workspaceUuid: $workspaceUuid, teamUuid: $teamUuid) {
			id
			label
		}
	}
	`

	WorkspaceGetTeamsRequest = `
	query workspaceGetTeams($workspaceUuid: Uuid!) {
		workspaceTeams(
			workspaceUuid: $workspaceUuid
		){
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
)

// AddTeamToWorkspace - add a team to a workspace
func (h ClientImplementation) AddWorkspaceTeam(request AddWorkspaceTeamRequest) (*Workspace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return []Team{}, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return "", err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
