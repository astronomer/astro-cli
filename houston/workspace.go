package houston

// CreateWorkspaceRequest - properties to create a workspace
type CreateWorkspaceRequest struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// UpdateWorkspaceRequest - properties to update in a workspace
type UpdateWorkspaceRequest struct {
	WorkspaceID string            `json:"workspaceId"`
	Args        map[string]string `json:"payload"`
}

// CreateWorkspace - create a workspace
func (h ClientImplementation) CreateWorkspace(request CreateWorkspaceRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceCreateRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.CreateWorkspace, nil
}

// ListWorkspaces - list workspaces
func (h ClientImplementation) ListWorkspaces(_ interface{}) ([]Workspace, error) {
	req := Request{
		Query: WorkspacesGetRequest,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.GetWorkspaces, nil
}

// PaginatedListWorkspaces - list workspaces
func (h ClientImplementation) PaginatedListWorkspaces(pageSize, pageNumber int) ([]Workspace, error) {
	req := Request{
		Query:     WorkspacesPaginatedGetRequest,
		Variables: map[string]interface{}{"pageSize": pageSize, "pageNumber": pageNumber},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.GetPaginatedWorkspaces, nil
}

// DeleteWorkspace - delete a workspace
func (h ClientImplementation) DeleteWorkspace(workspaceID string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceDeleteRequest,
		Variables: map[string]interface{}{"workspaceId": workspaceID},
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return res.Data.DeleteWorkspace, nil
}

// GetWorkspace - get a workspace
func (h ClientImplementation) GetWorkspace(workspaceID string) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID},
	}

	res, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	workspace := res.Data.GetWorkspace
	if workspace == nil {
		return nil, ErrWorkspaceNotFound{workspaceID: workspaceID}
	}

	return workspace, nil
}

// UpdateWorkspace - update a workspace
func (h ClientImplementation) UpdateWorkspace(request UpdateWorkspaceRequest) (*Workspace, error) {
	req := Request{
		Query:     WorkspaceUpdateRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateWorkspace, nil
}
