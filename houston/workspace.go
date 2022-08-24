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

// UpdateWorkspaceRequest - inputs to list in workspaces
type PaginatedListWorkspaceRequest struct {
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"`
}

var (
	WorkspaceCreateRequest = `
	mutation CreateWorkspace(
		$label: String!,
		$description: String = "N/A"
	) {
		createWorkspace(
			label: $label,
			description: $description
		) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceDeleteRequest = `
	mutation DeleteWorkspace($workspaceId: Uuid!) {
		deleteWorkspace(workspaceUuid: $workspaceId) {
			id
			label
			description
		}
	}`

	WorkspaceUpdateRequest = `
	mutation UpdateWorkspace(
		$workspaceId: Uuid!,
		$payload: JSON!
	) {
		updateWorkspace(
			workspaceUuid: $workspaceId,
			payload: $payload
		) {
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspacesGetRequest = `
	query GetWorkspaces {
		workspaces {
			id
			label
			description
			createdAt
			updatedAt
			roleBindings {
				role
				user {
					id
					username
				}
				serviceAccount {
					id
					label
				}
			}
		}
	}`

	WorkspacesPaginatedGetRequest = `
	query paginatedWorkspaces(
		$pageSize: Int
		$pageNumber: Int
	){
		paginatedWorkspaces(
			take: $pageSize
			pageNumber: $pageNumber
		){
			id
			label
			description
			createdAt
			updatedAt
		}
	}`

	WorkspaceGetRequest = `
	query GetWorkspace(
		$workspaceUuid: Uuid!
	){
		workspace(
			workspaceUuid: $workspaceUuid
		){
			id
			label
			description
			createdAt
			updatedAt
			roleBindings {
				role
				user {
					id
					username
				}
				serviceAccount {
					id
					label
				}
			}
		}
	}`
)

// CreateWorkspace - create a workspace
func (h ClientImplementation) CreateWorkspace(request CreateWorkspaceRequest) (*Workspace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
func (h ClientImplementation) ListWorkspaces() ([]Workspace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return []Workspace{}, err
	}

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
func (h ClientImplementation) PaginatedListWorkspaces(request PaginatedListWorkspaceRequest) ([]Workspace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return []Workspace{}, err
	}

	req := Request{
		Query:     WorkspacesPaginatedGetRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.GetPaginatedWorkspaces, nil
}

// DeleteWorkspace - delete a workspace
func (h ClientImplementation) DeleteWorkspace(workspaceID string) (*Workspace, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

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
