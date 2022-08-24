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

var (
	TeamGetRequest = `
	query team($teamUuid: Uuid!, $workspaceUuid: Uuid) {
		team(teamUuid: $teamUuid, workspaceUuid: $workspaceUuid) {
			name
			id
			createdAt
			roleBindings {
				id
				role
				workspace {
					id
					label
				}
				deployment {
					id
					label
					releaseName
				}
			}
		}
	}
	`

	TeamGetUsersRequest = `
	query GetTeamUsers(
		$teamUuid: Uuid!
	){
		teamUsers(
			teamUuid: $teamUuid
		){
			username
			id
			emails {
				address
				verified
				primary
			}
			status
		}
	}`

	PaginatedTeamsRequest = `
	query paginatedTeams (
		$take: Int
		$cursor: Uuid
		$workspaceUuid: Uuid
	){
		paginatedTeams(take:$take, cursor:$cursor, workspaceUuid:$workspaceUuid) {
			count
			teams {
				id
				name
			}
		}
	}`

	CreateTeamSystemRoleBindingMutation = `
	mutation createTeamSystemRoleBinding (
		$teamUuid: Uuid!
		$role: Role!
	){
		createTeamSystemRoleBinding(teamUuid:$teamUuid, role:$role) {
			role
		}
	}`

	DeleteTeamSystemRoleBindingMutation = `
	mutation deleteTeamSystemRoleBinding (
		$teamUuid: Uuid!
		$role: Role!
	){
		deleteTeamSystemRoleBinding(teamUuid:$teamUuid, role:$role) {
			role
		}
	}`
)

// GetTeam - return a specific team
func (h ClientImplementation) GetTeam(teamID string) (*Team, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return []User{}, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return ListTeamsResp{}, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return "", err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return "", err
	}

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
