package deployment

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
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

// Add a user to a deployment with specified role
func Add(deploymentId string, email string, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserAddRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentId,
			"role":         role,
		},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	d := r.Data.AddDeploymentUser

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.Id, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, role)
	tab.Print(out)

	return nil
}

// UpdateUser updates a user's deployment role
func UpdateUser(deploymentId string, email string, role string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserUpdateRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentId,
			"role":         role,
		},
	}

	r, err := req.DoWithClient(client)
	if err != nil {
		return err
	}
	d := r.Data.UpdateDeploymentUser

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.Id, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully updated %s to a %s", email, role)
	tab.Print(out)

	return nil
}

// DeleteUser removes user access for a deployment
func DeleteUser(deploymentId string, email string, client *houston.Client, out io.Writer) error {
	req := houston.Request{
		Query: houston.DeploymentUserDeleteRequest,
		Variables: map[string]interface{}{
			"email":        email,
			"deploymentId": deploymentId,
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

	tab.AddRow([]string{deploymentId, email, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully removed the %s role for %s from deployment %s", d.Role, email, deploymentId)
	tab.Print(out)

	return nil
}
