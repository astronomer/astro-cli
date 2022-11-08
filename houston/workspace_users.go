package houston

// AddWorkspaceUserRequest - properties to add a workspace user
type AddWorkspaceUserRequest struct {
	WorkspaceID string `json:"workspaceId"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

// DeleteWorkspaceUserRequest - properties to remove a workspace user
type DeleteWorkspaceUserRequest struct {
	WorkspaceID string `json:"workspaceId"`
	UserID      string `json:"userId"`
}

// UpdateWorkspaceUserRoleRequest - properties to update a user role in a workspace
type UpdateWorkspaceUserRoleRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

// GetWorkspaceUserRoleRequest - filter to get a workspace user
type GetWorkspaceUserRoleRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	Email       string `json:"email"`
}

// GetWorkspaceUserRoleRequest - input to list a workspace user & roles
type PaginatedWorkspaceUserRolesRequest struct {
	WorkspaceID string  `json:"workspaceUuid"`
	CursorID    string  `json:"cursorUuid"`
	Take        float64 `json:"take"`
}

var (
	WorkspaceUserAddRequest = `
	mutation AddWorkspaceUser(
		$workspaceId: Uuid!,
		$email: String!,
		$role: Role!
	){
		workspaceAddUser(
			workspaceUuid: $workspaceId,
			email: $email,
			role: $role
		) {
			id
			label
			description
			users {
				id
				username
				roleBindings {
          			role
        		}
			}
			createdAt
			updatedAt
		}
	}`

	WorkspaceUserRemoveRequest = `
	mutation RemoveWorkspaceUser(
		$workspaceId: Uuid!
		$userId: Uuid!
	){
		workspaceRemoveUser(
			workspaceUuid: $workspaceId
			userUuid: $userId
		){
			id
			label
			description
			users {
				id
				username
			}
			createdAt
			updatedAt
		}
	}`

	WorkspaceGetUsersRequest = `
	query workspaceListUsers($workspaceUuid: Uuid!) {
		workspaceUsers(
        	workspaceUuid: $workspaceUuid
        ){
			id
			username
			fullName
			emails {
				address
			}
			roleBindings {
		  		workspace{
					id
		  		}
		  		role
			}
		}
	}`

	WorkspacePaginatedGetUsersRequest = `
	query paginatedWorkspaceUsers(
		$workspaceUuid: Uuid!,
		$cursorUuid: Uuid,
		$take: Int
	){
		paginatedWorkspaceUsers(
			workspaceUuid: $workspaceUuid
			cursor: $cursorUuid
			take: $take
		){
			id
			username
			fullName
			emails {
				address
			}
			roleBindings {
				workspace{
					id
				}
				role
			}
		}
	}`

	WorkspaceUserUpdateRequest = `
	mutation workspaceUpsertUserRole(
		$workspaceUuid: Uuid!,
		$email: String!,
		$role: Role!
	) {
		workspaceUpsertUserRole(
        	workspaceUuid: $workspaceUuid
            email: $email
            role: $role
        )
	}`

	WorkspaceGetUserRequest = `
	query workspaceGetUser(
		$workspaceUuid: Uuid!,
		$email: String!
	) {
		workspaceUser(
        	workspaceUuid: $workspaceUuid
            user: { email: $email }
        ){
			id
			username
			roleBindings {
		  		workspace{
					id
		  		}
		  		role
			}
		}
	}`
)

// AddUserToWorkspace - add a user to a workspace
func (h ClientImplementation) AddWorkspaceUser(request AddWorkspaceUserRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceUserAddRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddWorkspaceUser, nil
}

// RemoveUserFromWorkspace - remove a user from a workspace
func (h ClientImplementation) DeleteWorkspaceUser(request DeleteWorkspaceUserRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceUserRemoveRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.RemoveWorkspaceUser, nil
}

// ListUserAndRolesFromWorkspace - list users and roles from a workspace
func (h ClientImplementation) ListWorkspaceUserAndRoles(workspaceID string) ([]WorkspaceUserRoleBindings, error) {
	req := Request{
		Query:     WorkspaceGetUsersRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []WorkspaceUserRoleBindings{}, handleAPIErr(err)
	}

	return r.Data.WorkspaceGetUsers, nil
}

// ListWorkspacePaginatedUserAndRoles - list users and roles from a workspace
func (h ClientImplementation) ListWorkspacePaginatedUserAndRoles(request PaginatedWorkspaceUserRolesRequest) ([]WorkspaceUserRoleBindings, error) {
	req := Request{
		Query:     WorkspacePaginatedGetUsersRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []WorkspaceUserRoleBindings{}, handleAPIErr(err)
	}

	return r.Data.WorkspacePaginatedGetUsers, nil
}

// UpdateUserRoleInWorkspace - update a user role in a workspace
func (h ClientImplementation) UpdateWorkspaceUserRole(request UpdateWorkspaceUserRoleRequest) (string, error) {
	req := Request{
		Query:     WorkspaceUserUpdateRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.WorkspaceUpsertUserRole, nil
}

// GetUserRoleInWorkspace - get a user role in a workspace
func (h ClientImplementation) GetWorkspaceUserRole(request GetWorkspaceUserRoleRequest) (WorkspaceUserRoleBindings, error) {
	req := Request{
		Query:     WorkspaceGetUserRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return WorkspaceUserRoleBindings{}, handleAPIErr(err)
	}

	return r.Data.WorkspaceGetUser, nil
}
