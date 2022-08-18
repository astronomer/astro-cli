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
	WorkspaceID string `json:"workspaceId"`
	Email       string `json:"email"`
	Role        string `json:"role"`
}

// GetWorkspaceUserRoleRequest - filter to get a workspace user
type GetWorkspaceUserRoleRequest struct {
	WorkspaceID string `json:"workspaceUuid"`
	Email       string `json:"email"`
}

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
func (h ClientImplementation) ListWorkspacePaginatedUserAndRoles(workspaceID, cursorID string, take float64) ([]WorkspaceUserRoleBindings, error) {
	req := Request{
		Query:     WorkspacePaginatedGetUsersRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "cursorUuid": cursorID, "take": take},
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
