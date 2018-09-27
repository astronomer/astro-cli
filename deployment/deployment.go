package deployment

import (
	"fmt"

	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/printutil"
)

var (
	tab = printutil.Table{
		Padding: []int{30, 50, 50},
		Header:  []string{"NAME", "RELEASE NAME", "DEPLOYMENT ID"},
	}
)

func Create(label, ws string) error {
	req := houston.Request{
		Query:     houston.DeploymentCreateRequest,
		Variables: map[string]interface{}{"label": label, "workspaceUuid": ws},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	d := r.Data.CreateDeployment

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
	req := houston.Request{
		Query:     houston.DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentUuid": uuid},
	}

	_, err := req.Do()
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
	var r *houston.HoustonResponse
	var err error

	req := houston.Request{
		Query: houston.DeploymentsGetRequest,
	}

	if all {
		r, err = req.Do()
		if err != nil {
			return err
		}
	} else {
		req.Variables = map[string]interface{}{"workspaceUuid": ws}
		r, err = req.Do()
		if err != nil {
			return err
		}
	}

	deployments = r.Data.GetDeployments

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
func Update(uuid string, args map[string]string) error {
	// s := jsonstr.MapToJsonObjStr(args)

	req := houston.Request{
		Query:     houston.DeploymentUpdateRequest,
		Variables: make(map[string]interface{}),
	}

	req.Variables["deploymentUuid"] = uuid
	req.Variables["payload"] = args

	r, err := req.Do()
	if err != nil {
		return err
	}

	fmt.Println(req.Query)
	fmt.Println(req.Variables)

	d := r.Data.UpdateDeployment

	tab.AddRow([]string{d.Label, d.ReleaseName, d.Id}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print()

	return nil
}
