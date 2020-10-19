package deployment

import (
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
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
		fmt.Println(err)
		return err
	}
	d := r.Data.AddDeploymentUser
	header := []string{"DEPLOYMENT NAME", "DEPLOYMENT ID", "USER", "ROLE"}

	tab := printutil.Table{
		Padding:        []int{44, 50},
		DynamicPadding: true,
		Header:         header,
	}

	tab.AddRow([]string{d.Deployment.ReleaseName, d.Deployment.Id, d.User.Username, d.Role}, false)
	tab.SuccessMsg = fmt.Sprintf("\n Successfully added %s as a %s", email, role)
	tab.Print(out)

	return nil
}
