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

type DeleteDeploymentUserRequest struct {
	DeploymentID string `json:"deploymentId"`
	Email        string `json:"email"`
}

var (
	// DeploymentUserListRequest return the users for a specific deployment by ID
	DeploymentUserListRequest = `
	query deploymentUsers(
		$deploymentId: Id!
		$user: UserSearch
	){
		deploymentUsers(
				deploymentId: $deploymentId
				user: $user
		){
			id
			fullName
			username
				roleBindings {
				role
				}
		}
	}`

	// DeploymentUserAddRequest Mutation for AddDeploymentUser
	DeploymentUserAddRequest = `
	mutation AddDeploymentUser(
		$userId: Id
		$email: String!
		$deploymentId: Id!
		$role: Role!
	){
		deploymentAddUserRole(
			userId: $userId
			email: $email
			deploymentId: $deploymentId
			role: $role
		){
			id
			user {
				username
			}
			role
			deployment {
				id
				releaseName
			}
		}
	}`

	// DeploymentUserDeleteRequest Mutation for AddDeploymentUser
	DeploymentUserDeleteRequest = `
	mutation DeleteDeploymentUser(
		$userId: Id
		$email: String!
		$deploymentId: Id!
	){
		deploymentRemoveUserRole(
			userId: $userId
			email: $email
			deploymentId: $deploymentId
		){
			id
			role
		}
	}`

	// DeploymentUserUpdateRequest Mutation for UpdateDeploymentUser
	DeploymentUserUpdateRequest = `
	mutation UpdateDeploymentUser(
		$userId: Id
		$email: String!
		$deploymentId: Id!
		$role: Role!
	){
		deploymentUpdateUserRole(
			userId: $userId
			email: $email
			deploymentId: $deploymentId
			role: $role
		){
			id
			user {
				username
			}
			role
			deployment {
				id
				releaseName
			}
		}
	}`
)

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
func (h ClientImplementation) DeleteDeploymentUser(request DeleteDeploymentUserRequest) (*RoleBinding, error) {
	req := Request{
		Query:     DeploymentUserDeleteRequest,
		Variables: request,
	}

	r, err := req.DoWithClient(h.client)
	if err != nil {
		return nil, handleAPIErr(err)
	}

	return r.Data.DeleteDeploymentUser, nil
}
