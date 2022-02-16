package houston

// AddUserToWorkspace - add a user to a workspace
func (h ClientImplementation) AddWorkspaceUser(workspaceID, email, role string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceUserAddRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID, "email": email, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddWorkspaceUser, nil
}

// RemoveUserFromWorkspace - remove a user from a workspace
func (h ClientImplementation) DeleteWorkspaceUser(workspaceID, userID string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceUserRemoveRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID, "userId": userID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.RemoveWorkspaceUser, nil
}

// ListUserAndRolesFromWorkspace - list users and roles from a workspace
func (h ClientImplementation) ListWorkspaceUserAndRoles(workspaceID string) (*Workspace, error) {
	req := Request{
		Query:     WorkspacesGetRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return &r.Data.GetWorkspaces[0], nil
}

// UpdateUserRoleInWorkspace - update a user role in a workspace
func (h ClientImplementation) UpdateWorkspaceUserRole(workspaceID, email, role string) (string, error) {
	req := Request{
		Query:     WorkspaceUserUpdateRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "email": email, "role": role},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return "", handleAPIErr(err)
	}

	return r.Data.WorkspaceUpdateUserRole, nil
}

// GetUserRoleInWorkspace - get a user role in a workspace
func (h ClientImplementation) GetWorkspaceUserRole(workspaceID, email string) (WorkspaceUserRoleBindings, error) {
	req := Request{
		Query:     WorkspaceGetUserRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID, "email": email},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return WorkspaceUserRoleBindings{}, handleAPIErr(err)
	}

	return r.Data.WorkspaceGetUser, nil
}
