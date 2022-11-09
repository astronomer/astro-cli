package deployment

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

const (
	houstonInvalidDeploymentUsersMsg = "No users were found for this deployment.  Check the deploymentId and try again.\n"
)

var (
	header = []string{"DEPLOYMENT ID", "USER", "ROLE"}
	tab    = printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}
)

// UserList returns a list of user with deployment access
func UserList(deploymentID, email, userID, fullName string, client houston.ClientInterface, out io.Writer) error {
	filters := houston.ListDeploymentUsersRequest{
		UserID:       userID,
		Email:        email,
		FullName:     fullName,
		DeploymentID: deploymentID,
	}
	deploymentUsers, err := client.ListDeploymentUsers(filters)
	if err != nil {
		fmt.Println(err)
		return err
	}

	if len(deploymentUsers) < 1 {
		_, err = out.Write([]byte(houstonInvalidDeploymentUsersMsg))
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
		role := getDeploymentLevelRole(d.RoleBindings, deploymentID)
		if role != houston.NoneRole {
			tab.AddRow([]string{d.ID, d.FullName, d.Username, role}, false)
		}
	}

	tab.Print(out)

	return nil
}

// Add a user to a deployment with specified role
func Add(deploymentID, email, role string, client houston.ClientInterface, out io.Writer) error { //nolint:dupl
	addUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	}
	d, err := client.AddDeploymentUser(addUserRequest)
	if err != nil {
		return err
	}

	tab.AddRow([]string{deploymentID, email, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, role)
	tab.Print(out)

	return nil
}

// UpdateUser updates a user's deployment role
func UpdateUser(deploymentID, email, role string, client houston.ClientInterface, out io.Writer) error { //nolint:dupl
	updateUserRequest := houston.UpdateDeploymentUserRequest{
		Email:        email,
		Role:         role,
		DeploymentID: deploymentID,
	}
	d, err := client.UpdateDeploymentUser(updateUserRequest)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Deployment.ID, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", email, role)
	tab.Print(out)

	return nil
}

// RemoveUser removes user access for a deployment
func RemoveUser(deploymentID, email string, client houston.ClientInterface, out io.Writer) error {
	d, err := client.DeleteDeploymentUser(deploymentID, email)
	if err != nil {
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
