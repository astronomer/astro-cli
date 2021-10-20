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
func UserList(deploymentID, email, userID, fullName string, client *houston.Client, out io.Writer) error {
	user := map[string]interface{}{
		"userId":   userID,
		"email":    email,
		"fullName": fullName,
	}
	variables := map[string]interface{}{
		"user":         user,
		"deploymentId": deploymentID,
	}
	req := houston.Request{
		Query:     houston.DeploymentUserListRequest,
		Variables: variables,
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		fmt.Println(err)
		return err
	}
	deploymentUsers := r.Data.DeploymentUserList

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
func Add(deploymentID, email, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserAddRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentID,
			"role":         role,
		},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	d := r.Data.AddDeploymentUser

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, role)
	tab.Print(out)

	return nil
}

// nolint:dupl
// UpdateUser updates a user's deployment role
func UpdateUser(deploymentID, email, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserUpdateRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentID,
			"role":         role,
		},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	d := r.Data.UpdateDeploymentUser

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.ID, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", email, role)
	tab.Print(out)

	return nil
}

// DeleteUser removes user access for a deployment
func DeleteUser(deploymentID, email string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserDeleteRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentID,
		},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		fmt.Println(err)
		return err
	}
	d := r.Data.DeleteDeploymentUser
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
