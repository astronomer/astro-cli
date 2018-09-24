package deployment

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/httputil"
	"github.com/astronomerio/astro-cli/pkg/jsonstr"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

var (
	http = httputil.NewHTTPClient()
	api  = houston.NewHoustonClient(http)

	tab = printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "RELEASE NAME", "DEPLOYMENT ID"},
	}
)

func Create(label, ws string) error {
	d, err := api.CreateDeployment(label, ws)
	if err != nil {
		return err
	}

	c, err := config.GetCurrentContext()
	if err != nil {
		return err
	}
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Id}, false)
	tab.SuccessMsg = "\n Successfully created deployment. Deployment can be accessed at the following URLs \n" +
		fmt.Sprintf("\n Airflow Dashboard: https://%s-airflow.%s", d.ReleaseName, c.Domain) +
		fmt.Sprintf("\n Flower Dashboard: https://%s-flower.%s", d.ReleaseName, c.Domain)

	tab.Print()

	return nil
}

func Delete(uuid string) error {
	_, err := api.DeleteDeployment(uuid)
	if err != nil {
		return err
	}

	// TODO - add back in tab print once houston returns all relevant information
	// tab.AddRow([]string{d.Label, d.ReleaseName, d.Id, d.Workspace.Uuid}, false)
	// tab.SuccessMsg = "\n Successfully deleted deployment"
	// tab.Print()
	fmt.Println("\n Successfully deleted deployment")

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

	// Build rows
	for _, d := range deployments {
		if all {
			ws = d.Workspace.Uuid
		}

		tab.AddRow([]string{d.Label, d.ReleaseName, d.Id}, false)
	}

	tab.Print()

	return nil
}

// Update an airflow deployment
func Update(deploymentId string, args map[string]string) error {
	s := jsonstr.MapToJsonObjStr(args)

	d, err := api.UpdateDeployment(deploymentId, s)
	if err != nil {
		return err
	}

	tab.AddRow([]string{d.Label, d.ReleaseName, d.Id}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print()

	return nil
}
