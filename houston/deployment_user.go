package houston

// ListUsersInDeploymentRequest - properties to filter users in a deployment
type ListUsersInDeploymentRequest struct {
	UserID       string
	Email        string
	FullName     string
	DeploymentID string
}

// UpdateUserInDeploymentRequest - properties to create a user in a deployment
type UpdateUserInDeploymentRequest struct {
	Email        string
	Role         string
	DeploymentID string
}

// ListUsersInDeployment - list users with deployment access
func (h ClientImplementation) ListUsersInDeployment(filters ListUsersInDeploymentRequest) ([]DeploymentUser, error) {
	user := map[string]interface{}{
		"userId":   filters.UserID,
		"email":    filters.Email,
		"fullName": filters.FullName,
	}
	variables := map[string]interface{}{
		"user":         user,
		"deploymentId": filters.DeploymentID,
	}
	req := Request{
		Query:     DeploymentUserListRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return []DeploymentUser{}, err
	}

	return r.Data.DeploymentUserList, nil
}

// AddUserToDeployment - Add a user to a deployment with specified role
func (h ClientImplementation) AddUserToDeployment(variables UpdateUserInDeploymentRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentUserAddRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return r.Data.AddDeploymentUser, nil
}

// UpdateUserInDeployment - update a user's role inside a deployment
func (h ClientImplementation) UpdateUserInDeployment(variables UpdateUserInDeploymentRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentUserUpdateRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return r.Data.UpdateDeploymentUser, nil
}

// DeleteUserFromDeployment - remove a user from a deployment
func (h ClientImplementation) DeleteUserFromDeployment(deploymentID, email string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentUserDeleteRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentID,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, err
	}

	return r.Data.DeleteDeploymentUser, nil
}
