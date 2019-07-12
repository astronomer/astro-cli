package deployment

import (
	"fmt"
	"sort"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

var (
	tab = printutil.Table{
		Padding:        []int{30, 30, 10, 50},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID"},
	}
)

func Create(label, ws string, deploymentConfig map[string]string) error {
	req := houston.Request{
		Query:     houston.DeploymentCreateRequest,
		Variables: map[string]interface{}{"label": label, "workspaceId": ws, "config": deploymentConfig},
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
	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.Id}, false)
	tab.SuccessMsg =
		fmt.Sprintf("\n Successfully created deployment with %s executor", deploymentConfig["executor"]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: https://%s-airflow.%s", d.ReleaseName, c.Domain) +
			fmt.Sprintf("\n Flower Dashboard: https://%s-flower.%s", d.ReleaseName, c.Domain)

	tab.Print()

	return nil
}

func Delete(id string) error {
	req := houston.Request{
		Query:     houston.DeploymentDeleteRequest,
		Variables: map[string]interface{}{"deploymentId": id},
	}

	_, err := req.Do()
	if err != nil {
		return err
	}

	// TODO - add back in tab print once houston returns all relevant information
	// tab.AddRow([]string{d.Label, d.ReleaseName, d.Id, d.Workspace.Id}, false)
	// tab.SuccessMsg = "\n Successfully deleted deployment"
	// tab.Print()
	fmt.Println("\n Successfully deleted deployment")

	return nil
}

// List all airflow deployments
func List(ws string, all bool) error {
	var deployments []houston.Deployment
	var r *houston.Response
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
		req.Variables = map[string]interface{}{"workspaceId": ws}
		r, err = req.Do()
		if err != nil {
			return err
		}
	}

	deployments = r.Data.GetDeployments

	sort.Slice(deployments, func(i, j int) bool { return deployments[i].Label > deployments[j].Label })

	// Build rows
	for _, d := range deployments {
		if all {
			ws = d.Workspace.Id
		}
		tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.Id}, false)
	}

	tab.Print()

	return nil
}

// Update an airflow deployment
func Update(id string, args map[string]string) error {
	req := houston.Request{
		Query:     houston.DeploymentUpdateRequest,
		Variables: map[string]interface{}{"deploymentId": id, "payload": args},
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	d := r.Data.UpdateDeployment

	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.Id}, false)
	tab.SuccessMsg = "\n Successfully updated deployment"
	tab.Print()

	return nil
}
