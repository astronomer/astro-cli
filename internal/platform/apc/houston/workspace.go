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

// PaginatedListWorkspaceRequest is a page of the workspaces to list.
// PageNumber goes to Houston as it is, in Houston's numbering: build it with
// WorkspacesPageNumber.
type PaginatedListWorkspaceRequest struct {
	PageSize   int `json:"pageSize"`
	PageNumber int `json:"pageNumber"`
}

// oneBasedPagesSince is the first Houston whose paginatedWorkspaces counts
// pages from 1. It skips (pageNumber - 1) * take rows, and reads a
// pageNumber of 0 as 1, first released in v0.31.6. Before it (as in
// v0.30.3), the resolver skipped pageNumber * take. Sent unchanged to a
// newer Houston, the second page (1) came back as the first, and every later
// one a page behind.
const oneBasedPagesSince = "0.31.6"

// WorkspacesPageNumber is the pageNumber that Houston at houstonVersion reads
// as page, counted from 0. The first page is 0 on every Houston: a newer one
// reads 0 as 1, an older one skips nothing. Only the pages after it differ,
// so only they depend on the version, and a version not known (read as a
// current one, as VerifyVersionMatch reads it) cannot lose the first page.
// houstonVersion is the version of the Houston the request goes to, which
// during a login to another domain is not the current context's.
func WorkspacesPageNumber(page int, houstonVersion string) int {
	if page == 0 || VerifyVersionMatch(houstonVersion, VersionRestrictions{LT: oneBasedPagesSince}) {
		return page
	}
	return page + 1
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

	ValidateWorkspaceIDGetRequest = `
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
		}
	}
    `
)

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
func (h ClientImplementation) PaginatedListWorkspaces(request PaginatedListWorkspaceRequest) ([]Workspace, error) {
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

// ValidateWorkspaceID - get a workspace
func (h ClientImplementation) ValidateWorkspaceID(workspaceID string) (*Workspace, error) {
	req := Request{
		Query:     ValidateWorkspaceIDGetRequest,
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
