package deployment

import (
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"

	"github.com/fatih/camelcase"
)

var (
	tab = printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "TAG"},
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

	splitted := []string{"Celery", ""}

	if deploymentConfig["executor"] != "" {
		// trim executor from console message
		splitted = camelcase.Split(deploymentConfig["executor"])
	}

	tab.SuccessMsg =
		fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: https://%s-airflow.%s", d.ReleaseName, c.Domain)

	// The Flower URL is specific to CeleryExecutor only
	if deploymentConfig["executor"] == "CeleryExecutor" || deploymentConfig["executor"] == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: https://%s-flower.%s", d.ReleaseName, c.Domain)
	}
	tab.Print(os.Stdout)

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
	// tab.Print(os.Stdout)
	fmt.Println("\n Successfully deleted deployment")

	return nil
}

// List all airflow deployments
func List(ws string, all bool, client *houston.Client, out io.Writer) error {
	var deployments []houston.Deployment
	var r *houston.Response
	var err error

	req := houston.Request{
		Query: houston.DeploymentsGetRequest,
	}

	if all {
		r, err = req.DoWithClient(client)
		if err != nil {
			return err
		}
	} else {
		req.Variables = map[string]interface{}{"workspaceId": ws}
		r, err = req.DoWithClient(client)
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

		currentTag := d.DeploymentInfo.Current
		if currentTag == "" {
			currentTag = "?"
		}
		tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.Id, currentTag}, false)
	}

	return tab.Print(out)
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
	tab.Print(os.Stdout)

	return nil
}
