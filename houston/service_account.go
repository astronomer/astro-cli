package houston

// CreateServiceAccountRequest - properties to create a service account
type CreateServiceAccountRequest struct {
	WorkspaceID  string `json:"workspaceUuid"`
	DeploymentID string `json:"deploymentUuid"`
	Label        string `json:"label"`
	Category     string `json:"category"`
	Role         string `json:"role"`
}

// DeleteServiceAccountRequest - properties to delete a service account
type DeleteServiceAccountRequest struct {
	WorkspaceID      string `json:"workspaceUuid"`
	DeploymentID     string `json:"deploymentUuid"`
	ServiceAccountID string `json:"serviceAccountUuid"`
}

var (
	CreateDeploymentServiceAccountRequest = `
	mutation createDeploymentServiceAccount(
		$label: String!,
		$category: String,
		$deploymentUuid: Uuid!,
		$role: Role!
  	){
		createDeploymentServiceAccount(
		  label: $label,
		  category: $category,
		  deploymentUuid: $deploymentUuid,
		  role: $role
    	){
		    id
		    label
		    apiKey
		    entityType
		    deploymentUuid
		    category
		    active
		    lastUsedAt
		    createdAt
		    updatedAt
    	}
  	}`

	CreateWorkspaceServiceAccountRequest = `
	mutation createWorkspaceServiceAccount(
		$label: String!,
		$category: String,
		$workspaceUuid: Uuid!,
		$role: Role!
	){
		createWorkspaceServiceAccount(
		  label: $label,
		  category: $category,
		  workspaceUuid: $workspaceUuid,
		  role: $role
		){
			id
			label
			apiKey
			entityType
			workspaceUuid
			category
			active
			lastUsedAt
			createdAt
			updatedAt
		}
    }`

	DeploymentServiceAccountDeleteRequest = `
	mutation deleteDeploymentServiceAccount(
         $serviceAccountUuid: Uuid!
         $deploymentUuid: Uuid!
    ){
		deleteDeploymentServiceAccount(
          serviceAccountUuid: $serviceAccountUuid
          deploymentUuid: $deploymentUuid
        ){
			id
			apiKey
			label
			category
			entityType
			entityUuid
			active
			createdAt
			updatedAt
			lastUsedAt
		}
	}`

	WorkspaceServiceAccountDeleteRequest = `
	mutation deleteWorkspaceServiceAccount(
          $serviceAccountUuid: Uuid!
          $workspaceUuid: Uuid!
    ){
        deleteWorkspaceServiceAccount(
          serviceAccountUuid: $serviceAccountUuid
          workspaceUuid: $workspaceUuid
        ){
            id
			apiKey
			label
			category
			entityType
			entityUuid
			active
			createdAt
			updatedAt
			lastUsedAt
         }
    }`

	DeploymentServiceAccountsGetRequest = `
	query GetDeploymentServiceAccounts(
		$deploymentUuid: Uuid!
  	){
		deploymentServiceAccounts(
			deploymentUuid: $deploymentUuid
		){
			id
			apiKey
			label
			category
			entityType
			entityUuid
			active
			createdAt
			updatedAt
			lastUsedAt
		}
  	}`

	WorkspaceServiceAccountsGetRequest = `
	query GetWorkspaceServiceAccounts(
		$workspaceUuid: Uuid!
	){
		workspaceServiceAccounts(
			workspaceUuid: $workspaceUuid
		){
			id
			apiKey
			label
			category
			entityType
			entityUuid
			active
			createdAt
			updatedAt
			lastUsedAt
		}
	}`
)

// CreateServiceAccountInDeployment - create a service account in a deployment
func (h ClientImplementation) CreateDeploymentServiceAccount(variables *CreateServiceAccountRequest) (*DeploymentServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

	req := Request{
		Query:     CreateDeploymentServiceAccountRequest,
		Variables: variables,
	}
	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.CreateDeploymentServiceAccount, nil
}

// CreateServiceAccountInWorkspace - create a service account into a workspace
func (h ClientImplementation) CreateWorkspaceServiceAccount(variables *CreateServiceAccountRequest) (*WorkspaceServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

	req := Request{
		Query:     CreateWorkspaceServiceAccountRequest,
		Variables: variables,
	}
	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.CreateWorkspaceServiceAccount, nil
}

// DeleteServiceAccountFromDeployment - delete a service account from a workspace
func (h ClientImplementation) DeleteDeploymentServiceAccount(request DeleteServiceAccountRequest) (*ServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

	req := Request{
		Query:     DeploymentServiceAccountDeleteRequest,
		Variables: request,
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.DeleteDeploymentServiceAccount, nil
}

// DeleteServiceAccountFromWorkspace - delete a service account from a workspace
func (h ClientImplementation) DeleteWorkspaceServiceAccount(request DeleteServiceAccountRequest) (*ServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return nil, err
	}

	req := Request{
		Query:     WorkspaceServiceAccountDeleteRequest,
		Variables: request,
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.DeleteWorkspaceServiceAccount, nil
}

// ListServiceAccountsInDeployment - list service accounts in a deployment
func (h ClientImplementation) ListDeploymentServiceAccounts(deploymentID string) ([]ServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return []ServiceAccount{}, err
	}

	req := Request{
		Query:     DeploymentServiceAccountsGetRequest,
		Variables: map[string]interface{}{"deploymentUuid": deploymentID},
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.GetDeploymentServiceAccounts, nil
}

// ListServiceAccountsInWorkspace - list service accounts in a workspace
func (h ClientImplementation) ListWorkspaceServiceAccounts(workspaceID string) ([]ServiceAccount, error) {
	if err := h.ValidateAvailability(); err != nil {
		return []ServiceAccount{}, err
	}

	req := Request{
		Query:     WorkspaceServiceAccountsGetRequest,
		Variables: map[string]interface{}{"workspaceUuid": workspaceID},
	}

	resp, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return resp.Data.GetWorkspaceServiceAccounts, nil
}
