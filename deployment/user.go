package deployment

import (
	"fmt"
	"io"
	"strings"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/messages"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	header = []string{"DEPLOYMENT NAME", "DEPLOYMENT ID", "USER", "ROLE"}
	tab    = printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}
)

// UserList returns a list of user with deployment access
func UserList(deploymentID, email, userID, fullName string, client houston.HoustonClientInterface, out io.Writer) error {
	filters := houston.ListUsersInDeploymentRequest{
		UserID: userID,
		Email: email,
		FullName: fullName,
		DeploymentID: deploymentID,
	}
	deploymentUsers, err := client.ListUsersInDeployment(filters)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if len(deploymentUsers) < 1 {
		_, err = out.Write([]byte(messages.HoustonInvalidDeploymentUsers))
		return err
	}

	header = []string{"USER ID", "NAME", "EMAIL", "ROLE"}
	tab = printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}

	// Build rows
	for _, d := range deploymentUsers {
		role := filterByRoleType(d.RoleBindings, houston.DeploymentRole)
		tab.AddRow([]string{d.ID, d.FullName, d.Username, role}, false)
	}

	tab.Print(out)

	return nil
}

// filterByRoleType selects the type of role from a list of roles
func filterByRoleType(roleBindings []houston.RoleBinding, roleType string) string {
	for i := range roleBindings {
		roleBinding := roleBindings[i]
		if strings.Contains(roleBinding.Role, roleType) {
			return roleBinding.Role
		}
	}

	return ""
}

// nolint:dupl
// Add a user to a deployment with specified role
func Add(deploymentID, email, role string, client houston.HoustonClientInterface, out io.Writer) error {
	addUserRequest := houston.UpdateUserInDeploymentRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	}
	d, err := client.AddUserToDeployment(addUserRequest)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, role)
	tab.Print(out)

	return nil
}

// nolint:dupl
// UpdateUser updates a user's deployment role
func UpdateUser(deploymentID, email, role string, client houston.HoustonClientInterface, out io.Writer) error {
	updateUserRequest := houston.UpdateUserInDeploymentRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	}
	d, err := client.UpdateUserInDeployment(updateUserRequest)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", email, role)
	tab.Print(out)

	return nil
}

// DeleteUser removes user access for a deployment
func DeleteUser(deploymentID, email string, client houston.HoustonClientInterface, out io.Writer) error {
	d, err := client.DeleteUserFromDeployment(deploymentID, email)
	if err != nil {
		fmt.Println(err)
		return err
	}
	header := []string{"DEPLOYMENT ID", "USER", "ROLE"}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}

	tab.AddRow([]string{deploymentID, email, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully removed the %s role for %s from deployment %s", d.Role, email, deploymentID)
	tab.Print(out)

	return nil
}
