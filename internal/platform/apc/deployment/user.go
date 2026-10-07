package deployment

import (
	"errors"

	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
)

// ErrNoDeploymentUsers is UserList's answer when Houston lists no user for
// the Deployment at all.
var ErrNoDeploymentUsers = errors.New("no users were found for this deployment")

// UserRole is a user and the role they hold on a Deployment. ID is empty
// when Houston's answer did not name them.
type UserRole struct {
	ID       string
	FullName string
	Username string
	Role     string
}

// UserList returns the users with a role on the Deployment who match the
// filters, and the role each holds there. It is ErrNoDeploymentUsers when
// Houston lists none.
func UserList(deploymentID, email, userID, fullName string, client houston.ClientInterface) ([]UserRole, error) {
	filters := houston.ListDeploymentUsersRequest{
		UserID:       userID,
		Email:        email,
		FullName:     fullName,
		DeploymentID: deploymentID,
	}
	deploymentUsers, err := houston.Call(client.ListDeploymentUsers)(filters)
	if err != nil {
		return nil, err
	}
	if len(deploymentUsers) < 1 {
		return nil, ErrNoDeploymentUsers
	}
	users := make([]UserRole, 0, len(deploymentUsers))
	for _, d := range deploymentUsers {
		role := getDeploymentLevelRole(d.RoleBindings, deploymentID)
		if role != houston.NoneRole {
			users = append(users, UserRole{ID: d.ID, FullName: d.FullName, Username: d.Username, Role: role})
		}
	}
	return users, nil
}

// Add gives the user with email role on a Deployment, and returns them with
// the role Houston recorded.
func Add(deploymentID, email, role string, client houston.ClientInterface) (UserRole, error) {
	rb, err := houston.Call(client.AddDeploymentUser)(houston.UpdateDeploymentUserRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	})
	if err != nil {
		return UserRole{}, err
	}
	return boundUser(rb, role), nil
}

// UpdateUser changes the role of the user with email on a Deployment, and
// returns them with the role Houston recorded.
func UpdateUser(deploymentID, email, role string, client houston.ClientInterface) (UserRole, error) {
	rb, err := houston.Call(client.UpdateDeploymentUser)(houston.UpdateDeploymentUserRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	})
	if err != nil {
		return UserRole{}, err
	}
	return boundUser(rb, role), nil
}

// RemoveUser removes the role of the user with email on a Deployment, and
// returns them, as Houston names them, with the role they held.
func RemoveUser(deploymentID, email string, client houston.ClientInterface) (UserRole, error) {
	rb, err := houston.Call(client.DeleteDeploymentUser)(houston.DeleteDeploymentUserRequest{DeploymentID: deploymentID, Email: email})
	if err != nil {
		return UserRole{}, err
	}
	return boundUser(rb, ""), nil
}

// boundUser is the user a role binding Houston returned names, as Houston
// names them: ID and Username are empty when its answer does not.
func boundUser(rb *houston.RoleBinding, role string) UserRole {
	u := UserRole{Role: boundRole(rb, role)}
	if rb != nil {
		u.ID, u.Username = rb.User.ID, rb.User.Username
	}
	return u
}
