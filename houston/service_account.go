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

// CreateServiceAccountInDeployment - create a service account in a deployment
func (h ClientImplementation) CreateDeploymentServiceAccount(variables *CreateServiceAccountRequest) (*DeploymentServiceAccount, error) {
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
