package deployment

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/messages"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/jsonstr"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)
)

func Create(label, ws string) error {
	deployment, err := api.CreateDeployment(label, ws)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_DEPLOYMENT_CREATE_SUCCESS, deployment.Id)

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}

	cloudDomain := c.Domain
	if len(cloudDomain) == 0 {
		return errors.New("No domain set, re-authenticate.")
	}

	fmt.Printf("\n"+messages.EE_LINK_AIRFLOW+"\n", deployment.ReleaseName, cloudDomain)
	fmt.Printf(messages.EE_LINK_FLOWER+"\n", deployment.ReleaseName, cloudDomain)

	return nil
}

func Delete(uuid string) error {
	resp, err := api.DeleteDeployment(uuid)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_DEPLOYMENT_DELETE_SUCCESS, resp.Id)

	return nil
}

// List all airflow deployments
func List(ws string, all bool) error {
	var deployments []houston.Deployment
	var err error

	if all {
		deployments, err = api.GetAllDeployments()
		if err != nil {
			return err
		}
	} else {
		deployments, err = api.GetDeployments(ws)
		if err != nil {
			return err
		}
	}

	tab := printutil.Table{
		Padding: []int{30, 50, 50, 50},
		Header:  []string{"NAME", "RELEASE NAME", "DEPLOYMENT ID", "WORKSPACE"},
	}

	// Build rows
	for _, d := range deployments {
		if all {
			ws = d.Workspace.Uuid
		}

		tab.AddRow([]string{d.Label, d.ReleaseName, d.Id, ws}, false)
	}

	tab.Print()

	return nil
}

// Update an airflow deployment
func Update(deploymentId string, args map[string]string) error {
	s := jsonstr.MapToJsonObjStr(args)

	dep, err := api.UpdateDeployment(deploymentId, s)
	if err != nil {
		return err
	}

	fmt.Printf(messages.HOUSTON_DEPLOYMENT_UPDATE_SUCCESS, dep.Id)

	return nil
}
