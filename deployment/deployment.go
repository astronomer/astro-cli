package deployment

import (
	"fmt"
	"io"
	"os"
	"sort"

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

func checkManualReleaseNames() bool {
	req := houston.Request{
		Query:     houston.AppConfigRequest,
	}
	r, err := req.Do()
	if err != nil {
		return false
	}

	return r.Data.GetAppConfig.ManualReleaseNames
}

func Create(label, ws, releaseName, cloudRole string, deploymentConfig map[string]string) error {
	vars := map[string]interface{}{"label": label, "workspaceId": ws, "config": deploymentConfig, "cloudRole": cloudRole}

	if releaseName != "" && checkManualReleaseNames() {
		vars["releaseName"] = releaseName
	}

	req := houston.Request{
		Query:     houston.DeploymentCreateRequest,
		Variables: vars,
	}

	r, err := req.Do()
	if err != nil {
		return err
	}

	d := r.Data.CreateDeployment

	tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.Id}, false)

	splitted := []string{"Celery", ""}

	if deploymentConfig["executor"] != "" {
		// trim executor from console message
		splitted = camelcase.Split(deploymentConfig["executor"])
	}

	var airflowUrl, flowerUrl string
	for _, url := range r.Data.CreateDeployment.Urls {
		if url.Type == "airflow" {
			airflowUrl = url.Url
		}
		if url.Type == "flower" {
			flowerUrl = url.Url
		}
	}

	tab.SuccessMsg =
		fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: %s", airflowUrl)

	// The Flower URL is specific to CeleryExecutor only
	if deploymentConfig["executor"] == "CeleryExecutor" || deploymentConfig["executor"] == "" {
		tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerUrl)
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
func Update(id, cloudRole string, args map[string]string) error {
	req := houston.Request{
		Query:     houston.DeploymentUpdateRequest,
		Variables: map[string]interface{}{"deploymentId": id, "payload": args, "cloudRole": cloudRole},
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
