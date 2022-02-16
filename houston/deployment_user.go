package houston

// ListDeploymentUsersRequest - properties to filter users in a deployment
type ListDeploymentUsersRequest struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	FullName     string `json:"fullName"`
	DeploymentID string `json:"deploymentId"`
}

// UpdateDeploymentUserRequest - properties to create a user in a deployment
type UpdateDeploymentUserRequest struct {
	Email        string `json:"email"`
	Role         string `json:"role"`
	DeploymentID string `json:"deploymentId"`
}

// ListUsersInDeployment - list users with deployment access
func (h ClientImplementation) ListDeploymentUsers(filters ListDeploymentUsersRequest) ([]DeploymentUser, error) {
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
		return []DeploymentUser{}, handleAPIErr(err)
	}

	return r.Data.DeploymentUserList, nil
}

// AddUserToDeployment - Add a user to a deployment with specified role
func (h ClientImplementation) AddDeploymentUser(variables UpdateDeploymentUserRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentUserAddRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.AddDeploymentUser, nil
}

// UpdateUserInDeployment - update a user's role inside a deployment
func (h ClientImplementation) UpdateDeploymentUser(variables UpdateDeploymentUserRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentUserUpdateRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.UpdateDeploymentUser, nil
}

// DeleteUserFromDeployment - remove a user from a deployment
func (h ClientImplementation) DeleteDeploymentUser(deploymentID, email string) (*RoleBinding, error) {
	req := Request{
		Query: DeploymentUserDeleteRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentID,
		},
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.DeleteDeploymentUser, nil
}
